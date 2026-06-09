package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"tttracker/internal/apperr"
	"tttracker/internal/clock"
	"tttracker/internal/db"
)

var keyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,9}$`)

// AttachmentRemover deletes the on-disk attachment files for a project. It is
// satisfied by the attachment service and injected here so project deletion can
// clean up files without the project package importing the attachment package
// (which already imports project).
type AttachmentRemover interface {
	RemoveProjectFiles(projectKey string) error
}

// Service holds the project business logic.
type Service struct {
	db          *sql.DB
	repo        *Repository
	clock       clock.Clock
	attachments AttachmentRemover
}

func NewService(database *sql.DB, repo *Repository, clk clock.Clock, attachments AttachmentRemover) *Service {
	return &Service{db: database, repo: repo, clock: clk, attachments: attachments}
}

// Create validates the key, ensures it is unique, and inserts the project.
func (s *Service) Create(ctx context.Context, key, name, description string) (*Project, error) {
	key = strings.TrimSpace(key)
	if !keyPattern.MatchString(key) {
		return nil, fmt.Errorf("%w: project key %q must be 1-10 chars, A-Z then A-Z0-9", apperr.ErrInvalid, key)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = key
	}

	var out *Project
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		switch _, err := s.repo.GetByKey(ctx, tx, key); {
		case err == nil:
			return fmt.Errorf("%w: project %q already exists", apperr.ErrConflict, key)
		case !errors.Is(err, apperr.ErrNotFound):
			return err
		}
		now := s.clock.Now()
		p := &Project{Key: key, Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
		if err := s.repo.Insert(ctx, tx, p); err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

// SetRepoPath sets (or clears, when path is empty) the optional path to the
// project's git repository. A non-empty path is validated on the filesystem: it
// must be an existing directory containing a .git entry. The stored path is
// absolute. No git binary is invoked — this is just a recorded pointer for
// future checks/integrations.
func (s *Service) SetRepoPath(ctx context.Context, key, repoPath string) (*Project, error) {
	cleaned, err := normalizeRepoPath(repoPath)
	if err != nil {
		return nil, err
	}
	var out *Project
	err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		p, err := s.repo.GetByKey(ctx, tx, key)
		if err != nil {
			return err
		}
		p.RepoPath = cleaned
		p.UpdatedAt = s.clock.Now()
		if err := s.repo.Update(ctx, tx, p); err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

// SetName updates the project's free-form display name (any characters).
func (s *Service) SetName(ctx context.Context, key, name string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: empty project name", apperr.ErrInvalid)
	}
	var out *Project
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		p, err := s.repo.GetByKey(ctx, tx, key)
		if err != nil {
			return err
		}
		p.Name = name
		p.UpdatedAt = s.clock.Now()
		if err := s.repo.Update(ctx, tx, p); err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

// SetDescription replaces the project's description (may be empty).
func (s *Service) SetDescription(ctx context.Context, key, description string) (*Project, error) {
	var out *Project
	err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		p, err := s.repo.GetByKey(ctx, tx, key)
		if err != nil {
			return err
		}
		p.Description = description
		p.UpdatedAt = s.clock.Now()
		if err := s.repo.Update(ctx, tx, p); err != nil {
			return err
		}
		out = p
		return nil
	})
	return out, err
}

func normalizeRepoPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil // clearing the repo path is allowed
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	// Pointing at a repo's ".git" entry is treated as pointing at the repo root.
	if filepath.Base(abs) == ".git" {
		abs = filepath.Dir(abs)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not an existing directory", apperr.ErrInvalid, p)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return "", fmt.Errorf("%w: %q is not a git repository (no .git)", apperr.ErrInvalid, p)
	}
	return abs, nil
}

// Get returns the project with the given key.
func (s *Service) Get(ctx context.Context, key string) (*Project, error) {
	return s.repo.GetByKey(ctx, s.db, key)
}

// Delete removes the project and (via DB cascade) all its tickets, subtasks,
// comments, attachment metadata, and activity; the FTS index is cleaned by the
// ticket triggers. After the transaction commits, the project's attachment files
// are removed best-effort (filesystem work is not part of the transaction, and
// an orphaned file is safer than a dangling row).
func (s *Service) Delete(ctx context.Context, key string) error {
	if err := db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		p, err := s.repo.GetByKey(ctx, tx, key)
		if err != nil {
			return err
		}
		return s.repo.Delete(ctx, tx, p.ID)
	}); err != nil {
		return err
	}
	_ = s.attachments.RemoveProjectFiles(key)
	return nil
}

// List returns all projects ordered by key.
func (s *Service) List(ctx context.Context) ([]Project, error) {
	return s.repo.List(ctx, s.db)
}
