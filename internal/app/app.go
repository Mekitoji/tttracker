// Package app is the composition root: it wires repositories and services over
// a single database handle and exposes them as one facade that every adapter
// (TUI, CLI, future Neovim) consumes. Adapters hold no business logic.
package app

import (
	"database/sql"

	"tttracker/internal/activity"
	"tttracker/internal/attachment"
	"tttracker/internal/clock"
	"tttracker/internal/comment"
	"tttracker/internal/project"
	"tttracker/internal/subtask"
	"tttracker/internal/ticket"
)

// App bundles the application services.
type App struct {
	DB *sql.DB

	Projects    *project.Service
	Tickets     *ticket.Service
	Subtasks    *subtask.Service
	Comments    *comment.Service
	Attachments *attachment.Service
	Activity    *activity.Service
}

// New constructs the service graph over database, using clk as the time source
// and attachmentsDir as the root for stored attachment files.
func New(database *sql.DB, clk clock.Clock, attachmentsDir string) *App {
	projectRepo := project.NewRepository()
	ticketRepo := ticket.NewRepository()
	subtaskRepo := subtask.NewRepository()
	commentRepo := comment.NewRepository()
	attachmentRepo := attachment.NewRepository()
	events := activity.NewRepository()
	storage := attachment.NewStorage(attachmentsDir)

	// Built before the project and ticket services, which take it as their
	// AttachmentRemover so deleting either also removes the associated files.
	attachments := attachment.NewService(database, attachmentRepo, ticketRepo, projectRepo, events, storage, clk)

	return &App{
		DB:          database,
		Projects:    project.NewService(database, projectRepo, clk, attachments),
		Tickets:     ticket.NewService(database, ticketRepo, projectRepo, events, clk, attachments),
		Subtasks:    subtask.NewService(database, subtaskRepo, ticketRepo, projectRepo, events, clk),
		Comments:    comment.NewService(database, commentRepo, ticketRepo, projectRepo, events, clk),
		Attachments: attachments,
		Activity:    activity.NewService(database, events),
	}
}
