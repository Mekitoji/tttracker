// Package ticket defines the core domain model for work items, including types and
package ticket

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tttracker/internal/apperr"
)

// Type is the kind of work a ticket represents.
type Type string

const (
	TypeTask  Type = "task"
	TypeBug   Type = "bug"
	TypeIdea  Type = "idea"
	TypeChore Type = "chore"
)

func (t Type) Valid() bool {
	switch t {
	case TypeTask, TypeBug, TypeIdea, TypeChore:
		return true
	}
	return false
}

// Status is the workflow column a ticket sits in. Transitions are unrestricted
// (no workflow engine); only the target value is validated.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
	StatusArchived   Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusBacklog, StatusTodo, StatusInProgress, StatusBlocked, StatusDone, StatusArchived:
		return true
	}
	return false
}

// Priority is the urgency of a ticket.
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

func (p Priority) Valid() bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return true
	}
	return false
}

// Ticket is the core work item. Its display key (e.g. "PET-12") is derived from
// the project key and Number, not stored.
type Ticket struct {
	ID          int64
	ProjectID   int64
	Number      int
	Title       string
	Description string
	Type        Type
	Status      Status
	Priority    Priority
	Labels      []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

// Key formats a display key from a project key and ticket number.
func Key(projectKey string, number int) string {
	return projectKey + "-" + strconv.Itoa(number)
}

// ParseKey splits a display key like "PET-12" into its project key and number.
func ParseKey(s string) (projectKey string, number int, err error) {
	i := strings.LastIndex(s, "-")
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("%w: bad ticket key %q", apperr.ErrInvalid, s)
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil || n <= 0 {
		return "", 0, fmt.Errorf("%w: bad ticket key %q", apperr.ErrInvalid, s)
	}
	return s[:i], n, nil
}
