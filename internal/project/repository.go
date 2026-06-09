package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"tttracker/internal/apperr"
	"tttracker/internal/clock"
	"tttracker/internal/db"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository is a stateless data-access type; every method takes a db.DBTX so
// the caller decides whether the work runs on the pool or inside a transaction.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const projectCols = `id, key, name, description, repo_path, created_at, updated_at`

func (Repository) Insert(ctx context.Context, q db.DBTX, p *Project) error {
	res, err := q.ExecContext(
		ctx,
		`INSERT INTO projects(key, name, description, repo_path, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?)`,
		p.Key, p.Name, p.Description, p.RepoPath, clock.Format(p.CreatedAt), clock.Format(p.UpdatedAt),
	)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

// Update writes the project's mutable fields (name, description, repo_path).
func (Repository) Update(ctx context.Context, q db.DBTX, p *Project) error {
	_, err := q.ExecContext(
		ctx,
		`UPDATE projects SET name=?, description=?, repo_path=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, p.RepoPath, clock.Format(p.UpdatedAt), p.ID,
	)
	return err
}

func (Repository) GetByKey(ctx context.Context, q db.DBTX, key string) (*Project, error) {
	row := q.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE key = ?`, key)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: project %q", apperr.ErrNotFound, key)
	}
	return p, err
}

func (Repository) GetByID(ctx context.Context, q db.DBTX, id int64) (*Project, error) {
	row := q.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: project id %d", apperr.ErrNotFound, id)
	}
	return p, err
}

func (Repository) List(ctx context.Context, q db.DBTX) ([]Project, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+projectCols+` FROM projects ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func scanProject(rs rowScanner) (*Project, error) {
	var p Project
	var created, updated string
	if err := rs.Scan(&p.ID, &p.Key, &p.Name, &p.Description, &p.RepoPath, &created, &updated); err != nil {
		return nil, err
	}
	p.CreatedAt, _ = clock.Parse(created)
	p.UpdatedAt, _ = clock.Parse(updated)
	return &p, nil
}
