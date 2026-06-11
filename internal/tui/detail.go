package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"tttracker/internal/activity"
	"tttracker/internal/app"
	"tttracker/internal/comment"
	"tttracker/internal/subtask"
	"tttracker/internal/ticket"
)

// detailModel shows one ticket. A single cursor walks a combined list of
// subtasks (first) then comments, so up/down navigates both sections and the
// action keys apply to whichever item is highlighted.
type detailModel struct {
	key      string
	ticket   ticket.Ticket
	subtasks []subtask.Subtask
	comments []comment.Comment
	events   []activity.Event
	cursor   int // 0..len(subtasks)-1 = subtasks; then comments
	width    int
	height   int
}

func newDetailModel(a *app.App, ctx context.Context, key string, w, h int) (detailModel, error) {
	t, err := a.Tickets.Get(ctx, key)
	if err != nil {
		return detailModel{}, err
	}
	subs, err := a.Subtasks.List(ctx, key)
	if err != nil {
		return detailModel{}, err
	}
	coms, err := a.Comments.List(ctx, key)
	if err != nil {
		return detailModel{}, err
	}
	evs, err := a.Activity.List(ctx, t.ID)
	if err != nil {
		return detailModel{}, err
	}
	return detailModel{key: key, ticket: *t, subtasks: subs, comments: coms, events: evs, width: w, height: h}, nil
}

func (m detailModel) total() int { return len(m.subtasks) + len(m.comments) }

func (m detailModel) selSub() (subtask.Subtask, bool) {
	if m.cursor >= 0 && m.cursor < len(m.subtasks) {
		return m.subtasks[m.cursor], true
	}
	return subtask.Subtask{}, false
}

func (m detailModel) selCom() (comment.Comment, bool) {
	i := m.cursor - len(m.subtasks)
	if i >= 0 && i < len(m.comments) {
		return m.comments[i], true
	}
	return comment.Comment{}, false
}

func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
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
		if m.cursor < m.total()-1 {
			m.cursor++
		}
	case key.Matches(km, keys.BoardMoveStatus):
		return m, m.action(actionStatus, 0, string(m.ticket.Status))
	case key.Matches(km, keys.DetailSetPriority):
		return m, m.action(actionPriority, 0, string(m.ticket.Priority))
	case key.Matches(km, keys.DetailSetType):
		return m, m.action(actionType, 0, string(m.ticket.Type))
	case key.Matches(km, keys.DetailEditTitle):
		return m, m.action(actionTitle, 0, m.ticket.Title)
	case key.Matches(km, keys.DetailEditLabels):
		return m, m.action(actionLabels, 0, strings.Join(m.ticket.Labels, ", "))
	case key.Matches(km, keys.DetailEditDescription):
		return m, m.action(actionDescription, 0, "")
	case key.Matches(km, keys.DetailAddComment):
		return m, m.action(actionComment, 0, "")
	case key.Matches(km, keys.DetailAddSubtask):
		return m, m.action(actionSubtaskAdd, 0, "")
	case key.Matches(km, keys.DetailToggleSubtask):
		if st, ok := m.selSub(); ok {
			return m, m.action(actionSubtaskToggle, st.ID, "")
		}
	case key.Matches(km, keys.DetailRenameSubtask):
		if st, ok := m.selSub(); ok {
			return m, m.action(actionSubtaskRename, st.ID, st.Title)
		}
	case key.Matches(km, keys.DetailEditComment):
		if c, ok := m.selCom(); ok {
			return m, m.action(actionCommentEdit, c.ID, c.Body)
		}
	case key.Matches(km, keys.DetailDeleteItem):
		if st, ok := m.selSub(); ok {
			return m, m.action(actionSubtaskDelete, st.ID, "")
		}
		if c, ok := m.selCom(); ok {
			return m, m.action(actionCommentDelete, c.ID, "")
		}
	case key.Matches(km, keys.BoardDeleteTicket):
		k := m.key
		return m, func() tea.Msg { return askDeleteTicketMsg{key: k} }
	case key.Matches(km, keys.Back), key.Matches(km, keys.Quit):
		return m, func() tea.Msg { return backMsg{} }
	}
	return m, nil
}

func (m detailModel) action(kind actionKind, entityID int64, current string) tea.Cmd {
	key := m.key
	return func() tea.Msg {
		return startActionMsg{kind: kind, ticketKey: key, entityID: entityID, current: current, origin: screenDetail}
	}
}

func (m detailModel) View() string {
	t := m.ticket
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("%s  %s", m.key, t.Title)))
	b.WriteString("\n")
	meta := fmt.Sprintf("status:%s  priority:%s  type:%s", t.Status, t.Priority, t.Type)
	if len(t.Labels) > 0 {
		meta += "  labels:" + strings.Join(t.Labels, ",")
	}
	b.WriteString(fieldStyle.Render(meta))
	b.WriteString("\n\n")

	if desc := renderMarkdown(t.Description, m.width); strings.TrimSpace(desc) != "" {
		b.WriteString(desc)
	} else {
		b.WriteString(helpStyle.Render("(no description)"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Subtasks"))
	b.WriteString("\n")
	if len(m.subtasks) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for i, s := range m.subtasks {
		box := "[ ]"
		if s.IsDone {
			box = "[x]"
		}
		b.WriteString(m.row(i, fmt.Sprintf("%s %s", box, s.Title)))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Comments"))
	b.WriteString("\n")
	if len(m.comments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for j, c := range m.comments {
		b.WriteString(m.row(len(m.subtasks)+j, "• "+firstLine(c.Body)))
	}

	if len(m.events) > 0 {
		b.WriteString("\n")
		b.WriteString(columnTitleStyle.Render("Activity"))
		b.WriteString("\n")
		for _, e := range m.events {
			b.WriteString(fieldStyle.Render(fmt.Sprintf("  %s  %s",
				e.CreatedAt.Local().Format("2006-01-02 15:04"), e.Type)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpLine(keys.BoardMoveStatus, keys.DetailSetPriority, keys.DetailSetType, keys.DetailEditTitle,
		keys.DetailEditLabels, keys.DetailEditDescription, keys.DetailAddSubtask, keys.DetailAddComment))
	b.WriteString("\n")
	b.WriteString(helpLine(keys.Up, keys.DetailToggleSubtask, keys.DetailRenameSubtask, keys.DetailEditComment,
		keys.DetailDeleteItem, keys.BoardDeleteTicket, keys.Back))
	return b.String()
}

// row renders one selectable line, highlighted when index == cursor.
func (m detailModel) row(index int, text string) string {
	if index == m.cursor {
		return selectedStyle.Render("> "+text) + "\n"
	}
	return "  " + text + "\n"
}

func (m detailModel) setSize(w, h int) detailModel {
	m.width, m.height = w, h
	return m
}

func renderMarkdown(s string, width int) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	w := width - 4
	switch {
	case w < 20:
		w = 20
	case w > 120:
		w = 120
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(w))
	if err != nil {
		return s
	}
	out, err := r.Render(s)
	if err != nil {
		return s
	}
	return out
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}
