// Package apperr defines the small set of sentinel errors that services return
// at their boundary, so adapters (CLI/TUI) can map them to messages and exit
// codes with errors.Is.
package apperr

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalid is returned for invalid input (bad enum, empty title, ...).
	ErrInvalid = errors.New("invalid input")
	// ErrConflict is returned when an operation violates a uniqueness rule.
	ErrConflict = errors.New("conflict")
)
