package tui

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tttracker/internal/activity"
	"tttracker/internal/app"
	"tttracker/internal/attachment"
	"tttracker/internal/clock"
	"tttracker/internal/db"
	"tttracker/internal/editor"
	"tttracker/internal/preview"
	"tttracker/internal/subtask"
	"tttracker/internal/ticket"
)

func newTestApp(t *testing.T) *app.App {
	t.Helper()
	a, _ := newTestAppDB(t)
	return a
}

// newTestAppDB also returns the underlying DB so a test can corrupt it (e.g.
// drop a table) to force a read failure.
func newTestAppDB(t *testing.T) (*app.App, *sql.DB) {
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
	return app.New(d, clock.Real{}, filepath.Join(base, "attachments")), d
}

func newTestModel(t *testing.T, a *app.App) model {
	t.Helper()
	pm, err := newProjectsModel(a, context.Background())
	if err != nil {
		t.Fatalf("projects model: %v", err)
	}
	return model{app: a, ctx: context.Background(), screen: screenProjects, projects: pm, finder: fakeFinder{}, width: 120, height: 40}
}

type fakeFinder struct{ repos []string }

func (f fakeFinder) allRepos() []string { return f.repos }

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
	if got := len(m.board.columns[1]); got != 1 {
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
	if got := len(m.board.columns[1]); got != 0 { // todo
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

func TestRepoManualFinder(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(t, a)
	m.finder = fakeFinder{repos: []string{repo}} // finder returns our known repo

	m = send(t, m, keyRunes("e"))                 // edit project
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown}) // cursor -> Repo path
	m = send(t, m, keyEnter)                      // open filepicker
	m = send(t, m, keyRunes("i"))                 // -> manual search
	if m.projectEdit.mode != peRepoManual {
		t.Fatalf("want manual mode, got %v", m.projectEdit.mode)
	}
	if len(m.projectEdit.results) != 1 || m.projectEdit.results[0] != repo {
		t.Fatalf("finder results wrong: %+v", m.projectEdit.results)
	}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlJ}) // select first result (ctrl+j)
	m = send(t, m, keyEnter)                       // pick -> SetRepoPath (validates .git)
	if m.projectEdit.repoPath != repo {
		t.Fatalf("repo path not set from finder: %q (want %q)", m.projectEdit.repoPath, repo)
	}
}

func TestProjectDeleteFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Projects.Create(ctx, "WRK", "Work", ""); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)

	m = send(t, m, keyRunes("x")) // cursor on PET (sorted first) -> confirm
	if m.screen != screenConfirm {
		t.Fatalf("want confirm, got %v", m.screen)
	}
	// Empty/wrong input keeps the confirm screen with an error.
	m = send(t, m, keyEnter)
	if m.screen != screenConfirm || m.confirm.errMsg == "" {
		t.Fatalf("empty confirm should stay with error: screen=%v err=%q", m.screen, m.confirm.errMsg)
	}
	// Type the key exactly, then confirm.
	m = send(t, m, keyRunes("PET"))
	m = send(t, m, keyEnter)
	if m.screen != screenProjects {
		t.Fatalf("want projects after delete, got %v", m.screen)
	}
	if len(m.projects.projects) != 1 || m.projects.projects[0].Key != "WRK" {
		t.Fatalf("PET should be deleted, got %+v", m.projects.projects)
	}
}

func TestBoardRefreshesAfterDetailEdit(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "old title"}); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)

	m = send(t, m, keyEnter) // board
	m = send(t, m, keyEnter) // detail
	if m.screen != screenDetail {
		t.Fatalf("want detail, got %v", m.screen)
	}

	// Rename the ticket from the detail view.
	m = send(t, m, keyRunes("r"))
	if m.screen != screenActionForm {
		t.Fatalf("want rename form, got %v", m.screen)
	}
	m = send(t, m, submitFormMsg{value: "new title"})
	if m.detail.ticket.Title != "new title" {
		t.Fatalf("detail not updated: %q", m.detail.ticket.Title)
	}

	// Back to the board: it must show the new title, not the stale one.
	m = send(t, m, keyEsc)
	if m.screen != screenBoard {
		t.Fatalf("want board, got %v", m.screen)
	}
	if got := m.board.columns[1][0].Title; got != "new title" {
		t.Fatalf("board not refreshed: %q", got)
	}
}

func TestBoardDeleteTicketFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "first"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "second"}); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)
	m = send(t, m, keyEnter) // board

	m = send(t, m, keyRunes("x")) // confirm delete selected (PET-1)
	if m.screen != screenConfirm {
		t.Fatalf("want confirm, got %v", m.screen)
	}
	m = send(t, m, keyRunes("y")) // confirm
	if m.screen != screenBoard {
		t.Fatalf("want board after delete, got %v", m.screen)
	}
	if got := len(m.board.columns[1]); got != 1 {
		t.Fatalf("todo column should have 1 ticket left, got %d", got)
	}
	if m.board.columns[1][0].Title != "second" {
		t.Fatalf("wrong ticket remains: %q", m.board.columns[1][0].Title)
	}
}

func TestDetailDeleteTicketFlow(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "only one"}); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)
	m = send(t, m, keyEnter) // board
	m = send(t, m, keyEnter) // detail
	if m.screen != screenDetail {
		t.Fatalf("want detail, got %v", m.screen)
	}

	m = send(t, m, keyRunes("x")) // confirm delete the ticket
	if m.screen != screenConfirm {
		t.Fatalf("want confirm, got %v", m.screen)
	}
	m = send(t, m, keyRunes("y")) // confirm -> deleted, back to board
	if m.screen != screenBoard {
		t.Fatalf("want board after delete, got %v", m.screen)
	}
	if got := len(m.board.columns[1]); got != 0 {
		t.Fatalf("todo column should be empty, got %d", got)
	}
}

func TestDeleteTicketConfirmCancel(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "keep"}); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)
	m = send(t, m, keyEnter)      // board
	m = send(t, m, keyRunes("x")) // confirm
	m = send(t, m, keyRunes("n")) // cancel -> back to where we came from (board)
	if m.screen != screenBoard {
		t.Fatalf("cancel should return to board, got %v", m.screen)
	}
	if len(m.board.columns[1]) != 1 {
		t.Fatalf("ticket should remain after cancel, got %d", len(m.board.columns[1]))
	}
}

func TestBoardUsesConfiguredDeleteKey(t *testing.T) {
	orig := keys
	t.Cleanup(func() { keys = orig })
	keys.BoardDeleteTicket = bind([]string{"D"}, "D", "del") // rebind delete to D

	a := newTestApp(t)
	ctx := context.Background()
	if _, err := a.Projects.Create(ctx, "PET", "Pet", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "one"}); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, a)
	m = send(t, m, keyEnter) // board

	// The old key no longer deletes.
	if got := send(t, m, keyRunes("x")); got.screen == screenConfirm {
		t.Fatal("unbound key x should not trigger delete")
	}
	// The configured key does.
	m = send(t, m, keyRunes("D"))
	if m.screen != screenConfirm {
		t.Fatalf("configured delete key should open confirm, got %v", m.screen)
	}
}

// Regression: changing a ticket's status via the `m` picker must not drop the
// blocked column toggle, and focus must follow the moved ticket.
func TestStatusChangeKeepsBlockedView(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a) // PET-1 "first" in todo
	m := newTestModel(t, a)

	m = send(t, m, keyEnter)      // board
	m = send(t, m, keyRunes("!")) // show blocked column
	if !m.board.showBlocked {
		t.Fatalf("blocked column should be visible after !")
	}
	m = send(t, m, keyRunes("m")) // status picker (cursor on PET-1 in todo)
	m = send(t, m, pickedMsg{value: "done"})

	if m.screen != screenBoard {
		t.Fatalf("want board, got %v", m.screen)
	}
	if !m.board.showBlocked {
		t.Fatalf("blocked column disappeared after status change")
	}
	if got, ok := m.board.selected(); !ok || got.Number != 1 || got.Status != ticket.StatusDone {
		t.Fatalf("focus did not follow moved ticket: %+v ok=%v", got, ok)
	}
}

