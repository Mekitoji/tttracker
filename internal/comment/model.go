// Package comment defines the Comment struct, which represents a Markdown note attached to a ticket.
package comment

import "time"

// Comment is a Markdown note attached to a ticket.
type Comment struct {
	ID        int64
	TicketID  int64
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}
