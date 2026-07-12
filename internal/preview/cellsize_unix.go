//go:build unix

package preview

import (
	"os"

	"golang.org/x/sys/unix"
)

// detectCellSize reads the terminal cell size via TIOCGWINSZ (a syscall, not an
// escape query, so it never collides with the TUI's raw-mode input).
func detectCellSize() (int, int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws == nil {
		return defaultCellW, defaultCellH
	}
	return deriveCellSize(int(ws.Col), int(ws.Row), int(ws.Xpixel), int(ws.Ypixel))
}
