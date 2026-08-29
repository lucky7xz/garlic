package remote

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

func epicsBulb() domain.BoardOptions {
	return domain.BoardOptions{Name: "epics", Path: "/x/epics", Extension: ".md"}
}

// A wipe takes everything in the folder it names, harvested or not: the census
// is raw, so an ignore list shields nothing.
func TestDoomedFiles(t *testing.T) {
	remote := Census{
		"epics/bioz/mealprep.md":         "A",
		"epics/bioz/mealprep/log.csv":    "B",
		"epics/bioz/mealprep/scratch.md": "C", // the agent's, never harvested
		"epics/bioz/sleeplog.md":         "D",
		"epics/work/report.md":           "E",
		"scripts/garlic/main.go":         "F",
	}
	found := map[string]Baseline{
		"epics": {Hashes: Manifest{
			"epics/bioz/mealprep.md":      "A",
			"epics/bioz/mealprep/log.csv": "B",
			"epics/bioz/sleeplog.md":      "D",
		}},
	}

	opts := []domain.BoardOptions{
		epicsBulb(),
		{Name: "scripts", Path: "/x/scripts", Extension: ".clove.md", WholeFolder: true},
	}

	cases := []struct {
		name      string
		addr      Address
		only      []domain.BoardOptions
		want      []string
		wantLoose []string
	}{
		{
			"a project takes its file and its folder",
			Address{Bulb: "epics", Area: "bioz", Project: "mealprep"},
			opts[:1],
			[]string{
				"epics/bioz/mealprep.md",
				"epics/bioz/mealprep/log.csv",
				"epics/bioz/mealprep/scratch.md",
			},
			[]string{"epics/bioz/mealprep/scratch.md"},
		},
		{
			"an area takes every project in it",
			Address{Bulb: "epics", Area: "bioz"},
			opts[:1],
			[]string{
				"epics/bioz/mealprep.md",
				"epics/bioz/mealprep/log.csv",
				"epics/bioz/mealprep/scratch.md",
				"epics/bioz/sleeplog.md",
			},
			[]string{"epics/bioz/mealprep/scratch.md"},
		},
		{
			"a bulb takes every area",
			Address{Bulb: "epics"},
			opts[:1],
			[]string{
				"epics/bioz/mealprep.md",
				"epics/bioz/mealprep/log.csv",
				"epics/bioz/mealprep/scratch.md",
				"epics/bioz/sleeplog.md",
				"epics/work/report.md",
			},
			[]string{"epics/bioz/mealprep/scratch.md", "epics/work/report.md"},
		},
		{
			"no address takes every bulb",
			Address{},
			opts,
			[]string{
				"epics/bioz/mealprep.md",
				"epics/bioz/mealprep/log.csv",
				"epics/bioz/mealprep/scratch.md",
				"epics/bioz/sleeplog.md",
				"epics/work/report.md",
				"scripts/garlic/main.go",
			},
			[]string{
				"epics/bioz/mealprep/scratch.md",
				"epics/work/report.md",
				"scripts/garlic/main.go",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, loose := doomedFiles(c.only, c.addr, remote, found)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("doomed: got  %v\nwant %v", got, c.want)
			}
			// Naming the unharvested files, not merely counting them: the
			// summary lists these, and listing the wrong ones would warn about
			// work that was in fact already collected.
			if !reflect.DeepEqual(loose, c.wantLoose) {
				t.Errorf("never harvested: got %v\nwant %v", loose, c.wantLoose)
			}
		})
	}
}

// "bio" is not "bioz". A wipe turns an address into a delete list, so a prefix
// compared without its separator would destroy a tree nobody named.
func TestDoomedFilesAreaIsAWholeSegment(t *testing.T) {
	remote := Census{
		"epics/bio/cardio.md":    "A",
		"epics/bioz/mealprep.md": "B",
	}

	got, _ := doomedFiles([]domain.BoardOptions{epicsBulb()},
		Address{Bulb: "epics", Area: "bio"}, remote, nil)

	if !reflect.DeepEqual(got, []string{"epics/bio/cardio.md"}) {
		t.Errorf("got %v, want only bio's own file", got)
	}
}

// Nothing there is a typo, not a no-op: rm on a path that does not exist looks
// like success, and you would believe you had wiped something.
func TestDoomedFilesEmptyForAnAddressThatNamesNothing(t *testing.T) {
	remote := Census{"epics/bioz/mealprep.md": "A"}

	got, _ := doomedFiles([]domain.BoardOptions{epicsBulb()},
		Address{Bulb: "epics", Area: "bioz", Project: "mealprepp"}, remote, nil)

	if len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

// The summary is what the gates are answered against, so it has to show the
// split and the level below -- six areas when you wanted one is the tell.
func TestRenderDoomed(t *testing.T) {
	doomed := []string{
		"epics/bioz/mealprep.md",
		"epics/bioz/mealprep/log.csv",
		"epics/bioz/mealprep/scratch.md",
		"epics/work/report.md",
	}
	loose := []string{"epics/bioz/mealprep/scratch.md", "epics/work/report.md"}

	got := renderDoomed(Address{Bulb: "epics"}, "berta:/home/berta/shara",
		doomed, loose, []domain.BoardOptions{epicsBulb()})

	for _, want := range []string{"4 files", "2 you planted", "2 never harvested", "areas: bioz, work"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary is missing %q:\n%s", want, got)
		}
	}

	// It must list the unharvested files and no others: naming one you already
	// collected would warn about work that is safely home.
	for _, rel := range loose {
		if !strings.Contains(got, rel) {
			t.Errorf("summary never names %q:\n%s", rel, got)
		}
	}
	for _, planted := range []string{"epics/bioz/mealprep.md", "epics/bioz/mealprep/log.csv"} {
		if strings.Contains(got, planted) {
			t.Errorf("summary lists %q as never harvested, but it was:\n%s", planted, got)
		}
	}
}

