package remote

import (
	"reflect"
	"testing"
	"time"
)

var noon = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// Fold turns "what each remote admits to holding" into "who holds this path".
// It is the whole of a check that is worth testing: the rest is one cat.
func TestFold(t *testing.T) {
	cases := []struct {
		name  string
		order []string
		seen  map[string]Manifest
		want  map[string][]string
	}{
		{
			"one remote, one path",
			[]string{"agent"},
			map[string]Manifest{"agent": {"epics/bioz/mealprep.md": "A"}},
			map[string][]string{"epics/bioz/mealprep.md": {"agent"}},
		},
		{
			"a path on two remotes names both, in config order",
			[]string{"agent", "berta"},
			map[string]Manifest{
				"berta": {"epics/bioz/mealprep.md": "B"},
				"agent": {"epics/bioz/mealprep.md": "A"},
			},
			map[string][]string{"epics/bioz/mealprep.md": {"agent", "berta"}},
		},
		{
			"hashes are irrelevant: a check asks where, not whether it moved",
			[]string{"agent"},
			map[string]Manifest{"agent": {"a.md": "X", "b.md": "Y"}},
			map[string][]string{"a.md": {"agent"}, "b.md": {"agent"}},
		},
		{
			"a remote that answered with nothing still answered",
			[]string{"agent"},
			map[string]Manifest{"agent": {}},
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
	got := Fold(noon, []string{"agent", "berta"}, map[string]Manifest{
		"agent": {"a.md": "A"},
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

	answered := Fold(noon, []string{"agent"}, map[string]Manifest{"agent": {}})
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
	s := Fold(noon, []string{"agent"}, map[string]Manifest{
		"agent": {"epics/bioz/mealprep.md": "A"},
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
