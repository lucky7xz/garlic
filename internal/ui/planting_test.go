package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/remote"
)

var checkedAt = time.Date(2026, 8, 28, 14, 20, 0, 0, time.UTC)

// plantedBoard is one area holding two projects, addressed the way a manifest
// would name them: epics/bioz/<name>.md
func plantedBoard() domain.Board {
	projects := []domain.Project{
		{Name: "mealprep.md", Path: "/home/me/shara/epics/bioz/mealprep.md", Category: "bioz", Status: "toDo"},
		{Name: "sleeplog.md", Path: "/home/me/shara/epics/bioz/sleeplog.md", Category: "bioz", Status: "toDo"},
	}
	return domain.Board{
		Name:          "epics",
		Grid:          map[string]map[string][]domain.Project{"toDo": {"bioz": projects}},
		HiddenGrid:    map[string]map[string][]domain.Project{},
		CategoryOrder: []string{"bioz"},
		Statuses:      []string{"toDo"},
		Opts:          domain.BoardOptions{Path: "/home/me/shara/epics", Name: "epics", Extension: ".md"},
	}
}

func plantedModel(where map[string][]string) Model {
	m := modelAt(plantedBoard(), 100, 40)
	if where != nil {
		m.Planted = remote.Sighting{When: checkedAt, Hosts: []string{"agent"}, Where: where}
	}
	return m
}

// Every planting fact shares the footer with the help hint. An unchecked board
// and a checked-but-empty one must not read the same: one means "I have not
// asked", the other "nothing is out there".
func TestIdleFooter(t *testing.T) {
	planted := map[string][]string{"epics/bioz/mealprep.md": {"agent"}}

	cases := []struct {
		name     string
		model    func() Model
		want     string
		wantHint bool
	}{
		{
			"before any check, only the hint",
			func() Model { return plantedModel(nil) },
			"", true,
		},
		{
			"checked, nothing out there",
			func() Model { return plantedModel(map[string][]string{}) },
			plantedMark + " 14:20", true,
		},
		{
			"cursor on a planted project takes the line",
			func() Model { return plantedModel(planted) },
			plantedMark + " planted on agent • 14:20", false,
		},
		{
			"cursor moved off it, back to the stamp",
			func() Model {
				m := plantedModel(planted)
				m.GridCursor.Project = 1 // sleeplog, never planted
				return m
			},
			plantedMark + " 14:20", true,
		},
		{
			"a check still in flight says so",
			func() Model {
				m := plantedModel(nil)
				m.Checking = true
				return m
			},
			plantedMark + " checking…", true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.model().idleFooter()

			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("got %q, want it to contain %q", got, c.want)
			}
			if c.want == "" && strings.Contains(got, plantedMark) {
				t.Errorf("got %q, want nothing about planting", got)
			}
			if hasHint := strings.Contains(got, "q: quit"); hasHint != c.wantHint {
				t.Errorf("got %q, help hint present = %v, want %v", got, hasHint, c.wantHint)
			}
		})
	}
}

func TestIdleFooterNamesEveryHost(t *testing.T) {
	m := plantedModel(map[string][]string{"epics/bioz/mealprep.md": {"agent", "berta"}})
	if got := m.idleFooter(); !strings.Contains(got, "agent, berta") {
		t.Errorf("got %q, want both hosts named", got)
	}
}

// The hint is what you already know; the stamp is what you pressed a key for.
// When only one of them fits, the stamp wins. At the narrowest board garlic will
// draw at all, both still do -- the short hint and the stamp come to 18 columns.
func TestIdleFooterDropsHintOnlyWhenItMustNarrow(t *testing.T) {
	for _, c := range []struct {
		width    int
		wantHint bool
	}{
		{minBoardWidth, true},
		{12, false},
	} {
		m := plantedModel(map[string][]string{})
		m.TermWidth = c.width

		got := m.idleFooter()
		if !strings.Contains(got, "14:20") {
			t.Errorf("width %d: got %q, the stamp must always survive", c.width, got)
		}
		if hasHint := strings.Contains(got, "q: quit"); hasHint != c.wantHint {
			t.Errorf("width %d: got %q, hint present = %v, want %v", c.width, got, hasHint, c.wantHint)
		}
	}
}

// The view is joined with JoinVertical(Center), so anything a check adds to the
// workspace title shifts the whole board sideways -- it visibly jumps for a fact
// that is not about the board. That title is why planting lives in the footer,
// and it must come out byte-identical however much else a check marks.
//
// Measured on a board of ordinary width, where the grid is the widest block and
// therefore fixes the centering. On a single-column board the footer can exceed
// the grid, and then lipgloss re-centers everything -- but that is how every
// variable footer here already behaves, the delete prompt most of all.
func TestCheckNeverMovesTheTitle(t *testing.T) {
	wide := plantedBoard()
	wide.CategoryOrder = []string{"bioz", "work", "learning"}
	for _, cat := range wide.CategoryOrder[1:] {
		wide.Grid["toDo"][cat] = nil
	}

	before := modelAt(wide, 100, 40)

	checked := before
	checked.Planted = remote.Sighting{When: checkedAt, Hosts: []string{"agent"},
		Where: map[string][]string{"epics/bioz/mealprep.md": {"agent"}}}

	checking := before
	checking.Checking = true

	titleOf := func(m Model) string {
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "Workspace:") {
				return line
			}
		}
		t.Fatal("no workspace title in the rendered view")
		return ""
	}

	want := titleOf(before)
	for _, c := range []struct {
		name string
		m    Model
	}{{"after a check", checked}, {"while checking", checking}} {
		t.Run(c.name, func(t *testing.T) {
			if got := titleOf(c.m); got != want {
				t.Errorf("title moved\n  before %q\n   after %q", want, got)
			}
		})
	}

	// And a mark really did land, or the assertion above proves nothing.
	if !strings.Contains(checked.View(), "mealprep"+plantedMark) {
		t.Error("the checked board carries no mark at all, so this test is vacuous")
	}
}

