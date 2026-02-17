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
	ADBPath               string
	UIA2BasePort          int
	UIA2DevicePort        int
	LogLevel              string
}

func Load() Config {
	return Config{
		ListenAddr:            getEnv("GRPC_LISTEN_ADDR", ":50051"),
		DeviceCacheTTL:        getDuration("DEVICE_CACHE_TTL", 3*time.Second),
		SnapshotTTL:           getDuration("SNAPSHOT_TTL", 30*time.Second),
		SnapshotCleanup:       getDuration("SNAPSHOT_CLEANUP_INTERVAL", 10*time.Second),
		MaxSnapshotsPerDevice: getInt("MAX_SNAPSHOTS_PER_DEVICE", 8),
		ActionTimeout:         getDuration("ACTION_TIMEOUT", 2*time.Second),
		StreamChunkBytes:      getInt("STREAM_CHUNK_BYTES", 65536),
		StreamMaxFPS:          getInt("STREAM_MAX_FPS", 15),
		ADBPath:               getEnv("ADB_PATH", "adb"),
		UIA2BasePort:          getInt("UIA2_BASE_PORT", 7900),
		UIA2DevicePort:        getInt("UIA2_DEVICE_PORT", 7912),
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
