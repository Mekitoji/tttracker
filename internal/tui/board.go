package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tttracker/internal/app"
	"tttracker/internal/ticket"
)

// boardStatuses is the set (and order) of columns shown on the board; archived
// tickets are intentionally omitted.
var boardStatuses = []ticket.Status{
	ticket.StatusTodo, ticket.StatusInProgress, ticket.StatusBlocked, ticket.StatusDone,
}

var boardTitles = map[ticket.Status]string{
	ticket.StatusTodo:       "Todo",
	ticket.StatusInProgress: "In Progress",
	ticket.StatusBlocked:    "Blocked",
	ticket.StatusDone:       "Done",
}

type boardModel struct {
	projectKey  string
	projectName string
	columns     [][]ticket.Ticket // indexed by boardStatuses order
	col, row    int
	width       int
	height      int
}

func newBoardModel(a *app.App, ctx context.Context, projectKey string, w, h int) (boardModel, error) {
	proj, err := a.Projects.Get(ctx, projectKey)
	if err != nil {
		return boardModel{}, err
	}
	tickets, err := a.Tickets.List(ctx, projectKey)
	if err != nil {
		return boardModel{}, err
	}
	cols := make([][]ticket.Ticket, len(boardStatuses))
	for _, t := range tickets {
		for i, st := range boardStatuses {
			if t.Status == st {
				cols[i] = append(cols[i], t)
				break
			}
		}
	}
	return boardModel{projectKey: projectKey, projectName: proj.Name, columns: cols, width: w, height: h}, nil
}

func (m boardModel) Update(msg tea.Msg) (boardModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "left", "h":
			if m.col > 0 {
				m.col--
				m.clampRow()
			}
		case "right", "l":
			if m.col < len(boardStatuses)-1 {
				m.col++
				m.clampRow()
			}
		case "up", "k":
			if m.row > 0 {
				m.row--
			}
		case "down", "j":
			if m.row < len(m.columns[m.col])-1 {
				m.row++
			}
		case "enter":
			if t, ok := m.selected(); ok {
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return openTicketMsg{key: k} }
			}
		case "/":
			return m, func() tea.Msg { return openSearchMsg{} }
		case "n":
			return m, func() tea.Msg { return newTicketFormMsg{} }
		case "m":
			if t, ok := m.selected(); ok {
				k := ticket.Key(m.projectKey, t.Number)
				cur := string(t.Status)
				return m, func() tea.Msg {
					return startActionMsg{kind: actionStatus, ticketKey: k, current: cur, origin: screenBoard}
				}
			}
		case "p", "esc":
			return m, func() tea.Msg { return backMsg{} }
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *boardModel) clampRow() {
	if max := len(m.columns[m.col]) - 1; m.row > max {
		m.row = max
	}
	if m.row < 0 {
		m.row = 0
	}
}

func (m boardModel) selected() (ticket.Ticket, bool) {
	if m.col < 0 || m.col >= len(m.columns) {
		return ticket.Ticket{}, false
	}
	col := m.columns[m.col]
	if m.row < 0 || m.row >= len(col) {
		return ticket.Ticket{}, false
	}
	return col[m.row], true
}

func (m boardModel) View() string {
	header := titleStyle.Render(fmt.Sprintf("%s — %s", m.projectKey, m.projectName))

	innerW := m.width/len(boardStatuses) - 4
	if innerW < 16 {
		innerW = 16
	}

	cols := make([]string, len(boardStatuses))
	for i, st := range boardStatuses {
		var b strings.Builder
		b.WriteString(columnTitleStyle.Render(boardTitles[st]) + "\n\n")
		if len(m.columns[i]) == 0 {
			b.WriteString(helpStyle.Render("—"))
		}
		for j, t := range m.columns[i] {
			card := fmt.Sprintf("%s %s", ticket.Key(m.projectKey, t.Number), t.Title)
			if i == m.col && j == m.row {
				b.WriteString(selectedStyle.Render(truncate(card, innerW)) + "\n")
			} else {
				b.WriteString(truncate(card, innerW) + "\n")
			}
		}
		style := columnStyle
		if i == m.col {
			style = columnSelStyle
		}
		cols[i] = style.Width(innerW).Render(b.String())
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	help := helpStyle.Render("←/→ col • ↑/↓ ticket • enter open • m status • / search • n new • p projects • q")
	return header + "\n\n" + board + "\n\n" + help
}

func (m boardModel) setSize(w, h int) boardModel {
	m.width, m.height = w, h
	return m
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}
