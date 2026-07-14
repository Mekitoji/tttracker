package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tttracker/internal/ticket"
)

// TestRenderCardFitsWidth guards the bug that broke the board layout: a card's
// *display* width (ignoring ANSI escape codes from colored labels) must never
// exceed the column width, or the column border shifts.
func TestRenderCardFitsWidth(t *testing.T) {
	tk := ticket.Ticket{
		Number: 12,
		Title:  "Add pull request url to ticket",
		Type:   ticket.TypeTask,
		Labels: []string{"backend", "urgent", "v2"},
	}
	for _, w := range []int{16, 24, 40, 80} {
		if got := ansi.StringWidth(renderCard(tk, false, w)); got > w {
			t.Fatalf("width %d: card too wide (%d cells)", w, got)
		}
		if got := ansi.StringWidth(renderCard(tk, true, w)); got > w {
			t.Fatalf("width %d (selected): card too wide (%d cells)", w, got)
		}
	}
}

func TestBoardColumnViewportFollowsCursor(t *testing.T) {
	tickets := make([]ticket.Ticket, 30)
	for i := range tickets {
		tickets[i] = ticket.Ticket{Number: i + 1, Title: fmt.Sprintf("ticket-%02d", i+1), Type: ticket.TypeTask, Status: ticket.StatusTodo}
	}
	m := boardModel{
		columns:     groupTickets(tickets),
		col:         1,
		visibleCols: []int{1, 2, 3},
		colScroll:   make([]int, len(boardStatuses)),
		width:       120,
		height:      15,
	}

	for i := 0; i < 20; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.row != 20 {
		t.Fatalf("cursor should be 20, got %d", m.row)
	}
	if start := m.colScroll[m.col]; start != 16 {
		t.Fatalf("viewport should start at 16, got %d", start)
	}
	view := m.View()
	if !strings.Contains(view, "ticket-21") {
		t.Fatal("selected ticket is not visible")
	}
	if strings.Contains(view, "ticket-01") {
		t.Fatal("viewport still renders the first ticket")
	}
	if lines := strings.Split(view, "\n"); len(lines) > m.height {
		t.Fatalf("view has %d lines, exceeds height %d", len(lines), m.height)
	}

	// Moving upward past the top of the window scrolls it back as well.
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if start := m.colScroll[m.col]; start != 15 {
		t.Fatalf("viewport should follow upward to 15, got %d", start)
	}
}
