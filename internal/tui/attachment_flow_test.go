package tui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
	"tttracker/internal/attachment"
	"tttracker/internal/ticket"
)

// drain applies a message and keeps running resulting commands until the async
// attachment flow settles.
func drain(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	for msg != nil {
		updated, cmd := m.Update(msg)
		m = updated.(model)
		if cmd == nil {
			break
		}
		msg = cmd()
		if _, quit := msg.(tea.QuitMsg); quit {
			break
		}
	}
	return m
}

func TestDetailShowsAttachments(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	attached, err := a.Attachments.Attach(context.Background(), "PET-1", "/etc/hosts")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	m := openDetail(t, a)
	if len(m.detail.attachments) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(m.detail.attachments))
	}
	if m.detail.attachments[0].FileName != attached.FileName {
		t.Fatalf("want %s, got %s", attached.FileName, m.detail.attachments[0].FileName)
	}

	for range 2 {
		m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	selected, ok := m.detail.selAtt()
	if !ok || selected.ID != attached.ID {
		t.Fatalf("selected wrong attachment: %+v, ok=%v", selected, ok)
	}
}

// Pressing enter on a selected attachment opens it via the opener seam.
func TestDetailOpenAttachment(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	ctx := context.Background()
	att, err := a.Attachments.Attach(ctx, "PET-1", "/etc/hosts")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	var opened string
	orig := openAttachment
	openAttachment = func(path string) error { opened = path; return nil }
	t.Cleanup(func() { openAttachment = orig })

	m := openDetail(t, a)
	// No subtasks/comments, so the cursor sits on the single attachment.
	if _, ok := m.detail.selAtt(); !ok {
		t.Fatal("expected attachment selected")
	}
	m = drain(t, m, keyEnter) // enter -> openAttachmentMsg -> async open

	if opened != att.StoredPath {
		t.Fatalf("opened %q, want %q", opened, att.StoredPath)
	}
}

// On a wide terminal, selecting an image attachment renders the preview pane
// with a half-block thumbnail.
func TestDetailImagePreviewPane(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	ctx := context.Background()

	// Write a small PNG and attach it.
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}
	pngPath := filepath.Join(t.TempDir(), "pic.png")
	f, err := os.Create(pngPath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	_ = f.Close()
	if _, err := a.Attachments.Attach(ctx, "PET-1", pngPath); err != nil {
		t.Fatalf("attach: %v", err)
	}

	m := openDetail(t, a)
	m.detail = m.detail.setSize(200, 50) // wide enough to split
	if _, ok := m.detail.selAtt(); !ok {
		t.Fatal("expected attachment selected")
	}

	// The image preview renders off the event loop; before that it's a placeholder.
	if view := m.detail.View(); !strings.Contains(view, "rendering preview") {
		t.Fatal("expected a rendering placeholder before the async render")
	}
	dm, cmd := m.detail.withPreview()
	m.detail = dm
	if cmd == nil {
		t.Fatal("expected a preview render command for the selected image")
	}
	cmd() // run the off-loop render, populating the cache

	view := m.detail.View()
	if !strings.Contains(view, "Preview") {
		t.Fatal("preview pane title missing")
	}
	if !strings.Contains(view, "▀") {
		t.Fatal("expected half-block thumbnail in preview after render")
	}
}

