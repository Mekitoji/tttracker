package attachment

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tttracker/internal/apperr"
)

// Storage copies attachment files into a per-ticket folder under root.
type Storage struct {
	root string
}

func NewStorage(root string) *Storage { return &Storage{root: root} }

// Stored is the result of copying a file into the store.
type Stored struct {
	FileName   string
	StoredPath string
	MimeType   string
	SizeBytes  int64
}

// Store copies srcPath into root/<projectKey>/<ticketKey>/, de-duplicating the
// filename on collision. The destination is chosen and created in one atomic
// O_EXCL step, so concurrent Store calls for the same name never truncate or
// interleave into a shared file. The source file is left untouched.
func (s *Storage) Store(srcPath, projectKey, ticketKey string) (*Stored, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", apperr.ErrInvalid, srcPath)
	}

	dir := filepath.Join(s.root, projectKey, ticketKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dest, size, err := copyToUnique(srcPath, dir, filepath.Base(srcPath))
	if err != nil {
		return nil, err
	}

	return &Stored{
		FileName:   filepath.Base(dest),
		StoredPath: dest,
		MimeType:   detectMime(srcPath, dest),
		SizeBytes:  size,
	}, nil
}

// Remove deletes a stored file. A missing file is not an error.
func (s *Storage) Remove(storedPath string) error {
	err := os.Remove(storedPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// copyToUnique copies src into dir, choosing the first free name derived from
// base and creating it atomically with O_CREATE|O_EXCL. The exclusive create is
// what makes concurrent Store calls safe: two copies of the same name cannot land
// on one file — the loser of the create race just tries the next name instead of
// truncating or interleaving into a shared file. It returns the chosen path and
// the number of bytes actually written.
func copyToUnique(src, dir, base string) (string, int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = in.Close() }() // read-only: a close error is inconsequential
	info, err := in.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%w: %q is not a regular file", apperr.ErrInvalid, src)
	}

	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 0; ; i++ {
		candidate := filepath.Join(dir, base)
		if i > 0 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, i, ext))
		}
		out, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) {
			continue // name taken (possibly by a concurrent copy) — try the next one
		}
		if err != nil {
			return "", 0, err
		}
		size, err := writeAndClose(out, in, candidate)
		if err != nil {
			return "", 0, err
		}
		return candidate, size, nil
	}
}

// writeAndClose copies in into out, then closes out. On any error it closes out
// and removes the partially written dest. It returns the number of bytes copied.
func writeAndClose(out *os.File, in io.Reader, dest string) (int64, error) {
	size, err := io.Copy(out, in)
	if err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return 0, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return 0, err
	}
	return size, nil
}

func detectMime(origName, path string) string {
	if f, err := os.Open(path); err == nil {
		defer func() { _ = f.Close() }() // read-only: a close error is inconsequential
		buf := make([]byte, 512)
		if n, _ := f.Read(buf); n > 0 {
			if ct := http.DetectContentType(buf[:n]); ct != "" && ct != "application/octet-stream" {
				return ct
			}
		}
	}
	if byExt := mime.TypeByExtension(filepath.Ext(origName)); byExt != "" {
		return byExt
	}
	return "application/octet-stream"
}

// RemoveProjectDir deletes all stored attachment files for a project. A missing
// directory is not an error.
func (s *Storage) RemoveProjectDir(projectKey string) error {
	return os.RemoveAll(filepath.Join(s.root, projectKey))
}

// RemoveTicketDir deletes all stored attachment files for a ticket. A missing
// directory is not an error.
func (s *Storage) RemoveTicketDir(projectKey, ticketKey string) error {
	return os.RemoveAll(filepath.Join(s.root, projectKey, ticketKey))
}
