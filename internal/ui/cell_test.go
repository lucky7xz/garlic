package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestProjectCell(t *testing.T) {
	plain := lipgloss.NewStyle()

	cases := []struct {
		name        string
		project     string
		hasResource bool
		agentTask   bool
		width       int
		want        string
	}{
		{"plain", "running", false, false, 20, "running"},
		{"resource folder", "running", true, false, 20, "running" + resourceMark},
		{"outstanding agent task", "running", false, true, 20, "running" + agentMark},
		{"both", "running", true, true, 20, "running" + resourceMark + agentMark},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := projectCell(c.project, c.hasResource, c.agentTask, c.width, plain, plain)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The grid is column-aligned, so a marker must cost width rather than steal it.
func TestProjectCellNeverExceedsWidth(t *testing.T) {
	plain := lipgloss.NewStyle()
	long := strings.Repeat("x", 60)

	for _, width := range []int{4, 8, 12, 20} {
		for _, hasResource := range []bool{false, true} {
			for _, agentTask := range []bool{false, true} {
				got := projectCell(long, hasResource, agentTask, width, plain, plain)
				if w := lipgloss.Width(got); w > width {
					t.Errorf("width %d, resource=%v agent=%v: rendered %d columns (%q)",
						width, hasResource, agentTask, w, got)
				}
			}
		}
	}
}
