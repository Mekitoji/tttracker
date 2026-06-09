package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// confirmModel guards a destructive action by requiring the user to type a token
// (the project key) exactly before it proceeds.
type confirmModel struct {
	title  string
	expect string
	input  textinput.Model
	errMsg string
}

func newConfirmDelete(projectKey string) confirmModel {
	ti := textinput.New()
	ti.Placeholder = projectKey
	ti.CharLimit = 20
	ti.Focus()
	return confirmModel{
		title:  "Delete project " + projectKey + " and ALL its tickets — this cannot be undone.",
		expect: projectKey,
		input:  ti,
	}
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter:
			if strings.TrimSpace(m.input.Value()) == m.expect {
				key := m.expect
				return m, func() tea.Msg { return deleteProjectMsg{key: key} }
			}
			m.errMsg = "type the key exactly to confirm"
			return m, nil
		case tea.KeyEsc:
			return m, func() tea.Msg { return backMsg{} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render(m.title))
	b.WriteString("\n\n")
	b.WriteString("Type the key to confirm:\n")
	b.WriteString(m.input.View())
	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter confirm • esc cancel"))
	return b.String()
}