// The overflow line counts what is left of the unharvested files, not of every
// doomed file -- mixing the two produced "... and -2 more".
func TestRenderDoomedOverflowCountsTheRightThing(t *testing.T) {
	var doomed, loose []string
	for _, rel := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		doomed = append(doomed, "epics/bioz/"+rel+".md")
	}
	loose = doomed[:6]

	got := renderDoomed(Address{Bulb: "epics"}, "host", doomed, loose,
		[]domain.BoardOptions{epicsBulb()})

	if !strings.Contains(got, "... and 1 more") {
		t.Errorf("want one unlisted file reported:\n%s", got)
	}
	if strings.Contains(got, "-") {
		t.Errorf("negative overflow count:\n%s", got)
	}
}

// keptAfterWipe is pruneManifest's decision without its I/O: which entries
// survive a wipe at this address.
func keptAfterWipe(bulb domain.BoardOptions, addr Address, base Baseline) Baseline {
	if addr.Bulb == "" {
		addr.Bulb = bulb.Name
	}
	kept := Baseline{Hashes: Manifest{}, Planted: Plantings{}}
	for rel, hash := range base.Hashes {
		if inScope(rel, addr, bulb.Extension, bulb.WholeFolder) {
			continue
		}
		kept.Hashes[rel] = hash
		if at, ok := base.Planted[rel]; ok {
			kept.Planted[rel] = at
		}
	}
	return kept
}

func TestPruneManifestKeepsWhatItShould(t *testing.T) {
	base := Baseline{
		Hashes: Manifest{
			"epics/bioz/mealprep.md":      "A",
			"epics/bioz/mealprep/log.csv": "B",
			"epics/bioz/sleeplog.md":      "C",
			"epics/work/report.md":        "D",
		},
		Planted: Plantings{
			"epics/bioz/mealprep.md":      noon,
			"epics/bioz/mealprep/log.csv": noon,
			"epics/bioz/sleeplog.md":      noon,
			"epics/work/report.md":        noon,
		},
	}

	kept := keptAfterWipe(epicsBulb(), Address{Bulb: "epics", Area: "bioz", Project: "mealprep"}, base)

	want := []string{"epics/bioz/sleeplog.md", "epics/work/report.md"}
	if got := sortedKeys(kept.Hashes); !reflect.DeepEqual(got, want) {
		t.Errorf("hashes: got %v, want %v", got, want)
	}
	// Planting times must go with their hashes, or they pile up for files that
	// no longer exist.
	if got := sortedKeys(kept.Planted); !reflect.DeepEqual(got, want) {
		t.Errorf("planted: got %v, want %v", got, want)
	}
}

// scope also applies the ignore list, so pruning through it would strand the
// entries of anything ignore-listed after it was planted.
func TestPruneManifestReachesIgnoredEntries(t *testing.T) {
	bulb := epicsBulb()
	bulb.Ignore = []string{"dist"}

	base := Baseline{Hashes: Manifest{
		"epics/bioz/mealprep.md":          "A",
		"epics/bioz/mealprep/dist/out.js": "B", // planted before `dist` was ignored
	}}

	kept := keptAfterWipe(bulb, Address{Bulb: "epics", Area: "bioz", Project: "mealprep"}, base)
	if len(kept.Hashes) != 0 {
		t.Errorf("stranded %v where nothing could ever remove it", sortedKeys(kept.Hashes))
	}
}

// THE regression this feature exists for. Leave the manifest entries behind and
// Classify reads planted + local + no-remote as RemoteGone, which PlantPlan
// files under Gone -- so the replant refuses to re-send and prints "removed on
// remote (not re-sent)", silently breaking the wipe-then-replant workflow.
func TestWipedProjectCanBeReplanted(t *testing.T) {
	bulb := epicsBulb()
	addr := Address{Bulb: "epics", Area: "bioz", Project: "mealprep"}

	before := Baseline{Hashes: Manifest{
		"epics/bioz/mealprep.md": "A",
		"epics/bioz/sleeplog.md": "C",
	}}
	local := Census{
		"epics/bioz/mealprep.md": "A",
		"epics/bioz/sleeplog.md": "C",
	}

	// After the wipe: mealprep gone from the remote, and pruned from the manifest.
	after := keptAfterWipe(bulb, addr, before)
	remote := Census{"epics/bioz/sleeplog.md": "C"}

	p := PlantPlan(Classify(after.Hashes, local, remote))

	if !reflect.DeepEqual(p.Push, []string{"epics/bioz/mealprep.md"}) {
		t.Errorf("Push = %v, want the wiped project re-sent", p.Push)
	}
	if len(p.Gone) != 0 {
		t.Errorf("Gone = %v — the manifest was not pruned, so plant will not re-send", p.Gone)
	}

	// And the proof that pruning is what does it: leave the entries in place and
	// the replant refuses.
	stale := PlantPlan(Classify(before.Hashes, local, remote))
	if len(stale.Push) != 0 || !reflect.DeepEqual(stale.Gone, []string{"epics/bioz/mealprep.md"}) {
		t.Errorf("unpruned: Push=%v Gone=%v — expected the refusal this test guards against",
			stale.Push, stale.Gone)
	}
}
