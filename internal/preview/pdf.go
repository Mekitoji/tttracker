package preview

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// PDF previews render the first page to a PNG with poppler's pdftoppm, then feed
// it through the image pipeline. Like video, extraction is slow, so it runs off
// the UI thread and the rendered result is cached by the PDF's path.

var (
	pdftoppmOnce sync.Once
	pdftoppmPath string // "" when pdftoppm (poppler) is not installed
)

func pdftoppmAvailable() bool {
	pdftoppmOnce.Do(func() { pdftoppmPath, _ = exec.LookPath("pdftoppm") })
	return pdftoppmPath != ""
}

// renderPDF rasterises the first page and renders it. Any failure (no pdftoppm,
// unreadable PDF) becomes a cached placeholder.
func renderPDF(path string, cols, rows int) string {
	out, err := cachedRender(path, cols, rows, func() (string, error) {
		if !pdftoppmAvailable() {
			return placeholder("pdf — install poppler (pdftoppm) for preview"), nil
		}
		page, err := thumbnail(path, extractPDFPage)
		if err != nil {
			return placeholder("cannot render pdf"), nil
		}
		s, err := imagePreviewer.Preview(page, cols, rows)
		if err != nil {
			return placeholder("cannot render pdf"), nil
		}
		return s, nil
	})
	if err != nil {
		return placeholder("cannot render pdf")
	}
	return out
}

// extractPDFPage renders the first page of pdf to out. pdftoppm's -singlefile mode
// writes "<prefix>.png", so the prefix is out without its .png suffix.
func extractPDFPage(pdf, out string) error {
	prefix := strings.TrimSuffix(out, ".png")
	cmd := exec.Command(pdftoppmPath,
		"-png", "-f", "1", "-l", "1", "-singlefile",
		"-scale-to", "1600",
		pdf, prefix,
	)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pdftoppm failed for %q: %w", pdf, err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		return fmt.Errorf("pdftoppm produced no output for %q", pdf)
	}
	return nil
}
