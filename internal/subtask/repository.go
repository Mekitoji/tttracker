package subtask

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

// Repository is stateless data access for subtasks.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const subtaskCols = `id, ticket_id, title, is_done, position, created_at, updated_at`

// NextPosition returns the next 1-based position within a ticket.
func (Repository) NextPosition(ctx context.Context, q db.DBTX, ticketID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM subtasks WHERE ticket_id = ?`, ticketID).Scan(&n)
	return n, err
}

func (Repository) Insert(ctx context.Context, q db.DBTX, s *Subtask) error {
	res, err := q.ExecContext(
		ctx,
		`INSERT INTO subtasks(ticket_id, title, is_done, position, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		s.TicketID, s.Title, boolToInt(s.IsDone), s.Position, clock.Format(s.CreatedAt), clock.Format(s.UpdatedAt),
	)
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}

func (Repository) Update(ctx context.Context, q db.DBTX, s *Subtask) error {
	_, err := q.ExecContext(
		ctx,
		`UPDATE subtasks SET title=?, is_done=?, position=?, updated_at=? WHERE id=?`,
		s.Title, boolToInt(s.IsDone), s.Position, clock.Format(s.UpdatedAt), s.ID,
	)
	return err
}

// SetPosition updates a single subtask's position (used by Reorder), scoped to
// its ticket so a wrong id cannot move another ticket's item.
func (Repository) SetPosition(ctx context.Context, q db.DBTX, id, ticketID int64, position int, updatedAt string) error {
	_, err := q.ExecContext(
		ctx,
		`UPDATE subtasks SET position=?, updated_at=? WHERE id=? AND ticket_id=?`,
		position, updatedAt, id, ticketID,
	)
	return err
}

func (Repository) Delete(ctx context.Context, q db.DBTX, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM subtasks WHERE id = ?`, id)
	return err
}

func (Repository) GetByID(ctx context.Context, q db.DBTX, id int64) (*Subtask, error) {
	row := q.QueryRowContext(ctx, `SELECT `+subtaskCols+` FROM subtasks WHERE id = ?`, id)
	s, err := scanSubtask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: subtask id %d", apperr.ErrNotFound, id)
	}
	return s, err
}

func (Repository) ListByTicket(ctx context.Context, q db.DBTX, ticketID int64) ([]Subtask, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+subtaskCols+` FROM subtasks WHERE ticket_id = ? ORDER BY position, id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	var out []Subtask
	for rows.Next() {
		s, err := scanSubtask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func scanSubtask(rs rowScanner) (*Subtask, error) {
	var s Subtask
	var done int
	var created, updated string
	if err := rs.Scan(&s.ID, &s.TicketID, &s.Title, &done, &s.Position, &created, &updated); err != nil {
		return nil, err
	}
	s.IsDone = done != 0
	s.CreatedAt, _ = clock.Parse(created)
	s.UpdatedAt, _ = clock.Parse(updated)
	return &s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
