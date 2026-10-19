// Package migrations embeds the versioned SQL migrations applied at startup.
package migrations

import "embed"

// FS holds every migration file in this directory, named NNNNN_name.up.sql.
// It is the single source of truth for the database schema.
//
//go:embed *.sql
var FS embed.FS
