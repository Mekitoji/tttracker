package tui

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tttracker/internal/app"
	"tttracker/internal/ticket"
)

// boardStatuses is the set (and order) of columns shown on the board.
// Blocked is toggleable (left), todo/in_progress/done are primary (middle),
// backlog/archived are toggleable inactive (right).
var boardStatuses = []ticket.Status{
	ticket.StatusBlocked, ticket.StatusTodo, ticket.StatusInProgress, ticket.StatusDone,
	ticket.StatusBacklog, ticket.StatusArchived,
}

var boardTitles = map[ticket.Status]string{
	ticket.StatusBlocked:    "Blocked",
	ticket.StatusTodo:       "Todo",
	ticket.StatusInProgress: "In Progress",
	ticket.StatusDone:       "Done",
	ticket.StatusBacklog:    "Backlog",
	ticket.StatusArchived:   "Archived",
}

type boardModel struct {
	projectKey   string
	projectName  string
	columns      [][]ticket.Ticket // indexed by boardStatuses order
	col, row     int
	colScroll    []int // first visible ticket row, indexed by boardStatuses order
	width        int
	height       int
	showBlocked  bool  // toggle visibility of blocked column
	showInactive bool  // toggle visibility of backlog/archived columns
	visibleCols  []int // indices of visible columns in boardStatuses
	filters      ticket.ListOptions
	allLabels    []string
}

func loadBoard(a *app.App, ctx context.Context, projectKey string, w, h int) (boardModel, error) {
	proj, err := a.Projects.Get(ctx, projectKey)
	if err != nil {
		return boardModel{}, err
	}
	tickets, err := a.Tickets.List(ctx, projectKey)
	if err != nil {
		return boardModel{}, err
	}
	bm := boardModel{projectKey: projectKey, projectName: proj.Name, columns: groupTickets(tickets), width: w, height: h}
	bm.filters.Sort = ticket.SortManual
	bm.allLabels = collectLabels(tickets)
	bm.colScroll = make([]int, len(boardStatuses))
	bm.updateVisibleCols()
	// Start at first primary column (todo, index 1 in boardStatuses)
	bm.col = bm.visibleCols[0]
	bm.clampRow()
	return bm, nil
}

// groupTickets buckets tickets into columns by status, in boardStatuses order.
func groupTickets(tickets []ticket.Ticket) [][]ticket.Ticket {
	cols := make([][]ticket.Ticket, len(boardStatuses))
	for _, t := range tickets {
		for i, st := range boardStatuses {
			if t.Status == st {
				cols[i] = append(cols[i], t)
				break
			}
		}
	}
	return cols
}

// reload re-fetches this project's tickets and regroups them into columns,
// refreshing the data in place while keeping all view state (toggles, cursor)
// untouched. The cursor is re-clamped to surviving tickets and parked on the
// first visible column only if its column became hidden. Use this after any
// mutation instead of rebuilding the model, so view state never gets dropped.
func (m boardModel) reload(a *app.App, ctx context.Context) (boardModel, error) {
	tickets, err := a.Tickets.ListWithOptions(ctx, m.projectKey, m.filters)
	if err != nil {
		return m, err
	}
	m.columns = groupTickets(tickets)
	all, err := a.Tickets.List(ctx, m.projectKey)
	if err != nil {
		return m, err
	}
	m.allLabels = collectLabels(all)
	if m.visibleColIndex(m.col) < 0 {
		m.col = m.visibleCols[0]
	}
	m.clampRow()
	m.followCursor()
	return m, nil
}

func collectLabels(tickets []ticket.Ticket) []string {
	set := make(map[string]bool)
	for _, t := range tickets {
		for _, label := range t.Labels {
			set[label] = true
		}
	}
	out := make([]string, 0, len(set))
	for label := range set {
		out = append(out, label)
	}
	slices.Sort(out)
	return out
}

