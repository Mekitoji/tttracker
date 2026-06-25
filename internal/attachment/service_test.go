package attachment

import (
	"errors"
	"os"
	"testing"
)

func TestAttachRollbackErrorPreservesBothFailures(t *testing.T) {
	txErr := errors.New("transaction failed")
	cleanupErr := &os.PathError{Op: "remove", Path: "/tmp/copied", Err: errors.New("permission denied")}

	err := attachRollbackError(txErr, cleanupErr)
	if !errors.Is(err, txErr) {
		t.Fatalf("transaction error was lost: %v", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("cleanup error was lost: %v", err)
	}

	if got := attachRollbackError(txErr, nil); got != txErr {
		t.Fatalf("without cleanup failure, want original error, got %v", got)
	}
}
