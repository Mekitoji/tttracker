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
	bodyScroll  int // top line of the body viewport (see View)
	width       int
	height      int
	// previewPending is the cache key of the image preview currently being
	// rendered off the UI thread, so the same render is not dispatched twice.
	previewPending string
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
	return m.clampCursor().clampScroll(), nil
}

// bodyBudget is the number of rows the body viewport may occupy: the terminal
// height minus the fixed chrome (2 header + 2 separators + 2 help).
func (m detailModel) bodyBudget() int { return max(m.height-6, 1) }

// contentWidth is the width available to the body: the full width, or the left
// column when the preview pane is shown.
func (m detailModel) contentWidth() int {
	if _, ok := m.selAtt(); ok {
		if pw := m.previewWidth(); pw > 0 {
			return m.width - pw
		}
	}
	return m.width
}

// clampScroll keeps the body scroll offset within [0, maxScroll] for the current
// content and budget.
func (m detailModel) clampScroll() detailModel {
	body, _ := m.bodyView(m.contentWidth())
	m.bodyScroll = clampScroll(m.bodyScroll, lineCount(body), m.bodyBudget())
	return m
}

// followCursor scrolls the body so the selected row stays visible.
func (m detailModel) followCursor() detailModel {
	body, selLine := m.bodyView(m.contentWidth())
	m.bodyScroll = scrollToLine(m.bodyScroll, lineCount(body), m.bodyBudget(), selLine)
	return m
}

// scrollBody moves the viewport by delta rows (manual scroll), bounds-clamped.
func (m detailModel) scrollBody(delta int) detailModel {
	body, _ := m.bodyView(m.contentWidth())
	m.bodyScroll = clampScroll(m.bodyScroll+delta, lineCount(body), m.bodyBudget())
	return m
}

