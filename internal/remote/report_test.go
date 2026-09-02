package remote

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

	// Names are printed relative to the address in the header line, which is
	// the whole point of the header line.
	got := renderReport("harvest", "epics/fitness", "you@host:/home/agent/shara", p)
	for _, want := range []string{
		"harvest", "epics/fitness", "you@host:/home/agent/shara",
		"running.md",
		"hiking.md",
		"notes.txt",
		"old.md",
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
	if !strings.Contains(got, "fitness/running.md") {
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
		"fitness/running.md",
		"fitness/hiking.md",
		"fitness/dropped.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
}

// A report is read in a terminal, so it lays names out in the terminal it has.
// Filling each column downwards keeps a sorted list sorted down the page.
func TestColumnsFillTheWidth(t *testing.T) {
	names := []string{"a.md", "b.md", "c.md", "d.md", "e.md"}

	// Built from the constant rather than typed out, so moving the margin does
	// not look like a layout regression.
	pad := strings.Repeat(" ", reportIndent)

	cases := []struct {
		name  string
		width int
		want  string
	}{
		{
			"wide enough for every name",
			80,
			pad + "a.md  b.md  c.md  d.md  e.md\n",
		},
		{
			// Six columns to a cell: at 25 usable there is room for three.
			"three columns",
			reportIndent + 20,
			pad + "a.md  c.md  e.md\n" + pad + "b.md  d.md\n",
		},
		{
			"nowhere to put them but downwards",
			8,
			pad + "a.md\n" + pad + "b.md\n" + pad + "c.md\n" + pad + "d.md\n" + pad + "e.md\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := columns(names, reportIndent, c.width); got != c.want {
				t.Errorf("at width %d got\n%q\nwant\n%q", c.width, got, c.want)
			}
		})
	}
}

// A name wider than the terminal is printed whole. Truncating it would save a
// line and cost the reader the only thing the line is for.
func TestColumnsNeverTruncate(t *testing.T) {
	long := "internal/remote/an-extremely-long-file-name.md"

	got := columns([]string{long}, reportIndent, 20)
	if !strings.Contains(got, long) {
		t.Errorf("got %q, want the whole name", got)
	}
}

// The address is printed once, in the header. A path that does not sit under it
// keeps its own name rather than being mangled to fit the rule.
func TestShorten(t *testing.T) {
	cases := []struct {
		name   string
		target string
		rel    string
		want   string
	}{
		{"under the address", "epics/fitness", "epics/fitness/running.md", "running.md"},
		{"under a bulb", "epics", "epics/fitness/running.md", "fitness/running.md"},
		{"a project address trims its resource folder", "epics/fitness/running", "epics/fitness/running/plan.pdf", "plan.pdf"},
		{"and the project file itself, via the area", "epics/fitness/running", "epics/fitness/running.md", "running.md"},
		{"somewhere else entirely", "epics/fitness", "decks/ggml/notes.md", "decks/ggml/notes.md"},
		{"the whole remote trims nothing", "", "epics/fitness/running.md", "epics/fitness/running.md"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shorten([]string{c.rel}, c.target)
			if got[0] != c.want {
				t.Errorf("shorten(%q, %q) = %q, want %q", c.rel, c.target, got[0], c.want)
			}
		})
	}
}

// Colour lives on the headings and nowhere near a file name, and a report that
// is not going to a terminal carries no escapes at all -- which is what the
// assertions in this file are quietly relying on.
func TestReportIsPlainWhenPiped(t *testing.T) {
	p := Plan{Take: []string{"epics/fitness/running.md"}}

	got := renderReportAt("status", "epics/fitness", "host", p, 80)
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("piped report carries escape codes:\n%q", got)
	}
}

// One long name among short ones must not set the cell width for all of them.
// Uniform columns would fit a single column here, which is the layout the grid
// exists to avoid.
func TestColumnsMeasureEachColumnSeparately(t *testing.T) {
	names := []string{"internal/remote/report.go", "a.md", "b.md", "c.md"}

	got := columns(names, reportIndent, 40)
	if lines := strings.Count(got, "\n"); lines != 2 {
		t.Errorf("got %d lines, want 2 columns of 2:\n%s", lines, got)
	}
}

// A report of the whole remote covers several bulbs, and a flat list of paths
// hides where one ends and the next begins.
func TestReportSplitsByBulb(t *testing.T) {
	p := Plan{
		Take: []string{"epics/bioz/mealprep.md", "scripts/garlic/main.go"},
		Park: []string{"epics/bioz/drills.md"},
	}

	got := renderReportAt("status", "", "host", p, 100)
	for _, want := range []string{"epics", "scripts", "bioz/mealprep.md", "garlic/main.go"} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	// Park holds one bulb, but the report as a whole spans two: the shape has
	// to stay the same down the page or it reads as a distinction.
	if strings.Count(got, "epics") != 2 {
		t.Errorf("every bucket should be split, not just the mixed one:\n%s", got)
	}
	if strings.Contains(got, "epics/bioz/mealprep.md") {
		t.Errorf("the bulb heading is there to be trimmed off the names:\n%s", got)
	}
}

// One bulb needs no headings -- the address at the top already named it.
func TestReportWithinOneBulbIsNotSplit(t *testing.T) {
	p := Plan{Take: []string{"epics/bioz/mealprep.md", "epics/work/report.md"}}

	got := renderReportAt("status", "epics", "host", p, 100)
	if strings.Count(got, "epics") != 1 {
		t.Errorf("a single bulb should be named once, in the header:\n%s", got)
	}
}

// The frame is drawn to the header it sits under, not to the whole terminal.
func TestRuleFollowsTheHeader(t *testing.T) {
	p := Plan{Take: []string{"epics/bioz/mealprep.md"}}

	got := renderReportAt("status", "epics", "host", p, 200)
	header := strings.Split(got, "\n")[0]
	rule := strings.Split(got, "\n")[1]

	if want := max(lipgloss.Width(header), ruleFloor); lipgloss.Width(rule) != want {
		t.Errorf("rule is %d wide, want %d", lipgloss.Width(rule), want)
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), rule) {
		t.Errorf("the report should close with the same rule it opened with:\n%s", got)
	}
}
