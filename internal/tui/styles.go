package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	helpStyle        = lipgloss.NewStyle().Faint(true)
	fieldStyle       = lipgloss.NewStyle().Faint(true)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	selectedStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63"))
	columnStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	columnSelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	columnTitleStyle = lipgloss.NewStyle().Bold(true).Underline(true)
)
