// Package tui is the Bubble Tea terminal UI. It holds no business logic: every
// view reads and mutates state through the app facade. A root model routes
// between screens; sub-models emit transition/action messages that the root
// handles, opening modals (picker/form/$EDITOR) and reloading on completion.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
	"tttracker/internal/editor"
	"tttracker/internal/ticket"
)

type screen int

const (
	screenProjects screen = iota
	screenBoard
	screenDetail
	screenProjectForm
	screenTicketForm
	screenPicker
	screenActionForm
	screenSearch
	screenProjectEdit
	screenConfirm
)

type actionKind int

const (
	actionStatus actionKind = iota
	actionPriority
	actionType
	actionTitle
	actionLabels
	actionDescription
	actionComment
	actionCommentEdit
	actionCommentDelete
	actionSubtaskAdd
	actionSubtaskToggle
	actionSubtaskDelete
	actionSubtaskRename
)

// messages emitted by sub-models via commands.
type (
	openProjectMsg      struct{ key string }
	openTicketMsg       struct{ key string }
	backMsg             struct{}
	newProjectFormMsg   struct{}
	newTicketFormMsg    struct{}
	openSearchMsg       struct{}
	openProjectEditMsg  struct{ key string }
	createProjectMsg    struct{ key, name string }
	reposLoadedMsg      struct{ repos []string }
	askDeleteProjectMsg struct{ key string }
	deleteProjectMsg    struct{ key string }
	askDeleteTicketMsg  struct{ key string }
	deleteTicketMsg     struct{ key string }
	moveTicketMsg       struct{ key, newStatus string }
	submitFormMsg       struct{ value string }
	startActionMsg      struct {
		kind      actionKind
		ticketKey string
		entityID  int64  // subtask or comment id, depending on kind
		current   string // current value, for picker preselect / form prefill / editor seed
		origin    screen // view to reload + return to
	}
)

// pendingState remembers what an open modal will do on completion.
type pendingState struct {
	kind      actionKind
	ticketKey string
	entityID  int64
	origin    screen
}

type model struct {
	app    *app.App
	ctx    context.Context
	screen screen

	projects      projectsModel
	board         boardModel
	detail        detailModel
	form          formModel
	picker        pickerModel
	search        searchModel
	projectEdit   projectEditModel
	projectCreate projectCreateModel
	confirm       confirmModel
	confirmReturn screen
	finder        repoFinder

	pending pendingState

	width, height int
	status        string
}

