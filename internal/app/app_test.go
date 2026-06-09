package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tttracker/internal/activity"
	"tttracker/internal/app"
	"tttracker/internal/apperr"
	"tttracker/internal/db"
	"tttracker/internal/ticket"
)

// testClock is an advanceable clock so tests can assert that updated_at moves.
type testClock struct{ t time.Time }

func (c *testClock) Now() time.Time          { return c.t }
func (c *testClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newApp(t *testing.T) (*app.App, *testClock) {
	t.Helper()
	base := t.TempDir()
	d, err := db.Open(filepath.Join(base, "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clk := &testClock{t: time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC)}
	return app.New(d, clk, filepath.Join(base, "attachments")), clk
}

func TestProjectCreateValidationAndConflict(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()

	p, err := a.Projects.Create(ctx, "PET", "Pet project", "stuff")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == 0 || p.Key != "PET" || p.Name != "Pet project" {
		t.Fatalf("unexpected project: %+v", p)
	}

	if _, err := a.Projects.Create(ctx, "PET", "again", ""); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if _, err := a.Projects.Create(ctx, "pet", "lower", ""); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for lowercase key, got %v", err)
	}

	list, err := a.Projects.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: len=%d err=%v", len(list), err)
	}
}

func TestTicketNumberingDefaultsAndCreatedEvent(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	mustProject(t, a, "WORK")

	p1, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "first"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	p2, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "second"})
	w1, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "WORK", Title: "work first"})

	if p1.Number != 1 || p2.Number != 2 || w1.Number != 1 {
		t.Fatalf("numbering wrong: %d %d %d", p1.Number, p2.Number, w1.Number)
	}
	if p1.Type != ticket.TypeTask || p1.Status != ticket.StatusTodo || p1.Priority != ticket.PriorityMedium {
		t.Fatalf("defaults wrong: %+v", p1)
	}

	evs, err := a.Activity.List(ctx, p1.ID)
	if err != nil || len(evs) != 1 || evs[0].Type != activity.TicketCreated {
		t.Fatalf("created event missing: %+v err=%v", evs, err)
	}
}

func TestTicketStatusCompletedAtAndEvents(t *testing.T) {
	a, clk := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	created, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "x"})

	clk.advance(time.Hour)
	done, err := a.Tickets.SetStatus(ctx, "PET-1", "done")
	if err != nil {
		t.Fatalf("set done: %v", err)
	}
	if done.Status != ticket.StatusDone || done.CompletedAt == nil {
		t.Fatalf("expected done + completed_at, got %+v", done)
	}
	if !done.UpdatedAt.After(created.CreatedAt) {
		t.Fatalf("updated_at should advance: created=%v updated=%v", created.CreatedAt, done.UpdatedAt)
	}

	reopened, _ := a.Tickets.SetStatus(ctx, "PET-1", "in_progress")
	if reopened.CompletedAt != nil {
		t.Fatalf("leaving done should clear completed_at, got %v", reopened.CompletedAt)
	}

	// Re-setting the same status is a no-op: no extra event.
	before := len(mustEvents(t, a, created.ID))
	if _, err := a.Tickets.SetStatus(ctx, "PET-1", "in_progress"); err != nil {
		t.Fatalf("noop set: %v", err)
	}
	if after := len(mustEvents(t, a, created.ID)); after != before {
		t.Fatalf("no-op status change recorded an event: %d -> %d", before, after)
	}

	if _, err := a.Tickets.SetStatus(ctx, "PET-1", "bogus"); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad status, got %v", err)
	}

	// The first status change payload decodes to todo -> done.
	evs := mustEvents(t, a, created.ID)
	var sc *activity.Event
	for i := range evs {
		if evs[i].Type == activity.TicketStatusChanged {
			sc = &evs[i]
			break
		}
	}
	if sc == nil {
		t.Fatal("no status_changed event")
	}
	var change activity.StringChange
	if err := json.Unmarshal(sc.Payload, &change); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if change.From != "todo" || change.To != "done" {
		t.Fatalf("payload wrong: %+v", change)
	}
}

