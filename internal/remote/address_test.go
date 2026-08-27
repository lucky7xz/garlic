package remote

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

func TestParseCommand(t *testing.T) {
	cmd, err := ParseCommand([]string{"plant", "epics", "@", "agent"})
	if err != nil {
		t.Fatalf("ParseCommand failed: %v", err)
	}

	if cmd.Verb != "plant" {
		t.Errorf("verb: got %q, want %q", cmd.Verb, "plant")
	}
	if cmd.Remote != "agent" {
		t.Errorf("remote: got %q, want %q", cmd.Remote, "agent")
	}
	if cmd.Address.Bulb != "epics" {
		t.Errorf("bulb: got %q, want %q", cmd.Address.Bulb, "epics")
	}
	if cmd.Address.Area != "" || cmd.Address.Project != "" {
		t.Errorf("expected bulb-only address, got area=%q project=%q", cmd.Address.Area, cmd.Address.Project)
	}
}

func TestParseCommandAccepts(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want Command
	}{
		{
			"glued @",
			[]string{"plant", "scripts/drako", "@agent"},
			Command{"plant", Address{"scripts", "drako", ""}, "agent"},
		},
		{
			"project depth",
			[]string{"harvest", "epics/fitness/running", "@", "agent"},
			Command{"harvest", Address{"epics", "fitness", "running"}, "agent"},
		},
		{
			"status without address",
			[]string{"status", "@", "agent"},
			Command{"status", Address{}, "agent"},
		},
		{
			"status with address",
			[]string{"status", "epics/fitness", "@agent"},
			Command{"status", Address{"epics", "fitness", ""}, "agent"},
		},
		{
			"wipe takes no address",
			[]string{"wipe", "@", "agent"},
			Command{"wipe", Address{}, "agent"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCommand(c.args)
			if err != nil {
				t.Fatalf("ParseCommand(%v) failed: %v", c.args, err)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestParseCommandRejects(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no @ clause", []string{"plant", "epics"}},
		{"@ with no remote", []string{"plant", "epics", "@"}},
		{"glued @ with no remote", []string{"plant", "epics", "@"}},
		{"plant without address", []string{"plant", "@", "agent"}},
		{"harvest without address", []string{"harvest", "@", "agent"}},
		{"wipe with address", []string{"wipe", "epics", "@", "agent"}},
		{"address too deep", []string{"plant", "epics/fitness/running/extra", "@", "agent"}},
		{"empty address segment", []string{"plant", "epics//running", "@", "agent"}},
		{"unknown verb", []string{"frobnicate", "epics", "@", "agent"}},
		{"nothing at all", []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseCommand(c.args); err == nil {
				t.Errorf("ParseCommand(%v) succeeded, want error", c.args)
			}
		})
	}
}

func selectTestBoard(t *testing.T) []domain.BoardOptions {
	t.Helper()
	root := t.TempDir()

	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("epics/fitness/running.md", "#statustag-inProgress\n- [ ] log it #AT\n")
	write("epics/fitness/running/plan.pdf", "pdf")
	write("epics/fitness/running/logs/day1.txt", "ran")
	write("epics/fitness/hiking.md", "#statustag-toDo\n")
	write("epics/fitness/swimming.md", "#garlic-hide\n#statustag-toDo\n")
	write("epics/fitness/notes.txt", "not a project\n")
	write("epics/learning/golang.md", "#statustag-toDo\n")

	return []domain.BoardOptions{{
		Path:                filepath.Join(root, "epics"),
		Name:                "epics",
		Extension:           ".md",
		Statuses:            []string{"inProgress", "toDo"},
		ShowEmptyCategories: true,
	}}
}

func TestSelect(t *testing.T) {
	opts := selectTestBoard(t)

	cases := []struct {
		name string
		addr Address
		want []string
	}{
		{
			"whole bulb",
			Address{Bulb: "epics"},
			[]string{
				"epics/fitness/hiking.md",
				"epics/fitness/running.md",
				"epics/fitness/running/logs/day1.txt",
				"epics/fitness/running/plan.pdf",
				"epics/fitness/swimming.md",
				"epics/learning/golang.md",
			},
		},
		{
			"one area",
			Address{Bulb: "epics", Area: "fitness"},
			[]string{
				"epics/fitness/hiking.md",
				"epics/fitness/running.md",
				"epics/fitness/running/logs/day1.txt",
				"epics/fitness/running/plan.pdf",
				"epics/fitness/swimming.md",
			},
		},
		{
			"one project takes its resource folder",
			Address{Bulb: "epics", Area: "fitness", Project: "running"},
			[]string{
				"epics/fitness/running.md",
				"epics/fitness/running/logs/day1.txt",
				"epics/fitness/running/plan.pdf",
			},
		},
		{
			"project without a resource folder",
			Address{Bulb: "epics", Area: "fitness", Project: "hiking"},
			[]string{"epics/fitness/hiking.md"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files, err := Select(c.addr, opts)
			if err != nil {
				t.Fatalf("Select(%+v) failed: %v", c.addr, err)
			}
			var got []string
			for _, f := range files {
				got = append(got, f.Rel)
			}
			sort.Strings(got)
			if !slices.Equal(got, c.want) {
				t.Errorf("got  %v\nwant %v", got, c.want)
			}
		})
	}
}

func TestSelectLocalPathsPointAtRealFiles(t *testing.T) {
	opts := selectTestBoard(t)

	files, err := Select(Address{Bulb: "epics", Area: "fitness", Project: "running"}, opts)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	for _, f := range files {
		if _, err := os.Stat(f.Local); err != nil {
			t.Errorf("Local path %q for %q does not exist: %v", f.Local, f.Rel, err)
		}
	}
}

func TestSelectRejectsUnknownAddresses(t *testing.T) {
	opts := selectTestBoard(t)

	cases := []struct {
		name string
		addr Address
	}{
		{"unknown bulb", Address{Bulb: "nope"}},
		{"unknown area", Address{Bulb: "epics", Area: "nope"}},
		{"unknown project", Address{Bulb: "epics", Area: "fitness", Project: "nope"}},
		{"project that is not on the board", Address{Bulb: "epics", Area: "fitness", Project: "notes"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Select(c.addr, opts); err == nil {
				t.Errorf("Select(%+v) succeeded, want error", c.addr)
			}
		})
	}
}

// A parked copy is garlic's own scratch from resolving a conflict, not work of
// yours, so it must not be planted back at the machine it came from.
func TestSelectSkipsParkedCopies(t *testing.T) {
	opts := selectTestBoard(t)
	bulb := opts[0]

	parked := filepath.Join(bulb.Path, "fitness", "running", "running.remote.md")
	if err := os.WriteFile(parked, []byte("#statustag-inProgress\nthe agent's version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	alsoParked := filepath.Join(bulb.Path, "fitness", "running", "plan.remote.pdf")
	if err := os.WriteFile(alsoParked, []byte("pdf"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := Select(Address{Bulb: "epics", Area: "fitness", Project: "running"}, opts)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	for _, f := range files {
		if strings.Contains(f.Rel, ".remote.") {
			t.Errorf("parked copy %q should not be selected", f.Rel)
		}
	}
	if len(files) != 3 {
		var rels []string
		for _, f := range files {
			rels = append(rels, f.Rel)
		}
		t.Errorf("expected the project's own 3 files, got %v", rels)
	}
}
