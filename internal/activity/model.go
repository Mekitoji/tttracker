// Package activity records an append-only history of ticket changes. Activity
// events are not the source of truth (the normal tables are); they exist for
// history, debugging, and a future export/undo.
package activity

import (
	"encoding/json"
	"time"
)

// EventType enumerates the recorded change kinds.
type EventType string

const (
	TicketCreated            EventType = "ticket.created"
	TicketTitleChanged       EventType = "ticket.title_changed"
	TicketDescriptionChanged EventType = "ticket.description_changed"
	TicketStatusChanged      EventType = "ticket.status_changed"
	TicketPriorityChanged    EventType = "ticket.priority_changed"
	TicketTypeChanged        EventType = "ticket.type_changed"
	TicketLabelsChanged      EventType = "ticket.labels_changed"

	SubtaskCreated   EventType = "subtask.created"
	SubtaskCompleted EventType = "subtask.completed"
	SubtaskReopened  EventType = "subtask.reopened"
	SubtaskRenamed   EventType = "subtask.renamed"
	SubtaskDeleted   EventType = "subtask.deleted"

	CommentCreated EventType = "comment.created"
	CommentUpdated EventType = "comment.updated"
	CommentDeleted EventType = "comment.deleted"

	AttachmentAdded   EventType = "attachment.added"
	AttachmentRemoved EventType = "attachment.removed"
)

// Event is one recorded change. Payload is event-type-specific JSON; its shape
// is versioned by SchemaVersion.
type Event struct {
	ID            int64
	TicketID      int64
	Type          EventType
	SchemaVersion int
	Payload       json.RawMessage
	CreatedAt     time.Time
}

// Payload shapes (kept small and stable; schema_version guards future changes).

// CreatedPayload is recorded for TicketCreated.
type CreatedPayload struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// StringChange is recorded for single-field string transitions (title, status,
// priority, type, description-touched).
type StringChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// LabelsChange is recorded for TicketLabelsChanged.
type LabelsChange struct {
	From []string `json:"from"`
	To   []string `json:"to"`
}

// SubtaskRef is recorded for subtask.* events.
type SubtaskRef struct {
	SubtaskID int64  `json:"subtask_id"`
	Title     string `json:"title,omitempty"`
}

// CommentRef is recorded for comment.* events.
type CommentRef struct {
	CommentID int64 `json:"comment_id"`
}

// AttachmentRef is recorded for attachment.* events.
type AttachmentRef struct {
	AttachmentID int64  `json:"attachment_id"`
	FileName     string `json:"file_name"`
}
