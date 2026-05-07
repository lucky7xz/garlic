package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	key  string
	desc string
}

func (m Model) helpMenu() string {
	entries := []helpEntry{
		{"arrows/hjkl", "navigate"},
		{"enter/space", "open"},
		{"o/p", "cycle boards"},
		{"tab", "toglge hidden"},
		{"r", "open resources"},
		{"i", "insert file"},
		{"e", "edit filename"},
		{"m", "move file"},
		{"u", "hide/unhide"},
		{"Del", "delete file"},
		{"q/esc", "close"},
	}

	// Adaptive column count
	numCols := 1
	if m.TermWidth > 80 {
		numCols = 3
	} else if m.TermWidth > 45 {
		numCols = 2
	}

	if len(entries) < numCols {
		numCols = len(entries)
	}

	maxKeyLen := 0
	for _, e := range entries {
		if len(e.key) > maxKeyLen {
			maxKeyLen = len(e.key)
		}
	}

	var columns []string
	itemsPerCol := (len(entries) + numCols - 1) / numCols

	// Simple key style without border
	keyStyle := m.HeaderStyle.Copy().UnsetBorderStyle().Width(maxKeyLen + 1)

	for i := 0; i < numCols; i++ {
		start := i * itemsPerCol
		end := start + itemsPerCol
		if end > len(entries) {
			end = len(entries)
		}
		if start >= len(entries) {
			break
		}

		var colRows []string
		for _, e := range entries[start:end] {
			key := keyStyle.Render(e.key)
			desc := m.HelpStyle.Render(e.desc)
			colRows = append(colRows, fmt.Sprintf("%s %s", key, desc))
		}

		colStr := lipgloss.JoinVertical(lipgloss.Left, colRows...)
		if i < numCols-1 {
			colStr = lipgloss.NewStyle().PaddingRight(4).Render(colStr)
		}
		columns = append(columns, colStr)
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, columns...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.SeparatorStyle.GetForeground()).
		Padding(1, 2).
		Render(content)
}
