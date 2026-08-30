package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
)

func readTags(t *testing.T, path string) bool {
	t.Helper()
	return filesystem.GetTags(path).Hidden
}

// hiddenBoard: one hidden project with resources, one visible beside it, and a
// hidden one in another area.
func hiddenBoard(t *testing.T) domain.BoardOptions {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("epics/fitness/running.md", "#statustag-toDo\n")
	write("epics/fitness/running/plan.pdf", "pdf")
	write("epics/fitness/swimming.md", "#garlic-hide\n#statustag-toDo\n")
	write("epics/fitness/swimming/log.csv", "1,2\n")
	write("epics/fitness/swimming-log.md", "#statustag-toDo\n")
	write("epics/learning/golang.md", "#garlic-hide\n#statustag-toDo\n")

	return domain.BoardOptions{
		Path:                filepath.Join(root, "epics"),
		Name:                "epics",
		Extension:           ".md",
		Statuses:            []string{"toDo"},
		ShowEmptyCategories: true,
	}
}

func TestHiddenUnder(t *testing.T) {
	bulb := hiddenBoard(t)
	hidden := hiddenUnder(bulb)

	cases := []struct {
		rel  string
		want bool
	}{
		{"epics/fitness/swimming.md", true},
		{"epics/fitness/swimming/log.csv", true},
		{"epics/learning/golang.md", true},

		{"epics/fitness/running.md", false},
		{"epics/fitness/running/plan.pdf", false},
		// A prefix test without the separator would swallow this one.
		{"epics/fitness/swimming-log.md", false},
	}

	for _, c := range cases {
		if got := isHidden(c.rel, hidden); got != c.want {
			t.Errorf("isHidden(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

// Plant must filter BOTH sides. Filtering only the local one leaves Classify
// seeing planted + no-local + remote-present, which it calls LocalGone -- so
// plant would report a hidden project as "gone from your side". It is not gone.
func TestHiddenPlantedThenHiddenIsNotReportedGone(t *testing.T) {
	bulb := hiddenBoard(t)
	hidden := hiddenUnder(bulb)
	addr := Address{Bulb: "epics", Area: "fitness"}

	// swimming.md was planted before it was hidden, and is still on the remote.
	manifest := Manifest{
		"epics/fitness/running.md":  "A",
		"epics/fitness/swimming.md": "B",
	}
	remote := Census{
		"epics/fitness/running.md":  "A",
		"epics/fitness/swimming.md": "B",
	}
	local := Census{"epics/fitness/running.md": "A"} // hidden ones filtered out

	both := PlantPlan(Classify(
		Manifest(scope(Census(manifest), addr, bulb)),
		local,
		visible(scope(remote, addr, bulb), hidden),
	))
	if len(both.LocalGone) != 0 {
		t.Errorf("plant claims %v is gone from your side; it is hidden", both.LocalGone)
	}
	if !both.Empty() {
		t.Errorf("plant found something to say about a hidden project: %+v", both)
	}

	// And the proof that filtering both sides is what does it: filter only the
	// local side and the false report comes straight back.
	onlyLocal := PlantPlan(Classify(
		Manifest(scope(Census(manifest), addr, bulb)),
		local,
		scope(remote, addr, bulb),
	))
	if len(onlyLocal.LocalGone) == 0 {
		t.Error("expected the wrong behaviour this test guards against")
	}
}

// Harvest keeps comparing hidden projects, or it could never ask about them.
func TestHiddenStillReachesTheHarvestPlan(t *testing.T) {
	bulb := hiddenBoard(t)
	addr := Address{Bulb: "epics", Area: "fitness"}

	all, err := bulbCensus(bulb)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{"epics/fitness/swimming.md": all["epics/fitness/swimming.md"]}
	remote := Census{"epics/fitness/swimming.md": "agent-changed-it"}

	p, _ := reckon(bulb, addr, manifest, remote, all)

	if len(p.Take) != 1 || p.Take[0] != "epics/fitness/swimming.md" {
		t.Fatalf("Take = %v, want the hidden project the agent changed", p.Take)
	}
	if !isHidden(p.Take[0], hiddenUnder(bulb)) {
		t.Error("the plan entry is not recognised as hidden, so nothing would be asked")
	}
}

// Declining leaves the baseline un-advanced, so the question returns.
func TestWithoutTakesDropsOnlyTheNamedPaths(t *testing.T) {
	p := Plan{Take: []string{"a.md", "b.md", "c.md"}, Park: []string{"d.md"}}
	got := withoutTakes(p, []string{"b.md"})

	if len(got.Take) != 2 || got.Take[0] != "a.md" || got.Take[1] != "c.md" {
		t.Errorf("Take = %v, want a.md and c.md", got.Take)
	}
	if len(got.Park) != 1 {
		t.Errorf("Park was disturbed: %v", got.Park)
	}
}

// THE bug this change closes: the agent's copy never carried the marker, so a
// plain collect would return the project to the visible board without saying so.
func TestRehidePutsTheMarkerBack(t *testing.T) {
	bulb := hiddenBoard(t)
	swimming := filepath.Join(bulb.Path, "fitness", "swimming.md")

	// Simulate the collect: the agent's version, with no marker.
	if err := os.WriteFile(swimming, []byte("#statustag-toDo\nagent edited this\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := readTags(t, swimming); got {
		t.Fatal("the fixture is wrong: the marker is still there")
	}

	if err := rehide(bulb, []string{"epics/fitness/swimming.md"}); err != nil {
		t.Fatalf("rehide failed: %v", err)
	}
	if !readTags(t, swimming) {
		body, _ := os.ReadFile(swimming)
		t.Errorf("harvest silently un-hid the project:\n%s", body)
	}

	// Idempotent: running it again must not toggle it back off.
	if err := rehide(bulb, []string{"epics/fitness/swimming.md"}); err != nil {
		t.Fatal(err)
	}
	if !readTags(t, swimming) {
		t.Error("a second rehide un-hid it")
	}

	// And the agent's content survived.
	body, err := os.ReadFile(swimming)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "agent edited this") {
		t.Errorf("rehide lost the collected content:\n%s", body)
	}
}
