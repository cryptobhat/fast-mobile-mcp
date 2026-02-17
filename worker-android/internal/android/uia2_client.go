package android

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fast-mobile-mcp/shared/snapshot"
)

type UIA2Client struct {
	baseURL string
	http    *http.Client
}

type ActiveApp struct {
	BundleID string
	AppName  string
}

type hierarchyNode struct {
	XMLName  xml.Name        `xml:"node"`
	Attrs    []xml.Attr      `xml:",any,attr"`
	Children []hierarchyNode `xml:"node"`
}

func NewUIA2Client(baseURL string) *UIA2Client {
	transport := &http.Transport{
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}
	return &UIA2Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   8 * time.Second,
			Transport: transport,
		},
	}
}

func (c *UIA2Client) EnsureSession(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("uia2 session unhealthy status=%d", resp.StatusCode)
	}
	return nil
}

func (c *UIA2Client) GetActiveApp(ctx context.Context) (ActiveApp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/app/current", nil)
	if err != nil {
		return ActiveApp{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ActiveApp{}, err
	}
	defer resp.Body.Close()

	payload := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ActiveApp{}, err
	}

	return ActiveApp{
		BundleID: stringValue(payload, "package"),
		AppName:  stringValue(payload, "activity"),
	}, nil
}

func (c *UIA2Client) DumpHierarchy(ctx context.Context) ([]snapshot.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/dump/hierarchy", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var root struct {
		Nodes []hierarchyNode `xml:"node"`
	}
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}

	nodes := make([]snapshot.Node, 0, 256)
	counter := 0
	for i := range root.Nodes {
		walkHierarchy(root.Nodes[i], "", int32(i), &counter, &nodes)
	}
	return nodes, nil
}

func (c *UIA2Client) Tap(ctx context.Context, x, y int32, tapCount int32) error {
	body := map[string]any{"x": x, "y": y, "count": tapCount}
	return c.postJSON(ctx, "/click", body)
}

func (c *UIA2Client) Type(ctx context.Context, text string, clear bool) error {
	body := map[string]any{"text": text, "clear": clear}
	return c.postJSON(ctx, "/send_keys", body)
}

func (c *UIA2Client) Swipe(ctx context.Context, sx, sy, ex, ey, durationMS int32) error {
	body := map[string]any{
		"sx":          sx,
		"sy":          sy,
		"ex":          ex,
		"ey":          ey,
		"duration_ms": durationMS,
	}
	return c.postJSON(ctx, "/swipe", body)
}

func (c *UIA2Client) Screenshot(ctx context.Context) ([]byte, int32, int32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/screenshot/0", nil)
	if err != nil {
		return nil, 0, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, 0, err
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return data, 0, 0, nil
	}

	return data, int32(cfg.Width), int32(cfg.Height), nil
}

func (c *UIA2Client) postJSON(ctx context.Context, route string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+route, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bodyRaw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("uia2 request failed status=%d body=%s", resp.StatusCode, string(bodyRaw))
	}
	return nil
}

func walkHierarchy(node hierarchyNode, parentRef string, index int32, counter *int, out *[]snapshot.Node) {
	ref := fmt.Sprintf("n-%d", *counter)
	*counter = *counter + 1

	item := snapshot.Node{
		RefID:       ref,
		ParentRefID: parentRef,
		Index:       index,
		Text:        attrValue(node.Attrs, "text"),
		ContentDesc: attrValue(node.Attrs, "content-desc"),
		ResourceID:  attrValue(node.Attrs, "resource-id"),
		ClassName:   attrValue(node.Attrs, "class"),
		PackageName: attrValue(node.Attrs, "package"),
		Enabled:     attrBool(node.Attrs, "enabled"),
		Clickable:   attrBool(node.Attrs, "clickable"),
		Focusable:   attrBool(node.Attrs, "focusable"),
		Visible:     attrBool(node.Attrs, "visible-to-user"),
		Selected:    attrBool(node.Attrs, "selected"),
		Checked:     attrBool(node.Attrs, "checked"),
		Bounds:      parseBounds(attrValue(node.Attrs, "bounds")),
	}
	*out = append(*out, item)

	for i, child := range node.Children {
		walkHierarchy(child, ref, int32(i), counter, out)
	}
}

func attrValue(attrs []xml.Attr, key string) string {
	for _, attr := range attrs {
		if attr.Name.Local == key {
			return attr.Value
		}
	}
	return ""
}

func attrBool(attrs []xml.Attr, key string) bool {
	return strings.EqualFold(attrValue(attrs, key), "true")
}

var boundsPattern = regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

func parseBounds(raw string) snapshot.Bounds {
	match := boundsPattern.FindStringSubmatch(raw)
	if len(match) != 5 {
		return snapshot.Bounds{}
	}
	left, _ := strconv.Atoi(match[1])
	top, _ := strconv.Atoi(match[2])
	right, _ := strconv.Atoi(match[3])
	bottom, _ := strconv.Atoi(match[4])
	return snapshot.Bounds{Left: int32(left), Top: int32(top), Right: int32(right), Bottom: int32(bottom)}
}

func stringValue(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
