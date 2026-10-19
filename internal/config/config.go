package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
)

// Config holds runtime configuration loaded from the environment.
type Config struct {
	// Addr is the HTTP listen address (host:port).
	Addr string
	// DatabaseURL is the PostgreSQL connection string.
	// Empty means the database is not used yet (bootstrap stage).
	DatabaseURL string
}

// Load reads configuration from the environment and validates it.
// It returns an error describing the offending variable if any value is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		Addr: os.Getenv("APP_ADDR"),
	}

	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return nil, fmt.Errorf("config: invalid APP_ADDR %q: %w", cfg.Addr, err)
	}

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL != "" {
		u, err := url.Parse(cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("config: invalid DATABASE_URL: %w", err)
		}
		if u.Scheme == "" {
			return nil, fmt.Errorf("config: invalid DATABASE_URL: missing scheme")
		}
		if u.Scheme != "postgres" && u.Scheme != "postgresql" {
			return nil, fmt.Errorf("config: invalid DATABASE_URL: unsupported scheme %q (want postgres or postgresql)", u.Scheme)
		}
	}

	return cfg, nil
}
