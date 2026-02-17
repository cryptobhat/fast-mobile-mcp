package ios

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fast-mobile-mcp/shared/snapshot"
)

type WDAClient struct {
	baseURL string
	http    *http.Client
}

type ActiveApp struct {
	BundleID string
	AppName  string
}

type sourceNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr   `xml:",any,attr"`
	Children []sourceNode `xml:",any"`
}

func NewWDAClient(baseURL string) *WDAClient {
	transport := &http.Transport{
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}
	return &WDAClient{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:   8 * time.Second,
			Transport: transport,
		},
	}
}

func (c *WDAClient) EnsureSession(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("wda unhealthy status=%d", resp.StatusCode)
	}
	return nil
}

func (c *WDAClient) GetActiveApp(ctx context.Context) (ActiveApp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/wda/activeAppInfo", nil)
	if err != nil {
		return ActiveApp{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ActiveApp{}, err
	}
	defer resp.Body.Close()

	var payload struct {
		Value map[string]any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ActiveApp{}, err
	}

	return ActiveApp{
		BundleID: stringValue(payload.Value, "bundleId"),
		AppName:  stringValue(payload.Value, "name"),
	}, nil
}

func (c *WDAClient) DumpHierarchy(ctx context.Context) ([]snapshot.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/source", nil)
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

	var payload struct {
		Value string `json:"value"`
	}
	xmlRaw := raw
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, err
		}
		xmlRaw = []byte(payload.Value)
	}

	var root sourceNode
	if err := xml.Unmarshal(xmlRaw, &root); err != nil {
		return nil, err
	}

	out := make([]snapshot.Node, 0, 256)
	counter := 0
	for i, child := range root.Children {
		walkSourceTree(child, "", int32(i), &counter, &out)
	}
	return out, nil
}

func (c *WDAClient) Tap(ctx context.Context, x, y int32, tapCount int32) error {
	body := map[string]any{"x": x, "y": y}
	if tapCount > 1 {
		body["count"] = tapCount
	}
	return c.postJSON(ctx, "/wda/tap/0", body)
}

func (c *WDAClient) Type(ctx context.Context, text string) error {
	body := map[string]any{"value": strings.Split(text, "")}
	return c.postJSON(ctx, "/wda/keys", body)
}

func (c *WDAClient) Swipe(ctx context.Context, sx, sy, ex, ey, durationMS int32) error {
	body := map[string]any{
		"fromX":    sx,
		"fromY":    sy,
		"toX":      ex,
		"toY":      ey,
		"duration": float64(durationMS) / 1000.0,
	}
	return c.postJSON(ctx, "/wda/dragfromtoforduration", body)
}

func (c *WDAClient) Screenshot(ctx context.Context) ([]byte, int32, int32, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/screenshot", nil)
	if err != nil {
		return nil, 0, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	var payload struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, 0, 0, err
	}

	data, err := base64.StdEncoding.DecodeString(payload.Value)
	if err != nil {
		return nil, 0, 0, err
	}

	// Dimensions can be derived if needed from decoded image; left as optional for latency.
	return data, 0, 0, nil
}

func (c *WDAClient) postJSON(ctx context.Context, route string, body any) error {
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
		return fmt.Errorf("wda request failed status=%d body=%s", resp.StatusCode, string(bodyRaw))
	}
	return nil
}

func walkSourceTree(node sourceNode, parentRef string, index int32, counter *int, out *[]snapshot.Node) {
	ref := fmt.Sprintf("n-%d", *counter)
	*counter = *counter + 1

	item := snapshot.Node{
		RefID:       ref,
		ParentRefID: parentRef,
		Index:       index,
		Text:        attrValue(node.Attrs, "label"),
		ContentDesc: attrValue(node.Attrs, "name"),
		ResourceID:  attrValue(node.Attrs, "identifier"),
		ClassName:   node.XMLName.Local,
		PackageName: "",
		Enabled:     attrBool(node.Attrs, "enabled"),
		Clickable:   attrBool(node.Attrs, "hittable"),
		Focusable:   true,
		Visible:     attrBoolDefault(node.Attrs, "visible", true),
		Selected:    attrBool(node.Attrs, "selected"),
		Checked:     attrBool(node.Attrs, "value"),
		Bounds:      parseRect(attrValue(node.Attrs, "rect")),
	}
	*out = append(*out, item)

	for i, child := range node.Children {
		walkSourceTree(child, ref, int32(i), counter, out)
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

var rectPattern = regexp.MustCompile(`\{\{(-?\d+),(-?\d+)\},\{(\d+),(\d+)\}\}`)

func parseRect(raw string) snapshot.Bounds {
	m := rectPattern.FindStringSubmatch(raw)
	if len(m) != 5 {
		return snapshot.Bounds{}
	}
	x, _ := strconv.Atoi(m[1])
	y, _ := strconv.Atoi(m[2])
	w, _ := strconv.Atoi(m[3])
	h, _ := strconv.Atoi(m[4])
	return snapshot.Bounds{
		Left:   int32(x),
		Top:    int32(y),
		Right:  int32(x + w),
		Bottom: int32(y + h),
	}
}

func stringValue(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func attrBoolDefault(attrs []xml.Attr, key string, fallback bool) bool {
	value := attrValue(attrs, key)
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true")
}
