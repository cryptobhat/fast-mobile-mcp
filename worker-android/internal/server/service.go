package server

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	mobilev1 "github.com/fast-mobile-mcp/proto/gen/go/mobile/v1"
	"github.com/fast-mobile-mcp/shared/snapshot"
	"github.com/fast-mobile-mcp/worker-android/internal/android"
	"github.com/fast-mobile-mcp/worker-android/internal/config"
	"github.com/fast-mobile-mcp/worker-android/internal/device"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MobileService struct {
	mobilev1.UnimplementedMobileAutomationServiceServer
	cfg      config.Config
	log      *slog.Logger
	registry *device.Registry
	store    *snapshot.Store
}

func NewMobileService(cfg config.Config, log *slog.Logger) *MobileService {
	return &MobileService{
		cfg:      cfg,
		log:      log,
		registry: device.NewRegistry(cfg),
		store:    snapshot.NewStore(cfg.SnapshotTTL, cfg.SnapshotCleanup, cfg.MaxSnapshotsPerDevice),
	}
}

func (s *MobileService) Close() {
	s.registry.Close()
	s.store.Close()
}

func (s *MobileService) ListDevices(ctx context.Context, req *mobilev1.ListDevicesRequest) (*mobilev1.ListDevicesResponse, error) {
	list, err := s.registry.ListDevices(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	devices := make([]*mobilev1.Device, 0, len(list))
	for _, d := range list {
		devices = append(devices, &mobilev1.Device{
			DeviceId:       d.DeviceID,
			Platform:       mobilev1.Platform_PLATFORM_ANDROID,
			Name:           d.Name,
			Model:          d.Model,
			OsVersion:      d.OSVersion,
			IsSimulator:    d.IsSimulator,
			Status:         mobilev1.DeviceStatus_DEVICE_STATUS_READY,
			LastSeenUnixMs: time.Now().UTC().UnixMilli(),
			Capabilities:   map[string]string{"automation": "uiautomator2"},
		})
	}

	return &mobilev1.ListDevicesResponse{
		Devices:    devices,
		CacheAgeMs: 0,
	}, nil
}

func (s *MobileService) GetActiveApp(ctx context.Context, req *mobilev1.GetActiveAppRequest) (*mobilev1.GetActiveAppResponse, error) {
	runtime, err := s.registry.RuntimeForDevice(ctx, req.DeviceId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	ctx, cancel := s.actionContext(ctx, req.Options)
	defer cancel()

	result, err := runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		return runtime.UIA2.GetActiveApp(runCtx)
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	app := result.(android.ActiveApp)

	return &mobilev1.GetActiveAppResponse{
		DeviceId:         req.DeviceId,
		BundleId:         app.BundleID,
		AppName:          app.AppName,
		IsForeground:     true,
		ObservedAtUnixMs: time.Now().UTC().UnixMilli(),
	}, nil
}

func (s *MobileService) GetUITree(ctx context.Context, req *mobilev1.GetUITreeRequest) (*mobilev1.GetUITreeResponse, error) {
	runtime, err := s.registry.RuntimeForDevice(ctx, req.DeviceId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	ctx, cancel := s.actionContext(ctx, req.Options)
	defer cancel()

	nodesAny, err := runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		return runtime.UIA2.DumpHierarchy(runCtx)
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	nodes := nodesAny.([]snapshot.Node)
	if req.DepthLimit > 0 {
		nodes = pruneByDepth(nodes, int(req.DepthLimit))
	}

	snap := s.store.Put(req.DeviceId, nodes)
	limit := int(req.NodeLimit)
	if limit <= 0 {
		limit = 300
	}
	paged, next, total, ok := s.store.Page(snap.ID, req.Cursor, limit)
	if !ok {
		return nil, status.Error(codes.NotFound, "snapshot expired")
	}

	return &mobilev1.GetUITreeResponse{
		DeviceId:        req.DeviceId,
		SnapshotId:      snap.ID,
		ExpiresAtUnixMs: snap.ExpiresAt.UnixMilli(),
		Nodes:           convertNodes(paged),
		TotalNodes:      uint32(total),
		NextCursor:      next,
	}, nil
}

func (s *MobileService) FindElements(ctx context.Context, req *mobilev1.FindElementsRequest) (*mobilev1.FindElementsResponse, error) {
	snap, err := s.resolveSnapshot(ctx, req.DeviceId, req.SnapshotId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	matches := filterNodes(snap.Nodes, req.Selector)
	start := parseCursor(req.Cursor)
	if start > len(matches) {
		start = len(matches)
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	end := start + limit
	if end > len(matches) {
		end = len(matches)
	}
	next := ""
	if end < len(matches) {
		next = strconv.Itoa(end)
	}

	elements := make([]*mobilev1.Element, 0, end-start)
	for _, node := range matches[start:end] {
		el := &mobilev1.Element{RefId: node.RefID}
		if req.IncludeNodes {
			el.Node = convertNode(node)
		}
		elements = append(elements, el)
	}

	return &mobilev1.FindElementsResponse{
		DeviceId:     req.DeviceId,
		SnapshotId:   snap.ID,
		Elements:     elements,
		NextCursor:   next,
		TotalMatched: uint32(len(matches)),
	}, nil
}

func (s *MobileService) Tap(ctx context.Context, req *mobilev1.TapRequest) (*mobilev1.ActionResponse, error) {
	start := time.Now().UTC()
	runtime, err := s.registry.RuntimeForDevice(ctx, req.DeviceId)
	if err != nil {
		return actionFailed(req.DeviceId, start, "DEVICE_NOT_FOUND", err), nil
	}

	ctx, cancel := s.actionContext(ctx, req.Options)
	defer cancel()

	point, resolveErr := s.resolveTargetPoint(ctx, req.DeviceId, req.SnapshotId, req.GetRefId(), req.GetSelector(), req.GetCoordinates())
	if resolveErr != nil {
		return actionFailed(req.DeviceId, start, "INVALID_TARGET", resolveErr), nil
	}

	_, err = runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		count := req.TapCount
		if count <= 0 {
			count = 1
		}
		return nil, runtime.UIA2.Tap(runCtx, point.x, point.y, count)
	})
	if err != nil {
		return actionFailed(req.DeviceId, start, "TAP_FAILED", err), nil
	}

	return actionOK(req.DeviceId, start), nil
}

func (s *MobileService) Type(ctx context.Context, req *mobilev1.TypeRequest) (*mobilev1.ActionResponse, error) {
	start := time.Now().UTC()
	runtime, err := s.registry.RuntimeForDevice(ctx, req.DeviceId)
	if err != nil {
		return actionFailed(req.DeviceId, start, "DEVICE_NOT_FOUND", err), nil
	}

	ctx, cancel := s.actionContext(ctx, req.Options)
	defer cancel()

	if _, resolveErr := s.resolveTargetPoint(ctx, req.DeviceId, req.SnapshotId, req.GetRefId(), req.GetSelector(), req.GetCoordinates()); resolveErr != nil {
		return actionFailed(req.DeviceId, start, "INVALID_TARGET", resolveErr), nil
	}

	_, err = runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		return nil, runtime.UIA2.Type(runCtx, req.Text, req.ClearBeforeType)
	})
	if err != nil {
		return actionFailed(req.DeviceId, start, "TYPE_FAILED", err), nil
	}
	return actionOK(req.DeviceId, start), nil
}

func (s *MobileService) Swipe(ctx context.Context, req *mobilev1.SwipeRequest) (*mobilev1.ActionResponse, error) {
	start := time.Now().UTC()
	runtime, err := s.registry.RuntimeForDevice(ctx, req.DeviceId)
	if err != nil {
		return actionFailed(req.DeviceId, start, "DEVICE_NOT_FOUND", err), nil
	}
	ctx, cancel := s.actionContext(ctx, req.Options)
	defer cancel()

	sx, sy, ex, ey := swipeCoordinates(req)
	duration := req.DurationMs
	if duration <= 0 {
		duration = 200
	}

	_, err = runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		return nil, runtime.UIA2.Swipe(runCtx, sx, sy, ex, ey, duration)
	})
	if err != nil {
		return actionFailed(req.DeviceId, start, "SWIPE_FAILED", err), nil
	}
	return actionOK(req.DeviceId, start), nil
}

