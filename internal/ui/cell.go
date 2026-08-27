package ui

import "github.com/charmbracelet/lipgloss"

const (
	// resourceMark says the project has a resource folder.
	resourceMark = "*"
	// agentMark says at least one bare #AT is still outstanding in the file.
	// The agent flips them to #AT-done and the mark disappears.
	agentMark = "⏳"
)

// projectCell lays out one card: the name, then its marks. Marks are measured
// in terminal columns, not bytes, so a wide glyph costs the name room rather
// than pushing the grid out of alignment.
func projectCell(name string, hasResource, agentTask bool, width int, resource, agent lipgloss.Style) string {
	marks := ""
	if hasResource {
		marks += resource.Render(resourceMark)
	}
	if agentTask {
		marks += agent.Render(agentMark)
	}

	room := width - lipgloss.Width(marks)
	if room < 0 {
		room = 0
	}
	return truncate(name, room) + marks
}
