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
	NewTicket    key.Binding
	MoveStatus   key.Binding
	DeleteTicket key.Binding
	Search       key.Binding
	Projects     key.Binding

	// detail
	SetPriority     key.Binding
	SetType         key.Binding
	EditTitle       key.Binding
	EditLabels      key.Binding
	EditDescription key.Binding
	AddComment      key.Binding
	AddSubtask      key.Binding
	ToggleSubtask   key.Binding
	RenameSubtask   key.Binding
	EditComment     key.Binding
	DeleteItem      key.Binding

	// shared text-input result lists (search, repo finder)
	ResultsUp   key.Binding
	ResultsDown key.Binding

	// projects
	NewProject    key.Binding
	EditProject   key.Binding
	DeleteProject key.Binding

	// confirm
	ConfirmYes key.Binding
	ConfirmNo  key.Binding

	// forms / project edit
	NextField    key.Binding
	PrevField    key.Binding
	ToggleHidden key.Binding
	ManualEntry  key.Binding
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

		NewTicket:    bind([]string{"n"}, "n", "new"),
		MoveStatus:   bind([]string{"m"}, "m", "status"),
		DeleteTicket: bind([]string{"x"}, "x", "del"),
		Search:       bind([]string{"/"}, "/", "search"),
		Projects:     bind([]string{"p"}, "p", "projects"),

		SetPriority:     bind([]string{"p"}, "p", "priority"),
		SetType:         bind([]string{"t"}, "t", "type"),
		EditTitle:       bind([]string{"r"}, "r", "rename"),
		EditLabels:      bind([]string{"l"}, "l", "labels"),
		EditDescription: bind([]string{"e"}, "e", "desc"),
		AddComment:      bind([]string{"c"}, "c", "comment"),
		AddSubtask:      bind([]string{"s"}, "s", "subtask"),
		ToggleSubtask:   bind([]string{" "}, "␣", "toggle"),
		RenameSubtask:   bind([]string{"R"}, "R", "rename-sub"),
		EditComment:     bind([]string{"enter"}, "enter", "edit-comment"),
		DeleteItem:      bind([]string{"d"}, "d", "delete"),

		ResultsUp:   bind([]string{"up", "ctrl+k"}, "↑/↓ ⌃k/⌃j", "select"),
		ResultsDown: bind([]string{"down", "ctrl+j"}, "", ""),

		NewProject:    bind([]string{"n"}, "n", "new"),
		EditProject:   bind([]string{"e"}, "e", "edit"),
		DeleteProject: bind([]string{"x"}, "x", "delete"),

		ConfirmYes: bind([]string{"y", "Y"}, "y", "confirm"),
		ConfirmNo:  bind([]string{"n", "N"}, "n", "cancel"),

		NextField:    bind([]string{"tab", "down"}, "tab", "next"),
		PrevField:    bind([]string{"shift+tab", "up"}, "shift+tab", "prev"),
		ToggleHidden: bind([]string{"."}, ".", "hidden"),
		ManualEntry:  bind([]string{"i", "tab"}, "i", "manual"),
	}
}

type bindingRef struct {
	name string
	bind *key.Binding
}

// refs maps the JSON config action names to the corresponding bindings.
func (k *KeyMap) refs() []bindingRef {
	return []bindingRef{
		{"up", &k.Up}, {"down", &k.Down}, {"left", &k.Left}, {"right", &k.Right},
		{"open", &k.Open}, {"back", &k.Back}, {"quit", &k.Quit},
		{"newTicket", &k.NewTicket}, {"moveStatus", &k.MoveStatus},
		{"deleteTicket", &k.DeleteTicket}, {"search", &k.Search}, {"projects", &k.Projects},
		{"setPriority", &k.SetPriority}, {"setType", &k.SetType}, {"editTitle", &k.EditTitle},
		{"editLabels", &k.EditLabels}, {"editDescription", &k.EditDescription},
		{"addComment", &k.AddComment}, {"addSubtask", &k.AddSubtask},
		{"toggleSubtask", &k.ToggleSubtask}, {"renameSubtask", &k.RenameSubtask},
		{"editComment", &k.EditComment}, {"deleteItem", &k.DeleteItem},
		{"resultsUp", &k.ResultsUp}, {"resultsDown", &k.ResultsDown},
		{"newProject", &k.NewProject}, {"editProject", &k.EditProject}, {"deleteProject", &k.DeleteProject},
		{"confirmYes", &k.ConfirmYes}, {"confirmNo", &k.ConfirmNo},
		{"nextField", &k.NextField}, {"prevField", &k.PrevField},
		{"toggleHidden", &k.ToggleHidden}, {"manualEntry", &k.ManualEntry},
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
