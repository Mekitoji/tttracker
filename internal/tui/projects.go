package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"tttracker/internal/app"
	"tttracker/internal/project"
)

type projectsModel struct {
	projects      []project.Project
	cursor        int
	width, height int
}

func newProjectsModel(a *app.App, ctx context.Context) (projectsModel, error) {
	ps, err := a.Projects.List(ctx)
	if err != nil {
		return projectsModel{}, err
	}
	return projectsModel{projects: ps, width: 80, height: 24}, nil
}

func (m projectsModel) Update(msg tea.Msg) (projectsModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.projects) > 0 {
				k := m.projects[m.cursor].Key
				return m, func() tea.Msg { return openProjectMsg{key: k} }
			}
		case "n":
			return m, func() tea.Msg { return newProjectFormMsg{} }
		case "e":
			if len(m.projects) > 0 {
				k := m.projects[m.cursor].Key
				return m, func() tea.Msg { return openProjectEditMsg{key: k} }
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m projectsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Projects") + "\n\n")
	if len(m.projects) == 0 {
		b.WriteString(helpStyle.Render("No projects yet — press n to create one.") + "\n")
	}
	for i, p := range m.projects {
		line := fmt.Sprintf("%-10s %s", p.Key, p.Name)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ move • enter open • n new • e edit • q quit"))
	return b.String()
}

func (m projectsModel) setSize(w, h int) projectsModel {
	m.width, m.height = w, h
	return m
}
