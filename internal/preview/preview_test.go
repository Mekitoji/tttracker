package preview

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
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
		{"text", writeTemp(t, "a.go", []byte("package main\n")), KindText},
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
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
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