// updateVisibleCols builds the list of visible column indices based on view mode
// showInactive=false: primary columns (blocked if enabled, todo, in_progress, done)
// showInactive=true: inactive columns (blocked if enabled, backlog, archived)
func (m *boardModel) updateVisibleCols() {
	m.visibleCols = nil
	for i, st := range boardStatuses {
		// Blocked is always available if showBlocked is true
		if st == ticket.StatusBlocked {
			if m.showBlocked {
				m.visibleCols = append(m.visibleCols, i)
			}
			continue
		}

		// Show either primary or inactive columns based on showInactive
		isPrimary := st == ticket.StatusTodo || st == ticket.StatusInProgress || st == ticket.StatusDone
		isInactive := st == ticket.StatusBacklog || st == ticket.StatusArchived

		if m.showInactive && isInactive {
			m.visibleCols = append(m.visibleCols, i)
		} else if !m.showInactive && isPrimary {
			m.visibleCols = append(m.visibleCols, i)
		}
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
			m.followCursor()
		}
	case key.Matches(km, keys.Right):
		visIdx := m.visibleColIndex(m.col)
		if visIdx >= 0 && visIdx < len(m.visibleCols)-1 {
			m.col = m.visibleCols[visIdx+1]
			m.clampRow()
			m.followCursor()
		}
	case key.Matches(km, keys.Up):
		if m.row > 0 {
			m.row--
			m.followCursor()
		}
	case key.Matches(km, keys.Down):
		if m.row < len(m.columns[m.col])-1 {
			m.row++
			m.followCursor()
		}
	case key.Matches(km, keys.Open):
		if t, ok := m.selected(); ok {
			k := ticket.Key(m.projectKey, t.Number)
			return m, func() tea.Msg { return openTicketMsg{key: k} }
		}
	case key.Matches(km, keys.BoardSearch):
		return m, func() tea.Msg { return openSearchMsg{} }
	case key.Matches(km, keys.BoardFilters):
		return m, func() tea.Msg { return openBoardFiltersMsg{} }
	case key.Matches(km, keys.BoardSort):
		switch m.filters.Sort {
		case ticket.SortManual:
			m.filters.Sort = ticket.SortPriority
		case ticket.SortPriority:
			m.filters.Sort = ticket.SortUpdated
		default:
			m.filters.Sort = ticket.SortManual
		}
		return m, func() tea.Msg { return reloadBoardMsg{} }
	case key.Matches(km, keys.BoardMoveUp):
		if m.filters.Sort == ticket.SortManual {
			if t, ok := m.selected(); ok {
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return moveManualMsg{key: k, delta: -1} }
			}
		}
	case key.Matches(km, keys.BoardMoveDown):
		if m.filters.Sort == ticket.SortManual {
			if t, ok := m.selected(); ok {
				k := ticket.Key(m.projectKey, t.Number)
				return m, func() tea.Msg { return moveManualMsg{key: k, delta: 1} }
			}
		}
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
		m.followCursor()
	case key.Matches(km, keys.BoardToggleInactive):
		m.showInactive = !m.showInactive
		m.updateVisibleCols()
		// clamp col to visible range
		visIdx := m.visibleColIndex(m.col)
		if visIdx < 0 || visIdx >= len(m.visibleCols) {
			m.col = m.visibleCols[0]
		}
		m.clampRow()
		m.followCursor()
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

// cardBudget is the number of ticket rows available inside a column after its
// title. It mirrors the height calculation in View.
func (m boardModel) cardBudget() int {
	return max(max(m.height-9, 6)-1, 1)
}

