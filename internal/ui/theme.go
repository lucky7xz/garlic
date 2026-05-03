package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
)

func ApplyTheme(theme domain.Theme, m *Model) {
	border := lipgloss.Border{
		Top:         "─",
		Bottom:      "─",
		Left:        "│",
		Right:       "│",
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
	}

	m.TitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Primary)).
		Bold(true).
		Padding(0, 0)

	m.HeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Secondary)).
		Bold(true).
		Border(border).
		BorderForeground(lipgloss.Color(theme.Comment)).
		Padding(0, 0)

	m.CellStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Foreground)).
		Border(border).
		BorderForeground(lipgloss.Color(theme.Comment)).
		Padding(0, 0)

	m.SelectedCellStyle = m.CellStyle.Copy().
		Foreground(lipgloss.Color(theme.Primary)).
		BorderForeground(lipgloss.Color(theme.Primary)).
		Bold(true)

	m.ResourceHintStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Accent)).
		Bold(true)

	m.EmptyCellStyle = m.CellStyle.Copy().Foreground(lipgloss.Color(theme.Comment))

	m.HelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Comment)).
		Faint(true)

	m.SeparatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Comment))
}
