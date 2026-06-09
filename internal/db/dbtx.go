package db

import (
	"context"
	"database/sql"
)

// DBTX is the subset of *sql.DB / *sql.Tx that repositories use. Read-only
// service methods pass the *sql.DB directly; mutating methods run inside
// WithTx and pass the *sql.Tx, so the state change and its activity event share
// one transaction.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// WithTx runs fn inside a transaction, committing on success and rolling back
// on error or panic. Inside fn, all database access must go through the
// provided tx (the pool is capped at one connection, so touching the *sql.DB
// while a tx is open would deadlock).
func WithTx(ctx context.Context, database *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
