package preview

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Video previews reuse the image pipeline: a representative frame is extracted
// with ffmpeg into a PNG (see thumbnail), then rendered like any other image
// (half-block or Kitty placeholders). Extraction is slow, so — like images — it
// runs off the UI thread and the rendered result is cached by the video's path.

var (
	ffmpegOnce sync.Once
	ffmpegPath string // "" when ffmpeg is not installed
)

func ffmpegAvailable() bool {
	ffmpegOnce.Do(func() { ffmpegPath, _ = exec.LookPath("ffmpeg") })
	return ffmpegPath != ""
}

// renderVideo extracts a frame and renders it through the image pipeline. Any
// failure (no ffmpeg, unreadable video) becomes a cached placeholder so the pane
// shows a stable message instead of retrying every navigation.
func renderVideo(path string, cols, rows int) string {
	out, err := cachedRender(path, cols, rows, func() (string, error) {
		if !ffmpegAvailable() {
			return placeholder("video — install ffmpeg for preview"), nil
		}
		frame, err := thumbnail(path, extractFrame)
		if err != nil {
			return placeholder("cannot render video"), nil
		}
		s, err := imagePreviewer.Preview(frame, cols, rows)
		if err != nil {
			return placeholder("cannot render video"), nil
		}
		return s, nil
	})
	if err != nil {
		return placeholder("cannot render video")
	}
	return out
}

// extractFrame writes a single scaled frame from video to out. It seeks a little
// into the video for a more representative frame, falling back to the first frame
// for very short clips.
func extractFrame(video, out string) error {
	for _, seek := range []string{"1", "0"} {
		cmd := exec.Command(ffmpegPath,
			"-y", "-ss", seek, "-i", video,
			"-frames:v", "1",
			"-vf", "scale='min(1280,iw)':-2",
			"-f", "image2", out,
		)
		cmd.Stdout, cmd.Stderr = nil, nil // discard ffmpeg's chatty output
		if err := cmd.Run(); err == nil {
			if fi, e := os.Stat(out); e == nil && fi.Size() > 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("ffmpeg could not extract a frame from %q", video)
}