// On a narrow terminal the preview pane is suppressed (no split).
func TestDetailNoPreviewWhenNarrow(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	if _, err := a.Attachments.Attach(context.Background(), "PET-1", "/etc/hosts"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	m := openDetail(t, a)
	m.detail = m.detail.setSize(80, 24) // below the split threshold

	if m.detail.previewWidth() != 0 {
		t.Fatalf("preview should be off at width 80, got %d", m.detail.previewWidth())
	}
	if strings.Contains(m.detail.View(), "Preview") {
		t.Fatal("preview pane should not render on a narrow terminal")
	}
}

// Pressing `a` in the detail view opens the attach file picker; selecting a file
// attaches it and returns to the detail view with the attachment present.
func TestDetailAddAttachmentFlow(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	m = send(t, m, keyRunes("a")) // open attach picker
	if m.screen != screenAttachPicker {
		t.Fatalf("want attach picker, got %v", m.screen)
	}
	if m.attachPicker.ticketKey != "PET-1" {
		t.Fatalf("picker bound to wrong ticket: %q", m.attachPicker.ticketKey)
	}

	// Simulate the file picker selecting a file.
	m = send(t, m, attachFileMsg{ticketKey: "PET-1", path: "/etc/hosts"})
	if m.screen != screenDetail {
		t.Fatalf("want detail after attach, got %v", m.screen)
	}
	if len(m.detail.attachments) != 1 {
		t.Fatalf("attachment not added: %d", len(m.detail.attachments))
	}
}

// Esc from the attach picker returns to the detail view without attaching.
func TestDetailAttachPickerCancel(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	m = send(t, m, keyRunes("a"))
	if m.screen != screenAttachPicker {
		t.Fatalf("want attach picker, got %v", m.screen)
	}
	m = send(t, m, keyEsc)
	if m.screen != screenDetail {
		t.Fatalf("want detail after cancel, got %v", m.screen)
	}
	if len(m.detail.attachments) != 0 {
		t.Fatalf("nothing should be attached on cancel, got %d", len(m.detail.attachments))
	}
}

// Deleting an attachment must keep the UI consistent even when the file removal
// fails after the metadata is already gone: the row is reloaded out of the list
// and the error is surfaced (otherwise a ghost row would report "not found" on a
// second delete).
func TestDeleteAttachmentReloadsEvenOnFileError(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	ctx := context.Background()

	src := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	att, err := a.Attachments.Attach(ctx, "PET-1", src)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	// Make the stored path a non-empty directory so os.Remove fails: this
	// simulates a partial delete where the DB row is removed but the file is not.
	if err := os.Remove(att.StoredPath); err != nil {
		t.Fatalf("remove stored: %v", err)
	}
	if err := os.MkdirAll(att.StoredPath, 0o755); err != nil {
		t.Fatalf("mkdir stored: %v", err)
	}
	if err := os.WriteFile(filepath.Join(att.StoredPath, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	m := openDetail(t, a)
	if len(m.detail.attachments) != 1 {
		t.Fatalf("want 1 attachment before delete, got %d", len(m.detail.attachments))
	}

	m = send(t, m, deleteAttachmentMsg{att.ID})
	if len(m.detail.attachments) != 0 {
		t.Fatalf("attachment should be gone after delete despite file error, got %d", len(m.detail.attachments))
	}
	// The metadata was removed; only the file cleanup failed, so this is a warning.
	if !strings.HasPrefix(m.bgStatus, "warning:") {
		t.Fatalf("file-cleanup failure should be a warning, got bgStatus %q", m.bgStatus)
	}
}

// A delete that fails before the commit (e.g. the attachment does not exist) is a
// real error, not a warning, and leaves the list unchanged.
func TestDeleteAttachmentNotFoundIsError(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	attachFile(t, a, "doc.txt", []byte("hello"))

	m := openDetail(t, a)
	if len(m.detail.attachments) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(m.detail.attachments))
	}

	m = send(t, m, deleteAttachmentMsg{id: 999999}) // no such row
	if m.bgStatus == "" || strings.HasPrefix(m.bgStatus, "warning:") {
		t.Fatalf("a not-found delete should be a plain error, got bgStatus %q", m.bgStatus)
	}
	if len(m.detail.attachments) != 1 {
		t.Fatalf("nothing should be deleted on a not-found delete, got %d", len(m.detail.attachments))
	}
}

// Opening an attachment is its own binding: rebinding detail_open_attachment must
// move only that action, leaving edit-comment ("enter") untouched.
func TestDetailOpenAttachmentRebindIndependent(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	ctx := context.Background()
	att, err := a.Attachments.Attach(ctx, "PET-1", "/etc/hosts")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	origKeys := keys
	keys.DetailOpenAttachment = bind([]string{"o"}, "o", "open") // move open off "enter"
	t.Cleanup(func() { keys = origKeys })

	var opened string
	origOpen := openAttachment
	openAttachment = func(path string) error { opened = path; return nil }
	t.Cleanup(func() { openAttachment = origOpen })

	m := openDetail(t, a)
	if _, ok := m.detail.selAtt(); !ok {
		t.Fatal("expected attachment selected")
	}

	m = drain(t, m, keyEnter) // "enter" no longer bound to open
	if opened != "" {
		t.Fatalf("enter should not open after rebind, opened %q", opened)
	}
	m = drain(t, m, keyRunes("o")) // the rebound key opens it
	if opened != att.StoredPath {
		t.Fatalf("o should open the attachment, opened %q", opened)
	}
}

// attachFile writes data to a temp file and attaches it to PET-1, returning the
// created attachment.
func attachFile(t *testing.T, a *app.App, name string, data []byte) *attachment.Attachment {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	att, err := a.Attachments.Attach(context.Background(), "PET-1", p)
	if err != nil {
		t.Fatalf("attach %s: %v", name, err)
	}
	return att
}

// Deleting an attachment asks for confirmation first; "y" removes both the row
// and the file.
func TestDeleteAttachmentConfirmYes(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	att := attachFile(t, a, "doc.txt", []byte("hello"))

	m := openDetail(t, a)
	if _, ok := m.detail.selAtt(); !ok {
		t.Fatal("expected the attachment selected")
	}

	m = send(t, m, keyRunes("d")) // ask
	if m.screen != screenConfirm {
		t.Fatalf("want confirm screen, got %v", m.screen)
	}
	m = drain(t, m, keyRunes("y")) // confirm -> async delete -> reload
	if m.screen != screenDetail {
		t.Fatalf("want detail after confirm, got %v", m.screen)
	}
	if len(m.detail.attachments) != 0 {
		t.Fatalf("attachment should be deleted, got %d", len(m.detail.attachments))
	}
	if _, err := os.Stat(att.StoredPath); !os.IsNotExist(err) {
		t.Fatalf("stored file should be removed, stat err = %v", err)
	}
}

// "n" at the confirmation keeps the attachment and its file.
func TestDeleteAttachmentConfirmCancel(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	att := attachFile(t, a, "doc.txt", []byte("hello"))

	m := openDetail(t, a)
	m = send(t, m, keyRunes("d"))
	if m.screen != screenConfirm {
		t.Fatalf("want confirm screen, got %v", m.screen)
	}
	m = send(t, m, keyRunes("n")) // cancel
	if m.screen != screenDetail {
		t.Fatalf("want detail after cancel, got %v", m.screen)
	}
	if len(m.detail.attachments) != 1 {
		t.Fatalf("attachment should remain, got %d", len(m.detail.attachments))
	}
	if _, err := os.Stat(att.StoredPath); err != nil {
		t.Fatalf("stored file should still exist: %v", err)
	}
}

// A reload failure after a successful delete is surfaced (not silently swallowed)
// and the model keeps the last-good data rather than crashing.
func TestDeleteAttachmentReloadFailureSurfaced(t *testing.T) {
	a, d := newTestAppDB(t)
	mustSeed(t, a)
	att := attachFile(t, a, "doc.txt", []byte("hello"))

	m := openDetail(t, a)

	// Drop a table that reload reads but Remove does not, so Remove succeeds while
	// the subsequent reload fails.
	if _, err := d.Exec("DROP TABLE subtasks"); err != nil {
		t.Fatalf("drop subtasks: %v", err)
	}

	m = send(t, m, deleteAttachmentMsg{att.ID})
	if m.bgStatus == "" {
		t.Fatal("expected the reload error to be surfaced in the background status")
	}
}

// An opener error is reported in the background status rather than failing silently.
func TestDetailOpenAttachmentError(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	attachFile(t, a, "doc.txt", []byte("hello"))

	orig := openAttachment
	openAttachment = func(string) error { return errors.New("no application found") }
	t.Cleanup(func() { openAttachment = orig })

	m := openDetail(t, a)
	m = drain(t, m, keyEnter) // enter -> openAttachmentMsg -> async open -> error
	if !strings.Contains(m.bgStatus, "no application found") {
		t.Fatalf("opener error not surfaced, bgStatus = %q", m.bgStatus)
	}
}

// Attaching a large file goes through the async copy path and is stored intact.
func TestAttachLargeFile(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)

	const size = 3 << 20 // 3 MiB
	big := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(big, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write big: %v", err)
	}

	m := openDetail(t, a)
	m = drain(t, m, attachFileMsg{ticketKey: "PET-1", path: big})
	if m.screen != screenDetail {
		t.Fatalf("want detail after attach, got %v", m.screen)
	}
	if len(m.detail.attachments) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(m.detail.attachments))
	}
	if got := m.detail.attachments[0].SizeBytes; got != size {
		t.Fatalf("attached size = %d, want %d", got, size)
	}
}

