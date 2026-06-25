package tui

import (
	"errors"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/attachment"
	"tttracker/internal/opener"
)

// openAttachment launches an attachment's file in the OS default app. It is a
// package-level seam so tests can stub the launch instead of opening real apps.
var openAttachment = opener.Open

const attachmentBusyStatus = "an attachment operation is already in progress"

// attachmentOp identifies which async attachment operation a result came from.
type attachmentOp int

const (
	attachmentAttach attachmentOp = iota
	attachmentDelete
	attachmentOpen
)

type (
	openAttachmentMsg      struct{ ticketKey, path string }
	openAttachPickerMsg    struct{ ticketKey string }
	attachFileMsg          struct{ ticketKey, path string }
	askDeleteAttachmentMsg struct {
		id   int64
		name string
	}
	deleteAttachmentMsg         struct{ id int64 }
	attachmentOperationFinished struct {
		op        attachmentOp
		ticketKey string
		err       error
	}
)

// attachmentFlowState is embedded in the root model because attachment
// operations may finish after the user navigates to another screen.
type attachmentFlowState struct {
	attachmentBusy bool
	bgStatus       string
	bgIsError      bool
}

// handleAttachmentMsg owns attachment transitions and async commands, keeping
// the root Update focused on routing messages between feature flows.
func (m model) handleAttachmentMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case openAttachmentMsg:
		path, ticketKey := msg.path, msg.ticketKey
		return m, func() tea.Msg {
			return attachmentOperationFinished{op: attachmentOpen, ticketKey: ticketKey, err: openAttachment(path)}
		}
	case openAttachPickerMsg:
		m.attachPicker = newAttachPicker(msg.ticketKey, m.width, m.height)
		m.screen = screenAttachPicker
		return m, m.attachPicker.picker.Init()
	case attachFileMsg:
		if m.attachmentBusy {
			return m.attachmentMutationBusy()
		}
		app, ctx := m.app, m.ctx
		key, path := msg.ticketKey, msg.path
		m = m.startAttachmentMutation("Attaching " + filepath.Base(path) + "…")
		return m, func() tea.Msg {
			_, err := app.Attachments.Attach(ctx, key, path)
			return attachmentOperationFinished{op: attachmentAttach, ticketKey: key, err: err}
		}
	case askDeleteAttachmentMsg:
		id, name := msg.id, msg.name
		m.confirmReturn = m.screen
		m.confirm = newConfirmYesNo(
			"Delete attachment "+name+"? This removes the file and cannot be undone.",
			func() tea.Msg { return deleteAttachmentMsg{id: id} },
		)
		m.screen = screenConfirm
		return m, nil
	case deleteAttachmentMsg:
		if m.attachmentBusy {
			return m.attachmentMutationBusy()
		}
		app, ctx, id := m.app, m.ctx, msg.id
		ticketKey := m.detail.key
		m = m.startAttachmentMutation("Deleting attachment…")
		return m, func() tea.Msg {
			return attachmentOperationFinished{op: attachmentDelete, ticketKey: ticketKey, err: app.Attachments.Remove(ctx, id)}
		}
	case attachmentOperationFinished:
		return m.finishAttachmentOp(msg)
	default:
		return m, nil
	}
}

func (m model) startAttachmentMutation(status string) model {
	m.attachmentBusy = true
	m.screen = screenDetail
	m.bgStatus, m.bgIsError = status, false
	return m
}

func (m model) attachmentMutationBusy() (tea.Model, tea.Cmd) {
	m.screen = screenDetail
	m.status = attachmentBusyStatus
	return m, nil
}

// finishAttachmentOp records the result and reloads only the detail targeted by
// a mutation. Read-only open operations never acquire the single-flight lock.
func (m model) finishAttachmentOp(res attachmentOperationFinished) (tea.Model, tea.Cmd) {
	if res.op != attachmentOpen {
		m.attachmentBusy = false
		if m.status == attachmentBusyStatus {
			m.status = ""
		}
	}

	switch {
	case errors.Is(res.err, attachment.ErrFileCleanup):
		m.bgStatus, m.bgIsError = "warning: "+res.err.Error(), true
	case res.err != nil:
		m.bgStatus, m.bgIsError = res.err.Error(), true
	case res.op != attachmentOpen && !m.bgIsError:
		m.bgStatus, m.bgIsError = "", false
	}

	if res.op != attachmentOpen && m.screen == screenDetail && m.detail.key == res.ticketKey {
		if dm, err := m.detail.reload(m.app, m.ctx); err != nil {
			m.bgStatus, m.bgIsError = err.Error(), true
		} else {
			m.detail = dm
		}
	}
	return m, nil
}

func (m model) attachmentStatusView() string {
	if m.bgStatus == "" {
		return ""
	}
	style := helpStyle
	if m.bgIsError {
		style = errorStyle
	}
	return "\n" + style.Render(m.bgStatus)
}

// deleteStatus distinguishes a successful delete with a cleanup warning from a
// failure that prevented deletion.
func deleteStatus(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, attachment.ErrFileCleanup):
		return "warning: " + err.Error()
	default:
		return err.Error()
	}
}
