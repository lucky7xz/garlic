package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

type helpEntry struct {
	key  string
	desc string
}

var helpEntries = []helpEntry{
	{"arrows/hjkl", "navigate"},
	{"enter/space", "open"},
	{"o/p", "cycle boards"},
	{"alt+1..9", "jump to workspace"},
	{"tab", "toglge hidden"},
	{"r", "open resources"},
	{"i", "insert file"},
	{"e", "edit filename"},
	{"m", "move file"},
	{"u", "hide/unhide"},
	{"Del", "delete file"},
	{"q/esc", "close"},
}

// helpMenu renders the overlay at the widest column count that actually fits.
// Measuring beats guessing at breakpoints: the two-column layout needs a little
// over 60 columns, so the old "> 45" threshold produced an overlay wider than
// the terminal it was drawn into.
func (m Model) helpMenu() string {
	for numCols := 3; numCols > 1; numCols-- {
		overlay := m.helpMenuCols(numCols)
		if lipgloss.Width(overlay) <= m.TermWidth {
			return overlay
		}
	}
	return m.helpMenuCols(1)
}

func (m Model) helpMenuCols(numCols int) string {
	entries := helpEntries

	if len(entries) < numCols {
		numCols = len(entries)
	}
	if numCols < 1 {
		numCols = 1
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
