// Package config loads worker configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// DefaultMaxChunkBytes is 1 MiB.
const DefaultMaxChunkBytes int64 = 1 << 20

// Config holds worker settings.
type Config struct {
	WorkerID      string // WORKER_ID, defaults to the hostname
	Port          int    // PORT, default 8080
	DataDir       string // DATA_DIR, default /data
	MaxChunkBytes int64  // MAX_CHUNK_BYTES, default 1 MiB
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		WorkerID:      os.Getenv("WORKER_ID"),
		Port:          8080,
		DataDir:       envOr("DATA_DIR", "/data"),
		MaxChunkBytes: DefaultMaxChunkBytes,
	}
	if cfg.WorkerID == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "worker"
		}
		cfg.WorkerID = host
	}
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 1 || p > 65535 {
			return Config{}, fmt.Errorf("invalid PORT %q", v)
		}
		cfg.Port = p
	}
	if v := os.Getenv("MAX_CHUNK_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid MAX_CHUNK_BYTES %q", v)
		}
		cfg.MaxChunkBytes = n
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
