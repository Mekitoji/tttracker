package ticket

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"tttracker/internal/activity"
	"tttracker/internal/apperr"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/project"
)

// Service holds the ticket business logic. Every mutation records an activity
// event in the same transaction as the state change.
type Service struct {
	db       *sql.DB
	tickets  *Repository
	projects *project.Repository
	activity *activity.Repository
	clock    clock.Clock
}

func NewService(database *sql.DB, tickets *Repository, projects *project.Repository, events *activity.Repository, clk clock.Clock) *Service {
	return &Service{db: database, tickets: tickets, projects: projects, activity: events, clock: clk}
}

// ResolveID resolves a display key (e.g. "PET-12") to a ticket id using the
// given DBTX. Shared by the subtask/comment/attachment services.
func ResolveID(ctx context.Context, q db.DBTX, projects *project.Repository, tickets *Repository, key string) (int64, error) {
	pk, num, err := ParseKey(key)
	if err != nil {
		return 0, err
	}
	proj, err := projects.GetByKey(ctx, q, pk)
	if err != nil {
		return 0, err
	}
	t, err := tickets.GetByProjectNumber(ctx, q, proj.ID, num)
	if err != nil {
		return 0, err
	}
	return t.ID, nil
}

// CreateParams are the inputs for Create. Empty Type/Status/Priority default to
// task/todo/medium.
type CreateParams struct {
	ProjectKey  string
	Title       string
	Description string
	Type        Type
	Status      Status
	Priority    Priority
	Labels      []string
}

// Create inserts a new ticket with the next per-project number and records a
// ticket.created event.
func (s *Service) Create(ctx context.Context, p CreateParams) (*Ticket, error) {
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: empty title", apperr.ErrInvalid)
	}
	if p.Type == "" {
		p.Type = TypeTask
	}
	if p.Status == "" {
		p.Status = StatusTodo
	}
	if p.Priority == "" {
		p.Priority = PriorityMedium
	}
	if !p.Type.Valid() || !p.Status.Valid() || !p.Priority.Valid() {
		return nil, fmt.Errorf("%w: invalid type/status/priority", apperr.ErrInvalid)
	}

	var out *Ticket
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		proj, err := s.projects.GetByKey(ctx, tx, p.ProjectKey)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		num, err := s.tickets.NextNumber(ctx, tx, proj.ID)
		if err != nil {
			return err
		}
		t := &Ticket{
			ProjectID: proj.ID, Number: num, Title: title, Description: p.Description,
			Type: p.Type, Status: p.Status, Priority: p.Priority, Labels: p.Labels,
			CreatedAt: now, UpdatedAt: now,
		}
		if p.Status == StatusDone {
			t.CompletedAt = &now
		}
		if err := s.tickets.Insert(ctx, tx, t); err != nil {
			return err
		}
		payload := activity.CreatedPayload{
			Number: num, Title: title,
			Type: string(t.Type), Status: string(t.Status), Priority: string(t.Priority),
		}
		if err := s.activity.Record(ctx, tx, t.ID, activity.TicketCreated, payload, now); err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, err
}

// Get returns a ticket by display key.
func (s *Service) Get(ctx context.Context, key string) (*Ticket, error) {
	pk, num, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	proj, err := s.projects.GetByKey(ctx, s.db, pk)
	if err != nil {
		return nil, err
	}
	return s.tickets.GetByProjectNumber(ctx, s.db, proj.ID, num)
}

// List returns all tickets in a project, ordered by number.
func (s *Service) List(ctx context.Context, projectKey string) ([]Ticket, error) {
	proj, err := s.projects.GetByKey(ctx, s.db, projectKey)
	if err != nil {
		return nil, err
	}
	return s.tickets.ListByProject(ctx, s.db, proj.ID)
}

// Delete removes the ticket and (via DB cascade) its subtasks, comments,
// attachment metadata, and activity; the FTS index is cleaned by the ticket
// trigger. Attachment files on disk are not cleaned here — attachments aren't
// yet creatable in the UI; mirror project deletion's file cleanup when they are.
func (s *Service) Delete(ctx context.Context, key string) error {
	pk, num, err := ParseKey(key)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		proj, err := s.projects.GetByKey(ctx, tx, pk)
		if err != nil {
			return err
		}
		t, err := s.tickets.GetByProjectNumber(ctx, tx, proj.ID, num)
		if err != nil {
			return err
		}
		return s.tickets.Delete(ctx, tx, t.ID)
	})
}

