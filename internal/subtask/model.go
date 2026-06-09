// Package subtask defines the Subtask struct, which represents a checklist item inside a ticket. It includes fields for ID, TicketID, Title, IsDone status, Position, and timestamps for creation and updates.
package subtask

import "time"

// Subtask is a checklist item inside a ticket (not a nested ticket).
type Subtask struct {
	ID        int64
	TicketID  int64
	Title     string
	IsDone    bool
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
}
