package tui

import (
	"testing"

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
