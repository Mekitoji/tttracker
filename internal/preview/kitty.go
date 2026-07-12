package preview

import (
	termimg "github.com/blacktop/go-termimg"
)

// graphicsImages is true when the active image previewer emits terminal graphics
// (Kitty Unicode placeholders) instead of half-block text.
var graphicsImages bool

// InitGraphics detects the terminal's image protocol and, when it supports the
// Kitty Unicode-placeholder protocol, switches the side-pane image previewer to
// crisp native graphics. Otherwise the half-block previewer stays. Call once at
// startup, before the TUI grabs the terminal.
func InitGraphics() {
	if termimg.DetectProtocol() == termimg.Kitty {
		imagePreviewer = kittyPreviewer{}
		graphicsImages = true
	}
}

// GraphicsImages reports whether image previews are terminal graphics (Kitty
// placeholders) rather than half-block text. Such output — a grid of placeholder
// cells plus a one-shot image transmission — must be composed directly, without a
// surrounding lipgloss border/padding that could corrupt the transmission.
func GraphicsImages() bool { return graphicsImages }

// kittyPreviewer renders an image as Kitty Unicode-placeholder cells: the image
// is transmitted once and referenced by placeholder characters (U+10EEEE …) that
// lipgloss lays out like ordinary cells and that scroll with the content. The
// image is visible only where its placeholders are, so no explicit clearing is
// needed when the selection changes.
type kittyPreviewer struct{}

func (kittyPreviewer) Preview(path string, width, height int) (string, error) {
	img, err := termimg.Open(path)
	if err != nil {
		return "", err
	}
	return img.
		Width(width).Height(height).
		Scale(termimg.ScaleFit).
		Protocol(termimg.Kitty).
		UseUnicode(true).
		Render()
}
