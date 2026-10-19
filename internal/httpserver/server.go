// Package httpserver owns the HTTP layer of the store backend:
// routing, middleware, and request/response encoding.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Server wraps the HTTP server and its routing.
type Server struct {
	http *http.Server
}

// New builds the server with all routes registered.
func New(addr string) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)

	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// ListenAndServe starts serving and blocks until the server stops.
// It returns nil on graceful shutdown.
func (s *Server) ListenAndServe() error {
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
