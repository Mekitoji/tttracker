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
// filename on collision. The source file is left untouched.
func (s *Storage) Store(srcPath, projectKey, ticketKey string) (*Stored, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %q is a directory", apperr.ErrInvalid, srcPath)
	}

	dir := filepath.Join(s.root, projectKey, ticketKey)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dest := uniquePath(dir, filepath.Base(srcPath))
	if err := copyFile(srcPath, dest); err != nil {
		return nil, err
	}

	return &Stored{
		FileName:   filepath.Base(dest),
		StoredPath: dest,
		MimeType:   detectMime(srcPath, dest),
		SizeBytes:  info.Size(),
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

func uniquePath(dir, base string) string {
	candidate := filepath.Join(dir, base)
	if !exists(candidate) {
		return candidate
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, i, ext))
		if !exists(candidate) {
			return candidate
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		defer func() {
			if err := out.Close(); err != nil {
				fmt.Printf("Error closing file: %v\n", err)
			}
		}()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func detectMime(origName, path string) string {
	if f, err := os.Open(path); err == nil {
		defer func() {
			if err := f.Close(); err != nil {
				fmt.Printf("Error closing file: %v\n", err)
			}
		}()
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
