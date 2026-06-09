package editor_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tttracker/internal/editor"
)

func TestResolvePrefersVisual(t *testing.T) {
	t.Setenv("VISUAL", "myvisual")
	t.Setenv("EDITOR", "myeditor")
	if got := editor.Resolve(); got != "myvisual" {
		t.Fatalf("want myvisual, got %q", got)
	}
	t.Setenv("VISUAL", "")
	if got := editor.Resolve(); got != "myeditor" {
		t.Fatalf("want myeditor, got %q", got)
	}
}

// GATE 2: the temp-file round-trip works end to end. A fake "editor" (a POSIX
// shell script) appends text to the file; Open must return the edited content.
func TestOpenRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake editor is a POSIX shell script")
	}
	script := filepath.Join(t.TempDir(), "fakeeditor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ' EDITED' >> \"$1\"\n"), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", script)

	got, err := editor.Open("hello")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if want := "hello EDITED"; got != want {
		t.Fatalf("round-trip: want %q, got %q", want, got)
	}
}
