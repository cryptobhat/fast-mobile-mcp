package device

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/fast-mobile-mcp/worker-android/internal/android"
	"github.com/fast-mobile-mcp/worker-android/internal/config"
)

type Runtime struct {
	DeviceID string
	Executor *Executor
	UIA2     *android.UIA2Client
}

type Registry struct {
	cfg       config.Config
	adbCache  *android.ADBCache
	mu        sync.Mutex
	runtimes  map[string]*Runtime
	portByDev map[string]int
	nextPort  int
}

func NewRegistry(cfg config.Config) *Registry {
	return &Registry{
		cfg:       cfg,
		adbCache:  android.NewADBCache(cfg.ADBPath, cfg.DeviceCacheTTL),
		runtimes:  make(map[string]*Runtime),
		portByDev: make(map[string]int),
		nextPort:  cfg.UIA2BasePort,
	}
}

func (r *Registry) ListDevices(ctx context.Context) ([]android.DeviceInfo, error) {
	return r.adbCache.List(ctx)
}

func (r *Registry) RuntimeForDevice(ctx context.Context, deviceID string) (*Runtime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.runtimes[deviceID]; ok {
		return existing, nil
	}

	port := r.assignPort(deviceID)
	if err := r.ensurePortForward(ctx, deviceID, port); err != nil {
		return nil, fmt.Errorf("adb port-forward failed: %w", err)
	}

	client := android.NewUIA2Client(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err := client.EnsureSession(ctx); err != nil {
		return nil, err
	}

	runtime := &Runtime{
		DeviceID: deviceID,
		Executor: NewExecutor(256),
		UIA2:     client,
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

func (r *Registry) ensurePortForward(ctx context.Context, deviceID string, localPort int) error {
	cmd := exec.CommandContext(ctx, r.cfg.ADBPath, "-s", deviceID, "forward", fmt.Sprintf("tcp:%d", localPort), fmt.Sprintf("tcp:%d", r.cfg.UIA2DevicePort))
	return cmd.Run()
}
