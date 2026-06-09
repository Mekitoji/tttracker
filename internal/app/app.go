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
	projects := project.NewRepository()
	tickets := ticket.NewRepository()
	subtasks := subtask.NewRepository()
	comments := comment.NewRepository()
	attachments := attachment.NewRepository()
	events := activity.NewRepository()
	storage := attachment.NewStorage(attachmentsDir)

	return &App{
		DB:          database,
		Projects:    project.NewService(database, projects, clk),
		Tickets:     ticket.NewService(database, tickets, projects, events, clk),
		Subtasks:    subtask.NewService(database, subtasks, tickets, projects, events, clk),
		Comments:    comment.NewService(database, comments, tickets, projects, events, clk),
		Attachments: attachment.NewService(database, attachments, tickets, projects, events, storage, clk),
		Activity:    activity.NewService(database, events),
	}
}