// Opening an attachment is read-only, so its completion must NOT reload the
// detail (a reload would pull in unrelated changes and waste a query).
func TestOpenAttachmentDoesNotReload(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	ctx := context.Background()
	attachFile(t, a, "doc.txt", []byte("hi"))

	orig := openAttachment
	openAttachment = func(string) error { return nil }
	t.Cleanup(func() { openAttachment = orig })

	m := openDetail(t, a)
	// Change data behind the model's back; only a reload would surface it.
	if _, err := a.Comments.Add(ctx, "PET-1", "added later"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	m = drain(t, m, keyEnter) // open the selected attachment
	if len(m.detail.comments) != 0 {
		t.Fatalf("open must not reload; the new comment should not appear, got %d", len(m.detail.comments))
	}
}

// A result for a ticket the user navigated away from must still surface its
// outcome on the background status (never lost), but must NOT reload the now-open
// different ticket.
func TestAttachmentResultForAwayTicketSurfacesButDoesNotReload(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a) // PET + PET-1
	ctx := context.Background()
	if _, err := a.Tickets.Create(ctx, ticket.CreateParams{ProjectKey: "PET", Title: "second"}); err != nil {
		t.Fatalf("create PET-2: %v", err)
	}

	m := openDetail(t, a)                       // PET-1
	m = send(t, m, openTicketMsg{key: "PET-2"}) // navigate to a different ticket
	if m.screen != screenDetail || m.detail.key != "PET-2" {
		t.Fatalf("want PET-2 detail, got screen=%v key=%q", m.screen, m.detail.key)
	}
	// Change PET-2 behind the model's back; a wrong reload would surface it.
	if _, err := a.Comments.Add(ctx, "PET-2", "behind"); err != nil {
		t.Fatalf("add comment: %v", err)
	}

	// A late FAILED delete for PET-1 arrives while PET-2 is on screen.
	m = send(t, m, attachmentOperationFinished{op: attachmentDelete, ticketKey: "PET-1", err: errors.New("boom")})

	if m.detail.key != "PET-2" {
		t.Fatalf("stale result must not switch the detail, got %q", m.detail.key)
	}
	if len(m.detail.comments) != 0 {
		t.Fatalf("the away ticket must not be reloaded, got %d comments", len(m.detail.comments))
	}
	if !strings.Contains(m.bgStatus, "boom") {
		t.Fatalf("background error must be surfaced, got bgStatus %q", m.bgStatus)
	}
}

