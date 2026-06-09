package comment

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

// Service holds the comment business logic.
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

// Add posts a comment on a ticket.
func (s *Service) Add(ctx context.Context, ticketKey, body string) (*Comment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%w: empty comment", apperr.ErrInvalid)
	}
	var out *Comment
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		tid, err := ticket.ResolveID(ctx, tx, s.projects, s.tickets, ticketKey)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		c := &Comment{TicketID: tid, Body: body, CreatedAt: now, UpdatedAt: now}
		if err := s.repo.Insert(ctx, tx, c); err != nil {
			return err
		}
		if err := s.activity.Record(ctx, tx, tid, activity.CommentCreated,
			activity.CommentRef{CommentID: c.ID}, now); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, err
}

// Edit replaces a comment's body.
func (s *Service) Edit(ctx context.Context, id int64, body string) (*Comment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%w: empty comment", apperr.ErrInvalid)
	}
	var out *Comment
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		c, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		c.Body = body
		c.UpdatedAt = now
		if err := s.repo.Update(ctx, tx, c); err != nil {
			return err
		}
		if err := s.activity.Record(ctx, tx, c.TicketID, activity.CommentUpdated,
			activity.CommentRef{CommentID: c.ID}, now); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, err
}

// Delete removes a comment and records the event.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		c, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, tx, id); err != nil {
			return err
		}
		now := s.clock.Now()
		return s.activity.Record(ctx, tx, c.TicketID, activity.CommentDeleted,
			activity.CommentRef{CommentID: c.ID}, now)
	})
}

// List returns a ticket's comments, oldest first.
func (s *Service) List(ctx context.Context, ticketKey string) ([]Comment, error) {
	tid, err := ticket.ResolveID(ctx, s.db, s.projects, s.tickets, ticketKey)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByTicket(ctx, s.db, tid)
}
