package attachment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"tttracker/internal/activity"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/project"
	"tttracker/internal/ticket"
)

// ErrFileCleanup signals that DB metadata was committed (an attachment removed,
// or a ticket/project deleted), but the corresponding files could not be deleted
// from disk. The records are gone from the user's perspective and the leftover
// files are orphans, so callers may treat this as a non-fatal warning. Any error
// that is NOT wrapped in ErrFileCleanup means nothing was deleted — a real failure.
var ErrFileCleanup = errors.New("records removed but their files could not be deleted")

func fileCleanupError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrFileCleanup, err)
}

func attachRollbackError(err, cleanupErr error) error {
	if cleanupErr == nil {
		return err
	}
	return errors.Join(err, fmt.Errorf("remove copied attachment after failed transaction: %w", cleanupErr))
}

// Service holds the attachment business logic. Filesystem writes are not part
// of the SQLite transaction, so the ordering is explicit: copy first, then
// commit metadata + event; if the commit fails, the copied file is removed.
type Service struct {
	db       *sql.DB
	repo     *Repository
	tickets  *ticket.Repository
	projects *project.Repository
	activity *activity.Repository
	storage  *Storage
	clock    clock.Clock
}

func NewService(database *sql.DB, repo *Repository, tickets *ticket.Repository, projects *project.Repository, events *activity.Repository, storage *Storage, clk clock.Clock) *Service {
	return &Service{
		db: database, repo: repo, tickets: tickets, projects: projects,
		activity: events, storage: storage, clock: clk,
	}
}

// RemoveProjectFiles deletes all stored attachment files for a project (used when
// the project is deleted; the metadata rows are removed by the DB cascade). A
// failure is wrapped in ErrFileCleanup so the caller can treat it as a warning.
func (s *Service) RemoveProjectFiles(projectKey string) error {
	return fileCleanupError(s.storage.RemoveProjectDir(projectKey))
}

// RemoveTicketFiles deletes all stored attachment files for a ticket (used when
// the ticket is deleted; the metadata rows are removed by the DB cascade). A
// failure is wrapped in ErrFileCleanup so the caller can treat it as a warning.
func (s *Service) RemoveTicketFiles(projectKey, ticketKey string) error {
	return fileCleanupError(s.storage.RemoveTicketDir(projectKey, ticketKey))
}

// Attach copies srcPath into the ticket's folder and records the metadata.
func (s *Service) Attach(ctx context.Context, ticketKey, srcPath string) (*Attachment, error) {
	pk, num, err := ticket.ParseKey(ticketKey)
	if err != nil {
		return nil, err
	}
	proj, err := s.projects.GetByKey(ctx, s.db, pk)
	if err != nil {
		return nil, err
	}
	tk, err := s.tickets.GetByProjectNumber(ctx, s.db, proj.ID, num)
	if err != nil {
		return nil, err
	}

	// 1. copy the file (outside the transaction).
	stored, err := s.storage.Store(srcPath, proj.Key, ticket.Key(proj.Key, tk.Number))
	if err != nil {
		return nil, err
	}

	// 2. record metadata + event atomically; 3. on failure, drop the orphan copy.
	var out *Attachment
	err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		now := s.clock.Now()
		a := &Attachment{
			TicketID: tk.ID, FileName: stored.FileName, OriginalPath: srcPath,
			StoredPath: stored.StoredPath, MimeType: stored.MimeType, SizeBytes: stored.SizeBytes,
			CreatedAt: now,
		}
		if err := s.repo.Insert(ctx, tx, a); err != nil {
			return err
		}
		if err := s.activity.Record(ctx, tx, tk.ID, activity.AttachmentAdded,
			activity.AttachmentRef{AttachmentID: a.ID, FileName: a.FileName}, now); err != nil {
			return err
		}
		out = a
		return nil
	})
	if err != nil {
		return nil, attachRollbackError(err, s.storage.Remove(stored.StoredPath))
	}
	return out, nil
}

// Remove deletes the metadata + event first, then the file (an orphaned file is
// safer than a metadata row pointing at nothing). A failure before the commit is
// returned as-is (nothing was deleted); a failure to delete the file afterwards
// is wrapped in ErrFileCleanup so callers can treat it as a non-fatal warning.
func (s *Service) Remove(ctx context.Context, id int64) error {
	var storedPath string
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		a, err := s.repo.GetByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.repo.Delete(ctx, tx, id); err != nil {
			return err
		}
		now := s.clock.Now()
		if err := s.activity.Record(ctx, tx, a.TicketID, activity.AttachmentRemoved,
			activity.AttachmentRef{AttachmentID: a.ID, FileName: a.FileName}, now); err != nil {
			return err
		}
		storedPath = a.StoredPath
		return nil
	})
	if err != nil {
		return err // tx rolled back: nothing was deleted — a real failure
	}
	return fileCleanupError(s.storage.Remove(storedPath))
}

// List returns a ticket's attachments in insertion order.
func (s *Service) List(ctx context.Context, ticketKey string) ([]Attachment, error) {
	tid, err := ticket.ResolveID(ctx, s.db, s.projects, s.tickets, ticketKey)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByTicket(ctx, s.db, tid)
}
