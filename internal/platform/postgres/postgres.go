// Package postgres owns the PostgreSQL connection layer of the store backend.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	// Registered purely for its database/sql driver; no direct use.
	_ "github.com/lib/pq"
)

// pingTimeout bounds each readiness check performed by this package.
const pingTimeout = 5 * time.Second

// DB wraps the database/sql connection pool.
type DB struct {
	*sql.DB
}

// Open validates databaseURL, opens a connection pool and verifies the
// server is reachable with a bounded ping. It returns descriptive errors
// and never embeds the connection string or credentials in them.
func Open(databaseURL string) (*DB, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("postgres: DATABASE_URL is empty")
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: invalid DATABASE_URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("postgres: unsupported DATABASE_URL scheme %q", u.Scheme)
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: could not open connection pool: %w", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)

	d := &DB{DB: db}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := d.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: cannot reach database: %w", err)
	}
	return d, nil
}
