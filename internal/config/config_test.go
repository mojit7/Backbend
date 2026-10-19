package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ADDR", "")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with defaults returned error: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoadCustomAddr(t *testing.T) {
	t.Setenv("APP_ADDR", "127.0.0.1:9000")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9000" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:9000")
	}
}

func TestLoadInvalidAddr(t *testing.T) {
	t.Setenv("APP_ADDR", "not-an-address")
	t.Setenv("DATABASE_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with invalid APP_ADDR succeeded, want error")
	}
}

func TestLoadDatabaseURL(t *testing.T) {
	t.Setenv("APP_ADDR", "")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/store?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL is empty, want the provided value")
	}
}

func TestLoadInvalidDatabaseURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"unsupported scheme", "mysql://user:pass@localhost/store"},
		{"unparseable", "://\x7f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_ADDR", "")
			t.Setenv("DATABASE_URL", tc.url)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() with DATABASE_URL=%q succeeded, want error", tc.url)
			}
		})
	}
}
