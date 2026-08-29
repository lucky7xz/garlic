package remote

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lucky7xz/garlic/internal/domain"
)

// Sighting is what one check saw: which remotes answered, and every path they
// admit to holding. It is a value held for the life of a session and never
// written down -- garlic does not remember what is planted, it goes and looks.
//
// The zero Sighting means nobody has asked yet, which is a different thing from
// having asked and found nothing.
type Sighting struct {
	When  time.Time
	Hosts []string             // remotes that answered, in config order
	Where map[string][]string  // manifest path -> remotes holding it
	Since map[string]time.Time // manifest path -> when it was first handed over
}

// Checked reports whether at least one remote actually answered. A check where
// every host was unreachable is not a check: stamping the board with it would
// claim "nothing is planted" when the truth is "nobody said". The zero Sighting
// has no hosts either, so this covers "you have not asked yet" too.
func (s Sighting) Checked() bool { return len(s.Hosts) > 0 }

// On names the remotes holding a path, or nil for one that is not planted.
func (s Sighting) On(rel string) []string { return s.Where[rel] }

// PlantedAt says when a path went out, or the zero time when nothing recorded
// it -- a manifest written before garlic kept times, or a file that arrived by
// harvest and was never planted. Across several remotes it is the earliest, so
// the answer reads "it has been out there at least this long".
func (s Sighting) PlantedAt(rel string) time.Time { return s.Since[rel] }

// Under reports whether anything is planted inside a category. Harvest decides
// at that granularity -- planting into an area puts the whole area in play --
// so this is the same question the collect rule asks, and the reason the board
// marks columns as well as cards.
//
// The separator is part of the comparison: without it "epics/bio" would match
// "epics/bioz/mealprep.md" and mark the wrong column.
func (s Sighting) Under(category string) bool {
	for rel := range s.Where {
		if strings.HasPrefix(rel, category+"/") {
			return true
		}
	}
	return false
}

// Fold turns each remote's manifest into the answer the board wants: for a
// given path, who has it. Hashes are dropped on the way through -- a check asks
// where a file is, never whether it moved. That question is `status`.
//
// Only remotes present in seen made it into Hosts: a host that never answered
// must not be reported as checked.
func Fold(when time.Time, order []string, seen map[string]Baseline) Sighting {
	s := Sighting{When: when, Where: map[string][]string{}, Since: map[string]time.Time{}}

	for _, name := range order {
		base, answered := seen[name]
		if !answered {
			continue
		}
		s.Hosts = append(s.Hosts, name)

		for rel := range base.Hashes {
			s.Where[rel] = append(s.Where[rel], name)

			// Earliest wins: with the same work on two remotes, what you want to
			// know is how long it has been out, not when it last went.
			at := base.Planted[rel]
			if at.IsZero() {
				continue
			}
			if got, ok := s.Since[rel]; !ok || at.Before(got) {
				s.Since[rel] = at
			}
		}
	}

	// Fold walks remotes in config order, but a manifest's own paths arrive in
	// map order, so the per-path lists need no sorting -- only determinism for
	// the paths themselves is left to callers that print them.
	for _, hosts := range s.Where {
		sort.SliceStable(hosts, func(i, j int) bool {
			return indexOf(order, hosts[i]) < indexOf(order, hosts[j])
		})
	}
	return s
}

func indexOf(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return len(names)
}

// Check asks every configured remote what it is holding. That is one `cat` of
// the manifest each: unlike status, a check does not need a census, because it
// asks what is out there rather than who moved it.
//
// A remote that fails to answer does not discard the ones that did. The error
// names what went wrong and is returned alongside a usable Sighting.
func Check(remotes []domain.Remote) (Sighting, error) {
	if len(remotes) == 0 {
		return Sighting{}, fmt.Errorf("no remotes configured: add a [[remote]] block to your config.toml")
	}

	order := make([]string, 0, len(remotes))
	seen := make(map[string]Baseline, len(remotes))
	var failures []error

	for _, r := range remotes {
		order = append(order, r.Name)

		base, err := readPlanting(r)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", r.Name, err))
			continue
		}
		seen[r.Name] = base
	}

	return Fold(time.Now(), order, seen), errors.Join(failures...)
}

// readPlanting fetches one remote's manifest. No manifest is not a failure: it
// is a remote that has been wiped, or never planted to, and holds nothing.
func readPlanting(r domain.Remote) (Baseline, error) {
	c, err := dial(r)
	if err != nil {
		return Baseline{}, err
	}

	base, planted, err := c.readManifest()
	if err != nil {
		return Baseline{}, err
	}
	if !planted {
		return Baseline{}, nil
	}
	return base, nil
}
