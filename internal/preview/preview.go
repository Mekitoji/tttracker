// Package preview renders a preview of an attachment for display in the TUI:
// images (png/jpeg/gif/webp/bmp/tiff) become half-block thumbnails or native
// Kitty graphics, videos are previewed by extracting a frame with ffmpeg, Markdown
// is rendered with glamour, source code is highlighted with Chroma, and other
// UTF-8 text is shown as-is. Binary files (and media that cannot be rendered) get
// a short placeholder.
package preview

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	_ "image/gif"  // register GIF decoder for image.Decode
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder

	_ "golang.org/x/image/bmp"  // register BMP decoder
	_ "golang.org/x/image/tiff" // register TIFF decoder
	_ "golang.org/x/image/webp" // register WebP decoder

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// Kind classifies how a file should be previewed.
type Kind int

const (
	KindBinary Kind = iota
	KindImage
	KindMarkdown
	KindCode
	KindText
	KindVideo
	KindPDF
)

// maxReadBytes caps how much of a text/markdown file is read for preview.
const maxReadBytes = 256 * 1024

// DetectKind classifies a file by extension, falling back to a content sniff
// (NUL byte / invalid UTF-8 => binary) for unknown extensions.
func DetectKind(path string) Kind {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif":
		return KindImage
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v", ".flv", ".wmv":
		return KindVideo
	case ".pdf":
		return KindPDF
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
	if codeLexer(path, string(data)) != nil {
		return KindCode
	}
	return KindText
}

func isTextual(data []byte) bool {
	if slices.Contains(data, 0) {
		return false
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
	case KindVideo:
		return renderVideo(path, width, height)
	case KindPDF:
		return renderPDF(path, width, height)
	case KindMarkdown:
		return renderMarkdown(path, width, height)
	case KindCode:
		return renderCode(path, width, height)
	case KindText:
		return renderText(path, width, height)
	default:
		return placeholder("binary file — no preview")
	}
}

// IsMedia reports whether the file previews as an image — directly, or a frame
// extracted from a video, or the first page of a PDF. Such previews are
// (relatively) slow and cached, so the TUI renders them off the event loop rather
// than inline in a frame.
func IsMedia(path string) bool {
	switch DetectKind(path) {
	case KindImage, KindVideo, KindPDF:
		return true
	default:
		return false
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

func renderCode(path string, width, height int) string {
	data, err := readCapped(path, maxReadBytes)
	if err != nil {
		return placeholder("cannot read file")
	}
	source := string(data)
	out, ok := highlightCode(path, source)
	if !ok {
		out = source
	}
	return clip(out, width, height)
}

func renderMarkdown(path string, width, height int) string {
	data, err := readCapped(path, maxReadBytes)
	if err != nil {
		return placeholder("cannot read file")
	}
	w := max(width, 20)
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

func highlightCode(path, source string) (string, bool) {
	lexer := codeLexer(path, source)
	if lexer == nil {
		return "", false
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, source)
	if err != nil {
		return "", false
	}
	var b bytes.Buffer
	if err := formatters.Get("terminal256").Format(&b, styles.Get("github-dark"), iterator); err != nil {
		return "", false
	}
	return b.String(), true
}

func codeLexer(path, source string) chroma.Lexer {
	if lexer := lexers.Match(path); isCodeLexer(lexer) {
		return lexer
	}
	if filepath.Ext(path) == "" {
		if lexer := lexers.Analyse(source); isCodeLexer(lexer) {
			return lexer
		}
	}
	return nil
}

func isCodeLexer(lexer chroma.Lexer) bool {
	if lexer == nil {
		return false
	}
	switch strings.ToLower(lexer.Config().Name) {
	case "fallback", "plaintext":
		return false
	default:
		return true
	}
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
