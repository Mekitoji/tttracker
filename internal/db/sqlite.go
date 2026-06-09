// Package db opens the SQLite database with the pragmas the tracker relies on
// and applies embedded migrations.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens (creating if needed) the SQLite database at path.
//
// Pragmas, set per-connection via the DSN:
//   - foreign_keys(1):      enforce ON DELETE CASCADE (off by default in SQLite)
//   - journal_mode(WAL):    better read/write concurrency, fewer locks
//   - busy_timeout(5000):   wait instead of failing immediately on a lock
//   - recursive_triggers(1): ensure FK-cascade deletes still fire the FTS sync
//     triggers, so the search index never goes stale
//
// A single open connection is used: a personal, single-user tool never needs
// concurrent writers and this sidesteps "database is locked" entirely.
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=recursive_triggers(1)",
		path,
	)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return database, nil
}
