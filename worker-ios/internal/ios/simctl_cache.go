package ios

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type DeviceInfo struct {
	DeviceID    string
	Name        string
	Model       string
	OSVersion   string
	IsSimulator bool
	Status      string
}

type SimctlCache struct {
	xcrunPath string
	ttl       time.Duration
	mu        sync.RWMutex
	lastFetch time.Time
	devices   []DeviceInfo
}

func NewSimctlCache(xcrunPath string, ttl time.Duration) *SimctlCache {
	return &SimctlCache{xcrunPath: xcrunPath, ttl: ttl}
}

func (c *SimctlCache) List(ctx context.Context) ([]DeviceInfo, error) {
	c.mu.RLock()
	if time.Since(c.lastFetch) < c.ttl && len(c.devices) > 0 {
		cached := append([]DeviceInfo(nil), c.devices...)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()
	return c.refresh(ctx)
}

func (c *SimctlCache) refresh(ctx context.Context) ([]DeviceInfo, error) {
	cmd := exec.CommandContext(ctx, c.xcrunPath, "simctl", "list", "devices", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var payload struct {
		Devices map[string][]struct {
			UDID         string `json:"udid"`
			Name         string `json:"name"`
			State        string `json:"state"`
			IsAvailable  bool   `json:"isAvailable"`
			Availability string `json:"availability"`
		} `json:"devices"`
	}

	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}

	parsed := make([]DeviceInfo, 0, 16)
	for runtime, devices := range payload.Devices {
		version := strings.TrimPrefix(runtime, "com.apple.CoreSimulator.SimRuntime.")
		version = strings.ReplaceAll(version, "-", " ")
		for _, d := range devices {
			if !d.IsAvailable && d.Availability == "(unavailable)" {
				continue
			}
			parsed = append(parsed, DeviceInfo{
				DeviceID:    d.UDID,
				Name:        d.Name,
				Model:       d.Name,
				OSVersion:   version,
				IsSimulator: true,
				Status:      strings.ToLower(d.State),
			})
		}
	}

	c.mu.Lock()
	c.devices = parsed
	c.lastFetch = time.Now().UTC()
	c.mu.Unlock()

	return append([]DeviceInfo(nil), parsed...), nil
}
