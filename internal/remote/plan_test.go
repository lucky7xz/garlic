package remote

import (
	"reflect"
	"testing"
)

// Classify answers one question per file: who moved it since it was planted?
// Every plant and harvest rule is a reading of that answer.
func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		manifest Manifest
		local    Census
		remote   Census
		want     map[string]Movement
	}{
		{
			"nobody touched it",
			Manifest{"a.md": "A"},
			Census{"a.md": "A"},
			Census{"a.md": "A"},
			map[string]Movement{"a.md": Still},
		},
		{
			"the agent worked, you did not",
			Manifest{"a.md": "A"},
			Census{"a.md": "A"},
			Census{"a.md": "C"},
			map[string]Movement{"a.md": RemoteMoved},
		},
		{
			"you worked, the agent did not",
			Manifest{"a.md": "A"},
			Census{"a.md": "D"},
			Census{"a.md": "A"},
			map[string]Movement{"a.md": LocalMoved},
		},
		{
			"both moved",
			Manifest{"a.md": "A"},
			Census{"a.md": "D"},
			Census{"a.md": "C"},
			map[string]Movement{"a.md": BothMoved},
		},
		{
			"the agent deleted it",
			Manifest{"a.md": "A"},
			Census{"a.md": "A"},
			Census{},
			map[string]Movement{"a.md": RemoteGone},
		},
		{
			"you deleted it",
			Manifest{"a.md": "A"},
			Census{},
			Census{"a.md": "A"},
			map[string]Movement{"a.md": LocalGone},
		},
		{
			"gone from both sides",
			Manifest{"a.md": "A"},
			Census{},
			Census{},
			map[string]Movement{"a.md": Still},
		},
		{
			// Deleting locally must not freeze the file: if the agent works on
			// it afterwards, that is new work and it has to be collectable.
			"you deleted it, then the agent worked on it",
			Manifest{"a.md": "A"},
			Census{},
			Census{"a.md": "C"},
			map[string]Movement{"a.md": RemoteMoved},
		},
		{
			"the agent created it",
			Manifest{},
			Census{},
			Census{"new.md": "E"},
			map[string]Movement{"new.md": RemoteNew},
		},
		{
			"you created it, never planted",
			Manifest{},
			Census{"new.md": "E"},
			Census{},
			map[string]Movement{"new.md": LocalNew},
		},
		{
			"present both sides with no baseline is contested",
			Manifest{},
			Census{"a.md": "D"},
			Census{"a.md": "C"},
			map[string]Movement{"a.md": BothMoved},
		},
		{
			"present both sides, identical, no baseline",
			Manifest{},
			Census{"a.md": "A"},
			Census{"a.md": "A"},
			map[string]Movement{"a.md": Still},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.manifest, c.local, c.remote)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyCoversEverySeenPath(t *testing.T) {
	got := Classify(
		Manifest{"planted.md": "A"},
		Census{"planted.md": "A", "mine.md": "B"},
		Census{"planted.md": "A", "theirs.md": "C"},
	)

	want := map[string]Movement{
		"planted.md": Still,
		"mine.md":    LocalNew,
		"theirs.md":  RemoteNew,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	// Dots in filenames are the hazard here: an unquoted TOML key would split
	// "running.md" into nested tables.
	want := Baseline{
		Hashes: Manifest{
			"epics/fitness/running.md":       "a3f9",
			"epics/fitness/running/plan.pdf": "7c21",
			"scripts/drako/revise.clove.md":  "0001",
		},
		Planted: Plantings{
			"epics/fitness/running.md":       noon,
			"epics/fitness/running/plan.pdf": noon,
			"scripts/drako/revise.clove.md":  noon,
		},
	}

	data, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v\n%s", err, data)
	}
	if !reflect.DeepEqual(got.Hashes, want.Hashes) {
		t.Errorf("hashes: got %v, want %v\nencoded as:\n%s", got.Hashes, want.Hashes, data)
	}
	for rel, when := range want.Planted {
		if !got.Planted[rel].Equal(when) {
			t.Errorf("planted[%q]: got %v, want %v\nencoded as:\n%s", rel, got.Planted[rel], when, data)
		}
	}
}

// A manifest written before garlic recorded times still reads: the hashes are
// what every rule compares, and an entry with no time simply has no age to show.
func TestManifestWithoutTimes(t *testing.T) {
	got, err := DecodeManifest([]byte("[files]\n\"epics/bioz/mealprep.md\" = \"a3f9\"\n"))
	if err != nil {
		t.Fatalf("DecodeManifest failed: %v", err)
	}
	if got.Hashes["epics/bioz/mealprep.md"] != "a3f9" {
		t.Errorf("hashes lost: %v", got.Hashes)
	}
	if !got.Planted["epics/bioz/mealprep.md"].IsZero() {
		t.Errorf("invented a planting time: %v", got.Planted)
	}
}

func TestDecodeManifestRejectsGarbage(t *testing.T) {
	if _, err := DecodeManifest([]byte("this is not toml {{{")); err == nil {
		t.Error("DecodeManifest accepted garbage, want error")
	}
}

func TestDecodeEmptyManifest(t *testing.T) {
	got, err := DecodeManifest(nil)
	if err != nil {
		t.Fatalf("DecodeManifest(nil) failed: %v", err)
	}
	if len(got.Hashes) != 0 || len(got.Planted) != 0 {
		t.Errorf("got %+v, want empty", got)
	}
}

func TestParseSums(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want Census
	}{
		{
			"empty output",
			"",
			Census{},
		},
		{
			"one file",
			"a3f9  ./epics/fitness/running.md\n",
			Census{"epics/fitness/running.md": "a3f9"},
		},
		{
			"several files",
			"a3f9  ./epics/fitness/running.md\n7c21  ./epics/fitness/running/plan.pdf\n",
			Census{
				"epics/fitness/running.md":       "a3f9",
				"epics/fitness/running/plan.pdf": "7c21",
			},
		},
		{
			"path containing spaces",
			"a3f9  ./epics/fitness/my long name.md\n",
			Census{"epics/fitness/my long name.md": "a3f9"},
		},
		{
			"binary-mode marker",
			"a3f9 *./epics/fitness/plan.pdf\n",
			Census{"epics/fitness/plan.pdf": "a3f9"},
		},
		{
			"blank lines are skipped",
			"\na3f9  ./a.md\n\n",
			Census{"a.md": "a3f9"},
		},
		{
			// GNU coreutils flags an escaped line with a leading backslash and
			// writes \\ for a backslash and \n for a newline in the name.
			"escaped backslash in name",
			"\\a3f9  ./epics/weird\\\\name.md\n",
			Census{"epics/weird\\name.md": "a3f9"},
		},
		{
			"escaped newline in name",
			"\\a3f9  ./epics/two\\nlines.md\n",
			Census{"epics/two\nlines.md": "a3f9"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseSums([]byte(c.out))
			if err != nil {
				t.Fatalf("parseSums failed: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseSumsRejectsMalformedLines(t *testing.T) {
	if _, err := parseSums([]byte("not-a-sum-line\n")); err == nil {
		t.Error("parseSums accepted a line with no separator, want error")
	}
}

func TestHarvestPlan(t *testing.T) {
	visible := func(p string) bool { return p != "epics/fitness/notes.txt" }

	moves := map[string]Movement{
		"still.md":                Still,
		"agent-worked.md":         RemoteMoved,
		"i-worked.md":             LocalMoved,
		"both-worked.md":          BothMoved,
		"agent-made.md":           RemoteNew,
		"epics/fitness/notes.txt": RemoteNew,
		"agent-deleted.md":        RemoteGone,
		"i-deleted.md":            LocalGone,
		"never-planted.md":        LocalNew,
	}

	got := HarvestPlan(moves, visible)
	want := Plan{
		Take: []string{"agent-made.md", "agent-worked.md"},
		Park: []string{"both-worked.md"},
		Left: []string{"epics/fitness/notes.txt"},
		Gone: []string{"agent-deleted.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestPlantPlan(t *testing.T) {
	moves := map[string]Movement{
		"still.md":         Still,
		"i-worked.md":      LocalMoved,
		"never-planted.md": LocalNew,
		"agent-worked.md":  RemoteMoved,
		"both-worked.md":   BothMoved,
		"agent-deleted.md": RemoteGone,
		"i-deleted.md":     LocalGone,
		"agent-made.md":    RemoteNew,
	}

	got := PlantPlan(moves)
	want := Plan{
		Push:      []string{"i-worked.md", "never-planted.md"},
		Blocked:   []string{"agent-worked.md", "both-worked.md"},
		Gone:      []string{"agent-deleted.md"},
		LocalGone: []string{"i-deleted.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestPlanEmptyWhenNothingMoved(t *testing.T) {
	moves := map[string]Movement{"a.md": Still, "b.md": Still}

	if h := HarvestPlan(moves, func(string) bool { return true }); !h.Empty() {
		t.Errorf("harvest plan should be empty, got %+v", h)
	}
	if p := PlantPlan(moves); !p.Empty() {
		t.Errorf("plant plan should be empty, got %+v", p)
	}
}

func TestInScope(t *testing.T) {
	cases := []struct {
		rel  string
		addr Address
		want bool
	}{
		// whole remote
		{"epics/fitness/running.md", Address{}, true},
		{"scripts/drako/revise.clove.md", Address{}, true},

		// bulb
		{"epics/fitness/running.md", Address{Bulb: "epics"}, true},
		{"scripts/drako/x.md", Address{Bulb: "epics"}, false},

		// area
		{"epics/fitness/running.md", Address{Bulb: "epics", Area: "fitness"}, true},
		{"epics/learning/golang.md", Address{Bulb: "epics", Area: "fitness"}, false},
		{"epics/fitness", Address{Bulb: "epics", Area: "fitness"}, false},

		// project: the file and its resource folder, nothing else
		{"epics/fitness/running.md", Address{"epics", "fitness", "running"}, true},
		{"epics/fitness/running/plan.pdf", Address{"epics", "fitness", "running"}, true},
		{"epics/fitness/running/logs/day1.txt", Address{"epics", "fitness", "running"}, true},
		{"epics/fitness/hiking.md", Address{"epics", "fitness", "running"}, false},
		{"epics/fitness/running-notes.md", Address{"epics", "fitness", "running"}, false},
	}

	for _, c := range cases {
		if got := inScope(c.rel, c.addr, ".md", false); got != c.want {
			t.Errorf("inScope(%q, %+v) = %v, want %v", c.rel, c.addr, got, c.want)
		}
	}
}

// Nothing comes home that was not planted for. The unit is the area: sending a
// project into one puts the whole area in play, so the agent can add work where
// you sent it -- but an area you never planted into stays where it is, however
// much it looks like a board.
func TestVisibilityAllows(t *testing.T) {
	v := visibility{
		Bulb:    "epics",
		Ignore:  []string{"dist"},
		Planted: Manifest{"epics/fitness/running.md": "A"},
	}

	cases := []struct {
		name string
		rel  string
		want bool
	}{
		{"the project that was planted", "epics/fitness/running.md", true},
		{"its resource folder", "epics/fitness/running/day1.txt", true},
		{"nested inside its resource folder", "epics/fitness/running/logs/day1.txt", true},

		// The area was sent, so new work in it belongs -- whatever it looks like.
		{"a project the agent created in a planted area", "epics/fitness/swimming.md", true},
		{"that project's own resources", "epics/fitness/swimming/notes.txt", true},
		{"an untagged file in a planted area", "epics/fitness/scratch.md", true},
		{"a non-markdown file in a planted area", "epics/fitness/notes.txt", true},

		// The reported bug: bio was never planted, bioz was.
		{"an area that was never planted", "epics/bio/cardio.md", false},
		{"under an area that was never planted", "epics/bio/cardio/dopamine.md", false},

		{"ignored, even inside a planted area", "epics/fitness/dist/bundle.js", false},
		{"loose file at bulb level", "epics/stray.md", false},
		{"file from another bulb", "scripts/drako/x.md", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.allows(c.rel); got != c.want {
				t.Errorf("allows(%q) = %v, want %v", c.rel, got, c.want)
			}
		})
	}
}

// "bio" must not match "bioz". This is the reported bug in miniature: comparing
// prefixes without the separator marks and harvests the wrong tree.
func TestVisibilityAreaIsAWholeSegment(t *testing.T) {
	v := visibility{Bulb: "epics", Planted: Manifest{"epics/bioz/mealprep.md": "A"}}

	if !v.allows("epics/bioz/cardio.md") {
		t.Error("bioz was planted, so its area is in play")
	}
	if v.allows("epics/bio/cardio.md") {
		t.Error("bio is a different area from bioz and was never planted")
	}
}
