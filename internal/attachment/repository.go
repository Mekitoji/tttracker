package attachment

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

// Repository is stateless data access for attachment metadata.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

const attachmentCols = `id, ticket_id, file_name, original_path, stored_path, mime_type, size_bytes, created_at`

func (Repository) Insert(ctx context.Context, q db.DBTX, a *Attachment) error {
	res, err := q.ExecContext(
		ctx,
		`INSERT INTO attachments(ticket_id, file_name, original_path, stored_path, mime_type, size_bytes, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		a.TicketID, a.FileName, nullIfEmpty(a.OriginalPath), a.StoredPath,
		nullIfEmpty(a.MimeType), a.SizeBytes, clock.Format(a.CreatedAt),
	)
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

func (Repository) Delete(ctx context.Context, q db.DBTX, id int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	return err
}

func (Repository) GetByID(ctx context.Context, q db.DBTX, id int64) (*Attachment, error) {
	row := q.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM attachments WHERE id = ?`, id)
	a, err := scanAttachment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: attachment id %d", apperr.ErrNotFound, id)
	}
	return a, err
}

func (Repository) ListByTicket(ctx context.Context, q db.DBTX, ticketID int64) ([]Attachment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() // close error is inconsequential after a read

	var out []Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAttachment(rs rowScanner) (*Attachment, error) {
	var a Attachment
	var orig, mimeType sql.NullString
	var created string
	if err := rs.Scan(
		&a.ID, &a.TicketID, &a.FileName, &orig, &a.StoredPath, &mimeType, &a.SizeBytes, &created,
	); err != nil {
		return nil, err
	}
	a.OriginalPath = orig.String
	a.MimeType = mimeType.String
	a.CreatedAt, _ = clock.Parse(created)
	return &a, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