func TestTicketGetErrors(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")

	if _, err := a.Tickets.Get(ctx, "NOPE"); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for keyless string, got %v", err)
	}
	if _, err := a.Tickets.Get(ctx, "PET-99"); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("want ErrNotFound for missing number, got %v", err)
	}
}

func TestSubtaskLifecycle(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	tk, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "x"})

	s1, err := a.Subtasks.Add(ctx, "PET-1", "write tests")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	s2, _ := a.Subtasks.Add(ctx, "PET-1", "make it pass")
	if s1.Position != 1 || s2.Position != 2 {
		t.Fatalf("positions wrong: %d %d", s1.Position, s2.Position)
	}

	toggled, _ := a.Subtasks.Toggle(ctx, s1.ID)
	if !toggled.IsDone {
		t.Fatal("expected toggled done")
	}
	reopened, _ := a.Subtasks.Toggle(ctx, s1.ID)
	if reopened.IsDone {
		t.Fatal("expected toggled back open")
	}

	if err := a.Subtasks.Delete(ctx, s2.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ := a.Subtasks.List(ctx, "PET-1")
	if len(list) != 1 || list[0].ID != s1.ID {
		t.Fatalf("list after delete wrong: %+v", list)
	}

	// Events: created x2, completed, reopened, deleted = 5 (+ ticket.created).
	evs := mustEvents(t, a, tk.ID)
	got := countTypes(evs)
	if got[activity.SubtaskCreated] != 2 || got[activity.SubtaskCompleted] != 1 ||
		got[activity.SubtaskReopened] != 1 || got[activity.SubtaskDeleted] != 1 {
		t.Fatalf("subtask events wrong: %v", got)
	}
}

func TestCommentLifecycle(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	tk, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "x"})

	c, err := a.Comments.Add(ctx, "PET-1", "first note")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := a.Comments.Edit(ctx, c.ID, "edited note"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, err := a.Comments.Add(ctx, "PET-1", ""); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty comment, got %v", err)
	}
	got, _ := a.Comments.List(ctx, "PET-1")
	if len(got) != 1 || got[0].Body != "edited note" {
		t.Fatalf("list wrong: %+v", got)
	}
	if err := a.Comments.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	evs := mustEvents(t, a, tk.ID)
	types := countTypes(evs)
	if types[activity.CommentCreated] != 1 || types[activity.CommentUpdated] != 1 || types[activity.CommentDeleted] != 1 {
		t.Fatalf("comment events wrong: %v", types)
	}
}

func TestAttachmentLifecycle(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	tk, _ := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "x"})

	src := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(src, []byte("hello attachment"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	at, err := a.Attachments.Attach(ctx, "PET-1", src)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if at.SizeBytes != int64(len("hello attachment")) {
		t.Fatalf("size wrong: %d", at.SizeBytes)
	}
	if got, err := os.ReadFile(at.StoredPath); err != nil || string(got) != "hello attachment" {
		t.Fatalf("stored content wrong: %q err=%v", got, err)
	}

	// Same source name again -> de-duplicated stored filename, file preserved.
	at2, err := a.Attachments.Attach(ctx, "PET-1", src)
	if err != nil {
		t.Fatalf("attach 2: %v", err)
	}
	if at2.FileName == at.FileName {
		t.Fatalf("expected de-duplicated name, both %q", at.FileName)
	}

	if list, _ := a.Attachments.List(ctx, "PET-1"); len(list) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(list))
	}

	if err := a.Attachments.Remove(ctx, at.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(at.StoredPath); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted, stat err=%v", err)
	}
	if list, _ := a.Attachments.List(ctx, "PET-1"); len(list) != 1 {
		t.Fatalf("want 1 after remove, got %d", len(list))
	}

	types := countTypes(mustEvents(t, a, tk.ID))
	if types[activity.AttachmentAdded] != 2 || types[activity.AttachmentRemoved] != 1 {
		t.Fatalf("attachment events wrong: %v", types)
	}
}

