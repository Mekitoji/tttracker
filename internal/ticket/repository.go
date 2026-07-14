package ticket

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tttracker/internal/apperr"
	"tttracker/internal/clock"
	"tttracker/internal/db"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository is stateless data access for tickets.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const ticketCols = `id, project_id, number, title, description, type, status, priority, labels, position, created_at, updated_at, completed_at`

// NextNumber returns the next per-project ticket number. Call it inside the
// same transaction as the insert so the number cannot be reused.
func (Repository) NextNumber(ctx context.Context, q db.DBTX, projectID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM tickets WHERE project_id = ?`, projectID).Scan(&n)
	return n, err
}

func (Repository) Insert(ctx context.Context, q db.DBTX, t *Ticket) error {
	res, err := q.ExecContext(
		ctx,
		`INSERT INTO tickets(project_id, number, title, description, type, status, priority, labels, position, created_at, updated_at, completed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ProjectID, t.Number, t.Title, t.Description, string(t.Type), string(t.Status), string(t.Priority),
		marshalLabels(t.Labels), t.Position, clock.Format(t.CreatedAt), clock.Format(t.UpdatedAt), formatNullable(t.CompletedAt),
	)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

// Update writes the whole mutable row; the FTS triggers keep search in sync.
func (Repository) Update(ctx context.Context, q db.DBTX, t *Ticket) error {
	_, err := q.ExecContext(
		ctx,
		`UPDATE tickets SET title=?, description=?, type=?, status=?, priority=?, labels=?, updated_at=?, completed_at=?
		 WHERE id=?`,
		t.Title, t.Description, string(t.Type), string(t.Status), string(t.Priority),
		marshalLabels(t.Labels), clock.Format(t.UpdatedAt), formatNullable(t.CompletedAt), t.ID,
	)
	return err
}

// Delete removes a ticket; its subtasks/comments/attachments/activity cascade,
// and the FTS index is cleaned by the AFTER DELETE trigger.
func (Repository) Delete(ctx context.Context, q db.DBTX, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM tickets WHERE id = ?`, id)
	return err
}

func (Repository) GetByID(ctx context.Context, q db.DBTX, id int64) (*Ticket, error) {
	row := q.QueryRowContext(ctx, `SELECT `+ticketCols+` FROM tickets WHERE id = ?`, id)
	t, err := scanTicket(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: ticket id %d", apperr.ErrNotFound, id)
	}
	return t, err
}

func (Repository) GetByProjectNumber(ctx context.Context, q db.DBTX, projectID int64, number int) (*Ticket, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+ticketCols+` FROM tickets WHERE project_id = ? AND number = ?`, projectID, number)
	t, err := scanTicket(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: ticket %d in project %d", apperr.ErrNotFound, number, projectID)
	}
	return t, err
}

func (Repository) ListByProject(ctx context.Context, q db.DBTX, projectID int64, opts ListOptions) ([]Ticket, error) {
	query := `SELECT ` + ticketCols + ` FROM tickets WHERE project_id = ?`
	args := []any{projectID}
	addIn := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		query += ` AND ` + column + ` IN (` + strings.TrimRight(strings.Repeat("?,", len(values)), ",") + `)`
		for _, value := range values {
			args = append(args, value)
		}
	}
	priorities := make([]string, len(opts.Priorities))
	for i, value := range opts.Priorities {
		priorities[i] = string(value)
	}
	types := make([]string, len(opts.Types))
	for i, value := range opts.Types {
		types[i] = string(value)
	}
	addIn("priority", priorities)
	addIn("type", types)
	if opts.OnlyCurrent {
		query += ` AND status IN ('todo','in_progress','blocked')`
	}
	if opts.WithoutLabels {
		query += ` AND json_array_length(labels) = 0`
	}
	if len(opts.Labels) > 0 {
		query += ` AND EXISTS (SELECT 1 FROM json_each(tickets.labels) WHERE value IN (` + strings.TrimRight(strings.Repeat("?,", len(opts.Labels)), ",") + `))`
		for _, label := range opts.Labels {
			args = append(args, label)
		}
	}
	if opts.StaleBefore != nil {
		query += ` AND updated_at < ?`
		args = append(args, clock.Format(*opts.StaleBefore))
	}
	switch opts.Sort {
	case SortPriority:
		query += ` ORDER BY CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 ELSE 3 END, updated_at DESC, number`
	case SortUpdated:
		query += ` ORDER BY updated_at DESC, number`
	default:
		query += ` ORDER BY position, number`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	var out []Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (Repository) SetPosition(ctx context.Context, q db.DBTX, id int64, position int) error {
	_, err := q.ExecContext(ctx, `UPDATE tickets SET position = ? WHERE id = ?`, position, id)
	return err
}

// Search runs an FTS5 MATCH over tickets and returns hits (with project key)
// ranked by relevance. match must already be a valid FTS5 query string.
func (Repository) Search(ctx context.Context, q db.DBTX, match string) ([]SearchHit, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT t.id, t.project_id, t.number, t.title, t.description, t.type, t.status,
		        t.priority, t.labels, t.position, t.created_at, t.updated_at, t.completed_at, p.key
		 FROM ticket_search s
		 JOIN tickets t ON t.id = s.rowid
		 JOIN projects p ON p.id = t.project_id
		 WHERE ticket_search MATCH ?
		 ORDER BY rank`, match)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	var out []SearchHit
	for rows.Next() {
		var t Ticket
		var typ, st, pr, labels, created, updated, pkey string
		var completed sql.NullString
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.Number, &t.Title, &t.Description,
			&typ, &st, &pr, &labels, &t.Position, &created, &updated, &completed, &pkey,
		); err != nil {
			return nil, err
		}
		t.Type = Type(typ)
		t.Status = Status(st)
		t.Priority = Priority(pr)
		t.Labels = unmarshalLabels(labels)
		t.CreatedAt, _ = clock.Parse(created)
		t.UpdatedAt, _ = clock.Parse(updated)
		if completed.Valid {
			if ct, err := clock.Parse(completed.String); err == nil {
				t.CompletedAt = &ct
			}
		}
		out = append(out, SearchHit{Ticket: t, ProjectKey: pkey})
	}
	return out, rows.Err()
}

func scanTicket(rs rowScanner) (*Ticket, error) {
	var t Ticket
	var typ, st, pr, labels, created, updated string
	var completed sql.NullString
	if err := rs.Scan(
		&t.ID, &t.ProjectID, &t.Number, &t.Title, &t.Description,
		&typ, &st, &pr, &labels, &t.Position, &created, &updated, &completed,
	); err != nil {
		return nil, err
	}
	t.Type = Type(typ)
	t.Status = Status(st)
	t.Priority = Priority(pr)
	t.Labels = unmarshalLabels(labels)
	t.CreatedAt, _ = clock.Parse(created)
	t.UpdatedAt, _ = clock.Parse(updated)
	if completed.Valid {
		if ct, err := clock.Parse(completed.String); err == nil {
			t.CompletedAt = &ct
		}
	}
	return &t, nil
}

func formatNullable(t *time.Time) any {
	if t == nil {
		return nil
	}
	return clock.Format(*t)
}

func marshalLabels(labels []string) string {
	if len(labels) == 0 {
		return "[]"
	}
	b, err := json.Marshal(labels)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalLabels(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
