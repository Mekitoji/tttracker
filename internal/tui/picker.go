package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Allowed values for the enum pickers; these mirror the ticket package enums.
var (
	statusValues   = []string{"todo", "in_progress", "blocked", "done", "archived"}
	priorityValues = []string{"low", "medium", "high", "critical"}
	typeValues     = []string{"task", "bug", "idea", "chore"}
)

type pickedMsg struct{ value string }

// pickerModel is a single-choice list used to set an enum field.
type pickerModel struct {
	title   string
	options []string
	cursor  int
}

func newPicker(title string, options []string, current string) pickerModel {
	cursor := 0
	for i, o := range options {
		if o == current {
			cursor = i
			break
		}
	}
	return pickerModel{title: title, options: options, cursor: cursor}
}

func (m pickerModel) Update(msg tea.Msg) (pickerModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(km, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(km, keys.Down):
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case key.Matches(km, keys.Open):
		v := m.options[m.cursor]
		return m, func() tea.Msg { return pickedMsg{value: v} }
	case key.Matches(km, keys.Back), key.Matches(km, keys.Quit):
		return m, func() tea.Msg { return backMsg{} }
	}
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n\n")
	for i, o := range m.options {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + o))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(o)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpLine(keys.Up, keys.Open, keys.Back))
	return b.String()
}
