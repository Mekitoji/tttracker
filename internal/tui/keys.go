package tui

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
)

// KeyMap holds the configurable key bindings for the board and detail views.
// Defaults come from DefaultKeyMap; a JSON file (action name -> list of keys)
// loaded by LoadKeyMap can override any of them. Views match via key.Matches and
// build their help text from the same bindings, so help always reflects config.
type KeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Open  key.Binding
	Back  key.Binding
	Quit  key.Binding

	// board
	BoardNewTicket       key.Binding
	BoardMoveStatus      key.Binding
	BoardMoveTicketLeft  key.Binding
	BoardMoveTicketRight key.Binding
	BoardDeleteTicket    key.Binding
	BoardSearch          key.Binding
	BoardProjects        key.Binding
	BoardToggleBlocked   key.Binding
	BoardToggleInactive  key.Binding

	// detail
	DetailSetPriority     key.Binding
	DetailSetType         key.Binding
	DetailEditTitle       key.Binding
	DetailEditLabels      key.Binding
	DetailEditDescription key.Binding
	DetailAddComment      key.Binding
	DetailAddSubtask      key.Binding
	DetailToggleSubtask   key.Binding
	DetailRenameSubtask   key.Binding
	DetailEditComment     key.Binding
	DetailOpenAttachment  key.Binding
	DetailAddAttachment   key.Binding
	DetailDeleteItem      key.Binding

	// shared text-input result lists (search, repo finder)
	ResultsUp   key.Binding
	ResultsDown key.Binding

	// projects
	ProjectsNew    key.Binding
	ProjectsEdit   key.Binding
	ProjectsDelete key.Binding

	// confirm
	ConfirmYes key.Binding
	ConfirmNo  key.Binding

	// forms / project edit
	FormNextField    key.Binding
	FormPrevField    key.Binding
	FormToggleHidden key.Binding
	FormManualEntry  key.Binding
}

// keys is the active key map. It is the default until Run loads overrides.
var keys = DefaultKeyMap()

func bind(keys []string, helpKey, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, desc))
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:    bind([]string{"up", "k"}, "↑/↓", "move"),
		Down:  bind([]string{"down", "j"}, "", ""),
		Left:  bind([]string{"left", "h"}, "←/→", "col"),
		Right: bind([]string{"right", "l"}, "", ""),
		Open:  bind([]string{"enter"}, "enter", "open"),
		Back:  bind([]string{"esc"}, "esc", "back"),
		Quit:  bind([]string{"q"}, "q", "quit"),

		BoardNewTicket:       bind([]string{"n"}, "n", "new"),
		BoardMoveStatus:      bind([]string{"m"}, "m", "status"),
		BoardMoveTicketLeft:  bind([]string{"ctrl+h"}, "⌃h", "←"),
		BoardMoveTicketRight: bind([]string{"ctrl+l"}, "⌃l", "→"),
		BoardDeleteTicket:    bind([]string{"x"}, "x", "del"),
		BoardSearch:          bind([]string{"/"}, "/", "search"),
		BoardProjects:        bind([]string{"p"}, "p", "projects"),
		BoardToggleBlocked:   bind([]string{"!"}, "!", "blocked"),
		BoardToggleInactive:  bind([]string{"@"}, "@", "inactive"),

		DetailSetPriority:     bind([]string{"p"}, "p", "priority"),
		DetailSetType:         bind([]string{"t"}, "t", "type"),
		DetailEditTitle:       bind([]string{"r"}, "r", "rename"),
		DetailEditLabels:      bind([]string{"l"}, "l", "labels"),
		DetailEditDescription: bind([]string{"e"}, "e", "desc"),
		DetailAddComment:      bind([]string{"c"}, "c", "comment"),
		DetailAddSubtask:      bind([]string{"s"}, "s", "subtask"),
		DetailToggleSubtask:   bind([]string{" "}, "␣", "toggle"),
		DetailRenameSubtask:   bind([]string{"R"}, "R", "rename-sub"),
		DetailEditComment:     bind([]string{"enter"}, "enter", "edit-comment"),
		DetailOpenAttachment:  bind([]string{"enter"}, "enter", "open"),
		DetailAddAttachment:   bind([]string{"a"}, "a", "attach"),
		DetailDeleteItem:      bind([]string{"d"}, "d", "delete"),

		ResultsUp:   bind([]string{"up", "ctrl+k"}, "↑/↓ ⌃k/⌃j", "select"),
		ResultsDown: bind([]string{"down", "ctrl+j"}, "", ""),

		ProjectsNew:    bind([]string{"n"}, "n", "new"),
		ProjectsEdit:   bind([]string{"e"}, "e", "edit"),
		ProjectsDelete: bind([]string{"x"}, "x", "delete"),

		ConfirmYes: bind([]string{"y", "Y"}, "y", "confirm"),
		ConfirmNo:  bind([]string{"n", "N"}, "n", "cancel"),

		FormNextField:    bind([]string{"tab", "down"}, "tab", "next"),
		FormPrevField:    bind([]string{"shift+tab", "up"}, "shift+tab", "prev"),
		FormToggleHidden: bind([]string{"."}, ".", "hidden"),
		FormManualEntry:  bind([]string{"i", "tab"}, "i", "manual"),
	}
}

