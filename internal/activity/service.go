package activity

import (
	"context"
	"database/sql"
)

// Service exposes read access to the activity log. Events are written by the
// other services (within their transactions) via the Repository.
type Service struct {
	db   *sql.DB
	repo *Repository
}

func NewService(database *sql.DB, repo *Repository) *Service {
	return &Service{db: database, repo: repo}
}

// List returns the events recorded for a ticket, oldest first.
func (s *Service) List(ctx context.Context, ticketID int64) ([]Event, error) {
	return s.repo.List(ctx, s.db, ticketID)
}
