package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/ticket"
)

type applyBoardFiltersMsg struct{ options ticket.ListOptions }

type boardFilterItem struct {
	group string
	value string
	label string
}

type boardFilterModel struct {
	items   []boardFilterItem
	cursor  int
	checked map[string]bool
	sort    ticket.SortMode
	height  int
}

func newBoardFilterModel(board boardModel) boardFilterModel {
	m := boardFilterModel{checked: make(map[string]bool), sort: board.filters.Sort, height: board.height}
	for _, value := range priorityValues {
		m.items = append(m.items, boardFilterItem{"priority", value, value})
	}
	for _, value := range typeValues {
		m.items = append(m.items, boardFilterItem{"type", value, value})
	}
	for _, value := range board.allLabels {
		m.items = append(m.items, boardFilterItem{"label", value, value})
	}
	m.items = append(m.items,
		boardFilterItem{"special", "current", "only my current"},
		boardFilterItem{"special", "unlabelled", "without labels"},
		boardFilterItem{"special", "stale", "not updated for 30 days"},
	)
	for _, value := range board.filters.Priorities {
		m.checked["priority:"+string(value)] = true
	}
	for _, value := range board.filters.Types {
		m.checked["type:"+string(value)] = true
	}
	for _, value := range board.filters.Labels {
		m.checked["label:"+value] = true
	}
	m.checked["special:current"] = board.filters.OnlyCurrent
	m.checked["special:unlabelled"] = board.filters.WithoutLabels
	m.checked["special:stale"] = board.filters.StaleBefore != nil
	return m
}

func (m boardFilterModel) Update(msg tea.Msg) (boardFilterModel, tea.Cmd) {
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
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case km.Type == tea.KeySpace:
		item := m.items[m.cursor]
		id := item.group + ":" + item.value
		m.checked[id] = !m.checked[id]
	case key.Matches(km, keys.Open):
		opts := ticket.ListOptions{Sort: m.sort}
		for _, item := range m.items {
			if !m.checked[item.group+":"+item.value] {
				continue
			}
			switch item.group {
			case "priority":
				opts.Priorities = append(opts.Priorities, ticket.Priority(item.value))
			case "type":
				opts.Types = append(opts.Types, ticket.Type(item.value))
			case "label":
				opts.Labels = append(opts.Labels, item.value)
			case "special":
				switch item.value {
				case "current":
					opts.OnlyCurrent = true
				case "unlabelled":
					opts.WithoutLabels = true
				case "stale":
					opts.StaleBefore = staleBefore(true)
				}
			}
		}
		return m, func() tea.Msg { return applyBoardFiltersMsg{opts} }
	case key.Matches(km, keys.Back), key.Matches(km, keys.Quit):
		return m, func() tea.Msg { return backMsg{} }
	}
	return m, nil
}

func (m boardFilterModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Board filters"))
	b.WriteString("\n\n")
	budget := max(m.height-8, 5)
	start := max(m.cursor-budget/2, 0)
	end := min(start+budget, len(m.items))
	start = max(end-budget, 0)
	group := ""
	for i := start; i < end; i++ {
		item := m.items[i]
		if item.group != group {
			if group != "" {
				b.WriteString("\n")
			}
			b.WriteString(helpStyle.Render(strings.ToUpper(item.group)))
			b.WriteString("\n")
			group = item.group
		}
		mark := "[ ]"
		if m.checked[item.group+":"+item.value] {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %s", mark, item.label)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpLine(keys.Up, key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")), keys.Open, keys.Back))
	return b.String()
}
