// Package preview renders a textual preview of an attachment for display in the
// TUI: images become half-block thumbnails, Markdown is rendered with glamour,
// and other UTF-8 text is shown as-is. Binary files get a short placeholder.
package preview

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// Kind classifies how a file should be previewed.
type Kind int

const (
	KindBinary Kind = iota
	KindImage
	KindMarkdown
	KindText
)

// maxReadBytes caps how much of a text/markdown file is read for preview.
const maxReadBytes = 256 * 1024

// DetectKind classifies a file by extension, falling back to a content sniff
// (NUL byte / invalid UTF-8 => binary) for unknown extensions.
func DetectKind(path string) Kind {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return KindImage
	case ".md", ".markdown":
		return KindMarkdown
	}
	data, err := readCapped(path, 8192)
	if err != nil {
		return KindBinary
	}
	if !isTextual(data) {
		return KindBinary
	}
	return KindText
}

func isTextual(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}

// Render returns a preview string that fits within width columns and height
// rows. It never returns an error; failures become a short placeholder.
func Render(path string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	switch DetectKind(path) {
	case KindImage:
		return renderImage(path, width, height)
	case KindMarkdown:
		return renderMarkdown(path, width, height)
	case KindText:
		return renderText(path, width, height)
	default:
		return placeholder("binary file — no preview")
	}
}

func placeholder(msg string) string { return "(" + msg + ")" }

func renderText(path string, width, height int) string {
	data, err := readCapped(path, maxReadBytes)
	if err != nil {
		return placeholder("cannot read file")
	}
	return clip(string(data), width, height)
}

func renderMarkdown(path string, width, height int) string {
	data, err := readCapped(path, maxReadBytes)
	if err != nil {
		return placeholder("cannot read file")
	}
	w := width
	if w < 20 {
		w = 20
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(w))
	if err != nil {
		return clip(string(data), width, height)
	}
	out, err := r.Render(string(data))
	if err != nil {
		return clip(string(data), width, height)
	}
	return clip(out, width, height)
}

// clip trims content to at most height lines, each truncated to width columns
// (ANSI-aware so styling escapes are not cut mid-sequence).
func clip(content string, width, height int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, ln := range lines {
		ln = strings.ReplaceAll(ln, "\t", "    ")
		lines[i] = ansi.Truncate(ln, width, "…")
	}
	return strings.Join(lines, "\n")
}

func readCapped(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, limit))
}
