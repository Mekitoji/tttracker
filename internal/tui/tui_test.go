package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/editor"
	"tttracker/internal/ticket"
)

func newTestApp(t *testing.T) *app.App {
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
	return app.New(d, clock.Real{}, filepath.Join(base, "attachments"))
}

func newTestModel(t *testing.T, a *app.App) model {
	t.Helper()
	pm, err := newProjectsModel(a, context.Background())
	if err != nil {
		t.Fatalf("projects model: %v", err)
	}
	return model{app: a, ctx: context.Background(), screen: screenProjects, projects: pm, width: 120, height: 40}
}

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// send applies msg and, if a transition command results, runs it once and
// applies the resulting message. It deliberately does not pump further commands
// (e.g. cursor blink) to avoid loops.
func send(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, cmd := m.Update(msg)
	m = updated.(model)
	if cmd != nil {
		if out := cmd(); out != nil {
			if _, quit := out.(tea.QuitMsg); !quit {
				updated, _ = m.Update(out)
				m = updated.(model)
			}
		}
	}
	return m
}

func TestNavigationProjectsBoardDetail(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "first"}); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, a)
	if m.screen != screenProjects {
		t.Fatalf("want projects, got %v", m.screen)
	}

	m = send(t, m, keyEnter) // open project -> board
	if m.screen != screenBoard || m.board.projectKey != "PET" {
		t.Fatalf("want board/PET, got screen=%v key=%q", m.screen, m.board.projectKey)
	}

	m = send(t, m, keyEnter) // open ticket (todo column, first row) -> detail
	if m.screen != screenDetail || m.detail.ticket.Title != "first" {
		t.Fatalf("want detail/first, got screen=%v title=%q", m.screen, m.detail.ticket.Title)
	}

	m = send(t, m, keyEsc) // back to board
	if m.screen != screenBoard {
		t.Fatalf("want board after esc, got %v", m.screen)
	}
	m = send(t, m, keyEsc) // back to projects
	if m.screen != screenProjects {
		t.Fatalf("want projects after esc, got %v", m.screen)
	}
}

func TestCreateProjectAndTicketFlow(t *testing.T) {
	a := newTestApp(t)
	m := newTestModel(t, a)

	m = send(t, m, keyRunes("n")) // -> project form
	if m.screen != screenProjectForm {
		t.Fatalf("want project form, got %v", m.screen)
	}
	m = send(t, m, createProjectMsg{key: "PET", name: "Pet One"})
	if m.screen != screenProjects {
		t.Fatalf("want projects after create, got %v", m.screen)
	}
	if len(m.projects.projects) != 1 || m.projects.projects[0].Key != "PET" {
		t.Fatalf("project not created/listed: %+v", m.projects.projects)
	}
	if m.projects.projects[0].Name != "Pet One" {
		t.Fatalf("name from create not applied: %q", m.projects.projects[0].Name)
	}

	m = send(t, m, keyEnter) // open board
	if m.screen != screenBoard {
		t.Fatalf("want board, got %v", m.screen)
	}
	m = send(t, m, keyRunes("n")) // -> ticket form
	if m.screen != screenTicketForm {
		t.Fatalf("want ticket form, got %v", m.screen)
	}
	m = send(t, m, submitFormMsg{value: "Fix bug"})
	if m.screen != screenBoard {
		t.Fatalf("want board after create, got %v", m.screen)
	}
	if got := len(m.board.columns[0]); got != 1 {
		t.Fatalf("todo column should have 1 ticket, got %d", got)
	}
}

func TestInvalidProjectKeyKeepsForm(t *testing.T) {
	a := newTestApp(t)
	m := newTestModel(t, a)
	m = send(t, m, keyRunes("n"))
	m = send(t, m, createProjectMsg{key: "bad-key"}) // lowercase/dash -> invalid KEY
	if m.screen != screenProjectForm {
		t.Fatalf("want to stay on form, got %v", m.screen)
	}
	if m.projectCreate.errMsg == "" {
		t.Fatal("expected a validation error message on the form")
	}
}

func mustSeed(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "first"}); err != nil {
		t.Fatal(err)
	}
}

func openDetail(t *testing.T, a *app.App) model {
	t.Helper()
	m := newTestModel(t, a)
	m = send(t, m, keyEnter) // projects -> board
	m = send(t, m, keyEnter) // board -> detail (PET-1, todo col, first row)
	if m.screen != screenDetail {
		t.Fatalf("want detail, got %v", m.screen)
	}
	return m
}

func TestStatusChangeFromBoard(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := newTestModel(t, a)

	m = send(t, m, keyEnter)      // board
	m = send(t, m, keyRunes("m")) // status picker
	if m.screen != screenPicker {
		t.Fatalf("want picker, got %v", m.screen)
	}
	m = send(t, m, pickedMsg{value: "done"})
	if m.screen != screenBoard {
		t.Fatalf("want board, got %v", m.screen)
	}
	if got := len(m.board.columns[0]); got != 0 { // todo
		t.Fatalf("todo column should be empty, got %d", got)
	}
	if got := len(m.board.columns[3]); got != 1 { // done
		t.Fatalf("done column should have 1, got %d", got)
	}
}

