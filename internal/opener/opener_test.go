package opener

import (
	"errors"
	"os/exec"
	"testing"
)

func TestOpenPassesPathToStart(t *testing.T) {
	var got *exec.Cmd
	orig := runProcess
	runProcess = func(cmd *exec.Cmd) error { got = cmd; return nil }
	t.Cleanup(func() { runProcess = orig })

	if err := Open("/tmp/report.pdf"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got == nil {
		t.Fatal("runProcess was not called")
	}
	// The path is always the final argument across all platforms.
	if last := got.Args[len(got.Args)-1]; last != "/tmp/report.pdf" {
		t.Fatalf("path not passed to command: args=%v", got.Args)
	}
}

func TestOpenPropagatesError(t *testing.T) {
	want := errors.New("boom")
	orig := runProcess
	runProcess = func(*exec.Cmd) error { return want }
	t.Cleanup(func() { runProcess = orig })

	if err := Open("/tmp/x"); !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}