func (s *MobileService) ScreenshotStream(req *mobilev1.ScreenshotStreamRequest, stream mobilev1.MobileAutomationService_ScreenshotStreamServer) error {
	runtime, err := s.registry.RuntimeForDevice(stream.Context(), req.DeviceId)
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}

	fps := int(req.MaxFps)
	if fps <= 0 {
		fps = 2
	}
	if fps > s.cfg.StreamMaxFPS {
		fps = s.cfg.StreamMaxFPS
	}
	interval := time.Second / time.Duration(fps)
	chunkSize := s.cfg.StreamChunkBytes
	if chunkSize <= 0 {
		chunkSize = 65536
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	framesSent := uint32(0)
	for {
		if req.MaxFrames > 0 && framesSent >= req.MaxFrames {
			_ = stream.Send(&mobilev1.ScreenshotStreamEvent{
				Payload: &mobilev1.ScreenshotStreamEvent_End{End: &mobilev1.StreamEnd{Reason: "max_frames_reached"}},
			})
			return nil
		}

		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-ticker.C:
			out, err := runtime.Executor.Submit(stream.Context(), func(runCtx context.Context) (any, error) {
				data, w, h, capErr := runtime.UIA2.Screenshot(runCtx)
				if capErr != nil {
					return nil, capErr
				}
				return struct {
					data []byte
					w    int32
					h    int32
				}{data: data, w: w, h: h}, nil
			})
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}

			frame := out.(struct {
				data []byte
				w    int32
				h    int32
			})
			frameID := uuid.NewString()
			chunkCount := int((len(frame.data) + chunkSize - 1) / chunkSize)

			if err := stream.Send(&mobilev1.ScreenshotStreamEvent{
				Payload: &mobilev1.ScreenshotStreamEvent_FrameMeta{FrameMeta: &mobilev1.ScreenshotFrameMeta{
					FrameId:          frameID,
					DeviceId:         req.DeviceId,
					Width:            uint32(frame.w),
					Height:           uint32(frame.h),
					MimeType:         "image/jpeg",
					TotalBytes:       uint64(len(frame.data)),
					ChunkCount:       uint32(chunkCount),
					CapturedAtUnixMs: time.Now().UTC().UnixMilli(),
				}},
			}); err != nil {
				return err
			}

			for i := 0; i < chunkCount; i++ {
				start := i * chunkSize
				end := start + chunkSize
				if end > len(frame.data) {
					end = len(frame.data)
				}
				if err := stream.Send(&mobilev1.ScreenshotStreamEvent{
					Payload: &mobilev1.ScreenshotStreamEvent_Chunk{Chunk: &mobilev1.ScreenshotChunk{
						FrameId:    frameID,
						ChunkIndex: uint32(i),
						Data:       frame.data[start:end],
					}},
				}); err != nil {
					return err
				}
			}

			framesSent++
		}
	}
}

