package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
)

type peMode int

const (
	peMenu peMode = iota
	peName
	peDesc
	peRepoPick
	peRepoManual
)

var peFields = []string{"Name", "Repo path", "Description"}

// projectEditModel edits a project's name, repo path, and description. The KEY
// is the project's stable identity and is not editable here. It is self-contained
// (holds the facade), opening a text input or filepicker per field.
type projectEditModel struct {
	app    *app.App
	ctx    context.Context
	finder repoFinder

	key         string
	name        string
	repoPath    string
	description string

	cursor int
	mode   peMode
	input  textinput.Model
	picker filepicker.Model

	// manual repo search state
	repos     []string
	results   []string
	resCursor int
	searching bool

	errMsg string

	width, height int
}

func newProjectEditModel(a *app.App, ctx context.Context, projectKey string, w, h int, finder repoFinder) (projectEditModel, error) {
	p, err := a.Projects.Get(ctx, projectKey)
	if err != nil {
		return projectEditModel{}, err
	}
	return projectEditModel{
		app: a, ctx: ctx, finder: finder, key: p.Key, name: p.Name, repoPath: p.RepoPath,
		description: p.Description, width: w, height: h,
	}, nil
}

func (m projectEditModel) reload() projectEditModel {
	if p, err := m.app.Projects.Get(m.ctx, m.key); err == nil {
		m.name, m.repoPath, m.description = p.Name, p.RepoPath, p.Description
	}
	return m
}

func (m projectEditModel) Update(msg tea.Msg) (projectEditModel, tea.Cmd) {
	switch m.mode {
	case peMenu:
		return m.updateMenu(msg)
	case peRepoPick:
		return m.updatePicker(msg)
	case peRepoManual:
		return m.updateManualRepo(msg)
	default: // peName, peDesc
		return m.updateInput(msg)
	}
}

func (m projectEditModel) updateMenu(msg tea.Msg) (projectEditModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(peFields)-1 {
			m.cursor++
		}
	case "enter":
		m.errMsg = ""
		switch m.cursor {
		case 0:
			m.input = newInput("Name", m.name)
			m.mode = peName
			return m, textinput.Blink
		case 1:
			return m.openPicker()
		case 2:
			m.input = newInput("Description", m.description)
			m.mode = peDesc
			return m, textinput.Blink
		}
	case "esc", "q":
		return m, func() tea.Msg { return backMsg{} }
	}
	return m, nil
}

func (m projectEditModel) openPicker() (projectEditModel, tea.Cmd) {
	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false
	fp.ShowHidden = false // hidden dirs off by default; toggle with "."
	start := m.repoPath
	if start == "" {
		if home, err := os.UserHomeDir(); err == nil {
			start = home
		}
	}
	fp.CurrentDirectory = start
	fp.AutoHeight = false
	h := m.height - 8
	switch {
	case h < 6:
		h = 6
	case h > 25:
		h = 25
	}
	fp.SetHeight(h)
	// Free esc for cancelling the modal (default Back also binds esc).
	fp.KeyMap.Back = key.NewBinding(key.WithKeys("h", "backspace", "left"))

	m.picker = fp
	m.mode = peRepoPick
	m.errMsg = ""
	return m, fp.Init()
}

func (m projectEditModel) updatePicker(msg tea.Msg) (projectEditModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.mode = peMenu
			return m, nil
		case ".":
			m.picker.ShowHidden = !m.picker.ShowHidden
			return m, m.picker.Init()
		case "i", "tab":
			return m.openManual()
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if ok, path := m.picker.DidSelectFile(msg); ok {
		return m.saveRepoPath(path), nil
	}
	return m, cmd
}

// openManual switches to search/manual-entry mode and kicks off an async scan
// for git repos, so a slow scan never freezes the UI.
func (m projectEditModel) openManual() (projectEditModel, tea.Cmd) {
	m.input = newInput("type to search repos, or enter a path", m.repoPath)
	m.repos = nil
	m.results = nil
	m.resCursor = -1
	m.searching = true
	m.mode = peRepoManual
	m.errMsg = ""

	finder := m.finder
	return m, func() tea.Msg {
		if finder == nil {
			return reposLoadedMsg{}
		}
		return reposLoadedMsg{repos: finder.allRepos()}
	}
}