// The transient progress message must resolve even after the user leaves detail:
// a finished background op clears it on success and replaces it with the error on
// failure, on whatever screen is showing.
func TestBackgroundResultResolvesProgressOffDetail(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)

	m := openDetail(t, a)  // PET-1
	m = send(t, m, keyEsc) // -> board, leaving an in-flight op behind
	if m.screen != screenBoard {
		t.Fatalf("want board, got %v", m.screen)
	}

	// Success clears the lingering "Attaching…".
	m.bgStatus, m.bgIsError = "Attaching foo…", false
	m = send(t, m, attachmentOperationFinished{op: attachmentAttach, ticketKey: "PET-1"})
	if m.bgStatus != "" {
		t.Fatalf("progress should clear on success, got %q", m.bgStatus)
	}

	// Failure replaces it instead of being lost.
	m.bgStatus, m.bgIsError = "Attaching foo…", false
	m = send(t, m, attachmentOperationFinished{op: attachmentAttach, ticketKey: "PET-1", err: errors.New("disk full")})
	if !strings.Contains(m.bgStatus, "disk full") {
		t.Fatalf("background error should surface on board, got %q", m.bgStatus)
	}
}

// A finished background op must never wipe a newer foreground status: the
// background result goes to its own line, leaving m.status (e.g. an error that
// appeared after the user navigated away) intact.
func TestBackgroundSuccessDoesNotWipeForegroundStatus(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)

	m := openDetail(t, a) // PET-1
	// Start an attach (sets bgStatus progress; leaves m.status alone), don't run it.
	updated, _ := m.Update(attachFileMsg{ticketKey: "PET-1", path: "/etc/hosts"})
	m = updated.(model)
	m = send(t, m, keyEsc) // navigate to the board

	m.status = "newer foreground error" // an unrelated error appears afterwards

	// The attach completes successfully.
	m = send(t, m, attachmentOperationFinished{op: attachmentAttach, ticketKey: "PET-1"})
	if m.status != "newer foreground error" {
		t.Fatalf("background success must not wipe a newer foreground status, got %q", m.status)
	}
	if m.bgStatus != "" {
		t.Fatalf("background success should clear its own progress, got %q", m.bgStatus)
	}
}

