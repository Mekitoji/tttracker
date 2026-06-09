// Package project is a container for tickets, identified by a short uppercase key.
package project

import "time"

// Project is a container for tickets, identified by a short uppercase key.
type Project struct {
	ID          int64
	Key         string
	Name        string
	Description string
	RepoPath    string // optional absolute path to the project's git repo
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