// setRepos receives the async scan result and refreshes the filtered list.
func (m projectEditModel) setRepos(repos []string) projectEditModel {
	m.repos = repos
	m.searching = false
	m.results = filterRepos(repos, m.input.Value())
	m.resCursor = -1
	return m
}

func (m projectEditModel) updateManualRepo(msg tea.Msg) (projectEditModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.mode = peRepoPick
			m.errMsg = ""
			return m, nil
		case "down", "ctrl+j":
			if m.resCursor < len(m.results)-1 {
				m.resCursor++
			}
			return m, nil
		case "up", "ctrl+k":
			if m.resCursor > -1 {
				m.resCursor--
			}
			return m, nil
		case "enter":
			if m.resCursor >= 0 && m.resCursor < len(m.results) {
				return m.saveRepoPath(m.results[m.resCursor]), nil
			}
			return m.submitManual(), nil
		}
	}
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.results = filterRepos(m.repos, m.input.Value())
		m.resCursor = -1
	}
	return m, cmd
}

func (m projectEditModel) updateInput(msg tea.Msg) (projectEditModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter:
			return m.submitInput(), nil
		case tea.KeyEsc:
			m.mode = peMenu
			m.errMsg = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m projectEditModel) submitInput() projectEditModel {
	val := strings.TrimSpace(m.input.Value())
	var err error
	switch m.mode {
	case peName:
		_, err = m.app.Projects.SetName(m.ctx, m.key, val)
	case peDesc:
		_, err = m.app.Projects.SetDescription(m.ctx, m.key, val)
	}
	if err != nil {
		m.errMsg = err.Error()
		return m // stay in input mode so the user can fix it
	}
	m = m.reload()
	m.errMsg = ""
	m.mode = peMenu
	return m
}

// submitManual saves the literally typed path (when no search result is picked).
func (m projectEditModel) submitManual() projectEditModel {
	return m.saveRepoPath(strings.TrimSpace(m.input.Value()))
}

func (m projectEditModel) saveRepoPath(path string) projectEditModel {
	if _, err := m.app.Projects.SetRepoPath(m.ctx, m.key, path); err != nil {
		m.errMsg = err.Error()
		return m // stay so the user sees the error
	}
	m = m.reload()
	m.errMsg = ""
	m.mode = peMenu
	return m
}

func (m projectEditModel) View() string {
	switch m.mode {
	case peName:
		return m.inputView("Edit name")
	case peDesc:
		return m.inputView("Edit description")
	case peRepoManual:
		return m.manualView()
	case peRepoPick:
		return m.pickerView()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit project " + m.key))
	b.WriteString("\n\n")
	values := []string{m.name, displayOrDash(m.repoPath), displayOrDash(m.description)}
	for i, label := range peFields {
		line := fmt.Sprintf("%-12s %s", label+":", values[i])
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + line))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ • enter edit • esc back   (KEY is fixed)"))
	return b.String()
}

func (m projectEditModel) inputView(title string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter save • esc cancel"))
	return b.String()
}

func (m projectEditModel) manualView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Repo path — search or type"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	switch {
	case m.searching:
		b.WriteString(helpStyle.Render("  scanning for git repos…"))
		b.WriteString("\n")
	case len(m.results) == 0:
		b.WriteString(helpStyle.Render("  (no matching git repos)"))
		b.WriteString("\n")
	}
	for i, r := range m.results {
		if i == m.resCursor {
			b.WriteString(selectedStyle.Render("> " + r))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("type to filter • ↑/↓ or ⌃j/⌃k pick • enter select / use typed • esc back"))
	return b.String()
}

func (m projectEditModel) pickerView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select repo directory"))
	b.WriteString("\n")
	b.WriteString(fieldStyle.Render(m.picker.CurrentDirectory))
	b.WriteString("\n\n")
	b.WriteString(m.picker.View())
	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ move • → open • ← up • enter select dir • . hidden • i search • esc cancel"))
	return b.String()
}

func (m projectEditModel) setSize(w, h int) projectEditModel {
	m.width, m.height = w, h
	return m
}

func newInput(placeholder, value string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 500
	ti.SetValue(value)
	ti.CursorEnd()
	ti.Focus()
	return ti
}

func displayOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
