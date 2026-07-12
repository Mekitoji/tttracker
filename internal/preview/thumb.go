package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Some previews (video frames, PDF pages) are produced by an external tool that
// writes a PNG, which is then rendered through the image pipeline. thumbnail
// centralises that: it runs the extractor once per source (keyed by path+modtime)
// into a temp PNG and returns its path, so a slow extraction is not repeated on
// resize or re-selection.

var (
	thumbOnce  sync.Once
	thumbDir   string // temp dir holding extracted PNGs
	thumbMu    sync.Mutex
	thumbCache = map[string]string{} // source key -> extracted PNG path
	thumbSeq   int
)

func thumbDirReady() bool {
	thumbOnce.Do(func() { thumbDir, _ = os.MkdirTemp("", "tttracker-thumbs-") })
	return thumbDir != ""
}

// thumbnail returns a cached PNG thumbnail for src, extracting it once via extract
// (which must write a PNG to the given output path).
func thumbnail(src string, extract func(src, out string) error) (string, error) {
	if !thumbDirReady() {
		return "", fmt.Errorf("no temp dir for thumbnails")
	}
	key := imageCacheKey(src, 0, 0) // path|modtime|0x0 — size doesn't affect the source frame

	thumbMu.Lock()
	if fp, ok := thumbCache[key]; ok {
		thumbMu.Unlock()
		if fi, err := os.Stat(fp); err == nil && fi.Size() > 0 {
			return fp, nil
		}
	} else {
		thumbMu.Unlock()
	}

	thumbMu.Lock()
	thumbSeq++
	out := filepath.Join(thumbDir, fmt.Sprintf("thumb-%d.png", thumbSeq))
	thumbMu.Unlock()

	if err := extract(src, out); err != nil {
		return "", err
	}
	thumbMu.Lock()
	thumbCache[key] = out
	thumbMu.Unlock()
	return out, nil
}
