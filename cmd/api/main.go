// Command api is the single entry point of the store backend.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mojit7/Backbend/internal/config"
	"github.com/mojit7/Backbend/internal/httpserver"
	"github.com/mojit7/Backbend/internal/platform/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("startup failed: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var db *postgres.DB
	if cfg.DatabaseURL == "" {
		log.Printf("DATABASE_URL not set; starting without a database (GET /readyz will report not_configured)")
	} else {
		db, err = postgres.Open(cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := db.Close(); cerr != nil {
				log.Printf("closing database: %v", cerr)
			}
		}()
	}

	srv := httpserver.New(cfg.Addr, db)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("api listening on %s", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
