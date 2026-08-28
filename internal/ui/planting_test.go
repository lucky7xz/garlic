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

// An unchecked board and a checked-but-empty board must not look the same:
// one means "I have not asked", the other means "nothing is out there".
func TestCheckStamp(t *testing.T) {
	if got := plantedModel(nil).checkStamp(); got != "" {
		t.Errorf("before any check the header must stay bare, got %q", got)
	}

	got := plantedModel(map[string][]string{}).checkStamp()
	if !strings.Contains(got, "14:20") {
		t.Errorf("a check that found nothing is still a check, got %q", got)
	}

	m := plantedModel(nil)
	m.Checking = true
	if got := m.checkStamp(); !strings.Contains(got, "checking") {
		t.Errorf("an in-flight check should say so, got %q", got)
	}
}

// The card only says a project is planted; the footer says where, because the
// cursor already establishes which project is meant.
func TestSelectionPlanting(t *testing.T) {
	planted := map[string][]string{"epics/bioz/mealprep.md": {"agent"}}

	m := plantedModel(planted)
	if got := m.selectionPlanting(); got != plantedMark+" planted on agent" {
		t.Errorf("cursor on a planted project: got %q", got)
	}

	m.GridCursor.Project = 1 // sleeplog, which was never planted
	if got := m.selectionPlanting(); got != "" {
		t.Errorf("cursor on an unplanted project must leave the footer alone, got %q", got)
	}

	if got := plantedModel(nil).selectionPlanting(); got != "" {
		t.Errorf("before any check the footer must leave the slot alone, got %q", got)
	}
}

func TestSelectionPlantingNamesEveryHost(t *testing.T) {
	m := plantedModel(map[string][]string{"epics/bioz/mealprep.md": {"agent", "berta"}})
	if got := m.selectionPlanting(); !strings.Contains(got, "agent, berta") {
		t.Errorf("got %q, want both hosts named", got)
	}
}

// The whole point of reusing the header suffix and the footer is that the view
// gains no row. totalHeight in View is hand-counted; if a check added a line,
// the centered Place() would clip the board at both ends and eat the "Status:"
// labels.
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
	if got.selectionPlanting() == "" {
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
	if got.selectionPlanting() == "" {
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
