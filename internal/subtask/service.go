package subtask

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"tttracker/internal/activity"
	"tttracker/internal/apperr"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/project"
	"tttracker/internal/ticket"
)

// Service holds the subtask business logic.
type Service struct {
	db       *sql.DB
	repo     *Repository
	tickets  *ticket.Repository
	projects *project.Repository
	activity *activity.Repository
	clock    clock.Clock
}

func NewService(database *sql.DB, repo *Repository, tickets *ticket.Repository, projects *project.Repository, events *activity.Repository, clk clock.Clock) *Service {
	return &Service{db: database, repo: repo, tickets: tickets, projects: projects, activity: events, clock: clk}
}

// Add appends a checklist item to a ticket.
func (s *Service) Add(ctx context.Context, ticketKey, title string) (*Subtask, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: empty subtask title", apperr.ErrInvalid)
	}
	var out *Subtask
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		tid, err := ticket.ResolveID(ctx, tx, s.projects, s.tickets, ticketKey)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		pos, err := s.repo.NextPosition(ctx, tx, tid)
		if err != nil {
			return err
		}
		st := &Subtask{TicketID: tid, Title: title, Position: pos, CreatedAt: now, UpdatedAt: now}
		if err := s.repo.Insert(ctx, tx, st); err != nil {
			return err
		}
		if err := s.activity.Record(ctx, tx, tid, activity.SubtaskCreated,
			activity.SubtaskRef{SubtaskID: st.ID, Title: title}, now); err != nil {
			return err
		}
		out = st
		return nil
	})
	return out, err
}

// Toggle flips a subtask's done state and records completed/reopened.
func (s *Service) Toggle(ctx context.Context, id int64) (*Subtask, error) {
	var out *Subtask
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		st, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		st.IsDone = !st.IsDone
		st.UpdatedAt = now
		if err := s.repo.Update(ctx, tx, st); err != nil {
			return err
		}
		evt := activity.SubtaskReopened
		if st.IsDone {
			evt = activity.SubtaskCompleted
		}
		if err := s.activity.Record(ctx, tx, st.TicketID, evt,
			activity.SubtaskRef{SubtaskID: st.ID, Title: st.Title}, now); err != nil {
			return err
		}
		out = st
		return nil
	})
	return out, err
}

// Rename changes a subtask's title.
func (s *Service) Rename(ctx context.Context, id int64, title string) (*Subtask, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: empty subtask title", apperr.ErrInvalid)
	}
	var out *Subtask
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		st, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		st.Title = title
		st.UpdatedAt = now
		if err := s.repo.Update(ctx, tx, st); err != nil {
			return err
		}
		if err := s.activity.Record(ctx, tx, st.TicketID, activity.SubtaskRenamed,
			activity.SubtaskRef{SubtaskID: st.ID, Title: title}, now); err != nil {
			return err
		}
		out = st
		return nil
	})
	return out, err
}

// Delete removes a subtask and records the event.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		st, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, tx, id); err != nil {
			return err
		}
		now := s.clock.Now()
		return s.activity.Record(ctx, tx, st.TicketID, activity.SubtaskDeleted,
			activity.SubtaskRef{SubtaskID: st.ID, Title: st.Title}, now)
	})
}

// Reorder sets subtask positions to match the given id order.
func (s *Service) Reorder(ctx context.Context, ticketKey string, orderedIDs []int64) error {
	return db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		tid, err := ticket.ResolveID(ctx, tx, s.projects, s.tickets, ticketKey)
		if err != nil {
			return err
		}
		now := clock.Format(s.clock.Now())
		for i, id := range orderedIDs {
			if err := s.repo.SetPosition(ctx, tx, id, tid, i+1, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// List returns a ticket's subtasks ordered by position.
func (s *Service) List(ctx context.Context, ticketKey string) ([]Subtask, error) {
	tid, err := ticket.ResolveID(ctx, s.db, s.projects, s.tickets, ticketKey)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByTicket(ctx, s.db, tid)
}
