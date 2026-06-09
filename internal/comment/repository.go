package comment

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

// Repository is stateless data access for comments.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const commentCols = `id, ticket_id, body, created_at, updated_at`

func (Repository) Insert(ctx context.Context, q db.DBTX, c *Comment) error {
	res, err := q.ExecContext(
		ctx,
		`INSERT INTO comments(ticket_id, body, created_at, updated_at) VALUES(?, ?, ?, ?)`,
		c.TicketID, c.Body, clock.Format(c.CreatedAt), clock.Format(c.UpdatedAt),
	)
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (Repository) Update(ctx context.Context, q db.DBTX, c *Comment) error {
	_, err := q.ExecContext(
		ctx,
		`UPDATE comments SET body=?, updated_at=? WHERE id=?`,
		c.Body, clock.Format(c.UpdatedAt), c.ID,
	)
	return err
}

func (Repository) Delete(ctx context.Context, q db.DBTX, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
	return err
}

func (Repository) GetByID(ctx context.Context, q db.DBTX, id int64) (*Comment, error) {
	row := q.QueryRowContext(ctx, `SELECT `+commentCols+` FROM comments WHERE id = ?`, id)
	c, err := scanComment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: comment id %d", apperr.ErrNotFound, id)
	}
	return c, err
}

func (Repository) ListByTicket(ctx context.Context, q db.DBTX, ticketID int64) ([]Comment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+commentCols+` FROM comments WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanComment(rs rowScanner) (*Comment, error) {
	var c Comment
	var created, updated string
	if err := rs.Scan(&c.ID, &c.TicketID, &c.Body, &created, &updated); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = clock.Parse(created)
	c.UpdatedAt, _ = clock.Parse(updated)
	return &c, nil
}
