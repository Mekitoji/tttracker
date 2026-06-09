// Package attachment stores files linked to a ticket. The bytes live on the
// filesystem under the attachments root; only metadata is kept in SQLite.
package attachment

import "time"

// Attachment is the metadata for one stored file.
type Attachment struct {
	ID           int64
	TicketID     int64
	FileName     string // display name within its ticket folder (de-duplicated)
	OriginalPath string // where the file was copied from
	StoredPath   string // canonical location on disk
	MimeType     string
	SizeBytes    int64
	CreatedAt    time.Time
}
