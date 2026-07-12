package preview

// Terminal cell size in pixels. Graphics-protocol previews (Kitty/iTerm2/Sixel)
// need it to map the preview pane's cell rectangle to a pixel budget, so the
// image is downscaled to the pane's real resolution instead of the coarse
// half-block grid. Detected once by InitCellSize; a default is used when the
// terminal does not report pixel dimensions (tmux, some Windows terminals, …).
//
// Most monospace cells are roughly 1:2 (width:height).
const (
	defaultCellW = 8
	defaultCellH = 16
)

var (
	cellW = defaultCellW
	cellH = defaultCellH
)

// InitCellSize queries the terminal for its cell size in pixels and stores it.
// Call it once at startup. It uses an ioctl (no escape query), so it is safe to
// call whether or not the TUI has taken over the terminal.
func InitCellSize() {
	cellW, cellH = detectCellSize()
}

// deriveCellSize computes per-cell pixel dimensions from a winsize report,
// falling back to the defaults for any dimension the terminal leaves at zero
// (common in tmux, VSCode and some Windows terminals).
func deriveCellSize(cols, rows, xpixel, ypixel int) (int, int) {
	cw, ch := defaultCellW, defaultCellH
	if cols > 0 && xpixel > 0 {
		cw = xpixel / cols
	}
	if rows > 0 && ypixel > 0 {
		ch = ypixel / rows
	}
	if cw <= 0 {
		cw = defaultCellW
	}
	if ch <= 0 {
		ch = defaultCellH
	}
	return cw, ch
}

// CellSize returns the detected (or default) terminal cell size in pixels.
func CellSize() (int, int) { return cellW, cellH }

// ImagePixels returns the pixel budget for a preview pane of cols×rows cells.
func ImagePixels(cols, rows int) (int, int) { return cols * cellW, rows * cellH }