// Run launches the terminal UI over the given application facade.
func Run(application *app.App, keysPath string) error {
	keys = LoadKeyMap(keysPath)
	ctx := context.Background()
	pm, err := newProjectsModel(application, ctx)
	if err != nil {
		return err
	}
	m := model{
		app:      application,
		ctx:      ctx,
		screen:   screenProjects,
		projects: pm,
		finder:   newExecRepoFinder(),
		width:    80,
		height:   24,
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.projects = m.projects.setSize(m.width, m.height)
		m.board = m.board.setSize(m.width, m.height)
		m.detail = m.detail.setSize(m.width, m.height)
		m.form = m.form.setSize(m.width, m.height)
		m.search = m.search.setSize(m.width, m.height)
		m.projectEdit = m.projectEdit.setSize(m.width, m.height)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case openProjectMsg:
		bm, err := loadBoard(m.app, m.ctx, msg.key, m.width, m.height)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.board, m.status, m.screen = bm, "", screenBoard
		return m, nil
	case openTicketMsg:
		dm, err := newDetailModel(m.app, m.ctx, msg.key, m.width, m.height)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.detail, m.status, m.screen = dm, "", screenDetail
		return m, nil
	case backMsg:
		switch m.screen {
		case screenDetail:
			// Reload the board so edits made in the detail view (title, status,
			// labels, …) are reflected when we return to it.
			if bm, err := m.board.reload(m.app, m.ctx); err == nil {
				m.board = bm
			}
			m.screen = screenBoard
		case screenBoard, screenProjectForm:
			m.screen = screenProjects
		case screenTicketForm:
			m.screen = screenBoard
		case screenSearch:
			m.screen = screenBoard
		case screenConfirm:
			m.screen = m.confirmReturn
		case screenProjectEdit:
			if pm, err := newProjectsModel(m.app, m.ctx); err == nil {
				m.projects = pm.setSize(m.width, m.height)
			}
			m.screen = screenProjects
		case screenPicker, screenActionForm:
			m.screen = m.pending.origin
			m.pending = pendingState{}
		}
		return m, nil
	case newProjectFormMsg:
		m.projectCreate = newProjectCreate()
		m.screen = screenProjectForm
		return m, textinput.Blink
	case createProjectMsg:
		return m.createProject(msg.key, msg.name)
	case newTicketFormMsg:
		m.form = newForm("New ticket", "Title")
		m.screen = screenTicketForm
		return m, textinput.Blink
	case openSearchMsg:
		m.search = newSearchModel(m.app, m.ctx, m.width, m.height)
		m.screen = screenSearch
		return m, textinput.Blink
	case openProjectEditMsg:
		pe, err := newProjectEditModel(m.app, m.ctx, msg.key, m.width, m.height, m.finder)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.projectEdit, m.status, m.screen = pe, "", screenProjectEdit
		return m, nil
	case reposLoadedMsg:
		if m.screen == screenProjectEdit {
			m.projectEdit = m.projectEdit.setRepos(msg.repos)
		}
		return m, nil
	case askDeleteProjectMsg:
		key := msg.key
		m.confirmReturn = m.screen
		m.confirm = newConfirmType(
			"Delete project "+key+" and ALL its tickets — this cannot be undone.",
			key,
			func() tea.Msg { return deleteProjectMsg{key: key} },
		)
		m.screen = screenConfirm
		return m, textinput.Blink
	case askDeleteTicketMsg:
		key := msg.key
		m.confirmReturn = m.screen
		m.confirm = newConfirmYesNo(
			"Delete ticket "+key+"? This cannot be undone.",
			func() tea.Msg { return deleteTicketMsg{key: key} },
		)
		m.screen = screenConfirm
		return m, nil
	case deleteProjectMsg:
		if err := m.app.Projects.Delete(m.ctx, msg.key); err != nil {
			m.status = err.Error()
		}
		pm, err := newProjectsModel(m.app, m.ctx)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.projects = pm.setSize(m.width, m.height)
		m.screen = screenProjects
		return m, nil
	case deleteTicketMsg:
		if err := m.app.Tickets.Delete(m.ctx, msg.key); err != nil {
			m.status = err.Error()
		}
		if bm, err := m.board.reload(m.app, m.ctx); err == nil {
			m.board = bm
		}
		m.screen = screenBoard
		return m, nil
	case moveTicketMsg:
		if _, err := m.app.Tickets.SetStatus(m.ctx, msg.key, msg.newStatus); err != nil {
			m.status = err.Error()
		}
		if bm, err := m.board.reload(m.app, m.ctx); err == nil {
			_, ticketNum, _ := ticket.ParseKey(msg.key)
			bm.focusTicketVisible(ticketNum)
			m.board = bm
		}
		m.screen = screenBoard
		return m, nil
	case submitFormMsg:
		if m.screen == screenActionForm {
			return m.applyPending(msg.value)
		}
		return m.handleSubmit(msg.value)
	case startActionMsg:
		return m.handleStartAction(msg)
	case pickedMsg:
		return m.applyPending(msg.value)
	case editor.EditedMsg:
		if msg.Err != nil {
			m.status = msg.Err.Error()
			m.screen = m.pending.origin
			m.pending = pendingState{}
			return m, nil
		}
		return m.applyPending(msg.Content)
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenProjects:
		m.projects, cmd = m.projects.Update(msg)
	case screenBoard:
		m.board, cmd = m.board.Update(msg)
	case screenDetail:
		m.detail, cmd = m.detail.Update(msg)
	case screenPicker:
		m.picker, cmd = m.picker.Update(msg)
	case screenSearch:
		m.search, cmd = m.search.Update(msg)
	case screenProjectEdit:
		m.projectEdit, cmd = m.projectEdit.Update(msg)
	case screenProjectForm:
		m.projectCreate, cmd = m.projectCreate.Update(msg)
	case screenConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	case screenTicketForm, screenActionForm:
		m.form, cmd = m.form.Update(msg)
	}
	return m, cmd
}