type point struct {
	x int32
	y int32
}

func (s *MobileService) resolveTargetPoint(ctx context.Context, deviceID, snapshotID, refID string, selector *mobilev1.Selector, coords *mobilev1.Coordinates) (point, error) {
	if coords != nil {
		return point{x: coords.X, y: coords.Y}, nil
	}

	snap, err := s.resolveSnapshot(ctx, deviceID, snapshotID)
	if err != nil {
		return point{}, err
	}

	if refID != "" {
		n, ok := s.store.ResolveRef(snap.ID, refID)
		if !ok {
			return point{}, fmt.Errorf("ref_id %s not found", refID)
		}
		return center(n.Bounds), nil
	}

	if selector != nil {
		matches := filterNodes(snap.Nodes, selector)
		if len(matches) == 0 {
			return point{}, fmt.Errorf("selector matched zero nodes")
		}
		return center(matches[0].Bounds), nil
	}

	return point{}, fmt.Errorf("missing action target")
}

func (s *MobileService) resolveSnapshot(ctx context.Context, deviceID, snapshotID string) (snapshot.Snapshot, error) {
	if snapshotID != "" {
		if snap, ok := s.store.Get(snapshotID); ok {
			return snap, nil
		}
	}

	if latest, ok := s.store.Latest(deviceID); ok {
		return latest, nil
	}

	runtime, err := s.registry.RuntimeForDevice(ctx, deviceID)
	if err != nil {
		return snapshot.Snapshot{}, err
	}

	out, err := runtime.Executor.Submit(ctx, func(runCtx context.Context) (any, error) {
		return runtime.UIA2.DumpHierarchy(runCtx)
	})
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	nodes := out.([]snapshot.Node)
	return s.store.Put(deviceID, nodes), nil
}

