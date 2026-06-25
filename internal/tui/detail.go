package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tttracker/internal/activity"
	"tttracker/internal/app"
	"tttracker/internal/attachment"
	"tttracker/internal/comment"
	"tttracker/internal/preview"
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
	case key.Matches(km, keys.DetailEditComment), key.Matches(km, keys.DetailOpenAttachment):
		// Both default to "enter" but are separate, independently-rebindable
		// actions, so each branch checks its own binding as well as the selection.
		if c, ok := m.selCom(); ok && key.Matches(km, keys.DetailEditComment) {
			return m, m.action(actionCommentEdit, c.ID, c.Body)
		}
		if a, ok := m.selAtt(); ok && key.Matches(km, keys.DetailOpenAttachment) {
			// Capture the ticket now, while this detail is current; reading it later
			// at message-handling time could attribute the result to another ticket
			// if the user navigated away in between.
			ticketKey, path := m.key, a.StoredPath
			return m, func() tea.Msg { return openAttachmentMsg{ticketKey: ticketKey, path: path} }
		}
	case key.Matches(km, keys.DetailAddAttachment):
		k := m.key
		return m, func() tea.Msg { return openAttachPickerMsg{ticketKey: k} }
	case key.Matches(km, keys.DetailDeleteItem):
		if st, ok := m.selSub(); ok {
			return m, m.action(actionSubtaskDelete, st.ID, "")
		}
		if c, ok := m.selCom(); ok {
			return m, m.action(actionCommentDelete, c.ID, "")
		}
		if a, ok := m.selAtt(); ok {
			id, name := a.ID, a.FileName
			return m, func() tea.Msg { return askDeleteAttachmentMsg{id: id, name: name} }
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
	var head strings.Builder
	head.WriteString(titleStyle.Render(fmt.Sprintf("%s  %s", m.key, t.Title)))
	head.WriteString("\n")
	meta := fmt.Sprintf("status:%s  priority:%s  type:%s", t.Status, t.Priority, t.Type)
	if len(t.Labels) > 0 {
		meta += "  labels:" + strings.Join(t.Labels, ",")
	}
	head.WriteString(fieldStyle.Render(meta))

	// When an attachment is selected and the terminal is wide enough, split into
	// the content on the left and a preview pane on the right.
	var middle string
	if att, ok := m.selAtt(); ok && m.previewWidth() > 0 {
		pane := m.previewPane(att, m.previewWidth())
		leftW := m.width - lipgloss.Width(pane)
		// Clamp the left column to the pane height so the whole view fits the
		// terminal without scrolling — otherwise a long activity log pushes the
		// view taller than the screen and the top-aligned pane appears to float up.
		left := lipgloss.NewStyle().Width(leftW).MaxHeight(lipgloss.Height(pane)).Render(m.bodyView(leftW))
		middle = lipgloss.JoinHorizontal(lipgloss.Top, left, pane)
	} else {
		middle = m.bodyView(m.width)
	}

	// The "enter" action depends on what's selected: open an attachment, else
	// edit a comment. Show the matching binding so the footer stays accurate.
	activate := keys.DetailEditComment
	if _, ok := m.selAtt(); ok {
		activate = keys.DetailOpenAttachment
	}
	help := helpLine(keys.BoardMoveStatus, keys.DetailSetPriority, keys.DetailSetType, keys.DetailEditTitle,
		keys.DetailEditLabels, keys.DetailEditDescription, keys.DetailAddSubtask, keys.DetailAddComment) + "\n" +
		helpLine(keys.Up, keys.DetailToggleSubtask, keys.DetailRenameSubtask, activate,
			keys.DetailAddAttachment, keys.DetailDeleteItem, keys.BoardDeleteTicket, keys.Back)

	return head.String() + "\n\n" + middle + "\n\n" + help
}

// bodyView renders the description, the selectable sections (subtasks, comments,
// attachments) and the activity log, sized to contentW columns.
func (m detailModel) bodyView(contentW int) string {
	var b strings.Builder

	if desc := renderMarkdown(m.ticket.Description, contentW); strings.TrimSpace(desc) != "" {
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
		b.WriteString(m.row(isSel(cursorSubtask, i), fmt.Sprintf("%s %s", box, s.Title), contentW))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Comments"))
	b.WriteString("\n")
	if len(m.comments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for j, c := range m.comments {
		b.WriteString(m.row(isSel(cursorComment, j), "• "+firstLine(c.Body), contentW))
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
		b.WriteString(m.row(isSel(cursorAttachment, k), a.FileName+" ("+size+")", contentW))
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
	return b.String()
}

// previewWidth returns the width of the right preview pane, or 0 when the
// terminal is too narrow to split. The pane takes about a third of the width,
// leaving the rest for the content.
func (m detailModel) previewWidth() int {
	if m.width < 100 {
		return 0 // too narrow to split
	}
	w := m.width / 3
	if w > 90 {
		w = 90
	}
	if w < 28 {
		return 0
	}
	return w
}

// previewPane renders the right-side preview as a full-height bordered box, sized
// to a total width of paneW columns. The box always spans the vertical slot
// between the header and the help line, so it reads as a panel (and gives text
// previews room) rather than a box that shrinks to the image.
func (m detailModel) previewPane(att attachment.Attachment, paneW int) string {
	boxW := paneW - 2          // columnStyle adds a 1-cell border on each side
	contentW := max(boxW-2, 8) // columnStyle's Padding(0,1) eats one cell per side
	boxH := max(m.height-8, 4) // fills the vertical slot; border adds 2 to total
	contentH := max(boxH-3, 2) // "Preview" + filename + blank line

	header := columnTitleStyle.Render("Preview") + "\n" +
		helpStyle.Render(ansi.Truncate(att.FileName, contentW, "…")) + "\n\n"
	content := preview.Render(att.StoredPath, contentW, contentH)

	return columnStyle.Width(boxW).Height(boxH).Render(header + content)
}

// row renders one selectable line, highlighted when selected, truncated to width.
func (m detailModel) row(selected bool, text string, width int) string {
	text = ansi.Truncate(text, max(width-2, 1), "…")
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
