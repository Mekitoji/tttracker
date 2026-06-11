package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
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
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
	case key.Matches(km, keys.Open):
		if len(m.projects) > 0 {
			k := m.projects[m.cursor].Key
			return m, func() tea.Msg { return openProjectMsg{key: k} }
		}
	case key.Matches(km, keys.NewProject):
		return m, func() tea.Msg { return newProjectFormMsg{} }
	case key.Matches(km, keys.EditProject):
		if len(m.projects) > 0 {
			k := m.projects[m.cursor].Key
			return m, func() tea.Msg { return openProjectEditMsg{key: k} }
		}
	case key.Matches(km, keys.DeleteProject):
		if len(m.projects) > 0 {
			k := m.projects[m.cursor].Key
			return m, func() tea.Msg { return askDeleteProjectMsg{key: k} }
		}
	case key.Matches(km, keys.Quit):
		return m, tea.Quit
	}
	return m, nil
}

func (m projectsModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Projects"))
	b.WriteString("\n\n")
	if len(m.projects) == 0 {
		b.WriteString(helpStyle.Render("No projects yet — press n to create one."))
		b.WriteString("\n")
	}
	for i, p := range m.projects {
		line := fmt.Sprintf("%-10s %s", p.Key, p.Name)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("> " + line))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(helpLine(keys.Up, keys.Open, keys.NewProject, keys.EditProject, keys.DeleteProject, keys.Quit))
	return b.String()
}

func (m projectsModel) setSize(w, h int) projectsModel {
	m.width, m.height = w, h
	return m
}