// followCursor adjusts the active column's viewport so its selected ticket is
// always visible. Each column keeps its own offset when focus moves sideways.
func (m *boardModel) followCursor() {
	if len(m.colScroll) != len(boardStatuses) {
		m.colScroll = make([]int, len(boardStatuses))
	}
	if m.col < 0 || m.col >= len(m.columns) {
		return
	}
	budget := m.cardBudget()
	scroll := clampScroll(m.colScroll[m.col], len(m.columns[m.col]), budget)
	if m.row < scroll {
		scroll = m.row
	} else if m.row >= scroll+budget {
		scroll = m.row - budget + 1
	}
	m.colScroll[m.col] = clampScroll(scroll, len(m.columns[m.col]), budget)
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
				m.followCursor()
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

// focusTicketVisible moves the cursor to the ticket with the given number when it
// is in a currently visible column, returning true. If the ticket is missing or
// sits in a hidden column, the cursor is parked on the first visible column.
func (m *boardModel) focusTicketVisible(number int) bool {
	if m.focusTicket(number) && m.visibleColIndex(m.col) >= 0 {
		m.clampRow()
		return true
	}
	m.col, m.row = m.visibleCols[0], 0
	m.followCursor()
	return false
}

func (m boardModel) View() string {
	header := titleStyle.Render(fmt.Sprintf("Board — %s", m.projectKey))
	meta := "sort:" + string(m.filters.Sort)
	if n := activeFilterCount(m.filters); n > 0 {
		meta += fmt.Sprintf("  filters:%d", n)
	}
	header += "  " + helpStyle.Render(meta)

	// Per-column total width budget; the column renders as innerW (content+padding)
	// plus 2 border cells, so keep a small margin to avoid horizontal overflow.
	innerW := max((m.width/len(m.visibleCols))-6, 14)
	// Text width inside a column. columnStyle has Padding(0,1), so the usable text
	// area is innerW-2; cards are truncated to this so they never wrap and grow.
	cardW := max(innerW-2, 6)
	// Fixed inner content height. Every column is pinned to exactly this height
	// below, so columns never change height between view switches no matter how
	// many cards a column holds.
	contentHeight := max(m.height-9, 6)
	cardRows := max(contentHeight-1, 1)

	cols := make([]string, len(m.visibleCols))
	for visIdx, colIdx := range m.visibleCols {
		st := boardStatuses[colIdx]
		var b strings.Builder
		tickets := m.columns[colIdx]
		start := 0
		if colIdx < len(m.colScroll) {
			start = clampScroll(m.colScroll[colIdx], len(tickets), cardRows)
		}
		end := min(start+cardRows, len(tickets))
		title := boardTitles[st]
		if len(tickets) > cardRows {
			title = fmt.Sprintf("%s  %d–%d/%d", title, start+1, end, len(tickets))
		}
		b.WriteString(columnTitleStyle.Render(ansi.Truncate(title, cardW, "…")))
		b.WriteString("\n")

		if len(tickets) == 0 {
			b.WriteString(helpStyle.Render("—"))
		} else {
			for j := start; j < end; j++ {
				t := tickets[j]
				selected := colIdx == m.col && j == m.row
				b.WriteString(renderCard(t, selected, cardW))
				b.WriteString("\n")
			}
		}

		style := columnStyle
		if colIdx == m.col {
			style = columnSelStyle
		}
		// Height pads short columns; MaxHeight clips tall ones. Together they pin
		// every column to contentHeight content rows + 2 border rows, so all
		// columns are always the same height.
		cols[visIdx] = style.Width(innerW).Height(contentHeight).MaxHeight(contentHeight + 2).Render(b.String())
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	help := helpLine(keys.Left, keys.Up, keys.Open, keys.BoardMoveTicketLeft, keys.BoardMoveStatus, keys.BoardMoveUp, keys.BoardDeleteTicket,
		keys.BoardFilters, keys.BoardSort, keys.BoardSearch, keys.BoardNewTicket, keys.BoardProjects, keys.BoardToggleBlocked, keys.BoardToggleInactive, keys.Quit)
	help = ansi.Truncate(help, m.width, "…")
	return header + "\n\n" + board + "\n\n" + help
}

func activeFilterCount(o ticket.ListOptions) int {
	n := len(o.Priorities) + len(o.Types) + len(o.Labels)
	if o.OnlyCurrent {
		n++
	}
	if o.WithoutLabels {
		n++
	}
	if o.StaleBefore != nil {
		n++
	}
	return n
}

func staleBefore(enabled bool) *time.Time {
	if !enabled {
		return nil
	}
	t := time.Now().AddDate(0, 0, -30)
	return &t
}

func (m boardModel) setSize(w, h int) boardModel {
	m.width, m.height = w, h
	m.followCursor()
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
