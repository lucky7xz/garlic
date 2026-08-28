package ui

import "github.com/charmbracelet/lipgloss"

const (
	// resourceMark says the project has a resource folder.
	resourceMark = "*"
	// plantedMark says a remote is holding this project, as of the last check.
	// Which remote, and when you asked, are said elsewhere -- the card only has
	// room to say yes.
	plantedMark = "🌱"
)

// projectCell lays out one card: the name, then its marks. Marks are measured
// in terminal columns, not bytes, so a wide glyph costs the name room rather
// than pushing the grid out of alignment.
func projectCell(name string, hasResource, planted bool, width int, resource, planting lipgloss.Style) string {
	marks := ""
	if hasResource {
		marks += resource.Render(resourceMark)
	}
	if planted {
		marks += planting.Render(plantedMark)
	}

	room := width - lipgloss.Width(marks)
	if room < 0 {
		room = 0
	}
	return truncate(name, room) + marks
}
