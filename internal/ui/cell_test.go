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
		planted     bool
		width       int
		want        string
	}{
		{"plain", "running", false, false, 20, "running"},
		{"resource folder", "running", true, false, 20, "running" + resourceMark},
		{"planted on a remote", "running", false, true, 20, "running" + plantedMark},
		{"both", "running", true, true, 20, "running" + resourceMark + plantedMark},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := projectCell(c.project, c.hasResource, c.planted, c.width, plain, plain)
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
			for _, planted := range []bool{false, true} {
				got := projectCell(long, hasResource, planted, width, plain, plain)
				if w := lipgloss.Width(got); w > width {
					t.Errorf("width %d, resource=%v planted=%v: rendered %d columns (%q)",
						width, hasResource, planted, w, got)
				}
			}
		}
	}
}
