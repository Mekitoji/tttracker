package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// attachPickerModel is a file browser for picking a file to attach to a ticket.
// It wraps bubbles/filepicker with files selectable; selecting one emits an
// attachFileMsg that the root model turns into an Attachments.Attach call.
type attachPickerModel struct {
	ticketKey     string
	picker        filepicker.Model
	width, height int
}

func newAttachPicker(ticketKey string, w, h int) attachPickerModel {
	fp := filepicker.New()
	fp.DirAllowed = false // only files can be attached
	fp.FileAllowed = true
	fp.ShowHidden = false // hidden files off by default; toggle with "."
	if home, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = home
	}
	fp.AutoHeight = false
	fp.SetHeight(pickerHeight(h))
	// Free esc for cancelling the modal (default Back also binds esc).
	fp.KeyMap.Back = key.NewBinding(key.WithKeys("h", "backspace", "left"))

	return attachPickerModel{ticketKey: ticketKey, picker: fp, width: w, height: h}
}

func pickerHeight(h int) int {
	h -= 8
	switch {
	case h < 6:
		return 6
	case h > 25:
		return 25
	default:
		return h
	}
}

func (m attachPickerModel) Update(msg tea.Msg) (attachPickerModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, keys.Back):
			return m, func() tea.Msg { return backMsg{} }
		case key.Matches(km, keys.FormToggleHidden):
			m.picker.ShowHidden = !m.picker.ShowHidden
			return m, m.picker.Init()
		}
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if ok, path := m.picker.DidSelectFile(msg); ok {
		key := m.ticketKey
		return m, func() tea.Msg { return attachFileMsg{ticketKey: key, path: path} }
	}
	return m, cmd
}

func (m attachPickerModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Attach file to " + m.ticketKey))
	b.WriteString("\n")
	b.WriteString(fieldStyle.Render(m.picker.CurrentDirectory))
	b.WriteString("\n\n")
	b.WriteString(m.picker.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑/↓ move • → open • ← up • enter select file"))
	b.WriteString("\n")
	b.WriteString(helpLine(keys.FormToggleHidden, keys.Back))
	return b.String()
}

func (m attachPickerModel) setSize(w, h int) attachPickerModel {
	m.width, m.height = w, h
	m.picker.SetHeight(pickerHeight(h))
	return m
}