// lineCount counts rendered lines the way View does: trailing newlines are not
// a final empty line.
func lineCount(s string) int {
	if s = strings.TrimRight(s, "\n"); s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// clampScroll bounds a scroll offset to [0, max(total-budget, 0)].
func clampScroll(scroll, total, budget int) int {
	if hi := max(total-budget, 0); scroll > hi {
		scroll = hi
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

// scrollToLine nudges scroll (already bounds-clamped) so line is within the window.
func scrollToLine(scroll, total, budget, line int) int {
	scroll = clampScroll(scroll, total, budget)
	switch {
	case line < 0:
		return scroll
	case line < scroll:
		return line
	case line >= scroll+budget:
		return line - budget + 1
	}
	return scroll
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
		return m.followCursor(), nil
	case key.Matches(km, keys.Down):
		if m.cursor < len(m.items())-1 {
			m.cursor++
		}
		return m.followCursor(), nil
	case key.Matches(km, keys.DetailScrollUp):
		return m.scrollBody(-m.bodyBudget() / 2), nil
	case key.Matches(km, keys.DetailScrollDown):
		return m.scrollBody(m.bodyBudget() / 2), nil
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

	// Header: exactly two single-line rows, truncated so a long title/label set
	// can never wrap and push the layout past the screen.
	titleText := fmt.Sprintf("%s  %s", m.key, t.Title)
	metaText := fmt.Sprintf("status:%s  priority:%s  type:%s", t.Status, t.Priority, t.Type)
	if len(t.Labels) > 0 {
		metaText += "  labels:" + strings.Join(t.Labels, ",")
	}
	head := titleStyle.Render(ansi.Truncate(titleText, m.width, "…")) + "\n" +
		fieldStyle.Render(ansi.Truncate(metaText, m.width, "…"))

	// Body: render fully, then show a window of bodyBudget rows at bodyScroll so
	// nothing ever exceeds the screen; the rest is reachable by scrolling.
	budget := m.bodyBudget()
	contentW := m.contentWidth()
	body, _ := m.bodyView(contentW)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	scroll := clampScroll(m.bodyScroll, len(lines), budget)
	end := min(scroll+budget, len(lines))
	window := strings.Join(lines[scroll:end], "\n")

	var middle string
	if att, ok := m.selAtt(); ok && m.previewWidth() > 0 {
		pane := m.previewPane(att, m.previewWidth())
		left := lipgloss.NewStyle().Width(contentW).Height(budget).Render(window)
		middle = lipgloss.JoinHorizontal(lipgloss.Top, left, pane)
	} else {
		middle = lipgloss.NewStyle().Height(budget).Render(window)
	}

	// The separator line doubles as a scroll indicator when the body overflows.
	sep := ""
	if len(lines) > budget {
		sep = helpStyle.Render(ansi.Truncate(
			fmt.Sprintf("  lines %d–%d of %d  (⌃u/⌃d to scroll)", scroll+1, end, len(lines)), m.width, "…"))
	}

	// The "enter" action depends on what's selected: open an attachment, else
	// edit a comment. Show the matching binding so the footer stays accurate.
	activate := keys.DetailEditComment
	if _, ok := m.selAtt(); ok {
		activate = keys.DetailOpenAttachment
	}
	help := ansi.Truncate(helpLine(keys.BoardMoveStatus, keys.DetailSetPriority, keys.DetailSetType, keys.DetailEditTitle,
		keys.DetailEditLabels, keys.DetailEditDescription, keys.DetailAddSubtask, keys.DetailAddComment), m.width, "…") + "\n" +
		ansi.Truncate(helpLine(keys.Up, keys.DetailScrollUp, keys.DetailToggleSubtask, keys.DetailRenameSubtask, activate,
			keys.DetailAddAttachment, keys.DetailDeleteItem, keys.BoardDeleteTicket, keys.Back), m.width, "…")

	return head + "\n\n" + middle + "\n" + sep + "\n" + help
}

// bodyView renders the description, the selectable sections (subtasks, comments,
// attachments) and the full activity log, sized to contentW columns. It also
// returns the line index of the selected row (or -1) so the viewport can keep it
// in view. The result is windowed by View — it may be taller than the screen.
func (m detailModel) bodyView(contentW int) (string, int) {
	var b strings.Builder
	selLine := -1

	if desc := renderMarkdown(m.ticket.Description, contentW); strings.TrimSpace(desc) != "" {
		b.WriteString(desc)
	} else {
		b.WriteString(helpStyle.Render("(no description)"))
		b.WriteString("\n")
	}

	sel, hasSel := m.selected()
	// writeRow appends a selectable row, recording its line index when selected.
	writeRow := func(section cursorSection, index int, text string) {
		selected := hasSel && sel.section == section && sel.index == index
		if selected {
			selLine = strings.Count(b.String(), "\n")
		}
		b.WriteString(m.row(selected, text, contentW))
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
		writeRow(cursorSubtask, i, fmt.Sprintf("%s %s", box, s.Title))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Comments"))
	b.WriteString("\n")
	if len(m.comments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for j, c := range m.comments {
		writeRow(cursorComment, j, "• "+firstLine(c.Body))
	}

	b.WriteString("\n")
	b.WriteString(columnTitleStyle.Render("Attachments"))
	b.WriteString("\n")
	if len(m.attachments) == 0 {
		b.WriteString(helpStyle.Render("  (none)"))
		b.WriteString("\n")
	}
	for k, a := range m.attachments {
		writeRow(cursorAttachment, k, a.FileName+" ("+formatSize(a.SizeBytes)+")")
	}

	if len(m.events) > 0 {
		b.WriteString("\n")
		b.WriteString(columnTitleStyle.Render("Activity"))
		b.WriteString("\n")
		for _, e := range m.events {
			line := fmt.Sprintf("  %s  %s", e.CreatedAt.Local().Format("2006-01-02 15:04"), e.Type)
			b.WriteString(fieldStyle.Render(ansi.Truncate(line, max(contentW, 1), "…")))
			b.WriteString("\n")
		}
	}
	return b.String(), selLine
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
	isImage := preview.DetectKind(att.StoredPath) == preview.KindImage
	cols, rows := m.previewImageDims()

	// Native graphics (Kitty placeholders): compose directly, no border box —
	// lipgloss's border/padding could corrupt the one-shot image transmission, and
	// the placeholder cells already form a clean grid the layout can align.
	if preview.GraphicsImages() && isImage {
		header := columnTitleStyle.Render("Preview") + "\n" +
			helpStyle.Render(ansi.Truncate(att.FileName, paneW, "…")) + "\n\n"
		// TrimRight the trailing newline so the pane is exactly budget lines and
		// can't push the view one row past the screen.
		return header + strings.TrimRight(m.previewContent(att.StoredPath, cols, rows, isImage), "\n")
	}

	// Text / markdown / half-block image: bordered box, total height = budget.
	boxH := max(m.bodyBudget()-2, 4) // border adds 2 => total = budget
	boxW := paneW - 2                // columnStyle adds a 1-cell border on each side
	header := columnTitleStyle.Render("Preview") + "\n" +
		helpStyle.Render(ansi.Truncate(att.FileName, max(boxW-2, 8), "…")) + "\n\n"
	content := m.previewContent(att.StoredPath, cols, rows, isImage)

	return columnStyle.Width(boxW).Height(boxH).Render(header + content)
}

// previewImageDims is the (cols, rows) the preview content is rendered at — the
// single source of truth so the async render and the View lookup use the same
// cache key.
func (m detailModel) previewImageDims() (int, int) {
	paneW := m.previewWidth()
	if preview.GraphicsImages() {
		return paneW, max(m.bodyBudget()-3, 1) // header is 3 lines
	}
	boxH := max(m.bodyBudget()-2, 4)
	return max(paneW-4, 8), max(boxH-3, 2) // -2 border, -2 padding; -3 header
}

// previewContent returns the preview body. Images are looked up from the cache
// (never rendered inline — that would block the event loop on a large decode);
// on a miss it shows a placeholder and relies on withPreview to render off-loop.
// Text and markdown are cheap, so they render synchronously.
func (m detailModel) previewContent(path string, cols, rows int, isImage bool) string {
	if !isImage {
		return preview.Render(path, cols, rows)
	}
	if s, ok := preview.Cached(path, cols, rows); ok {
		return s
	}
	return helpStyle.Render("  rendering preview…")
}

// withPreview dispatches an off-loop render for the selected image when it is not
// yet cached, so navigating onto a large attachment never blocks the UI. It marks
// the render pending to avoid dispatching it twice.
func (m detailModel) withPreview() (detailModel, tea.Cmd) {
	att, ok := m.selAtt()
	if !ok || m.previewWidth() == 0 || preview.DetectKind(att.StoredPath) != preview.KindImage {
		return m, nil
	}
	cols, rows := m.previewImageDims()
	key := fmt.Sprintf("%s|%dx%d", att.StoredPath, cols, rows)
	if key == m.previewPending {
		return m, nil // already rendering this one
	}
	if _, cached := preview.Cached(att.StoredPath, cols, rows); cached {
		return m, nil // already ready
	}
	m.previewPending = key
	path := att.StoredPath
	return m, func() tea.Msg {
		preview.Render(path, cols, rows) // slow decode/encode, off the event loop
		return previewReadyMsg{key: key}
	}
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
	return m.clampScroll()
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