// Regression: changing status while on the inactive (backlog/archived) view must
// keep you on that view rather than snapping back to the primary columns.
func TestStatusChangeKeepsInactiveView(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := newTestModel(t, a)

	m = send(t, m, keyEnter) // board
	// Move PET-1 todo -> backlog so it lives on the inactive view.
	m = send(t, m, keyRunes("m"))
	m = send(t, m, pickedMsg{value: "backlog"})
	// Switch to the inactive view; cursor should land on PET-1 in backlog.
	m = send(t, m, keyRunes("@"))
	if !m.board.showInactive {
		t.Fatalf("should be on inactive view after @")
	}
	if got, ok := m.board.selected(); !ok || got.Number != 1 {
		t.Fatalf("expected PET-1 selected on backlog, got %+v ok=%v", got, ok)
	}
	// Change status backlog -> archived (both inactive).
	m = send(t, m, keyRunes("m"))
	m = send(t, m, pickedMsg{value: "archived"})

	if !m.board.showInactive {
		t.Fatalf("kicked off inactive view after status change")
	}
	if got, ok := m.board.selected(); !ok || got.Status != ticket.StatusArchived {
		t.Fatalf("focus not on archived ticket: %+v ok=%v", got, ok)
	}
}

// Regression: moving a ticket with the keyboard (ctrl+h) keeps the blocked view
// and follows the ticket into its new column.
func TestMoveTicketKeyKeepsBlockedView(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := newTestModel(t, a)

	m = send(t, m, keyEnter)      // board
	m = send(t, m, keyRunes("!")) // show blocked column
	// Cursor on PET-1 in todo; ctrl+h moves it to the blocked column (left).
	m = send(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})

	if !m.board.showBlocked {
		t.Fatalf("blocked view lost after ctrl+h move")
	}
	if got, ok := m.board.selected(); !ok || got.Number != 1 || got.Status != ticket.StatusBlocked {
		t.Fatalf("focus not on moved ticket in blocked: %+v ok=%v", got, ok)
	}
}

