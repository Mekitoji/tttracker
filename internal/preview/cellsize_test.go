package preview

import "testing"

func TestDeriveCellSize(t *testing.T) {
	tests := []struct {
		name                       string
		cols, rows, xpixel, ypixel int
		wantW, wantH               int
	}{
		{"normal", 80, 24, 640, 384, 8, 16},
		{"non-square cell", 100, 40, 900, 800, 9, 20},
		{"no pixels reported", 80, 24, 0, 0, defaultCellW, defaultCellH},
		{"only width reported", 80, 24, 640, 0, 8, defaultCellH},
		{"zero cols guarded", 0, 24, 640, 384, defaultCellW, 16},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cw, ch := deriveCellSize(tc.cols, tc.rows, tc.xpixel, tc.ypixel)
			if cw != tc.wantW || ch != tc.wantH {
				t.Fatalf("deriveCellSize(%d,%d,%d,%d) = %d,%d; want %d,%d",
					tc.cols, tc.rows, tc.xpixel, tc.ypixel, cw, ch, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestImagePixelsUsesCellSize(t *testing.T) {
	origW, origH := cellW, cellH
	t.Cleanup(func() { cellW, cellH = origW, origH })

	cellW, cellH = 8, 16
	if px, py := ImagePixels(40, 20); px != 320 || py != 320 {
		t.Fatalf("ImagePixels(40,20) = %d,%d; want 320,320", px, py)
	}
	if w, h := CellSize(); w != 8 || h != 16 {
		t.Fatalf("CellSize() = %d,%d; want 8,16", w, h)
	}
}
