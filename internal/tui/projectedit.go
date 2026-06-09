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
	app *app.App
	ctx context.Context

	key         string
	name        string
	repoPath    string
	description string

	cursor int
	mode   peMode
	input  textinput.Model
	picker filepicker.Model
	errMsg string

	width, height int
}

func newProjectEditModel(a *app.App, ctx context.Context, projectKey string, w, h int) (projectEditModel, error) {
	p, err := a.Projects.Get(ctx, projectKey)
	if err != nil {
		return projectEditModel{}, err
	}
	return projectEditModel{
		app: a, ctx: ctx, key: p.Key, name: p.Name, repoPath: p.RepoPath,
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
	default: // peName, peDesc, peRepoManual
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
	fp.ShowHidden = true
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
		case "i", "tab":
			m.input = newInput("/path/to/repo", m.repoPath)
			m.mode = peRepoManual
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if ok, path := m.picker.DidSelectFile(msg); ok {
		return m.saveRepoPath(path), nil
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
	case peRepoManual:
		_, err = m.app.Projects.SetRepoPath(m.ctx, m.key, val)
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

func (m projectEditModel) saveRepoPath(path string) projectEditModel {
	if _, err := m.app.Projects.SetRepoPath(m.ctx, m.key, path); err != nil {
		m.errMsg = err.Error()
	} else {
		m = m.reload()
		m.errMsg = ""
	}
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
		return m.inputView("Repo path (manual)")
	case peRepoPick:
		return m.pickerView()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit project "+m.key) + "\n\n")
	values := []string{m.name, displayOrDash(m.repoPath), displayOrDash(m.description)}
	for i, label := range peFields {
		line := fmt.Sprintf("%-12s %s", label+":", values[i])
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	if m.errMsg != "" {
		b.WriteString("\n" + errorStyle.Render(m.errMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ • enter edit • esc back   (KEY is fixed)"))
	return b.String()
}

func (m projectEditModel) inputView(title string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	if m.errMsg != "" {
		b.WriteString("\n" + errorStyle.Render(m.errMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("enter save • esc cancel"))
	return b.String()
}

func (m projectEditModel) pickerView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select repo directory") + "\n")
	b.WriteString(fieldStyle.Render(m.picker.CurrentDirectory) + "\n\n")
	b.WriteString(m.picker.View() + "\n")
	if m.errMsg != "" {
		b.WriteString("\n" + errorStyle.Render(m.errMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ move • → open • ← up • enter select dir • i manual • esc cancel"))
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
