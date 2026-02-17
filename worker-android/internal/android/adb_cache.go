package android

import (
	"bufio"
	"context"
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

type ADBCache struct {
	adbPath   string
	ttl       time.Duration
	mu        sync.RWMutex
	lastFetch time.Time
	devices   []DeviceInfo
}

func NewADBCache(adbPath string, ttl time.Duration) *ADBCache {
	return &ADBCache{adbPath: adbPath, ttl: ttl}
}

func (c *ADBCache) List(ctx context.Context) ([]DeviceInfo, error) {
	c.mu.RLock()
	if time.Since(c.lastFetch) < c.ttl && len(c.devices) > 0 {
		cached := append([]DeviceInfo(nil), c.devices...)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	return c.refresh(ctx)
}

func (c *ADBCache) refresh(ctx context.Context) ([]DeviceInfo, error) {
	cmd := exec.CommandContext(ctx, c.adbPath, "devices", "-l")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	devices := parseADBDevices(string(out))
	c.mu.Lock()
	c.devices = devices
	c.lastFetch = time.Now().UTC()
	c.mu.Unlock()
	return append([]DeviceInfo(nil), devices...), nil
}

func parseADBDevices(raw string) []DeviceInfo {
	s := bufio.NewScanner(strings.NewReader(raw))
	out := make([]DeviceInfo, 0)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status := parts[1]

		info := DeviceInfo{
			DeviceID:  parts[0],
			Status:    status,
			Name:      parts[0],
			OSVersion: "unknown",
		}
		for _, p := range parts[2:] {
			if strings.HasPrefix(p, "model:") {
				info.Model = strings.TrimPrefix(p, "model:")
			}
			if strings.HasPrefix(p, "device:") {
				info.Name = strings.TrimPrefix(p, "device:")
			}
		}
		out = append(out, info)
	}

	return out
}
