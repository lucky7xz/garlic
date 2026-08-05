package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
)

// board builds a board with the given statuses and categories, placing
// projectsPerCell projects in every cell.
func board(statuses, categories []string, projectsPerCell int) domain.Board {
	grid := make(map[string]map[string][]domain.Project)
	for _, s := range statuses {
		grid[s] = make(map[string][]domain.Project)
		for _, c := range categories {
			var ps []domain.Project
			for i := 0; i < projectsPerCell; i++ {
				ps = append(ps, domain.Project{Name: "proj.md", Category: c, Status: s})
			}
			grid[s][c] = ps
		}
	}
	return domain.Board{
		Name:          "test",
		Grid:          grid,
		HiddenGrid:    make(map[string]map[string][]domain.Project),
		CategoryOrder: categories,
		Statuses:      statuses,
	}
}

func modelAt(b domain.Board, w, h int) Model {
	m := Model{
		Boards:       []domain.Board{b},
		TermWidth:    w,
		TermHeight:   h,
		SavedCursors: make([]cursorState, 1),
	}
	ApplyTheme(domain.Theme{}, &m)
	m.RecalculateOffsets()
	return m
}

// The rendered board must never be taller than the terminal, otherwise the
// centered Place() at the end of View() clips the top and bottom -- which is
// how "Status:" labels disappear.
func TestBoardNeverOverflowsTerminalHeight(t *testing.T) {
	cases := []struct {
		name     string
		statuses int
		height   int
	}{
		{"one status", 1, 24},
		{"three statuses", 3, 24},
		{"four statuses trips the old estimate", 4, 24},
		{"eight statuses", 8, 24},
		{"eight statuses tall term", 8, 60},
		{"at the height floor", 4, minBoardHeight},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var statuses []string
			for i := 0; i < tc.statuses; i++ {
				statuses = append(statuses, string(rune('a'+i)))
			}
			m := modelAt(board(statuses, []string{"cat"}, 1), 80, tc.height)

			got := lipgloss.Height(m.View())
			if got > tc.height {
				t.Errorf("rendered %d lines into a %d-line terminal (overflow of %d)",
					got, tc.height, got-tc.height)
			}
		})
	}
}

// Whatever else is dropped when space is tight, the active status label has to
// survive -- it is the only thing telling you where the cursor is.
func TestActiveStatusLabelAlwaysVisible(t *testing.T) {
	statuses := []string{"todo", "doing", "review", "done", "archived"}

	for _, h := range []int{minBoardHeight, 16, 20, 24, 40, 80} {
		for cursor := range statuses {
			m := modelAt(board(statuses, []string{"cat"}, 2), 80, h)
			m.GridCursor.Status = cursor

			view := m.View()
			// Only the lines the terminal can actually show.
			lines := strings.Split(view, "\n")
			if len(lines) > h {
				lines = lines[:h]
			}
			visible := strings.Join(lines, "\n")

			want := "Status: " + statuses[cursor]
			if !strings.Contains(visible, want) {
				t.Errorf("height %d, cursor on %q: %q not in visible output",
					h, statuses[cursor], want)
			}
		}
	}
}

// The size gate must not fire at sizes the renderer can actually handle.
func TestSizeGateMatchesWhatRenders(t *testing.T) {
	if tooSmallToRender(minBoardWidth, minBoardHeight) {
		t.Errorf("gate rejects %dx%d, its own declared floor", minBoardWidth, minBoardHeight)
	}
	if !tooSmallToRender(minBoardWidth-1, minBoardHeight) {
		t.Error("gate accepts a width below the floor")
	}
	if !tooSmallToRender(minBoardWidth, minBoardHeight-1) {
		t.Error("gate accepts a height below the floor")
	}

	// At the floor the board renders within its bounds.
	m := modelAt(board([]string{"todo"}, []string{"cat"}, 1), minBoardWidth, minBoardHeight)
	view := m.View()
	if h := lipgloss.Height(view); h > minBoardHeight {
		t.Errorf("at the floor the board renders %d lines, over the %d-line floor", h, minBoardHeight)
	}
	if w := lipgloss.Width(view); w > minBoardWidth {
		t.Errorf("at the floor the board renders %d cols, over the %d-col floor", w, minBoardWidth)
	}
}

// The help overlay has a higher floor than the board, so `?` must be refused
// rather than drawing an overlay wider or taller than the terminal.
func TestHelpOverlayFitsWhenOffered(t *testing.T) {
	for _, w := range []int{minBoardWidth, 25, 30, 33, 40, 46, 50, 63, 64, 80, 81, 120} {
		for _, h := range []int{minBoardHeight, 15, 16, 20, 40} {
			m := modelAt(board([]string{"todo"}, []string{"cat"}, 1), w, h)
			if !m.fitsHelpOverlay() {
				continue
			}
			overlay := m.helpMenu()
			if oh := lipgloss.Height(overlay); oh > h {
				t.Errorf("%dx%d: help offered but overlay is %d lines tall", w, h, oh)
			}
			if ow := lipgloss.Width(overlay); ow > w {
				t.Errorf("%dx%d: help offered but overlay is %d cols wide", w, h, ow)
			}
		}
	}
}
