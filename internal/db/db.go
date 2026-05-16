// Package db owns the SQLite connection, migration execution, and typed CRUD
// functions used by the API and scraper layers.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	// Pure-Go SQLite driver. Registered as "sqlite". No CGO required, which means
	// the final binary is a single statically-linked file (spec § 10).
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps *sql.DB so query methods can hang off a single type.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path, applies the connection
// PRAGMAs required by spec § 10, and returns a ready-to-use DB.
//
// PRAGMAs applied:
//   - journal_mode=WAL       concurrent readers + single writer; required for the poller-vs-API split
//   - foreign_keys=ON        SQLite defaults this off per connection; we want CASCADE to actually cascade
//   - synchronous=NORMAL     safe with WAL; durability on commit boundaries is fine for this app
//   - busy_timeout=5000      waits up to 5s on a locked DB before erroring; smooths over the rare poller/API write collision
func Open(path string) (*DB, error) {
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := sqldb.Exec(p); err != nil {
			_ = sqldb.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	return &DB{sqldb}, nil
}

// Migrate runs every embedded migration file that hasn't already been applied,
// in lexical filename order, each inside its own transaction. Applied filenames
// are recorded in schema_migrations so reruns are no-ops.
//
// Filename ordering is the contract here — name migrations 001_*, 002_*, etc.
// in lockstep with their dependencies.
func (db *DB) Migrate(ctx context.Context, logger *slog.Logger) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := db.appliedMigrations(ctx)
	if err != nil {
		return err
	}

	files, err := listMigrationFiles()
	if err != nil {
		return err
	}

	pending := 0
	for _, name := range files {
		if applied[name] {
			logger.Debug("migration already applied", "file", name)
			continue
		}
		if err := db.applyMigration(ctx, name); err != nil {
			return err
		}
		logger.Info("migration applied", "file", name)
		pending++
	}

	if pending == 0 {
		logger.Info("schema up to date", "applied", len(applied))
	}
	return nil
}

func (db *DB) appliedMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

func listMigrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func (db *DB) applyMigration(ctx context.Context, name string) error {
	sqlBytes, err := fs.ReadFile(migrationsFS, "migrations/"+name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if committed

	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("exec %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (filename) VALUES (?)", name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}
