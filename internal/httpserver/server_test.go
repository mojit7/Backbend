package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct {
	err error
}

func (f fakePinger) PingContext(ctx context.Context) error { return f.err }

func newTestServer(t *testing.T, db Pinger) *httptest.Server {
	t.Helper()
	srv := New(":0", db)
	ts := httptest.NewServer(srv.http.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t, fakePinger{})

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("body status = %q, want %q", body["status"], "ok")
	}
}

func TestHealthzMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, fakePinger{})

	resp, err := http.Post(ts.URL+"/healthz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /healthz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestReadyzDatabaseUp(t *testing.T) {
	ts := newTestServer(t, fakePinger{})

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["database"] != "up" {
		t.Errorf("body database = %q, want %q", body["database"], "up")
	}
}

func TestReadyzDatabaseDown(t *testing.T) {
	ts := newTestServer(t, fakePinger{err: errors.New("connection refused")})

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["database"] != "down" {
		t.Errorf("body database = %q, want %q", body["database"], "down")
	}
}

func TestReadyzDatabaseNotConfigured(t *testing.T) {
	ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/readyz")
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["database"] != "not_configured" {
		t.Errorf("body database = %q, want %q", body["database"], "not_configured")
	}
}

func TestReadyzMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t, fakePinger{})

	resp, err := http.Post(ts.URL+"/readyz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /readyz failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