// createProject handles the two-field project create form.
func (m model) createProject(key, name string) (tea.Model, tea.Cmd) {
	if key == "" {
		m.projectCreate.errMsg = "key is required"
		return m, nil
	}
	if _, err := m.app.Projects.Create(m.ctx, key, name, ""); err != nil {
		m.projectCreate.errMsg = err.Error()
		return m, nil
	}
	pm, err := newProjectsModel(m.app, m.ctx)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.projects = pm.setSize(m.width, m.height)
	m.screen = screenProjects
	return m, nil
}

// handleSubmit applies the (single-field) ticket create form.
func (m model) handleSubmit(value string) (tea.Model, tea.Cmd) {
	if m.screen != screenTicketForm {
		return m, nil
	}
	if value == "" {
		m.form.errMsg = "title is required"
		return m, nil
	}
	if _, err := m.app.Tickets.Create(m.ctx, ticket.CreateParams{ProjectKey: m.board.projectKey, Title: value}); err != nil {
		m.form.errMsg = err.Error()
		return m, nil
	}
	bm, err := m.board.reload(m.app, m.ctx)
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.board = bm
	m.screen = screenBoard
	return m, nil
}

// handleStartAction opens the modal (or performs the immediate mutation) for an
// action triggered from the board or detail view.
func (m model) handleStartAction(a startActionMsg) (tea.Model, tea.Cmd) {
	m.pending = pendingState{kind: a.kind, ticketKey: a.ticketKey, entityID: a.entityID, origin: a.origin}
	switch a.kind {
	case actionStatus:
		m.picker = newPicker("Move to status", statusValues, a.current)
		m.screen = screenPicker
		return m, nil
	case actionPriority:
		m.picker = newPicker("Set priority", priorityValues, a.current)
		m.screen = screenPicker
		return m, nil
	case actionType:
		m.picker = newPicker("Set type", typeValues, a.current)
		m.screen = screenPicker
		return m, nil
	case actionTitle:
		m.form = prefilledForm("Rename ticket", "Title", a.current)
		m.screen = screenActionForm
		return m, textinput.Blink
	case actionLabels:
		m.form = prefilledForm("Labels (comma-separated)", "bug, ui", a.current)
		m.screen = screenActionForm
		return m, textinput.Blink
	case actionSubtaskAdd:
		m.form = newForm("New subtask", "Title")
		m.screen = screenActionForm
		return m, textinput.Blink
	case actionSubtaskRename:
		m.form = prefilledForm("Rename subtask", "Title", a.current)
		m.screen = screenActionForm
		return m, textinput.Blink
	case actionDescription:
		return m, editor.OpenInTUI(m.detail.ticket.Description)
	case actionComment:
		return m, editor.OpenInTUI("")
	case actionCommentEdit:
		return m, editor.OpenInTUI(a.current)
	case actionSubtaskToggle, actionSubtaskDelete, actionCommentDelete:
		return m.applyImmediate()
	}
	return m, nil
}

// applyImmediate runs actions that need no input.
func (m model) applyImmediate() (tea.Model, tea.Cmd) {
	var err error
	switch m.pending.kind {
	case actionSubtaskToggle:
		_, err = m.app.Subtasks.Toggle(m.ctx, m.pending.entityID)
	case actionSubtaskDelete:
		err = m.app.Subtasks.Delete(m.ctx, m.pending.entityID)
	case actionCommentDelete:
		err = m.app.Comments.Delete(m.ctx, m.pending.entityID)
	}
	return m.finishAction(err)
}

