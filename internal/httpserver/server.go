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

// Pinger reports database readiness. It is satisfied by *sql.DB.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// readyTimeout bounds the database check performed by /readyz.
const readyTimeout = 2 * time.Second

// Server wraps the HTTP server and its routing.
type Server struct {
	http *http.Server
	db   Pinger
}

// New builds the server with all routes registered. db may be nil; a nil db
// makes /readyz report the database as not configured.
func New(addr string, db Pinger) *Server {
	s := &Server{
		http: &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
		},
		db: db,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	s.http.Handler = mux

	return s
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

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.db == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "unavailable",
			"database": "not_configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "unavailable",
			"database": "down",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"database": "up",
	})
}
