package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsInvalidURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", "  "},
		{"unsupported scheme", "mysql://user:pass@localhost/store"},
		{"unparseable", "://\x7f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Open(tc.url); err == nil {
				t.Fatalf("Open(%q) succeeded, want error", tc.url)
			}
		})
	}
}

func TestOpenErrorsDoNotLeakURL(t *testing.T) {
	const secretURL = "postgres://user:supersecret@localhost:5432/store"
	_, err := Open(secretURL)
	if err == nil {
		t.Fatal("Open() with unreachable host succeeded, want error")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("error message leaks credentials: %v", err)
	}
}

// TestOpenConnects is an integration test against a real PostgreSQL server.
// It is skipped when DATABASE_URL is not set, so local `go test` without a
// database still runs; CI provides a PostgreSQL service and always runs it.
func TestOpenConnects(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping PostgreSQL integration test")
	}

	db, err := Open(databaseURL)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext() failed: %v", err)
	}
}