// applyPending applies the result of a picker/form/$EDITOR modal.
func (m model) applyPending(value string) (tea.Model, tea.Cmd) {
	k := m.pending.ticketKey
	var err error
	switch m.pending.kind {
	case actionStatus:
		_, err = m.app.Tickets.SetStatus(m.ctx, k, value)
	case actionPriority:
		_, err = m.app.Tickets.SetPriority(m.ctx, k, value)
	case actionType:
		_, err = m.app.Tickets.SetType(m.ctx, k, value)
	case actionTitle:
		if value == "" {
			m.form.errMsg = "title is required"
			return m, nil
		}
		_, err = m.app.Tickets.SetTitle(m.ctx, k, value)
	case actionLabels:
		_, err = m.app.Tickets.SetLabels(m.ctx, k, parseLabels(value))
	case actionDescription:
		_, err = m.app.Tickets.SetDescription(m.ctx, k, value)
	case actionComment:
		if strings.TrimSpace(value) == "" {
			return m.finishAction(nil) // empty editor buffer: cancel quietly
		}
		_, err = m.app.Comments.Add(m.ctx, k, value)
	case actionCommentEdit:
		if strings.TrimSpace(value) == "" {
			return m.finishAction(nil)
		}
		_, err = m.app.Comments.Edit(m.ctx, m.pending.entityID, value)
	case actionSubtaskAdd:
		if value == "" {
			m.form.errMsg = "title is required"
			return m, nil
		}
		_, err = m.app.Subtasks.Add(m.ctx, k, value)
	case actionSubtaskRename:
		if value == "" {
			m.form.errMsg = "title is required"
			return m, nil
		}
		_, err = m.app.Subtasks.Rename(m.ctx, m.pending.entityID, value)
	}
	return m.finishAction(err)
}

// finishAction reloads the origin view and returns to it (or shows an error).
func (m model) finishAction(err error) (tea.Model, tea.Cmd) {
	origin := m.pending.origin
	ticketKey := m.pending.ticketKey
	cursor := m.detail.cursor
	m.pending = pendingState{}

	if err != nil {
		m.status = err.Error()
		m.screen = origin
		return m, nil
	}
	m.status = ""

	switch origin {
	case screenBoard:
		if bm, e := m.board.reload(m.app, m.ctx); e != nil {
			m.status = e.Error()
		} else {
			if _, num, err := ticket.ParseKey(ticketKey); err == nil {
				bm.focusTicketVisible(num)
			}
			m.board = bm
		}
	case screenDetail:
		if dm, e := newDetailModel(m.app, m.ctx, ticketKey, m.width, m.height); e != nil {
			m.status = e.Error()
		} else {
			dm.cursor = clampCursor(cursor, len(dm.subtasks)+len(dm.comments))
			m.detail = dm
		}
	}
	m.screen = origin
	return m, nil
}

func clampCursor(c, n int) int {
	switch {
	case n == 0 || c < 0:
		return 0
	case c > n-1:
		return n - 1
	default:
		return c
	}
}

// parseLabels splits a comma-separated label string into a trimmed, non-empty list.
func parseLabels(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (m model) View() string {
	var body string
	switch m.screen {
	case screenProjects:
		body = m.projects.View()
	case screenBoard:
		body = m.board.View()
	case screenDetail:
		body = m.detail.View()
	case screenProjectForm:
		body = m.projectCreate.View()
	case screenTicketForm, screenActionForm:
		body = m.form.View()
	case screenPicker:
		body = m.picker.View()
	case screenSearch:
		body = m.search.View()
	case screenProjectEdit:
		body = m.projectEdit.View()
	case screenConfirm:
		body = m.confirm.View()
	}
	if m.status != "" {
		body += "\n" + errorStyle.Render(m.status)
	}
	return body
}