// Regression: an action in the detail view (toggle/edit) must keep the cursor on
// the same item instead of resetting it to the top after the reload.
func TestDetailCursorPreservedAcrossAction(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	// Add three subtasks.
	for _, title := range []string{"one", "two", "three"} {
		m = send(t, m, keyRunes("s"))
		m = send(t, m, submitFormMsg{value: title})
	}
	if len(m.detail.subtasks) != 3 {
		t.Fatalf("want 3 subtasks, got %d", len(m.detail.subtasks))
	}

	// Move cursor to the third subtask (index 2).
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.detail.cursor != 2 {
		t.Fatalf("want cursor 2, got %d", m.detail.cursor)
	}
	sel, ok := m.detail.selSub()
	if !ok || sel.Title != "three" {
		t.Fatalf("want 'three' selected, got %+v ok=%v", sel, ok)
	}

	// Toggle it; after the reload the cursor must still be on the third subtask.
	m = send(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.detail.cursor != 2 {
		t.Fatalf("cursor reset after action: got %d, want 2", m.detail.cursor)
	}
	sel, ok = m.detail.selSub()
	if !ok || sel.Title != "three" {
		t.Fatalf("cursor not on 'three' after action: %+v ok=%v", sel, ok)
	}
	if !sel.IsDone {
		t.Fatal("third subtask should be toggled done")
	}
}

// The detail view must never exceed the terminal in either dimension — long
// title/labels, many items and a long activity log are all bounded.
func TestDetailViewFitsScreen(t *testing.T) {
	m := detailModel{
		key: "PET-1",
		ticket: ticket.Ticket{
			Title:  strings.Repeat("a very long title ", 20),
			Labels: []string{strings.Repeat("label", 40)},
		},
		subtasks: make([]subtask.Subtask, 8),
		events:   make([]activity.Event, 200),
		width:    80,
		height:   24,
	}
	m = m.clampScroll()

	assertFits := func(label string, m detailModel) {
		t.Helper()
		lines := strings.Split(m.View(), "\n")
		if len(lines) > m.height {
			t.Fatalf("%s: view has %d lines, exceeds height %d", label, len(lines), m.height)
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w > m.width {
				t.Fatalf("%s: line %d width %d exceeds %d: %q", label, i, w, m.width, ln)
			}
		}
	}

	assertFits("initial", m)
	for i := 0; i < 100; i++ { // scroll to the bottom
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	}
	assertFits("scrolled", m)
	m = m.setSize(40, 12) // shrink
	assertFits("resized", m)

	// Preview mode (wide terminal, attachment selected) splits into two columns
	// that together must still fit the width and height.
	src := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(src, []byte("hello preview"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mp := detailModel{
		key:         "PET-1",
		ticket:      ticket.Ticket{Title: "t"},
		subtasks:    make([]subtask.Subtask, 3),
		attachments: []attachment.Attachment{{FileName: "note.txt", StoredPath: src}},
		events:      make([]activity.Event, 50),
		width:       120,
		height:      30,
	}
	mp.cursor = 3 // the attachment sits after the 3 subtasks
	mp = mp.clampScroll()
	if _, ok := mp.selAtt(); !ok {
		t.Fatal("attachment should be selected for preview mode")
	}
	assertFits("preview", mp)
}

// Moving the cursor keeps the selected row inside the viewport; manual scroll
// moves the window freely and clamps at the ends.
func TestDetailBodyScrollFollowsCursor(t *testing.T) {
	m := detailModel{
		key:      "PET-1",
		ticket:   ticket.Ticket{Title: "t"},
		subtasks: make([]subtask.Subtask, 30),
		width:    80,
		height:   15,
	}
	m = m.clampScroll()

	for i := 0; i < 20; i++ { // move the cursor well past the first window
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != 20 {
		t.Fatalf("cursor should be 20, got %d", m.cursor)
	}
	_, selLine := m.bodyView(m.contentWidth())
	if selLine < m.bodyScroll || selLine >= m.bodyScroll+m.bodyBudget() {
		t.Fatalf("selected line %d not visible in [%d,%d)", selLine, m.bodyScroll, m.bodyScroll+m.bodyBudget())
	}

	// Manual scroll up moves the window and is not snapped back to the cursor.
	before := m.bodyScroll
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.bodyScroll >= before {
		t.Fatalf("ctrl+u should scroll up, was %d now %d", before, m.bodyScroll)
	}

	// Scrolling down past the end clamps.
	for i := 0; i < 100; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	}
	body, _ := m.bodyView(m.contentWidth())
	if maxScroll := lineCount(body) - m.bodyBudget(); m.bodyScroll != maxScroll {
		t.Fatalf("scroll should clamp at %d, got %d", maxScroll, m.bodyScroll)
	}
}

// In graphics (Kitty) mode a non-image attachment still uses the bordered box.
// Its dimensions must match the box so a long text preview can't expand it past
// the screen (regression: the whole view shifted down).
func TestDetailTextPreviewFitsInGraphicsMode(t *testing.T) {
	preview.SetGraphicsImages(true)
	t.Cleanup(func() { preview.SetGraphicsImages(false) })

	txt := filepath.Join(t.TempDir(), "code.ts")
	body := strings.Repeat("import { Something } from './somewhere';\n", 300)
	if err := os.WriteFile(txt, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := detailModel{
		key:         "PET-1",
		ticket:      ticket.Ticket{Title: "t"},
		attachments: []attachment.Attachment{{FileName: "code.ts", StoredPath: txt}},
		width:       120,
		height:      30,
	}
	m = m.clampScroll()
	if _, ok := m.selAtt(); !ok {
		t.Fatal("text attachment should be selected")
	}

	lines := strings.Split(m.View(), "\n")
	if len(lines) > m.height {
		t.Fatalf("text preview overflowed: %d lines > height %d", len(lines), m.height)
	}
}