type bindingRef struct {
	name string
	bind *key.Binding
}

// refs maps the JSON config action names to the corresponding bindings.
func (k *KeyMap) refs() []bindingRef {
	return []bindingRef{
		{"up", &k.Up},
		{"down", &k.Down},
		{"left", &k.Left},
		{"right", &k.Right},
		{"open", &k.Open},
		{"back", &k.Back},
		{"quit", &k.Quit},
		{"board_new_ticket", &k.BoardNewTicket},
		{"board_move_status", &k.BoardMoveStatus},
		{"board_move_ticket_left", &k.BoardMoveTicketLeft},
		{"board_move_ticket_right", &k.BoardMoveTicketRight},
		{"board_delete_ticket", &k.BoardDeleteTicket},
		{"board_search", &k.BoardSearch},
		{"board_projects", &k.BoardProjects},
		{"board_toggle_blocked", &k.BoardToggleBlocked},
		{"board_toggle_inactive", &k.BoardToggleInactive},
		{"detail_set_priority", &k.DetailSetPriority},
		{"detail_set_type", &k.DetailSetType},
		{"detail_edit_title", &k.DetailEditTitle},
		{"detail_edit_labels", &k.DetailEditLabels},
		{"detail_edit_description", &k.DetailEditDescription},
		{"detail_add_comment", &k.DetailAddComment},
		{"detail_add_subtask", &k.DetailAddSubtask},
		{"detail_toggle_subtask", &k.DetailToggleSubtask},
		{"detail_rename_subtask", &k.DetailRenameSubtask},
		{"detail_edit_comment", &k.DetailEditComment},
		{"detail_open_attachment", &k.DetailOpenAttachment},
		{"detail_add_attachment", &k.DetailAddAttachment},
		{"detail_delete_item", &k.DetailDeleteItem},
		{"results_up", &k.ResultsUp},
		{"results_down", &k.ResultsDown},
		{"projects_new", &k.ProjectsNew},
		{"projects_edit", &k.ProjectsEdit},
		{"projects_delete", &k.ProjectsDelete},
		{"confirm_yes", &k.ConfirmYes},
		{"confirm_no", &k.ConfirmNo},
		{"form_next_field", &k.FormNextField},
		{"form_prev_field", &k.FormPrevField},
		{"form_toggle_hidden", &k.FormToggleHidden},
		{"form_manual_entry", &k.FormManualEntry},
	}
}

// LoadKeyMap returns the default key map with any overrides from the JSON file at
// path applied. A missing or malformed file leaves the defaults untouched. The
// file maps action names to key lists, e.g. {"deleteTicket": ["x", "delete"]}.
func LoadKeyMap(path string) KeyMap {
	k := DefaultKeyMap()
	data, err := os.ReadFile(path)
	if err != nil {
		return k
	}
	var cfg map[string][]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		return k
	}
	for _, r := range k.refs() {
		newKeys, ok := cfg[r.name]
		if !ok || len(newKeys) == 0 {
			continue
		}
		*r.bind = bind(newKeys, strings.Join(newKeys, "/"), r.bind.Help().Desc)
	}
	return k
}

// helpLine renders a "key desc • key desc" footer from bindings, skipping those
// with no help key (e.g. the secondary Down/Right that share a label with Up/Left).
func helpLine(bs ...key.Binding) string {
	var parts []string
	for _, b := range bs {
		h := b.Help()
		if h.Key == "" {
			continue
		}
		if h.Desc == "" {
			parts = append(parts, h.Key)
		} else {
			parts = append(parts, h.Key+" "+h.Desc)
		}
	}
	return helpStyle.Render(strings.Join(parts, " • "))
}
