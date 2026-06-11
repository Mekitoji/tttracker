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
// tickets are intentionally omitted. Blocked is first (can be toggled hidden).
var boardStatuses = []ticket.Status{
	ticket.StatusBlocked, ticket.StatusTodo, ticket.StatusInProgress, ticket.StatusDone,
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
	showBlocked bool  // toggle visibility of blocked column
	visibleCols []int // indices of visible columns in boardStatuses
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
	bm := boardModel{projectKey: projectKey, projectName: proj.Name, columns: cols, width: w, height: h, showBlocked: false, col: 0, row: 0}
	bm.updateVisibleCols()
	// Start at first primary column (todo, index 1 in boardStatuses)
	bm.col = bm.visibleCols[0]
	bm.clampRow()
	return bm, nil
}

// updateVisibleCols builds the list of visible column indices based on showBlocked
func (m *boardModel) updateVisibleCols() {
	m.visibleCols = nil
	for i, st := range boardStatuses {
		if st == ticket.StatusBlocked && !m.showBlocked {
			continue
		}
		m.visibleCols = append(m.visibleCols, i)
	}
}

func (m boardModel) Update(msg tea.Msg) (boardModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(km, keys.Left):
		visIdx := m.visibleColIndex(m.col)
		if visIdx > 0 {
			m.col = m.visibleCols[visIdx-1]
			m.clampRow()
		}
	case key.Matches(km, keys.Right):
		visIdx := m.visibleColIndex(m.col)
		if visIdx >= 0 && visIdx < len(m.visibleCols)-1 {
			m.col = m.visibleCols[visIdx+1]
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
			visIdx := m.visibleColIndex(m.col)
			if visIdx > 0 {
				newStatusIdx := m.visibleCols[visIdx-1]
				newStatus := string(boardStatuses[newStatusIdx])
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return moveTicketMsg{key: k, newStatus: newStatus} }
			}
		}
	case key.Matches(km, keys.BoardMoveTicketRight):
		if t, ok := m.selected(); ok {
			visIdx := m.visibleColIndex(m.col)
			if visIdx >= 0 && visIdx < len(m.visibleCols)-1 {
				newStatusIdx := m.visibleCols[visIdx+1]
				newStatus := string(boardStatuses[newStatusIdx])
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
	case key.Matches(km, keys.BoardToggleBlocked):
		m.showBlocked = !m.showBlocked
		m.updateVisibleCols()
		// clamp col to visible range
		visIdx := m.visibleColIndex(m.col)
		if visIdx < 0 || visIdx >= len(m.visibleCols) {
			m.col = m.visibleCols[0]
		}
		m.clampRow()
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

// visibleColIndex returns the index of col in visibleCols, or -1 if not visible
func (m boardModel) visibleColIndex(col int) int {
	for i, vc := range m.visibleCols {
		if vc == col {
			return i
		}
	}
	return -1
}

func (m boardModel) View() string {
	header := titleStyle.Render(fmt.Sprintf("%s — %s", m.projectKey, m.projectName))

	// Calculate width per visible column (border: 2 chars, padding: 2 chars)
	totalBorderPadding := 4
	innerW := max((m.width/len(m.visibleCols))-totalBorderPadding-2, 14)
	// Account for header and help lines
	contentHeight := max(m.height-7, 8)

	cols := make([]string, len(m.visibleCols))
	for visIdx, colIdx := range m.visibleCols {
		st := boardStatuses[colIdx]
		var b strings.Builder
		b.WriteString(columnTitleStyle.Render(boardTitles[st]))
		b.WriteString("\n")

		if len(m.columns[colIdx]) == 0 {
			b.WriteString(helpStyle.Render("—"))
		} else {
			for j, t := range m.columns[colIdx] {
				selected := colIdx == m.col && j == m.row
				b.WriteString(renderCard(t, selected, innerW))
				b.WriteString("\n")
			}
		}

		// Pad with empty lines to fill the height
		lines := len(strings.Split(strings.TrimRight(b.String(), "\n"), "\n"))
		for i := lines; i < contentHeight; i++ {
			b.WriteString("\n")
		}

		style := columnStyle
		if colIdx == m.col {
			style = columnSelStyle
		}
		cols[visIdx] = style.Width(innerW).Render(b.String())
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	help := helpLine(keys.Left, keys.Up, keys.Open, keys.BoardMoveTicketLeft, keys.BoardMoveStatus, keys.BoardDeleteTicket,
		keys.BoardSearch, keys.BoardNewTicket, keys.BoardProjects, keys.BoardToggleBlocked, keys.Quit)
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
