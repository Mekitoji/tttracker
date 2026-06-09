// Package clock provides a small time abstraction so services can be tested
// with a deterministic clock. Timestamps are stored in the database in the
// canonical UTC RFC3339Nano form produced by Format.
package clock

import "time"

// Clock abstracts access to the current time.
type Clock interface {
	Now() time.Time
}

// Real is the production clock; it always reports the current UTC time.
type Real struct{}

func (Real) Now() time.Time { return time.Now().UTC() }

// Fixed is a deterministic clock for tests; it always returns T.
type Fixed struct{ T time.Time }

func (f Fixed) Now() time.Time { return f.T }

// Format renders t in the canonical DB representation (UTC, RFC3339Nano).
func Format(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// Parse reads a canonical DB timestamp back into a UTC time.Time.
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
