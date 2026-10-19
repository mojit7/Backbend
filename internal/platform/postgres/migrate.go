package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

// migrationFilePattern matches versioned migration files like "00001_name.up.sql".
var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_([a-z0-9_]+)\.up\.sql$`)

// Migration is a single parsed SQL migration file.
type Migration struct {
	Version  uint64
	Name     string
	Checksum string
	SQL      string
}

// ParseMigrations reads all migration files from fsys and returns them
// ordered by version. It rejects malformed filenames and duplicate versions.
func ParseMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: reading migrations: %w", err)
	}

	var ms []Migration
	seen := make(map[uint64]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: %q is not a valid migration filename (want NNNNN_name.up.sql)", e.Name())
		}
		version, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migrate: %q: %w", e.Name(), err)
		}
		if seen[version] {
			return nil, fmt.Errorf("migrate: duplicate migration version %d (%s)", version, e.Name())
		}
		seen[version] = true

		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: reading %q: %w", e.Name(), err)
		}
		sum := sha256.Sum256(data)
		ms = append(ms, Migration{
			Version:  version,
			Name:     m[2],
			Checksum: hex.EncodeToString(sum[:]),
			SQL:      string(data),
		})
	}

	sort.Slice(ms, func(i, j int) bool { return ms[i].Version < ms[j].Version })
	return ms, nil
}

// Migrate applies all pending migrations from fsys to db, in version order,
// each inside its own transaction so a failed migration leaves no partial
// state. Already-applied migrations are skipped after verifying their
// checksum matches the applied record; editing or deleting an applied
// migration is an error. It returns the number of newly applied migrations.
// Errors identify the offending migration but never include connection
// details or credentials.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) (int, error) {
	ms, err := ParseMigrations(fsys)
	if err != nil {
		return 0, err
	}

	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    bigint PRIMARY KEY,
		name       text NOT NULL,
		checksum   text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return 0, fmt.Errorf("migrate: ensuring schema_migrations table: %w", err)
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return 0, err
	}

	known := make(map[uint64]bool, len(ms))
	for _, m := range ms {
		known[m.Version] = true
	}
	for version := range applied {
		if !known[version] {
			return 0, fmt.Errorf("migrate: applied migration %d is missing from the migrations source", version)
		}
	}

	n := 0
	for _, m := range ms {
		if checksum, ok := applied[m.Version]; ok {
			if checksum != m.Checksum {
				return n, fmt.Errorf("migrate: migration %05d_%s was already applied with different content; editing applied migrations is not allowed", m.Version, m.Name)
			}
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func appliedMigrations(ctx context.Context, db *sql.DB) (map[uint64]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[uint64]string)
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
		}
		applied[uint64(version)] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: reading applied migrations: %w", err)
	}
	return applied, nil
}

func applyMigration(ctx context.Context, db *sql.DB, m Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: migration %05d_%s: %w", m.Version, m.Name, err)
	}
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrate: migration %05d_%s: %w", m.Version, m.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		int64(m.Version), m.Name, m.Checksum,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrate: migration %05d_%s: recording: %w", m.Version, m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: migration %05d_%s: commit: %w", m.Version, m.Name, err)
	}
	return nil
}
