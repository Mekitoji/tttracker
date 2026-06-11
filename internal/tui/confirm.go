package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// confirmModel guards a destructive action. In type-to-confirm mode (expect set)
// the user must type a token (e.g. the project key); otherwise it is a simple
// y/n prompt. onYes is the command emitted once the action is confirmed.
type confirmModel struct {
	title  string
	expect string // non-empty => type-to-confirm; empty => y/n
	onYes  tea.Cmd
	input  textinput.Model
	errMsg string
}

// newConfirmType builds a type-the-token confirmation (for very destructive ops).
func newConfirmType(title, expect string, onYes tea.Cmd) confirmModel {
	ti := textinput.New()
	ti.Placeholder = expect
	ti.CharLimit = 20
	ti.Focus()
	return confirmModel{title: title, expect: expect, onYes: onYes, input: ti}
}

// newConfirmYesNo builds a simple y/n confirmation.
func newConfirmYesNo(title string, onYes tea.Cmd) confirmModel {
	return confirmModel{title: title, onYes: onYes}
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.expect == "" { // y/n mode
		switch {
		case key.Matches(km, keys.ConfirmYes):
			return m, m.onYes
		case key.Matches(km, keys.ConfirmNo), key.Matches(km, keys.Back), key.Matches(km, keys.Quit):
			return m, func() tea.Msg { return backMsg{} }
		}
		return m, nil
	}

	// type-to-confirm mode
	switch {
	case key.Matches(km, keys.Open):
		if strings.TrimSpace(m.input.Value()) == m.expect {
			return m, m.onYes
		}
		m.errMsg = "type the key exactly to confirm"
		return m, nil
	case key.Matches(km, keys.Back):
		return m, func() tea.Msg { return backMsg{} }
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m confirmModel) View() string {
	var b strings.Builder
	b.WriteString(errorStyle.Render(m.title))
	b.WriteString("\n\n")
	if m.expect == "" {
		b.WriteString(helpLine(keys.ConfirmYes, keys.ConfirmNo))
		return b.String()
	}
	b.WriteString("Type the key to confirm:\n")
	b.WriteString(m.input.View())
	b.WriteString("\n")
	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.errMsg))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpLine(keys.Open, keys.Back))
	return b.String()
}