func TestDetailStatusDescriptionComment(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	// status via picker
	m = send(t, m, keyRunes("m"))
	if m.screen != screenPicker {
		t.Fatalf("want picker, got %v", m.screen)
	}
	m = send(t, m, pickedMsg{value: "in_progress"})
	if m.detail.ticket.Status != ticket.StatusInProgress {
		t.Fatalf("status not updated: %v", m.detail.ticket.Status)
	}

	// description via $EDITOR (simulate the editor result)
	m = send(t, m, keyRunes("e"))
	if m.pending.kind != actionDescription {
		t.Fatalf("pending should be description, got %v", m.pending.kind)
	}
	m = send(t, m, editor.EditedMsg{Content: "# Title\n\nbody"})
	if m.detail.ticket.Description != "# Title\n\nbody" {
		t.Fatalf("description not updated: %q", m.detail.ticket.Description)
	}

	// comment via $EDITOR
	m = send(t, m, keyRunes("c"))
	m = send(t, m, editor.EditedMsg{Content: "looks good"})
	if len(m.detail.comments) != 1 || m.detail.comments[0].Body != "looks good" {
		t.Fatalf("comment not added: %+v", m.detail.comments)
	}
}

func TestSubtaskAddToggleDelete(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	m = send(t, m, keyRunes("s"))
	if m.screen != screenActionForm {
		t.Fatalf("want action form, got %v", m.screen)
	}
	m = send(t, m, submitFormMsg{value: "write tests"})
	if len(m.detail.subtasks) != 1 {
		t.Fatalf("subtask not added: %d", len(m.detail.subtasks))
	}

	m = send(t, m, tea.KeyMsg{Type: tea.KeySpace}) // toggle selected
	if !m.detail.subtasks[0].IsDone {
		t.Fatal("subtask should be done after toggle")
	}

	m = send(t, m, keyRunes("d")) // delete selected
	if len(m.detail.subtasks) != 0 {
		t.Fatalf("subtask not deleted: %d", len(m.detail.subtasks))
	}
}

func TestSearchFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "fix sqlite bug"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "add board view"}); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, a)
	m = send(t, m, keyEnter)      // board
	m = send(t, m, keyRunes("/")) // open search
	if m.screen != screenSearch {
		t.Fatalf("want search, got %v", m.screen)
	}
	m = send(t, m, keyRunes("sqlite")) // type query (textinput inserts all runes)
	if len(m.search.results) != 1 || m.search.results[0].Ticket.Title != "fix sqlite bug" {
		t.Fatalf("search results wrong: %+v", m.search.results)
	}
	m = send(t, m, keyEnter) // open the hit
	if m.screen != screenDetail || m.detail.ticket.Title != "fix sqlite bug" {
		t.Fatalf("want detail of hit, got screen=%v", m.screen)
	}
}

func TestProjectEditFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)

	m = send(t, m, keyRunes("e")) // projects -> edit
	if m.screen != screenProjectEdit {
		t.Fatalf("want project edit, got %v", m.screen)
	}

	// Edit name (free-form, allows spaces/brackets).
	m = send(t, m, keyEnter) // enter name field (prefilled "Pet")
	if m.projectEdit.mode != peName {
		t.Fatalf("want name input mode, got %v", m.projectEdit.mode)
	}
	m = send(t, m, keyRunes(" [wip]"))
	m = send(t, m, keyEnter) // save
	if m.projectEdit.name != "Pet [wip]" {
		t.Fatalf("name not updated: %q", m.projectEdit.name)
	}

	// Set repo path via manual entry (toggled from the picker).
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown}) // cursor -> Repo path
	m = send(t, m, keyEnter)                      // open filepicker
	if m.projectEdit.mode != peRepoPick {
		t.Fatalf("want repo pick mode, got %v", m.projectEdit.mode)
	}
	m = send(t, m, keyRunes("i")) // switch to manual entry
	if m.projectEdit.mode != peRepoManual {
		t.Fatalf("want manual mode, got %v", m.projectEdit.mode)
	}
	m = send(t, m, keyRunes(repo))
	m = send(t, m, keyEnter) // save -> validates .git
	if m.projectEdit.repoPath != repo {
		t.Fatalf("repo path not set: %q (want %q)", m.projectEdit.repoPath, repo)
	}

	// Back to projects, new name reflected in the list.
	m = send(t, m, keyEsc)
	if m.screen != screenProjects {
		t.Fatalf("want projects after esc, got %v", m.screen)
	}
	if m.projects.projects[0].Name != "Pet [wip]" {
		t.Fatalf("name not reflected in list: %q", m.projects.projects[0].Name)
	}
}

func TestLabelsEdit(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	m = send(t, m, keyRunes("l")) // labels form
	if m.screen != screenActionForm {
		t.Fatalf("want action form, got %v", m.screen)
	}
	m = send(t, m, submitFormMsg{value: "bug, ui, p1"})
	if m.screen != screenDetail {
		t.Fatalf("want detail, got %v", m.screen)
	}
	got := m.detail.ticket.Labels
	if len(got) != 3 || got[0] != "bug" || got[1] != "ui" || got[2] != "p1" {
		t.Fatalf("labels not set correctly: %+v", got)
	}
}

func TestCommentEditAndDelete(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	// Add a comment via $EDITOR.
	m = send(t, m, keyRunes("c"))
	m = send(t, m, editor.EditedMsg{Content: "first version"})
	if len(m.detail.comments) != 1 {
		t.Fatalf("comment not added: %d", len(m.detail.comments))
	}

	// Cursor is on the comment (no subtasks); enter edits it via $EDITOR.
	m = send(t, m, keyEnter)
	if m.pending.kind != actionCommentEdit {
		t.Fatalf("want comment-edit pending, got %v", m.pending.kind)
	}
	m = send(t, m, editor.EditedMsg{Content: "edited version"})
	if m.detail.comments[0].Body != "edited version" {
		t.Fatalf("comment not edited: %q", m.detail.comments[0].Body)
	}

	// d deletes the selected comment.
	m = send(t, m, keyRunes("d"))
	if len(m.detail.comments) != 0 {
		t.Fatalf("comment not deleted: %d", len(m.detail.comments))
	}
}
