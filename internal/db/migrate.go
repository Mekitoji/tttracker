package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded migration that has not yet been recorded, in
// lexical filename order. Each migration runs in its own transaction together
// with the bookkeeping insert, so a failed migration leaves no partial state.
//
// The full SQL file (including multi-statement CREATE TRIGGER bodies) is handed
// to a single Exec; the SQLite driver executes all statements in the script,
// which avoids having to split on ";" and mangle trigger bodies.
func Migrate(database *sql.DB) error {
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name       TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := migrationFiles()
	if err != nil {
		return err
	}

	for _, name := range names {
		applied, err := isApplied(database, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := applyMigration(database, name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func migrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func isApplied(database *sql.DB, name string) (bool, error) {
	var n int
	err := database.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE name = ?`, name).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func applyMigration(database *sql.DB, name, body string) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(body); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations(name, applied_at) VALUES(?, datetime('now'))`, name,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record %s: %w", name, err)
	}
	return tx.Commit()
}
