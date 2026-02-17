package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr            string
	DeviceCacheTTL        time.Duration
	SnapshotTTL           time.Duration
	SnapshotCleanup       time.Duration
	MaxSnapshotsPerDevice int
	ActionTimeout         time.Duration
	StreamChunkBytes      int
	StreamMaxFPS          int
	SimctlPath            string
	WDABasePort           int
	WDAHost               string
	WDAScheme             string
	LogLevel              string
}

func Load() Config {
	return Config{
		ListenAddr:            getEnv("GRPC_LISTEN_ADDR", ":50052"),
		DeviceCacheTTL:        getDuration("DEVICE_CACHE_TTL", 5*time.Second),
		SnapshotTTL:           getDuration("SNAPSHOT_TTL", 30*time.Second),
		SnapshotCleanup:       getDuration("SNAPSHOT_CLEANUP_INTERVAL", 10*time.Second),
		MaxSnapshotsPerDevice: getInt("MAX_SNAPSHOTS_PER_DEVICE", 8),
		ActionTimeout:         getDuration("ACTION_TIMEOUT", 2*time.Second),
		StreamChunkBytes:      getInt("STREAM_CHUNK_BYTES", 65536),
		StreamMaxFPS:          getInt("STREAM_MAX_FPS", 12),
		SimctlPath:            getEnv("SIMCTL_PATH", "xcrun"),
		WDABasePort:           getInt("WDA_BASE_PORT", 8100),
		WDAHost:               getEnv("WDA_HOST", "127.0.0.1"),
		WDAScheme:             getEnv("WDA_SCHEME", "http"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}
