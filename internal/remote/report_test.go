package remote

import (
	"strings"
	"testing"
)

func TestRenderReportNothingToDo(t *testing.T) {
	got := renderReport("harvest", "epics/fitness", "you@host:/home/agent/shara", Plan{})
	if !strings.Contains(got, "nothing to do") {
		t.Errorf("an empty plan should say so, got:\n%s", got)
	}
}

func TestRenderReportListsEveryPath(t *testing.T) {
	p := Plan{
		Take: []string{"epics/fitness/running.md"},
		Park: []string{"epics/fitness/hiking.md"},
		Left: []string{"epics/fitness/notes.txt"},
		Gone: []string{"epics/fitness/old.md"},
	}

	got := renderReport("harvest", "epics/fitness", "you@host:/home/agent/shara", p)
	for _, want := range []string{
		"harvest", "epics/fitness", "you@host:/home/agent/shara",
		"epics/fitness/running.md",
		"epics/fitness/hiking.md",
		"epics/fitness/notes.txt",
		"epics/fitness/old.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}

// status reports the same reckoning as harvest but must not read as though
// anything was moved.
func TestRenderReportStatusIsNotPastTense(t *testing.T) {
	p := Plan{Take: []string{"epics/fitness/running.md"}}

	got := renderReport("status", "epics", "you@host:/home/agent/shara", p)
	if strings.Contains(got, "took") {
		t.Errorf("status should not claim to have taken anything:\n%s", got)
	}
	if !strings.Contains(got, "epics/fitness/running.md") {
		t.Errorf("status should still name the file:\n%s", got)
	}
}

func TestRenderReportPlantSideBuckets(t *testing.T) {
	p := Plan{
		Push:      []string{"epics/fitness/running.md"},
		Blocked:   []string{"epics/fitness/hiking.md"},
		LocalGone: []string{"epics/fitness/dropped.md"},
	}

	got := renderReport("plant", "epics", "you@host:/home/agent/shara", p)
	for _, want := range []string{
		"epics/fitness/running.md",
		"epics/fitness/hiking.md",
		"epics/fitness/dropped.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}