func TestProjectRepoPath(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")

	// A directory that looks like a git repo validates and is stored absolute.
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	p, err := a.Projects.SetRepoPath(ctx, "PET", repo)
	if err != nil {
		t.Fatalf("set repo path: %v", err)
	}
	if !filepath.IsAbs(p.RepoPath) || p.RepoPath != repo {
		t.Fatalf("repo path not stored correctly: %q (want %q)", p.RepoPath, repo)
	}
	if got, _ := a.Projects.Get(ctx, "PET"); got.RepoPath != repo {
		t.Fatalf("repo path not persisted: %q", got.RepoPath)
	}

	// A non-git directory is rejected.
	if _, err := a.Projects.SetRepoPath(ctx, "PET", t.TempDir()); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for non-git dir, got %v", err)
	}
	// A nonexistent path is rejected.
	if _, err := a.Projects.SetRepoPath(ctx, "PET", filepath.Join(repo, "nope")); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for missing path, got %v", err)
	}
	// Clearing is allowed.
	if cleared, err := a.Projects.SetRepoPath(ctx, "PET", ""); err != nil || cleared.RepoPath != "" {
		t.Fatalf("clear failed: %q err=%v", cleared.RepoPath, err)
	}
}

func TestSearch(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "fix sqlite migration"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "board view polish"}); err != nil {
		t.Fatal(err)
	}

	hits, err := a.Tickets.Search(ctx, "sqlite")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Ticket.Title != "fix sqlite migration" {
		t.Fatalf("want 1 sqlite hit, got %+v", hits)
	}
	if hits[0].ProjectKey != "PET" {
		t.Fatalf("project key wrong: %q", hits[0].ProjectKey)
	}

	// Prefix match on an in-progress token.
	if hits, _ := a.Tickets.Search(ctx, "migr"); len(hits) != 1 {
		t.Fatalf("prefix search want 1, got %d", len(hits))
	}
	// Special characters must be escaped, not interpreted as query syntax.
	if _, err := a.Tickets.Search(ctx, "sqlite-migration"); err != nil {
		t.Fatalf("special-char query errored: %v", err)
	}
	// Empty query: no results, no error.
	if hits, err := a.Tickets.Search(ctx, "   "); err != nil || len(hits) != 0 {
		t.Fatalf("empty query: hits=%d err=%v", len(hits), err)
	}
}

func TestProjectSetNameDescription(t *testing.T) {
	a, _ := newApp(t)
	ctx := context.Background()
	mustProject(t, a, "PET")

	if _, err := a.Projects.SetName(ctx, "PET", "my cool project [wip]"); err != nil {
		t.Fatalf("set name: %v", err)
	}
	if _, err := a.Projects.SetDescription(ctx, "PET", "the description"); err != nil {
		t.Fatalf("set description: %v", err)
	}
	p, _ := a.Projects.Get(ctx, "PET")
	if p.Name != "my cool project [wip]" {
		t.Fatalf("name not updated: %q", p.Name)
	}
	if p.Description != "the description" {
		t.Fatalf("description not updated: %q", p.Description)
	}
	// Empty name is rejected.
	if _, err := a.Projects.SetName(ctx, "PET", "  "); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty name, got %v", err)
	}
}

// --- helpers ---

func mustProject(t *testing.T, a *app.App, key string) {
	t.Helper()
	if _, err := a.Projects.Create(context.Background(), key, key, ""); err != nil {
		t.Fatalf("create project %s: %v", key, err)
	}
}

func mustEvents(t *testing.T, a *app.App, ticketID int64) []activity.Event {
	t.Helper()
	evs, err := a.Activity.List(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return evs
}

func countTypes(evs []activity.Event) map[activity.EventType]int {
	m := map[activity.EventType]int{}
	for _, e := range evs {
		m[e.Type]++
	}
	return m
}
