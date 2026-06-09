package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// projectCreateModel is the two-field new-project form: a required KEY (strict,
// path/CLI-safe) and an optional free-form NAME (defaults to the key).
type projectCreateModel struct {
	keyInput  textinput.Model
	nameInput textinput.Model
	focus     int // 0 = key, 1 = name
	errMsg    string
}

func newProjectCreate() projectCreateModel {
	k := textinput.New()
	k.Placeholder = "KEY (A-Z then A-Z0-9)"
	k.CharLimit = 10
	k.Focus()

	n := textinput.New()
	n.Placeholder = "Name (optional, free-form)"
	n.CharLimit = 200

	return projectCreateModel{keyInput: k, nameInput: n}
}

func (m projectCreateModel) Update(msg tea.Msg) (projectCreateModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			return m, func() tea.Msg { return backMsg{} }
		case "tab", "down", "shift+tab", "up":
			m.focus = 1 - m.focus
			if m.focus == 0 {
				m.keyInput.Focus()
				m.nameInput.Blur()
			} else {
				m.nameInput.Focus()
				m.keyInput.Blur()
			}
			return m, textinput.Blink
		case "enter":
			key := strings.TrimSpace(m.keyInput.Value())
			name := strings.TrimSpace(m.nameInput.Value())
			return m, func() tea.Msg { return createProjectMsg{key: key, name: name} }
		}
	}

	var cmd tea.Cmd
	if m.focus == 0 {
		m.keyInput, cmd = m.keyInput.Update(msg)
	} else {
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m projectCreateModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New project"))
	b.WriteString("\n\n")
	b.WriteString(m.fieldLabel("Key", 0))
	b.WriteString("\n")
	b.WriteString(m.keyInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.fieldLabel("Name", 1))
	b.WriteString("\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("tab switch field • enter create • esc cancel"))
	return b.String()
}

func (m projectCreateModel) fieldLabel(label string, idx int) string {
	if m.focus == idx {
		return titleStyle.Render("› " + label)
	}
	return fieldStyle.Render("  " + label)
}
