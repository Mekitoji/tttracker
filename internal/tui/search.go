package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
	"tttracker/internal/ticket"
)

// searchModel is a global full-text search (across all projects). It queries the
// facade incrementally as the user types.
type searchModel struct {
	app     *app.App
	ctx     context.Context
	input   textinput.Model
	results []ticket.SearchHit
	cursor  int
	width   int
	height  int
}

func newSearchModel(a *app.App, ctx context.Context, w, h int) searchModel {
	ti := textinput.New()
	ti.Placeholder = "search title / description / labels…"
	ti.Focus()
	return searchModel{app: a, ctx: ctx, input: ti, width: w, height: h}
}

func (m searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, keys.Back):
			return m, func() tea.Msg { return backMsg{} }
		case key.Matches(km, keys.ResultsUp):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case key.Matches(km, keys.ResultsDown):
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
			return m, nil
		case key.Matches(km, keys.Open):
			if h, ok := m.selected(); ok {
				k := ticket.Key(h.ProjectKey, h.Ticket.Number)
				return m, func() tea.Msg { return openTicketMsg{key: k} }
			}
			return m, nil
		}
	}

	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev { // re-query only when the text changed
		m.results, _ = m.app.Tickets.Search(m.ctx, m.input.Value())
		m.cursor = 0
	}
	return m, cmd
}

func (m searchModel) selected() (ticket.SearchHit, bool) {
	if m.cursor < 0 || m.cursor >= len(m.results) {
		return ticket.SearchHit{}, false
	}
	return m.results[m.cursor], true
}

func (m searchModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Search"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	if strings.TrimSpace(m.input.Value()) != "" && len(m.results) == 0 {
		b.WriteString(helpStyle.Render("(no matches)"))
		b.WriteString("\n")
	}
	for i, h := range m.results {
		line := fmt.Sprintf("%-10s %s", ticket.Key(h.ProjectKey, h.Ticket.Number), h.Ticket.Title)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + line))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpLine(keys.ResultsUp, keys.Open, keys.Back))
	return b.String()
}

func (m searchModel) setSize(w, h int) searchModel {
	m.width, m.height = w, h
	return m
}
