package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tttracker/internal/clock"
	"tttracker/internal/db"
)

// Repository reads and appends activity events.
type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Record marshals payload to JSON and appends an event. Callers pass the same
// `now` they use for the accompanying state change, and the same tx, so the two
// commit together.
func (Repository) Record(ctx context.Context, q db.DBTX, ticketID int64, t EventType, payload any, now time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(
		ctx,
		`INSERT INTO activity_events(ticket_id, event_type, schema_version, payload, created_at)
		 VALUES(?, ?, ?, ?, ?)`,
		ticketID, string(t), 1, string(raw), clock.Format(now),
	)
	return err
}

// List returns a ticket's events in chronological (insertion) order.
func (Repository) List(ctx context.Context, q db.DBTX, ticketID int64) ([]Event, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, ticket_id, event_type, schema_version, payload, created_at
		 FROM activity_events WHERE ticket_id = ? ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	var out []Event
	for rows.Next() {
		var e Event
		var et, payload, created string
		if err := rows.Scan(&e.ID, &e.TicketID, &et, &e.SchemaVersion, &payload, &created); err != nil {
			return nil, err
		}
		e.Type = EventType(et)
		e.Payload = json.RawMessage(payload)
		e.CreatedAt, _ = clock.Parse(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
