package ui

import (
	"fmt"
	"strings"
)

// Three facts about planting, three homes, none of them a new line in the view.
//
//	card    a bare mark             it is planted
//	footer  "planted on agent"      which remote holds it
//	header  "planted 14:20"         when you last asked
//
// Both the header suffix and the footer are existing single-line slots, so the
// hand-counted totalHeight in View stays correct.

// checkStamp rides the header beside [HIDDEN]. Without it a board with no marks
// is ambiguous: it could mean nothing is planted, or that nobody has asked yet.
func (m Model) checkStamp() string {
	switch {
	case m.Checking:
		return " " + plantedMark + " checking…"
	case m.Planted.Checked():
		return fmt.Sprintf(" %s checked %s", plantedMark, m.Planted.When.Format("15:04"))
	}
	return ""
}

// selectionPlanting is the footer line for whatever the cursor is on, or "" to
// leave the slot to the ordinary help hint. The card only says that a project is
// planted; naming the remotes needs room, and the cursor already says which
// project you mean.
func (m Model) selectionPlanting() string {
	p, ok := m.getSelectedProject()
	if !ok {
		return ""
	}

	hosts := m.plantedOn(m.Boards[m.ActiveBoard], p)
	if len(hosts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s planted on %s", plantedMark, strings.Join(hosts, ", "))
}
