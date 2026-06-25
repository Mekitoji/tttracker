package project_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/project"
)

// fakeRemover records the keys it was asked to clean up, standing in for the
// attachment service so the project package needn't depend on it. err, if set, is
// returned to simulate a file-cleanup failure.
type fakeRemover struct {
	called []string
	err    error
}

func (f *fakeRemover) RemoveProjectFiles(key string) error {
	f.called = append(f.called, key)
	return f.err
}

func newService(t *testing.T, rm project.AttachmentRemover) (*project.Service, context.Context) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return project.NewService(d, project.NewRepository(), clock.Real{}, rm), context.Background()
}

func TestDeleteRemovesProjectAndCleansFiles(t *testing.T) {
	rm := &fakeRemover{}
	svc, ctx := newService(t, rm)
	if _, err := svc.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, "PET"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(rm.called) != 1 || rm.called[0] != "PET" {
		t.Fatalf("RemoveProjectFiles should be called once with PET, got %v", rm.called)
	}
	if _, err := svc.Get(ctx, "PET"); err == nil {
		t.Fatal("project should be gone after delete")
	}
}

func TestDeletePropagatesCleanupError(t *testing.T) {
	rm := &fakeRemover{err: errors.New("cleanup boom")}
	svc, ctx := newService(t, rm)
	if _, err := svc.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}

	err := svc.Delete(ctx, "PET")
	if err == nil || !strings.Contains(err.Error(), "cleanup boom") {
		t.Fatalf("delete should propagate the cleanup error, got %v", err)
	}
	// The project is still deleted despite the cleanup failure.
	if _, err := svc.Get(ctx, "PET"); err == nil {
		t.Fatal("project should be gone even when cleanup fails")
	}
}

func TestDeleteMissingProjectSkipsCleanup(t *testing.T) {
	rm := &fakeRemover{}
	svc, ctx := newService(t, rm)
	if err := svc.Delete(ctx, "NOPE"); err == nil {
		t.Fatal("expected error deleting a missing project")
	}
	if len(rm.called) != 0 {
		t.Fatalf("remover must not be called when the delete fails, got %v", rm.called)
	}
}
