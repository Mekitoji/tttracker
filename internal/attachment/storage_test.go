package attachment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tttracker/internal/apperr"
)

func TestStoreSizeMatchesSavedFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "report.txt")
	content := []byte(strings.Repeat("payload", 1000))
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	stored, err := NewStorage(t.TempDir()).Store(src, "P", "P-1")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	info, err := os.Stat(stored.StoredPath)
	if err != nil {
		t.Fatalf("stat stored: %v", err)
	}
	if stored.SizeBytes != info.Size() || stored.SizeBytes != int64(len(content)) {
		t.Fatalf("metadata size %d, saved size %d, content size %d", stored.SizeBytes, info.Size(), len(content))
	}
}

func TestStoreRejectsNonRegularFile(t *testing.T) {
	_, err := NewStorage(t.TempDir()).Store(os.DevNull, "P", "P-1")
	if !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("device should be rejected as invalid, got %v", err)
	}
}

// Concurrent Store calls for the same source filename into the same ticket must
// never truncate or merge into one file: each gets a distinct destination with
// its own intact contents. Run with -race to also catch data races.
func TestStoreConcurrentSameNameNoCorruption(t *testing.T) {
	root := t.TempDir()
	s := NewStorage(root)

	const n = 16
	srcs := make([]string, n)
	want := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		// Each source lives in its own dir but shares the base name "report.txt".
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("s%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		p := filepath.Join(dir, "report.txt")
		content := fmt.Sprintf("payload-%d-%s", i, strings.Repeat("x", 2000))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write src: %v", err)
		}
		srcs[i] = p
		want[content] = true
	}

	results := make([]*Stored, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = s.Store(srcs[i], "P", "P-1")
		}(i)
	}
	wg.Wait()

	gotContent := make(map[string]bool, n)
	gotPath := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("store %d: %v", i, errs[i])
		}
		st := results[i]
		if gotPath[st.StoredPath] {
			t.Fatalf("two attachments share destination %q", st.StoredPath)
		}
		gotPath[st.StoredPath] = true

		b, err := os.ReadFile(st.StoredPath)
		if err != nil {
			t.Fatalf("read stored %d: %v", i, err)
		}
		if st.SizeBytes != int64(len(b)) {
			t.Fatalf("stored %d metadata size %d, saved size %d", i, st.SizeBytes, len(b))
		}
		gotContent[string(b)] = true
	}

	// Every source payload must survive exactly once: corruption or a merge would
	// drop or duplicate one, leaving fewer than n distinct contents.
	if len(gotContent) != n {
		t.Fatalf("want %d distinct intact files, got %d (corruption/merge)", n, len(gotContent))
	}
	for c := range want {
		if !gotContent[c] {
			t.Fatal("a source payload was lost or corrupted")
		}
	}
}