func convertNodes(nodes []snapshot.Node) []*mobilev1.UiNode {
	out := make([]*mobilev1.UiNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, convertNode(n))
	}
	return out
}

func convertNode(n snapshot.Node) *mobilev1.UiNode {
	return &mobilev1.UiNode{
		RefId:       n.RefID,
		ParentRefId: n.ParentRefID,
		Index:       n.Index,
		Text:        n.Text,
		ContentDesc: n.ContentDesc,
		ResourceId:  n.ResourceID,
		ClassName:   n.ClassName,
		PackageName: n.PackageName,
		Bounds:      &mobilev1.Bounds{Left: n.Bounds.Left, Top: n.Bounds.Top, Right: n.Bounds.Right, Bottom: n.Bounds.Bottom},
		Enabled:     n.Enabled,
		Clickable:   n.Clickable,
		Focusable:   n.Focusable,
		Visible:     n.Visible,
		Selected:    n.Selected,
		Checked:     n.Checked,
	}
}

func actionOK(deviceID string, startedAt time.Time) *mobilev1.ActionResponse {
	return &mobilev1.ActionResponse{
		DeviceId:          deviceID,
		ActionId:          uuid.NewString(),
		Status:            mobilev1.ActionStatus_ACTION_STATUS_OK,
		StartedAtUnixMs:   startedAt.UnixMilli(),
		CompletedAtUnixMs: time.Now().UTC().UnixMilli(),
		Metadata:          map[string]string{},
	}
}

func actionFailed(deviceID string, startedAt time.Time, code string, err error) *mobilev1.ActionResponse {
	statusCode := mobilev1.ActionStatus_ACTION_STATUS_FAILED
	if strings.Contains(strings.ToLower(code), "timeout") {
		statusCode = mobilev1.ActionStatus_ACTION_STATUS_TIMEOUT
	}
	return &mobilev1.ActionResponse{
		DeviceId:          deviceID,
		ActionId:          uuid.NewString(),
		Status:            statusCode,
		StartedAtUnixMs:   startedAt.UnixMilli(),
		CompletedAtUnixMs: time.Now().UTC().UnixMilli(),
		ErrorCode:         code,
		ErrorMessage:      err.Error(),
		Metadata:          map[string]string{},
	}
}

func (s *MobileService) actionContext(parent context.Context, options *mobilev1.RequestOptions) (context.Context, context.CancelFunc) {
	timeout := s.cfg.ActionTimeout
	if options != nil && options.TimeoutMs > 0 {
		timeout = time.Duration(options.TimeoutMs) * time.Millisecond
	}
	return context.WithTimeout(parent, timeout)
}

func pruneByDepth(nodes []snapshot.Node, maxDepth int) []snapshot.Node {
	if maxDepth <= 0 {
		return nodes
	}
	depthByRef := map[string]int{"": -1}
	out := make([]snapshot.Node, 0, len(nodes))
	for _, n := range nodes {
		parentDepth := depthByRef[n.ParentRefID]
		d := parentDepth + 1
		depthByRef[n.RefID] = d
		if d <= maxDepth {
			out = append(out, n)
		}
	}
	return out
}

func parseCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	v, err := strconv.Atoi(cursor)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func center(b snapshot.Bounds) point {
	return point{x: (b.Left + b.Right) / 2, y: (b.Top + b.Bottom) / 2}
}

var boolRegex = regexp.MustCompile(`^(true|false)$`)

