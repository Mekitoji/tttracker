// Package opener launches the OS default application for a file or URL. Unlike
// the editor (which needs the terminal), GUI handlers run detached. The platform
// launcher (open/xdg-open/start) hands the file to the GUI app and exits right
// away, so Open runs it to completion: waiting reaps the child (no zombie on
// Unix) and surfaces launcher errors such as "no application found for the file".
package opener

import (
	"os/exec"
	"runtime"
)

// runProcess runs cmd to completion (start + wait), reaping the child process and
// returning any launcher error. It is a package-level seam so tests can stub the
// launch instead of opening real applications.
var runProcess = func(cmd *exec.Cmd) error { return cmd.Run() }

// Open launches the OS default handler for path (a file path or URL). It returns
// once the launcher process exits, which is near-immediate since the launcher
// detaches the GUI app; callers should still run it off the UI event loop so a
// misbehaving launcher cannot freeze the interface.
func Open(path string) error {
	return runProcess(command(path))
}

// command builds the platform-specific "open" invocation.
func command(path string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path)
	case "windows":
		// The empty "" is the (ignored) window title start expects as its first arg.
		return exec.Command("cmd", "/c", "start", "", path)
	default:
		return exec.Command("xdg-open", path)
	}
}
