package postgres

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mojit7/Backbend/migrations"
)

func TestParseMigrationsOrdersByVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"00002_second.up.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		"00001_first.up.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	ms, err := ParseMigrations(fsys)
	if err != nil {
		t.Fatalf("ParseMigrations() failed: %v", err)
	}
	if len(ms) != 2 || ms[0].Version != 1 || ms[1].Version != 2 {
		t.Fatalf("got %+v, want versions [1 2]", ms)
	}
}

func TestParseMigrationsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		fsys fstest.MapFS
	}{
		{
			"stray file",
			fstest.MapFS{"notes.txt": &fstest.MapFile{Data: []byte("x")}},
		},
		{
			"bad extension",
			fstest.MapFS{"00001_a.sql": &fstest.MapFile{Data: []byte("SELECT 1;")}},
		},
		{
			"duplicate version",
			fstest.MapFS{
				"00001_a.up.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
				"00001_b.up.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseMigrations(tc.fsys); err == nil {
				t.Fatal("ParseMigrations() succeeded, want error")
			}
		})
	}
}

// openTestDB opens a real PostgreSQL connection, or skips the test when
// DATABASE_URL is not set. CI always provides it.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	db, err := Open(databaseURL)
	if err != nil {
		t.Fatalf("Open() failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// embeddedPlus returns an fs.FS containing the real embedded migrations plus
// extra test-only files, so the "applied migration missing from source" check
// is not tripped by unrelated versions already recorded in the test database.
func embeddedPlus(extra fstest.MapFS) fstest.MapFS {
	combined := fstest.MapFS{}
	_ = fs.WalkDir(migrations.FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(migrations.FS, p)
		if err != nil {
			return err
		}
		combined[p] = &fstest.MapFile{Data: data}
		return nil
	})
	for k, v := range extra {
		combined[k] = v
	}
	return combined
}

func TestMigrateAppliesEmbeddedMigrationsIdempotently(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The first run may find migrations already applied if the test database
	// is reused; the invariant is that a second run applies nothing new.
	if _, err := Migrate(ctx, db.DB, migrations.FS); err != nil {
		t.Fatalf("first Migrate() failed: %v", err)
	}
	applied, err := Migrate(ctx, db.DB, migrations.FS)
	if err != nil {
		t.Fatalf("second Migrate() failed: %v", err)
	}
	if applied != 0 {
		t.Errorf("second Migrate() applied %d migrations, want 0", applied)
	}

	var name string
	err = db.QueryRowContext(ctx, `SELECT name FROM schema_migrations WHERE version = 1`).Scan(&name)
	if err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if name != "platform_settings" {
		t.Errorf("recorded name = %q, want %q", name, "platform_settings")
	}
}

// The integration tests below share one database and run in source order.
// Because removing an applied migration is an error by design, every Migrate
// call in a later test must include every migration applied by earlier tests.
const tamperSQL = "CREATE TABLE tamper_probe (id int);"

func TestMigrateRejectsEditedAppliedMigration(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	original := embeddedPlus(fstest.MapFS{
		"90001_tamper.up.sql": &fstest.MapFile{Data: []byte(tamperSQL)},
	})
	if _, err := Migrate(ctx, db.DB, original); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	edited := embeddedPlus(fstest.MapFS{
		"90001_tamper.up.sql": &fstest.MapFile{Data: []byte("CREATE TABLE tamper_probe_edited (id int);")},
	})
	_, err := Migrate(ctx, db.DB, edited)
	if err == nil {
		t.Fatal("Migrate() with edited applied migration succeeded, want error")
	}
	if !strings.Contains(err.Error(), "different content") {
		t.Errorf("error = %v, want checksum mismatch error", err)
	}
}

func TestMigrateHoldsAdvisoryLockDuringRun(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Acquire the lock ourselves first; Migrate must block until we
	// release it, proving it takes (and waits on) the same advisory lock.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("getting connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		t.Fatalf("pg_advisory_lock failed: %v", err)
	}

	migDone := make(chan error, 1)
	go func() {
		_, err := Migrate(ctx, db.DB, migrations.FS)
		migDone <- err
	}()

	// Migrate must be blocked while we hold the lock. Give it a moment, then
	// confirm it has not completed.
	select {
	case err := <-migDone:
		if err == nil {
			t.Fatal("Migrate() completed while advisory lock was held elsewhere, want it to block")
		}
		t.Fatalf("Migrate() returned error while waiting for lock: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Still blocked: expected.
	}

	// Release the lock; Migrate should now finish successfully.
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockKey); err != nil {
		t.Fatalf("pg_advisory_unlock failed: %v", err)
	}
	select {
	case err := <-migDone:
		if err != nil {
			t.Fatalf("Migrate() after lock release failed: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate() did not finish after advisory lock was released")
	}
}

func TestMigrateReleasesAdvisoryLock(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := Migrate(ctx, db.DB, migrations.FS); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	// After Migrate returns, another session must be able to take the lock.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("getting connection: %v", err)
	}
	defer conn.Close()

	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, migrationLockKey).Scan(&acquired); err != nil {
		t.Fatalf("pg_try_advisory_lock failed: %v", err)
	}
	if !acquired {
		t.Fatal("advisory lock still held after Migrate returned, want released")
	}
	_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationLockKey)
}

func TestMigrateRejectsMissingAppliedMigration(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	withProbe := embeddedPlus(fstest.MapFS{
		"90001_tamper.up.sql": &fstest.MapFile{Data: []byte(tamperSQL)},
		"91001_gone.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE gone_probe (id int);")},
	})
	if _, err := Migrate(ctx, db.DB, withProbe); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	if _, err := Migrate(ctx, db.DB, migrations.FS); err == nil {
		t.Fatal("Migrate() with removed applied migration succeeded, want error")
	}
}
