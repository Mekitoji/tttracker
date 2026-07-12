package preview

import (
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
)

// ImagePreviewer renders an image file into a string sized for width×height
// terminal cells. Different implementations can use different techniques; the
// default is the half-block thumbnail below.
//
// It returns the rendered string (not just an error) because the TUI composes
// its frame from strings — a previewer that drew straight to the terminal would
// fight Bubble Tea's diffing renderer. Alternative backends (Kitty/iTerm2/Sixel)
// can still satisfy this by returning their escape sequence as the string.
type ImagePreviewer interface {
	Preview(path string, width, height int) (string, error)
}

// imagePreviewer is the active backend for the side pane (half-block thumbnails,
// which compose with the rest of the lipgloss layout). Swap it to try alternatives.
var imagePreviewer ImagePreviewer = halfBlockPreviewer{}

// renderImage produces the preview for an image, cached by path+modtime+size so
// the (relatively expensive) decode/resample runs once rather than every frame.
func renderImage(path string, cols, rows int) string {
	out, err := cachedImage(path, cols, rows)
	if err != nil {
		return placeholder("cannot render image")
	}
	return out
}

var (
	cacheMu  sync.Mutex
	imgCache = map[string]string{}
)

func cachedImage(path string, w, h int) (string, error) {
	key := imageCacheKey(path, w, h)

	cacheMu.Lock()
	if s, ok := imgCache[key]; ok {
		cacheMu.Unlock()
		return s, nil
	}
	cacheMu.Unlock()

	s, err := imagePreviewer.Preview(path, w, h)
	if err != nil {
		return "", err
	}

	cacheMu.Lock()
	if len(imgCache) > 64 { // bound the cache; previews are few
		imgCache = map[string]string{}
	}
	imgCache[key] = s
	cacheMu.Unlock()
	return s, nil
}

// Cached returns a rendered image preview if it is already in the cache, without
// rendering (which can be slow for large images). Callers can show a placeholder
// and render off the UI thread on a miss.
func Cached(path string, w, h int) (string, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	s, ok := imgCache[imageCacheKey(path, w, h)]
	return s, ok
}

func imageCacheKey(path string, w, h int) string {
	var mod int64
	if fi, err := os.Stat(path); err == nil {
		mod = fi.ModTime().UnixNano()
	}
	return fmt.Sprintf("%s|%d|%dx%d", path, mod, w, h)
}

// halfBlockPreviewer renders an image as a half-block thumbnail.
type halfBlockPreviewer struct{}

func (halfBlockPreviewer) Preview(path string, width, height int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}
	return halfBlocks(img, width, height), nil
}

// halfBlocks renders img using the upper-half-block glyph "▀": each cell shows
// two vertically stacked pixels (foreground = top, background = bottom), so one
// cell row covers two pixel rows. The image is downscaled with box-average
// sampling (never upscaled) so high-detail images stay recognisable instead of
// turning into nearest-neighbour noise.
func halfBlocks(img image.Image, cols, rows int) string {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW < 1 || srcH < 1 {
		return placeholder("empty image")
	}

	pxW, pxH := cols, rows*2
	scale := min(float64(pxW)/float64(srcW), float64(pxH)/float64(srcH))
	if scale > 1 {
		scale = 1 // don't upscale tiny images
	}
	outW := clampInt(int(float64(srcW)*scale), 1, pxW)
	outH := clampInt(int(float64(srcH)*scale), 2, pxH)
	outH -= outH % 2 // even height so pixels pair into cells
	if outH < 2 {
		outH = 2
	}

	var sb strings.Builder
	for y := 0; y < outH; y += 2 {
		for x := range outW {
			tr, tg, tb := avgRGB(img, b, srcW, srcH, outW, outH, x, y)
			br, bg, bb := avgRGB(img, b, srcW, srcH, outW, outH, x, y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", tr, tg, tb, br, bg, bb)
		}
		sb.WriteString("\x1b[0m")
		if y+2 < outH {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// avgRGB averages the source pixels covered by output pixel (ox, oy), so each
// thumbnail pixel reflects its whole source region rather than a single sample.
// Sampling is strided to at most maxSteps per axis to bound cost on large images.
func avgRGB(img image.Image, b image.Rectangle, srcW, srcH, outW, outH, ox, oy int) (uint8, uint8, uint8) {
	const maxSteps = 8
	sx0 := b.Min.X + ox*srcW/outW
	sx1 := b.Min.X + (ox+1)*srcW/outW
	sy0 := b.Min.Y + oy*srcH/outH
	sy1 := b.Min.Y + (oy+1)*srcH/outH
	if sx1 <= sx0 {
		sx1 = sx0 + 1
	}
	if sy1 <= sy0 {
		sy1 = sy0 + 1
	}
	stepX := max((sx1-sx0)/maxSteps, 1)
	stepY := max((sy1-sy0)/maxSteps, 1)

	var rs, gs, bs, n uint64
	for y := sy0; y < sy1; y += stepY {
		for x := sx0; x < sx1; x += stepX {
			r, g, bl, _ := img.At(x, y).RGBA()
			rs += uint64(r >> 8)
			gs += uint64(g >> 8)
			bs += uint64(bl >> 8)
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return uint8(rs / n), uint8(gs / n), uint8(bs / n)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
