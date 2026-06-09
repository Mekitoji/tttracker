package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formModel is a single-field text prompt used for the create flows. The root
// model decides what a submitted value means based on the active screen.
type formModel struct {
	title  string
	input  textinput.Model
	errMsg string
	width  int
}

func newForm(title, placeholder string) formModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 200
	ti.Focus()
	return formModel{title: title, input: ti}
}

// prefilledForm is a form whose input starts with value (cursor at the end).
func prefilledForm(title, placeholder, value string) formModel {
	f := newForm(title, placeholder)
	f.input.SetValue(value)
	f.input.CursorEnd()
	return f
}

func (m formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.Type {
		case tea.KeyEnter:
			return m, func() tea.Msg { return submitFormMsg{value: strings.TrimSpace(m.input.Value())} }
		case tea.KeyEsc:
			return m, func() tea.Msg { return backMsg{} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m formModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	if m.errMsg != "" {
		b.WriteString("\n" + errorStyle.Render(m.errMsg) + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("enter submit • esc cancel"))
	return b.String()
}

func (m formModel) setSize(w, h int) formModel {
	m.width = w
	return m
}
