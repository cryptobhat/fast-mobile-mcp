package device

import (
	"context"
	"fmt"
	"sync"

	"github.com/fast-mobile-mcp/worker-ios/internal/config"
	"github.com/fast-mobile-mcp/worker-ios/internal/ios"
)

type Runtime struct {
	DeviceID string
	Executor *Executor
	WDA      *ios.WDAClient
}

type Registry struct {
	cfg       config.Config
	deviceTTL *ios.SimctlCache
	mu        sync.Mutex
	runtimes  map[string]*Runtime
	portByDev map[string]int
	nextPort  int
}

func NewRegistry(cfg config.Config) *Registry {
	return &Registry{
		cfg:       cfg,
		deviceTTL: ios.NewSimctlCache(cfg.SimctlPath, cfg.DeviceCacheTTL),
		runtimes:  make(map[string]*Runtime),
		portByDev: make(map[string]int),
		nextPort:  cfg.WDABasePort,
	}
}

func (r *Registry) ListDevices(ctx context.Context) ([]ios.DeviceInfo, error) {
	return r.deviceTTL.List(ctx)
}

func (r *Registry) RuntimeForDevice(ctx context.Context, deviceID string) (*Runtime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.runtimes[deviceID]; ok {
		return existing, nil
	}

	port := r.assignPort(deviceID)
	baseURL := fmt.Sprintf("%s://%s:%d", r.cfg.WDAScheme, r.cfg.WDAHost, port)
	client := ios.NewWDAClient(baseURL)
	if err := client.EnsureSession(ctx); err != nil {
		return nil, err
	}

	runtime := &Runtime{
		DeviceID: deviceID,
		Executor: NewExecutor(256),
		WDA:      client,
	}
	r.runtimes[deviceID] = runtime
	return runtime, nil
}

func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runtime := range r.runtimes {
		runtime.Executor.Close()
	}
}

func (r *Registry) assignPort(deviceID string) int {
	if p, ok := r.portByDev[deviceID]; ok {
		return p
	}
	p := r.nextPort
	r.nextPort++
	r.portByDev[deviceID] = p
	return p
}
