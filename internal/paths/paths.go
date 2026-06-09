// Package paths resolves where the tracker keeps its data. The location is
// global by default (XDG data dir) and can be overridden by an explicit flag
// value or the TRACKER_DATA environment variable.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	appDirName = "tracker"
	envDataDir = "TRACKER_DATA"
)

// DataDir resolves the data directory with the precedence:
//
//	override (e.g. --data-dir) > $TRACKER_DATA > $XDG_DATA_HOME/tracker > ~/.local/share/tracker
func DataDir(override string) (string, error) {
	if override != "" {
		return expand(override)
	}
	if v := strings.TrimSpace(os.Getenv(envDataDir)); v != "" {
		return expand(v)
	}
	if v := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); v != "" {
		return filepath.Join(v, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", appDirName), nil
}

// DBPath returns the SQLite database file path inside dataDir.
func DBPath(dataDir string) string { return filepath.Join(dataDir, "app.db") }

// AttachmentsDir returns the attachments root inside dataDir.
func AttachmentsDir(dataDir string) string { return filepath.Join(dataDir, "attachments") }

// EnsureLayout creates dataDir and its attachments subdirectory.
func EnsureLayout(dataDir string) error {
	return os.MkdirAll(AttachmentsDir(dataDir), 0o755)
}

func expand(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return filepath.Abs(p)
}
