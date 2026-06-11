package tui

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(km, keys.Left):
		if m.col > 0 {
			m.col--
			m.clampRow()
		}
	case key.Matches(km, keys.Right):
		if m.col < len(boardStatuses)-1 {
			m.col++
			m.clampRow()
		}
	case key.Matches(km, keys.Up):
		if m.row > 0 {
			m.row--
		}
	case key.Matches(km, keys.Down):
		if m.row < len(m.columns[m.col])-1 {
			m.row++
		}
	case key.Matches(km, keys.Open):
		if t, ok := m.selected(); ok {
			k := ticket.Key(m.projectKey, t.Number)
			return m, func() tea.Msg { return openTicketMsg{key: k} }
		}
	case key.Matches(km, keys.BoardSearch):
		return m, func() tea.Msg { return openSearchMsg{} }
	case key.Matches(km, keys.BoardNewTicket):
		return m, func() tea.Msg { return newTicketFormMsg{} }
	case key.Matches(km, keys.BoardMoveStatus):
		if t, ok := m.selected(); ok {
			k := ticket.Key(m.projectKey, t.Number)
			cur := string(t.Status)
			return m, func() tea.Msg {
				return startActionMsg{kind: actionStatus, ticketKey: k, current: cur, origin: screenBoard}
			}
		}
	case key.Matches(km, keys.BoardMoveTicketLeft):
		if t, ok := m.selected(); ok {
			if m.col > 0 {
				newStatus := string(boardStatuses[m.col-1])
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return moveTicketMsg{key: k, newStatus: newStatus} }
			}
		}
	case key.Matches(km, keys.BoardMoveTicketRight):
		if t, ok := m.selected(); ok {
			if m.col < len(boardStatuses)-1 {
				newStatus := string(boardStatuses[m.col+1])
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return moveTicketMsg{key: k, newStatus: newStatus} }
			}
		}
	case key.Matches(km, keys.BoardDeleteTicket):
		if t, ok := m.selected(); ok {
			k := ticket.Key(m.projectKey, t.Number)
			return m, func() tea.Msg { return askDeleteTicketMsg{key: k} }
		}
	case key.Matches(km, keys.BoardProjects), key.Matches(km, keys.Back):
		return m, func() tea.Msg { return backMsg{} }
	case key.Matches(km, keys.Quit):
		return m, tea.Quit
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

// focusTicket finds a ticket by number and moves the cursor to it.
func (m *boardModel) focusTicket(number int) bool {
	for col, tickets := range m.columns {
		for row, t := range tickets {
			if t.Number == number {
				m.col, m.row = col, row
				return true
			}
		}
	}
	return false
}

func (m boardModel) View() string {
	header := titleStyle.Render(fmt.Sprintf("%s — %s", m.projectKey, m.projectName))

	innerW := max(m.width/len(boardStatuses)-4, 16)

	cols := make([]string, len(boardStatuses))
	for i, st := range boardStatuses {
		var b strings.Builder
		b.WriteString(columnTitleStyle.Render(boardTitles[st]))
		b.WriteString("\n\n")
		if len(m.columns[i]) == 0 {
			b.WriteString(helpStyle.Render("—"))
		}
		for j, t := range m.columns[i] {
			selected := i == m.col && j == m.row
			b.WriteString(renderCard(t, selected, innerW))
			b.WriteString("\n")
		}
		style := columnStyle
		if i == m.col {
			style = columnSelStyle
		}
		cols[i] = style.Width(innerW).Render(b.String())
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	help := helpLine(keys.Left, keys.Up, keys.Open, keys.BoardMoveTicketLeft, keys.BoardMoveStatus, keys.BoardDeleteTicket,
		keys.BoardSearch, keys.BoardNewTicket, keys.BoardProjects, keys.Quit)
	return header + "\n\n" + board + "\n\n" + help
}

func (m boardModel) setSize(w, h int) boardModel {
	m.width, m.height = w, h
	return m
}

// renderCard renders one board card: "[type] title" followed by colored label
// chips. Truncation is ANSI-aware (ansi.Truncate) so styling escape codes are
// never cut mid-sequence — that was what broke the layout and hid the last
// label. The title yields width so the labels always fit. The selected card is
// one solid highlighted line (labels plain) to avoid style bleed.
func renderCard(t ticket.Ticket, selected bool, width int) string {
	head := fmt.Sprintf("[%s] %s", t.Type, t.Title)
	if selected {
		line := head
		if len(t.Labels) > 0 {
			line += "  " + strings.Join(t.Labels, " ")
		}
		return selectedStyle.Render(ansi.Truncate(line, width, "…"))
	}
	if len(t.Labels) == 0 {
		return ansi.Truncate(head, width, "…")
	}
	labels := renderLabels(t.Labels)
	budget := width - ansi.StringWidth(labels) - 1
	if budget < 6 { // labels dominate; truncate the whole composed line
		return ansi.Truncate(head+" "+labels, width, "…")
	}
	return ansi.Truncate(head, budget, "…") + " " + labels
}

func renderLabels(labels []string) string {
	var b strings.Builder
	for i, l := range labels {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(labelChip(l))
	}
	return b.String()
}

// labelPalette is a set of distinguishable background colors; each label gets one
// deterministically by hashing its text, so a given label keeps a stable color.
var labelPalette = []lipgloss.Color{
	lipgloss.Color("203"), lipgloss.Color("215"), lipgloss.Color("179"),
	lipgloss.Color("114"), lipgloss.Color("116"), lipgloss.Color("141"),
	lipgloss.Color("211"), lipgloss.Color("180"),
}

func labelChip(label string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(label))
	bg := labelPalette[h.Sum32()%uint32(len(labelPalette))]
	return labelStyle.Background(bg).Foreground(lipgloss.Color("232")).Render(label)
}
