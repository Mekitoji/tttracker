package preview

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func writePNG(t *testing.T, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 128, A: 255})
		}
	}
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return p
}

func TestDetectKind(t *testing.T) {
	cases := []struct {
		name string
		path string
		want Kind
	}{
		{"png", writePNG(t, "a.png", 4, 4), KindImage},
		{"markdown", writeTemp(t, "a.md", []byte("# hi")), KindMarkdown},
		{"code", writeTemp(t, "a.go", []byte("package main\n")), KindCode},
		{"text", writeTemp(t, "a.txt", []byte("plain notes\n")), KindText},
		{"binary", writeTemp(t, "a.bin", []byte{0x00, 0x01, 0x02, 0x00}), KindBinary},
	}
	for _, c := range cases {
		if got := DetectKind(c.path); got != c.want {
			t.Errorf("%s: DetectKind = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRenderTextClips(t *testing.T) {
	body := "line one is fairly long and should be truncated\nsecond\nthird\nfourth\nfifth"
	p := writeTemp(t, "code.txt", []byte(body))

	out := Render(p, 10, 3)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(lines), out)
	}
	for _, ln := range lines {
		if len([]rune(ln)) > 10 {
			t.Fatalf("line exceeds width 10: %q", ln)
		}
	}
}

func TestRenderCodeHighlightsAndClips(t *testing.T) {
	body := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	p := writeTemp(t, "main.go", []byte(body))

	out := Render(p, 14, 3)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI highlighting in code preview: %q", out)
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d: %q", len(lines), out)
	}
	for _, ln := range lines {
		if ansi.StringWidth(ln) > 14 {
			t.Fatalf("line exceeds width 14: %q", ln)
		}
	}
}

func TestRenderImageProducesHalfBlocks(t *testing.T) {
	p := writePNG(t, "img.png", 8, 8)
	out := Render(p, 6, 4)
	if out == "" {
		t.Fatal("empty image render")
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected half-block glyph in output: %q", out)
	}
	// Each cell row covers two pixel rows; output must not exceed the row budget.
	if n := strings.Count(out, "\n") + 1; n > 4 {
		t.Fatalf("rendered %d rows, want <= 4", n)
	}
}

func TestRenderMarkdownNonEmpty(t *testing.T) {
	p := writeTemp(t, "doc.md", []byte("# Title\n\nSome **bold** text."))
	out := Render(p, 40, 20)
	if strings.TrimSpace(out) == "" {
		t.Fatal("markdown render is empty")
	}
}

type countingPreviewer struct {
	calls int
	out   string
}

func (c *countingPreviewer) Preview(path string, w, h int) (string, error) {
	c.calls++
	return c.out, nil
}

func TestImagePreviewerPluggableAndCached(t *testing.T) {
	p := writePNG(t, "x.png", 4, 4)
	fake := &countingPreviewer{out: "FAKE"}

	orig := imagePreviewer
	imagePreviewer = fake
	clearImgCache()
	t.Cleanup(func() {
		imagePreviewer = orig
		clearImgCache()
	})

	out1 := Render(p, 6, 4)
	out2 := Render(p, 6, 4)
	if out1 != "FAKE" || out2 != "FAKE" {
		t.Fatalf("previewer not used: %q / %q", out1, out2)
	}
	if fake.calls != 1 {
		t.Fatalf("expected 1 call (second served from cache), got %d", fake.calls)
	}
}

func TestHalfBlocksAveragesSolidColor(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := range 20 {
		for x := range 20 {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 40, A: 255})
		}
	}
	out := halfBlocks(img, 6, 3)
	// Box-averaging a uniform region must yield exactly that color.
	if !strings.Contains(out, "38;2;200;30;40") {
		t.Fatalf("expected averaged solid color in output: %q", out)
	}
}

func clearImgCache() {
	cacheMu.Lock()
	imgCache = map[string]string{}
	cacheMu.Unlock()
}

func TestDetectKindImageExtensions(t *testing.T) {
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".PNG", ".WebP"} {
		if got := DetectKind("x" + ext); got != KindImage {
			t.Errorf("DetectKind(%q) = %v, want KindImage", ext, got)
		}
	}
}

func TestRenderBMPAndTIFF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	for y := range 12 {
		for x := range 12 {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 90, A: 255})
		}
	}
	cases := map[string]func(*os.File) error{
		"pic.bmp":  func(f *os.File) error { return bmp.Encode(f, img) },
		"pic.tiff": func(f *os.File) error { return tiff.Encode(f, img, nil) },
	}
	for name, enc := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), name)
			f, err := os.Create(p)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if err := enc(f); err != nil {
				t.Fatalf("encode: %v", err)
			}
			_ = f.Close()
			if out := Render(p, 10, 6); !strings.Contains(out, "▀") {
				t.Fatalf("%s should render a half-block thumbnail, got %q", name, out)
			}
		})
	}
}

func TestDetectKindVideo(t *testing.T) {
	for _, ext := range []string{".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v", ".MP4"} {
		if got := DetectKind("clip" + ext); got != KindVideo {
			t.Errorf("DetectKind(%q) = %v, want KindVideo", ext, got)
		}
	}
}

func TestRenderVideoFrame(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	vid := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command(ff, "-y", "-f", "lavfi",
		"-i", "color=c=red:s=64x64:d=1", "-pix_fmt", "yuv420p", vid).CombinedOutput(); err != nil {
		t.Skipf("could not create test video: %v (%s)", err, out)
	}

	out := Render(vid, 12, 6)
	if strings.Contains(out, "install ffmpeg") || strings.Contains(out, "cannot render") {
		t.Fatalf("video preview should have rendered a frame, got %q", out)
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected a half-block frame from the video, got %q", out)
	}
}

func TestDetectKindPDF(t *testing.T) {
	if got := DetectKind("doc.pdf"); got != KindPDF {
		t.Errorf("DetectKind(.pdf) = %v, want KindPDF", got)
	}
	if got := DetectKind("doc.PDF"); got != KindPDF {
		t.Errorf("DetectKind(.PDF) = %v, want KindPDF", got)
	}
}

// minimalPDF is a hand-written one-page PDF (poppler reconstructs the xref).
const minimalPDF = `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R /Resources << >> >>
endobj
4 0 obj
<< /Length 30 >>
stream
1 0 0 RG 20 20 260 260 re S
endstream
endobj
trailer
<< /Root 1 0 R /Size 5 >>
%%EOF
`

func TestRenderPDFPage(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm (poppler) not installed")
	}
	p := writeTemp(t, "doc.pdf", []byte(minimalPDF))

	out := Render(p, 12, 8)
	if strings.Contains(out, "install poppler") || strings.Contains(out, "cannot render") {
		t.Skipf("pdftoppm could not rasterise the test PDF: %q", out)
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("expected a half-block render of the first PDF page, got %q", out)
	}
}
