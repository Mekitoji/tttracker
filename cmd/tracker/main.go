// Command tracker is the entry point for the local ticket tracker.
//
// Phase 0 wires only the foundation: `init` (create the data dir + run
// migrations) and `editor-smoke` (a manual gate for the $EDITOR-in-TUI flow).
// The real TUI lands in Phase 2.
package main

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/db"
	"tttracker/internal/editor"
	"tttracker/internal/paths"
)

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "init":
		if err := runInit(""); err != nil {
			fail(err)
		}
	case "editor-smoke":
		if err := runEditorSmoke(); err != nil {
			fail(err)
		}
	case "", "tui":
		fmt.Println("TUI not implemented yet (Phase 2).")
		fmt.Println("Available now: tracker init | tracker editor-smoke")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(2)
	}
}

func runInit(override string) error {
	dir, err := paths.DataDir(override)
	if err != nil {
		return err
	}
	if err := paths.EnsureLayout(dir); err != nil {
		return err
	}
	database, err := db.Open(paths.DBPath(dir))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Printf("Error closing database: %v", err)
		}
	}()
	if err := db.Migrate(database); err != nil {
		return err
	}
	fmt.Printf("Initialized tracker data at %s\n", dir)
	return nil
}

// --- editor-smoke: manual gate for tea.ExecProcess + $EDITOR ---

type smokeModel struct {
	content string
	status  string
}

func (m smokeModel) Init() tea.Cmd { return nil }

func (m smokeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "e":
			return m, editor.OpenInTUI(m.content)
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case editor.EditedMsg:
		if msg.Err != nil {
			m.status = "error: " + msg.Err.Error()
		} else {
			m.content = msg.Content
			m.status = "terminal restored OK; read back " + strconv.Itoa(len(msg.Content)) + " bytes"
		}
	}
	return m, nil
}

func (m smokeModel) View() string {
	return fmt.Sprintf(
		"editor smoke test\n\n"+
			"press 'e' to open $EDITOR, 'q' to quit\n\n"+
			"status: %s\n\n"+
			"--- content ---\n%s\n",
		m.status, m.content,
	)
}

func runEditorSmoke() error {
	_, err := tea.NewProgram(smokeModel{content: "# edit me\n", status: "ready"}).Run()
	return err
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