// The footer shares one line with the help hint, so a check must not add a row
// either. totalHeight in View is hand-counted; if it undercounts, the centered
// Place() clips the board at both ends and eats the "Status:" labels.
func TestCheckAddsNoLines(t *testing.T) {
	planted := map[string][]string{
		"epics/bioz/mealprep.md": {"agent"},
		"epics/bioz/sleeplog.md": {"agent"},
	}

	for _, size := range []struct{ w, h int }{{100, 40}, {60, 12}, {40, 8}} {
		before := modelAt(plantedBoard(), size.w, size.h)
		before.RecalculateOffsets()

		after := before
		after.Planted = remote.Sighting{When: checkedAt, Hosts: []string{"agent"}, Where: planted}

		gotBefore := strings.Count(before.View(), "\n")
		gotAfter := strings.Count(after.View(), "\n")
		if gotBefore != gotAfter {
			t.Errorf("%dx%d: %d lines unchecked, %d checked -- a check must not change the height",
				size.w, size.h, gotBefore, gotAfter)
		}
	}
}

// The marks describe a relationship to a remote, not the contents of a file, so
// they must survive the watcher rebuilding the board underneath them.
func TestPlantingSurvivesBoardRefresh(t *testing.T) {
	m := plantedModel(map[string][]string{"epics/bioz/mealprep.md": {"agent"}})

	next, _ := m.Update(RefreshMsg([]domain.Board{plantedBoard()}))
	got := next.(Model)

	if !got.Planted.Checked() {
		t.Fatal("a filesystem refresh cleared the check")
	}
	if got.plantedWhere() == "" {
		t.Error("a filesystem refresh lost the planting marks")
	}
}

// A check where nothing answered must not overwrite what you already knew, and
// must not stamp the board -- otherwise a dead host reads as an empty remote.
func TestFailedCheckKeepsTheLastAnswer(t *testing.T) {
	m := plantedModel(map[string][]string{"epics/bioz/mealprep.md": {"agent"}})

	next, _ := m.Update(checkMsg{
		sighting: remote.Sighting{When: checkedAt.Add(time.Minute)},
		err:      errors.New("agent: connection refused"),
	})
	got := next.(Model)

	if got.Checking {
		t.Error("a finished check must clear the in-flight flag")
	}
	if got.plantedWhere() == "" {
		t.Error("a failed check discarded the previous answer")
	}
	if got.ErrorMsg == "" {
		t.Error("a failed check must say what went wrong")
	}
}

// Pressing c while a check is already out must not launch a second one.
func TestCheckIsNotReentrant(t *testing.T) {
	m := plantedModel(nil)
	m.Checking = true

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Error("a second c while checking launched another check")
	}
	if !next.(Model).Checking {
		t.Error("the in-flight flag was lost")
	}
}

// The mark shows the granularity you planted at. Sending one project marks that
// project; sending the whole area marks the column instead. Marking the column
// whenever anything under it went would erase the distinction the mark is for.
func TestAreaPlanted(t *testing.T) {
	both := map[string][]string{
		"epics/bioz/mealprep.md": {"agent"},
		"epics/bioz/sleeplog.md": {"agent"},
	}
	one := map[string][]string{"epics/bioz/mealprep.md": {"agent"}}

	cases := []struct {
		name     string
		where    map[string][]string
		category string
		want     bool
	}{
		{"the whole area went", both, "bioz", true},
		{"only one project went", one, "bioz", false},
		{"an area that never went", both, "bio", false},
		{"an area with no projects at all", both, "empty", false},
		{"before any check", nil, "bioz", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := plantedModel(c.where)
			if got := m.areaPlanted(m.Boards[m.ActiveBoard], c.category); got != c.want {
				t.Errorf("areaPlanted(%q) = %v, want %v", c.category, got, c.want)
			}
		})
	}
}

// One mark per fact, never both: a project inside a fully-planted area does not
// repeat what its column already says.
func TestMarksDoNotDouble(t *testing.T) {
	whole := plantedModel(map[string][]string{
		"epics/bioz/mealprep.md": {"agent"},
		"epics/bioz/sleeplog.md": {"agent"},
	}).View()

	if !strings.Contains(whole, "bioz"+plantedMark) {
		t.Errorf("a fully planted area carries no header mark:\n%s", whole)
	}
	if strings.Contains(whole, "mealprep"+plantedMark) {
		t.Errorf("project repeats the mark its area already carries:\n%s", whole)
	}

	part := plantedModel(map[string][]string{"epics/bioz/mealprep.md": {"agent"}}).View()

	if strings.Contains(part, "bioz"+plantedMark) {
		t.Errorf("a partly planted area must not be marked:\n%s", part)
	}
	if !strings.Contains(part, "mealprep"+plantedMark) {
		t.Errorf("the project that went carries no mark:\n%s", part)
	}
	if strings.Contains(part, "sleeplog"+plantedMark) {
		t.Errorf("a project that never went is marked:\n%s", part)
	}

	if strings.Contains(plantedModel(nil).View(), plantedMark) {
		t.Error("something is marked before a check has run")
	}
}
