package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
)

// Everything the board says about planting lives on one line: the footer, which
// already changes with context. The header is left alone deliberately -- the
// view is centered as a block, so widening the title line would shift the whole
// board sideways the moment you pressed `c`.
//
//	never checked          ?: help • q: quit
//	checked, plain card    ?: help • q: quit • 🌱 14:20
//	checked, planted card  🌱 planted on agent • 14:20
//	check in flight        ?: help • q: quit • 🌱 checking…

const footerSep = " • "

// checkedAt says when the last check happened, or that one is still out. Empty
// means nobody has asked yet, which is a different thing from having asked and
// found nothing.
func (m Model) checkedAt() string {
	switch {
	case m.Checking:
		return "checking…"
	case m.Planted.Checked():
		return m.Planted.When.Format("15:04")
	}
	return ""
}

// plantedWhere names the remotes holding whatever the cursor is on, or "". The
// card only says that a project is planted; naming the hosts needs room, and
// the cursor already establishes which project is meant.
func (m Model) plantedWhere() string {
	p, ok := m.getSelectedProject()
	if !ok {
		return ""
	}

	hosts := m.plantedOn(m.Boards[m.ActiveBoard], p)
	if len(hosts) == 0 {
		return ""
	}
	return "planted on " + strings.Join(hosts, ", ")
}

// areaPlanted reports whether a whole column went: every project in it is on a
// remote. The mark shows the granularity you planted at -- send one project and
// that project is marked, send the area and the column is marked instead. A
// column marked whenever anything under it went would erase the distinction.
//
// An empty category has not been planted; it has nothing to plant.
func (m Model) areaPlanted(board domain.Board, category string) bool {
	grid := board.ActiveGrid(m.ShowHidden)

	found := false
	for _, status := range board.Statuses {
		for _, p := range grid[status][category] {
			if len(m.plantedOn(board, p)) == 0 {
				return false
			}
			found = true
		}
	}
	return found
}

// idleFooter is the footer line when nothing more urgent is happening. It owns
// the help hint too, because the two share the slot.
func (m Model) idleFooter() string {
	hint := "?: help • q: quit"
	if !m.fitsHelpOverlay() {
		hint = "q: quit"
	}

	// The seedling belongs to the line rather than to each fact, so a planted
	// card reads "🌱 planted on agent • 14:20" instead of repeating the glyph.
	var facts []string
	where := m.plantedWhere()
	if where != "" {
		facts = append(facts, where)
	}
	if when := m.checkedAt(); when != "" {
		facts = append(facts, when)
	}
	if len(facts) == 0 {
		return m.HelpStyle.Render(hint)
	}

	group := m.PlantedHintStyle.Render(plantedMark + " " + strings.Join(facts, footerSep))

	// A planted card has something specific to say and would otherwise run long,
	// so it takes the line to itself.
	if where != "" {
		return group
	}

	line := m.HelpStyle.Render(hint+footerSep) + group
	if lipgloss.Width(line) > m.TermWidth {
		// What you pressed the key for outranks what you already know.
		return group
	}
	return line
}
