// Package editor launches the user's external $EDITOR to edit Markdown content.
// There are two entry points: Open for plain CLI contexts, and OpenInTUI which
// returns a tea.Cmd that releases and restores the terminal around the editor
// (required when running inside a Bubble Tea program).
package editor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Resolve picks the editor command, honoring $VISUAL then $EDITOR and falling
// back to nvim/vim/vi.
func Resolve() string {
	if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("EDITOR")); v != "" {
		return v
	}
	for _, candidate := range []string{"nvim", "vim"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return "vi"
}

// EditedMsg is delivered after the editor closes inside a Bubble Tea program.
type EditedMsg struct {
	Content string
	Err     error
}

// Open seeds a temp Markdown file with content, runs the editor attached to the
// current stdio, and returns the edited content. For non-TUI (CLI) contexts.
func Open(content string) (string, error) {
	file, err := writeTemp(content)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := os.Remove(file); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()
	cmd := command(file)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited: %w", err)
	}
	return readBack(file)
}

// OpenInTUI returns a tea.Cmd that runs the editor via tea.ExecProcess (which
// safely hands the terminal to the editor and restores it afterwards) and then
// emits an EditedMsg with the result.
func OpenInTUI(content string) tea.Cmd {
	file, err := writeTemp(content)
	if err != nil {
		return func() tea.Msg { return EditedMsg{Err: err} }
	}
	return tea.ExecProcess(command(file), func(err error) tea.Msg {
		defer func() {
			if err := os.Remove(file); err != nil {
				fmt.Printf("Error closing rows: %v\n", err)
			}
		}()

		if err != nil {
			return EditedMsg{Err: fmt.Errorf("editor exited: %w", err)}
		}
		content, rerr := readBack(file)
		return EditedMsg{Content: content, Err: rerr}
	})
}

func writeTemp(content string) (string, error) {
	f, err := os.CreateTemp("", "tracker-*.md")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Error closing rows: %v\n", err)
		}
	}()

	if _, err := f.WriteString(content); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func readBack(file string) (string, error) {
	out, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// command builds the editor invocation. The editor string may include flags
// (e.g. "code --wait"), so it is split on whitespace before appending the file.
func command(file string) *exec.Cmd {
	parts := append(strings.Fields(Resolve()), file)
	return exec.Command(parts[0], parts[1:]...)
}
