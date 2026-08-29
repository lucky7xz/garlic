package remote

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var noon = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// held is a remote holding these paths, with no planting times recorded -- the
// shape of a manifest written before garlic kept them.
func held(rels ...string) Baseline {
	b := Baseline{Hashes: Manifest{}, Planted: Plantings{}}
	for i, rel := range rels {
		b.Hashes[rel] = fmt.Sprintf("hash%d", i)
	}
	return b
}

// Fold turns "what each remote admits to holding" into "who holds this path".
// It is the whole of a check that is worth testing: the rest is one cat.
func TestFold(t *testing.T) {
	cases := []struct {
		name  string
		order []string
		seen  map[string]Baseline
		want  map[string][]string
	}{
		{
			"one remote, one path",
			[]string{"agent"},
			map[string]Baseline{"agent": held("epics/bioz/mealprep.md")},
			map[string][]string{"epics/bioz/mealprep.md": {"agent"}},
		},
		{
			"a path on two remotes names both, in config order",
			[]string{"agent", "berta"},
			map[string]Baseline{
				"berta": held("epics/bioz/mealprep.md"),
				"agent": held("epics/bioz/mealprep.md"),
			},
			map[string][]string{"epics/bioz/mealprep.md": {"agent", "berta"}},
		},
		{
			"hashes are irrelevant: a check asks where, not whether it moved",
			[]string{"agent"},
			map[string]Baseline{"agent": held("a.md", "b.md")},
			map[string][]string{"a.md": {"agent"}, "b.md": {"agent"}},
		},
		{
			"a remote that answered with nothing still answered",
			[]string{"agent"},
			map[string]Baseline{"agent": held()},
			map[string][]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Fold(noon, tc.order, tc.seen)
			if !reflect.DeepEqual(got.Where, tc.want) {
				t.Errorf("Where = %v, want %v", got.Where, tc.want)
			}
			if !reflect.DeepEqual(got.Hosts, tc.order) {
				t.Errorf("Hosts = %v, want %v", got.Hosts, tc.order)
			}
			if !got.When.Equal(noon) {
				t.Errorf("When = %v, want %v", got.When, noon)
			}
		})
	}
}

// A remote that never answered must not be claimed as checked, or the header
// would say "checked" about a host that was down.
func TestFoldSkipsHostsThatDidNotAnswer(t *testing.T) {
	got := Fold(noon, []string{"agent", "berta"}, map[string]Baseline{
		"agent": held("a.md"),
	})

	if !reflect.DeepEqual(got.Hosts, []string{"agent"}) {
		t.Errorf("Hosts = %v, want only the host that answered", got.Hosts)
	}
	if !reflect.DeepEqual(got.Where["a.md"], []string{"agent"}) {
		t.Errorf("a dead remote lost the live one's paths: %v", got.Where)
	}
}

// Checked is what the board keys off to tell "I have not asked" apart from
// "I asked and nothing is out there". Those must not collapse -- and neither
// may be confused with "I asked and nobody answered".
func TestSightingChecked(t *testing.T) {
	if (Sighting{}).Checked() {
		t.Error("the zero Sighting must not claim to have been checked")
	}

	answered := Fold(noon, []string{"agent"}, map[string]Baseline{"agent": held()})
	if !answered.Checked() {
		t.Error("a remote that answered with an empty manifest is still an answer")
	}

	// Every host unreachable: Check still returns a Sighting so the caller can
	// report the failure, but it must not be mistaken for an empty remote.
	silent := Fold(noon, []string{"agent", "berta"}, nil)
	if silent.Checked() {
		t.Error("a check nobody answered must not stamp the board as checked")
	}
}

func TestSightingOn(t *testing.T) {
	s := Fold(noon, []string{"agent"}, map[string]Baseline{
		"agent": held("epics/bioz/mealprep.md"),
	})

	if got := s.On("epics/bioz/mealprep.md"); !reflect.DeepEqual(got, []string{"agent"}) {
		t.Errorf("On(planted) = %v, want [agent]", got)
	}
	if got := s.On("epics/bioz/sleeplog.md"); got != nil {
		t.Errorf("On(unplanted) = %v, want nil", got)
	}
	if got := (Sighting{}).On("anything"); got != nil {
		t.Errorf("On before any check = %v, want nil", got)
	}
}

// Check needs somewhere to look. Refusing by name beats reporting an empty
// board as though the remotes had answered with nothing.
func TestCheckWithoutRemotes(t *testing.T) {
	got, err := Check(nil)
	if err == nil {
		t.Fatal("expected an error when no remotes are configured")
	}
	if got.Checked() {
		t.Error("a refused check must not stamp the board as checked")
	}
}

// Harvest works at area granularity, so the board marks areas too. "bio" must
// not match "bioz": comparing prefixes without the separator marks the wrong
// column and, on the harvest side, collects a tree nobody sent.
func TestSightingUnder(t *testing.T) {
	s := Fold(noon, []string{"agent"}, map[string]Baseline{
		"agent": held("epics/bioz/mealprep.md"),
	})

	cases := []struct {
		prefix string
		want   bool
	}{
		{"epics/bioz", true},
		{"epics/bio", false},
		{"epics/biozz", false},
		{"scripts/garlic", false},
	}

	for _, c := range cases {
		if got := s.Under(c.prefix); got != c.want {
			t.Errorf("Under(%q) = %v, want %v", c.prefix, got, c.want)
		}
	}

	if (Sighting{}).Under("epics/bioz") {
		t.Error("nothing is planted before a check has run")
	}
}

// The age shown on the board is the planting time, not the check time. Across
// several remotes the earliest wins, so it reads "out there at least this long".
func TestSightingPlantedAt(t *testing.T) {
	earlier := noon.Add(-48 * time.Hour)

	stamped := func(rel string, at time.Time) Baseline {
		return Baseline{Hashes: Manifest{rel: "A"}, Planted: Plantings{rel: at}}
	}

	s := Fold(noon, []string{"agent", "berta"}, map[string]Baseline{
		"agent": stamped("epics/bioz/mealprep.md", noon),
		"berta": stamped("epics/bioz/mealprep.md", earlier),
	})

	if got := s.PlantedAt("epics/bioz/mealprep.md"); !got.Equal(earlier) {
		t.Errorf("PlantedAt = %v, want the earliest (%v)", got, earlier)
	}

	// A manifest with no times, or a path nobody planted, has no age to report.
	old := Fold(noon, []string{"agent"}, map[string]Baseline{"agent": held("a.md")})
	if !old.PlantedAt("a.md").IsZero() {
		t.Error("invented an age for a manifest that records no times")
	}
	if !s.PlantedAt("nothing/here.md").IsZero() {
		t.Error("invented an age for a path nobody planted")
	}
}