func filterNodes(nodes []snapshot.Node, selector *mobilev1.Selector) []snapshot.Node {
	if selector == nil || len(selector.Clauses) == 0 {
		return nodes
	}
	matchAll := selector.MatchAll
	within := selector.WithinRefId
	out := make([]snapshot.Node, 0)
	for _, n := range nodes {
		if within != "" && n.ParentRefID != within && n.RefID != within {
			continue
		}
		matched := 0
		for _, clause := range selector.Clauses {
			if clauseMatch(n, clause) {
				matched++
			}
		}
		if (matchAll && matched == len(selector.Clauses)) || (!matchAll && matched > 0) {
			out = append(out, n)
		}
	}
	if selector.Limit > 0 && len(out) > int(selector.Limit) {
		return out[:selector.Limit]
	}
	return out
}

func clauseMatch(node snapshot.Node, clause *mobilev1.SelectorClause) bool {
	fieldValue := ""
	switch clause.Field {
	case mobilev1.SelectorField_SELECTOR_FIELD_REF_ID:
		fieldValue = node.RefID
	case mobilev1.SelectorField_SELECTOR_FIELD_TEXT:
		fieldValue = node.Text
	case mobilev1.SelectorField_SELECTOR_FIELD_CONTENT_DESC:
		fieldValue = node.ContentDesc
	case mobilev1.SelectorField_SELECTOR_FIELD_RESOURCE_ID:
		fieldValue = node.ResourceID
	case mobilev1.SelectorField_SELECTOR_FIELD_CLASS_NAME:
		fieldValue = node.ClassName
	case mobilev1.SelectorField_SELECTOR_FIELD_PACKAGE_NAME:
		fieldValue = node.PackageName
	case mobilev1.SelectorField_SELECTOR_FIELD_ENABLED:
		if boolRegex.MatchString(strings.ToLower(clause.Value)) {
			want := strings.EqualFold(clause.Value, "true")
			return node.Enabled == want
		}
		return false
	case mobilev1.SelectorField_SELECTOR_FIELD_CLICKABLE:
		if boolRegex.MatchString(strings.ToLower(clause.Value)) {
			want := strings.EqualFold(clause.Value, "true")
			return node.Clickable == want
		}
		return false
	case mobilev1.SelectorField_SELECTOR_FIELD_VISIBLE:
		if boolRegex.MatchString(strings.ToLower(clause.Value)) {
			want := strings.EqualFold(clause.Value, "true")
			return node.Visible == want
		}
		return false
	default:
		return false
	}

	return compareWithOperator(fieldValue, clause.Operator, clause.Value)
}

func compareWithOperator(actual string, op mobilev1.SelectorOperator, expected string) bool {
	switch op {
	case mobilev1.SelectorOperator_SELECTOR_OPERATOR_EQ:
		return actual == expected
	case mobilev1.SelectorOperator_SELECTOR_OPERATOR_CONTAINS:
		return strings.Contains(actual, expected)
	case mobilev1.SelectorOperator_SELECTOR_OPERATOR_PREFIX:
		return strings.HasPrefix(actual, expected)
	case mobilev1.SelectorOperator_SELECTOR_OPERATOR_SUFFIX:
		return strings.HasSuffix(actual, expected)
	case mobilev1.SelectorOperator_SELECTOR_OPERATOR_REGEX:
		r, err := regexp.Compile(expected)
		if err != nil {
			return false
		}
		return r.MatchString(actual)
	default:
		return false
	}
}

func swipeCoordinates(req *mobilev1.SwipeRequest) (int32, int32, int32, int32) {
	if req.Start != nil && req.End != nil {
		return req.Start.X, req.Start.Y, req.End.X, req.End.Y
	}
	distance := req.DistancePx
	if distance <= 0 {
		distance = 400
	}
	sx, sy := int32(500), int32(1000)
	ex, ey := sx, sy
	switch req.Direction {
	case mobilev1.Direction_DIRECTION_UP:
		ey = sy - distance
	case mobilev1.Direction_DIRECTION_DOWN:
		ey = sy + distance
	case mobilev1.Direction_DIRECTION_LEFT:
		ex = sx - distance
	case mobilev1.Direction_DIRECTION_RIGHT:
		ex = sx + distance
	default:
		ey = sy - distance
	}
	return sx, sy, ex, ey
}
