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
	"tttracker/internal/attachment"
	"tttracker/internal/comment"
	"tttracker/internal/subtask"
	"tttracker/internal/ticket"
)

type cursorSection int

const (
	cursorSubtask cursorSection = iota
	cursorComment
	cursorAttachment
)

// item is one selectable line. The detail's flat selection order is the
// concatenation of subtasks, then comments, then attachments — defined once in
// items(). The cursor is an index into that flat list.
type item struct {
	section cursorSection
	index   int
}

// detailModel shows one ticket. A flat cursor walks the combined list of
// subtasks, comments and attachments (see items()); action keys apply to
// whichever item is highlighted.
type detailModel struct {
	key         string
	ticket      ticket.Ticket
	subtasks    []subtask.Subtask
	comments    []comment.Comment
	attachments []attachment.Attachment
	events      []activity.Event
	cursor      int // index into items()
	width       int
	height      int
}

func loadDetail(a *app.App, ctx context.Context, key string, w, h int) (detailModel, error) {
	return detailModel{key: key, width: w, height: h}.reload(a, ctx)
}

// reload re-fetches the ticket and all its sections, refreshing the data in
// place while keeping the cursor (clamped to the new item count). Use this after
// a mutation instead of rebuilding, so the cursor position is preserved.
func (m detailModel) reload(a *app.App, ctx context.Context) (detailModel, error) {
	t, err := a.Tickets.Get(ctx, m.key)
	if err != nil {
		return m, err
	}
	subs, err := a.Subtasks.List(ctx, m.key)
	if err != nil {
		return m, err
	}
	coms, err := a.Comments.List(ctx, m.key)
	if err != nil {
		return m, err
	}
	atts, err := a.Attachments.List(ctx, m.key)
	if err != nil {
		return m, err
	}
	evs, err := a.Activity.List(ctx, t.ID)
	if err != nil {
		return m, err
	}
	m.ticket = *t
	m.subtasks, m.comments, m.attachments, m.events = subs, coms, atts, evs
	return m.clampCursor(), nil
}

// items is the flat, ordered list of selectable rows: subtasks, then comments,
// then attachments. This single definition is the source of truth for
// navigation, selection and clamping.
func (m detailModel) items() []item {
	items := make([]item, 0, len(m.subtasks)+len(m.comments)+len(m.attachments))
	for i := range m.subtasks {
		items = append(items, item{cursorSubtask, i})
	}
	for i := range m.comments {
		items = append(items, item{cursorComment, i})
	}
	for i := range m.attachments {
		items = append(items, item{cursorAttachment, i})
	}
	return items
}

// selected returns the item under the cursor, or ok=false when there are none.
func (m detailModel) selected() (item, bool) {
	items := m.items()
	if m.cursor >= 0 && m.cursor < len(items) {
		return items[m.cursor], true
	}
	return item{}, false
}

// clampCursor keeps the cursor within the current item range.
func (m detailModel) clampCursor() detailModel {
	if n := len(m.items()); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

func (m detailModel) selSub() (subtask.Subtask, bool) {
	if it, ok := m.selected(); ok && it.section == cursorSubtask {
		return m.subtasks[it.index], true
	}
	return subtask.Subtask{}, false
}

func (m detailModel) selCom() (comment.Comment, bool) {
	if it, ok := m.selected(); ok && it.section == cursorComment {
		return m.comments[it.index], true
	}
	return comment.Comment{}, false
}

func (m detailModel) selAtt() (attachment.Attachment, bool) {
	if it, ok := m.selected(); ok && it.section == cursorAttachment {
		return m.attachments[it.index], true
	}
	return attachment.Attachment{}, false
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
		if m.cursor < len(m.items())-1 {
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

	sel, hasSel := m.selected()
	isSel := func(section cursorSection, index int) bool {
		return hasSel && sel.section == section && sel.index == index
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
		b.WriteString(m.row(isSel(cursorSubtask, i), fmt.Sprintf("%s %s", box, s.Title)))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Comments"))
	b.WriteString("\n")
	if len(m.comments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for j, c := range m.comments {
		b.WriteString(m.row(isSel(cursorComment, j), "• "+firstLine(c.Body)))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Attachments"))
	b.WriteString("\n")
	if len(m.attachments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for k, a := range m.attachments {
		size := formatSize(a.SizeBytes)
		b.WriteString(m.row(isSel(cursorAttachment, k), a.FileName+" ("+size+")"))
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

// row renders one selectable line, highlighted when selected.
func (m detailModel) row(selected bool, text string) string {
	if selected {
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

func formatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(bytes)
	for i := 0; i < len(units)-1; i++ {
		if size < 1024 {
			if i == 0 {
				return fmt.Sprintf("%d%s", int64(size), units[i])
			}
			return fmt.Sprintf("%.1f%s", size, units[i])
		}
		size /= 1024
	}
	return fmt.Sprintf("%.1f%s", size, units[len(units)-1])
}