// Attachment mutations are serialized: a second attach/delete is refused while
// one is in flight, and the lock is released once it completes.
func TestAttachmentMutationsSerialized(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	att := attachFile(t, a, "doc.txt", []byte("hi"))
	m := openDetail(t, a)

	// Start an attach but do NOT run its command, leaving it "in flight".
	updated, cmd := m.Update(attachFileMsg{ticketKey: "PET-1", path: "/etc/hosts"})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("expected an async attach command")
	}
	if !m.attachmentBusy {
		t.Fatal("attach should take the single-flight lock")
	}

	// A second mutation is refused (no command started) while busy.
	updated, cmd2 := m.Update(deleteAttachmentMsg{id: att.ID})
	m = updated.(model)
	if cmd2 != nil {
		t.Fatal("a second mutation must not start while one is in flight")
	}
	if m.status == "" {
		t.Fatal("a refused mutation should explain why")
	}

	// An open is read-only and not gated, even while a mutation is in flight.
	if _, ok := m.detail.selAtt(); ok {
		_, openCmd := m.Update(openAttachmentMsg{path: att.StoredPath})
		if openCmd == nil {
			t.Fatal("open must not be blocked by an in-flight mutation")
		}
	}

	// Completing the in-flight attach releases the lock.
	m = drain(t, m, cmd())
	if m.attachmentBusy {
		t.Fatal("lock should be released after the mutation finishes")
	}
	if m.status == attachmentBusyStatus {
		t.Fatal("busy status should clear after the mutation finishes")
	}
}

func TestMutationSuccessDoesNotEraseOpenError(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	updated, _ := m.Update(attachFileMsg{ticketKey: "PET-1", path: "/etc/hosts"})
	m = updated.(model)
	m = send(t, m, attachmentOperationFinished{op: attachmentOpen, ticketKey: "PET-1", err: errors.New("open failed")})
	m = send(t, m, attachmentOperationFinished{op: attachmentAttach, ticketKey: "PET-1"})

	if !strings.Contains(m.bgStatus, "open failed") {
		t.Fatalf("mutation success erased the open error: %q", m.bgStatus)
	}
}

// The single-flight lock is released even when the result arrives after the user
// navigated away, so attachments never become permanently un-mutable.
func TestAttachmentLockReleasedAfterNavigatingAway(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	m := openDetail(t, a)

	updated, _ := m.Update(attachFileMsg{ticketKey: "PET-1", path: "/etc/hosts"})
	m = updated.(model)
	if !m.attachmentBusy {
		t.Fatal("attach should take the lock")
	}
	m = send(t, m, keyEsc) // navigate to the board while the op is in flight

	m = send(t, m, attachmentOperationFinished{op: attachmentAttach, ticketKey: "PET-1"})
	if m.attachmentBusy {
		t.Fatal("lock must be released regardless of the current screen")
	}
}

// The open message captures its ticket at emit time, so a result can't be
// misattributed if the user navigates away before it is handled.
func TestOpenAttachmentMsgCarriesTicketKey(t *testing.T) {
	a := newTestApp(t)
	mustSeed(t, a)
	attachFile(t, a, "doc.txt", []byte("hi"))

	m := openDetail(t, a)
	if _, ok := m.detail.selAtt(); !ok {
		t.Fatal("expected the attachment selected")
	}

	_, cmd := m.detail.Update(keyEnter) // enter on the attachment emits the open msg
	if cmd == nil {
		t.Fatal("expected an open command")
	}
	om, ok := cmd().(openAttachmentMsg)
	if !ok {
		t.Fatalf("want openAttachmentMsg, got %T", cmd())
	}
	if om.ticketKey != "PET-1" {
		t.Fatalf("ticket key must be captured at emit time, got %q", om.ticketKey)
	}
	if om.path == "" {
		t.Fatal("path missing from open message")
	}
}

// deleteStatus classifies a synchronous delete error: nil clears, an
// ErrFileCleanup is a non-fatal warning, anything else is a plain error.
func TestDeleteStatusClassifiesCleanupWarning(t *testing.T) {
	if got := deleteStatus(nil); got != "" {
		t.Fatalf("nil should clear status, got %q", got)
	}
	if got := deleteStatus(errors.New("boom")); got != "boom" {
		t.Fatalf("plain error should pass through, got %q", got)
	}
	cleanup := fmt.Errorf("%w: disk", attachment.ErrFileCleanup)
	if got := deleteStatus(cleanup); !strings.HasPrefix(got, "warning:") {
		t.Fatalf("file-cleanup error should be a warning, got %q", got)
	}
}