// mutate loads the ticket addressed by key, runs apply (which mutates the
// ticket and records its event), and persists the row only if apply reports a
// change. updated_at is bumped on change.
func (s *Service) mutate(ctx context.Context, key string, apply func(tx *sql.Tx, t *Ticket, now time.Time) (changed bool, err error)) (*Ticket, error) {
	pk, num, err := ParseKey(key)
	if err != nil {
		return nil, err
	}
	var out *Ticket
	err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		proj, err := s.projects.GetByKey(ctx, tx, pk)
		if err != nil {
			return err
		}
		t, err := s.tickets.GetByProjectNumber(ctx, tx, proj.ID, num)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		changed, err := apply(tx, t, now)
		if err != nil {
			return err
		}
		if changed {
			t.UpdatedAt = now
			if err := s.tickets.Update(ctx, tx, t); err != nil {
				return err
			}
		}
		out = t
		return nil
	})
	return out, err
}

// SetStatus changes status, maintaining completed_at, and records the event.
func (s *Service) SetStatus(ctx context.Context, key, status string) (*Ticket, error) {
	st := Status(status)
	if !st.Valid() {
		return nil, fmt.Errorf("%w: status %q", apperr.ErrInvalid, status)
	}
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if t.Status == st {
			return false, nil
		}
		old := t.Status
		t.Status = st
		switch {
		case st == StatusDone && old != StatusDone:
			c := now
			t.CompletedAt = &c
		case st != StatusDone && old == StatusDone:
			t.CompletedAt = nil
		}
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketStatusChanged,
			activity.StringChange{From: string(old), To: string(st)}, now)
	})
}

// SetPriority changes priority and records the event.
func (s *Service) SetPriority(ctx context.Context, key, priority string) (*Ticket, error) {
	pr := Priority(priority)
	if !pr.Valid() {
		return nil, fmt.Errorf("%w: priority %q", apperr.ErrInvalid, priority)
	}
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if t.Priority == pr {
			return false, nil
		}
		old := t.Priority
		t.Priority = pr
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketPriorityChanged,
			activity.StringChange{From: string(old), To: string(pr)}, now)
	})
}

// SetType changes type and records the event.
func (s *Service) SetType(ctx context.Context, key, typ string) (*Ticket, error) {
	ty := Type(typ)
	if !ty.Valid() {
		return nil, fmt.Errorf("%w: type %q", apperr.ErrInvalid, typ)
	}
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if t.Type == ty {
			return false, nil
		}
		old := t.Type
		t.Type = ty
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketTypeChanged,
			activity.StringChange{From: string(old), To: string(ty)}, now)
	})
}

// SetTitle changes the title and records the event.
func (s *Service) SetTitle(ctx context.Context, key, title string) (*Ticket, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: empty title", apperr.ErrInvalid)
	}
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if t.Title == title {
			return false, nil
		}
		old := t.Title
		t.Title = title
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketTitleChanged,
			activity.StringChange{From: old, To: title}, now)
	})
}

// SetDescription replaces the Markdown description and records the event. The
// payload records only that it changed (descriptions can be large).
func (s *Service) SetDescription(ctx context.Context, key, description string) (*Ticket, error) {
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if t.Description == description {
			return false, nil
		}
		t.Description = description
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketDescriptionChanged,
			activity.StringChange{From: "", To: ""}, now)
	})
}

// SetLabels replaces the label set and records the event.
func (s *Service) SetLabels(ctx context.Context, key string, labels []string) (*Ticket, error) {
	return s.mutate(ctx, key, func(tx *sql.Tx, t *Ticket, now time.Time) (bool, error) {
		if equalStrings(t.Labels, labels) {
			return false, nil
		}
		old := t.Labels
		t.Labels = labels
		return true, s.activity.Record(ctx, tx, t.ID, activity.TicketLabelsChanged,
			activity.LabelsChange{From: old, To: labels}, now)
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SearchHit is a ticket returned by Search, paired with its project key so a
// display key (e.g. "PET-12") can be formed without another lookup.
type SearchHit struct {
	Ticket     Ticket
	ProjectKey string
}

// Search runs a full-text search across all projects. The free-text input is
// escaped into an FTS5 query: each whitespace-separated token becomes a quoted
// term (implicitly AND-ed) and the final token gets a prefix match, so typing
// is incremental. An empty query returns no results (and no error).
func (s *Service) Search(ctx context.Context, input string) ([]SearchHit, error) {
	match := toFTSQuery(input)
	if match == "" {
		return nil, nil
	}
	return s.tickets.Search(ctx, s.db, match)
}

func toFTSQuery(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	terms := make([]string, len(fields))
	for i, f := range fields {
		// Quote each token (escaping embedded quotes) so punctuation in the
		// user's input is never interpreted as FTS5 query syntax.
		terms[i] = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
	}
	terms[len(terms)-1] += "*" // prefix-match the last (in-progress) token
	return strings.Join(terms, " ")
}
