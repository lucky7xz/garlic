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

// A semi bulb's category IS the project: a .clove.md marks the folder as in
// play, and the whole folder belongs to it.
func semiBulb(t *testing.T) domain.BoardOptions {
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

	write("scripts/garlic/revise.clove.md", "#statustag-inProgress\n")
	write("scripts/garlic/release.clove.md", "#statustag-onHold\n")
	write("scripts/garlic/main.go", "package main\n")
	write("scripts/garlic/internal/ui/tui.go", "package ui\n")
	write("scripts/garlic/.git/HEAD", "ref: refs/heads/main\n")
	write("scripts/garlic/.git/objects/ab/cdef", "binary\n")
	write("scripts/garlic/dist/garlic_linux/garlic", "elf\n")
	write("scripts/neofetch/neofetch.sh", "#!/bin/sh\n") // no clove: invisible

	return domain.BoardOptions{
		Path:        filepath.Join(root, "scripts"),
		Name:        "scripts",
		Extension:   ".clove.md",
		Statuses:    []string{"inProgress", "onHold"},
		WholeFolder: true,
		Ignore:      []string{"dist"},
	}
}

func rels(files []File) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Rel)
	}
	sort.Strings(out)
	return out
}

func TestBulbFilesTakesTheWholeFolder(t *testing.T) {
	got, err := BulbFiles(semiBulb(t), false)
	if err != nil {
		t.Fatalf("BulbFiles failed: %v", err)
	}

	want := []string{
		"scripts/garlic/internal/ui/tui.go",
		"scripts/garlic/main.go",
		"scripts/garlic/release.clove.md",
		"scripts/garlic/revise.clove.md",
	}
	if !slices.Equal(rels(got), want) {
		t.Errorf("got  %v\nwant %v", rels(got), want)
	}
}

// Naming one clove still sends the folder: the folder belongs to every clove in it.
func TestSelectOnSemiBulbIgnoresProjectDepth(t *testing.T) {
	bulb := semiBulb(t)
	opts := []domain.BoardOptions{bulb}

	whole, err := Select(Address{Bulb: "scripts", Area: "garlic"}, opts, false)
	if err != nil {
		t.Fatalf("Select(area) failed: %v", err)
	}
	one, err := Select(Address{Bulb: "scripts", Area: "garlic", Project: "revise"}, opts, false)
	if err != nil {
		t.Fatalf("Select(project) failed: %v", err)
	}

	if !slices.Equal(rels(whole), rels(one)) {
		t.Errorf("naming a clove should send the whole folder:\n  area   %v\n  clove  %v", rels(whole), rels(one))
	}
}

// A full bulb keeps the old meaning: the category is an area of many projects.
func TestFullBulbUnaffectedByWholeFolder(t *testing.T) {
	opts := selectTestBoard(t)

	files, err := Select(Address{Bulb: "epics", Area: "fitness", Project: "running"}, opts, false)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	want := []string{
		"epics/fitness/running.md",
		"epics/fitness/running/logs/day1.txt",
		"epics/fitness/running/plan.pdf",
	}
	if !slices.Equal(rels(files), want) {
		t.Errorf("got  %v\nwant %v", rels(files), want)
	}
}

// A semi bulb needs no rule of its own any more. Its folder travels whole
// because it is the project; a full bulb's area travels whole because you
// planted into it. Both reduce to "was anything here planted?", so the two bulb
// kinds stopped needing separate answers.
func TestVisibilityWholeFolder(t *testing.T) {
	v := visibility{
		Bulb:    "scripts",
		Planted: Manifest{"scripts/garlic/revise.clove.md": "A"},
	}

	cases := []struct {
		name string
		rel  string
		want bool
	}{
		{"source file in a folder that was planted", "scripts/garlic/main.go", true},
		{"nested source file", "scripts/garlic/internal/ui/tui.go", true},
		{"a new clove in the same folder", "scripts/garlic/notes.clove.md", true},
		{"anything in a folder that was never planted", "scripts/neofetch/neofetch.sh", false},
		// A clove on the remote used to put a folder in play on its own. It no
		// longer does: the remote does not get to enlist folders here.
		{"a folder the remote gave a clove of its own", "scripts/theirs/theirs.clove.md", false},
		{"loose file at bulb level", "scripts/stray.go", false},
		{"another bulb", "epics/fitness/running.md", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.allows(c.rel); got != c.want {
				t.Errorf("allows(%q) = %v, want %v", c.rel, got, c.want)
			}
		})
	}
}

// --git seeds a repository so the agent can commit against your history. It
// changes only what plant selects; nothing that feeds harvest ever keeps .git.
func TestSelectWithGit(t *testing.T) {
	opts := []domain.BoardOptions{semiBulb(t)}
	addr := Address{Bulb: "scripts", Area: "garlic"}

	without, err := Select(addr, opts, false)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	for _, rel := range rels(without) {
		if strings.Contains(rel, "/.git/") {
			t.Errorf("plain plant sent %q", rel)
		}
	}

	with, err := Select(addr, opts, true)
	if err != nil {
		t.Fatalf("Select(--git) failed: %v", err)
	}

	var git []string
	for _, rel := range rels(with) {
		if strings.Contains(rel, "/.git/") {
			git = append(git, rel)
		}
	}
	want := []string{"scripts/garlic/.git/HEAD", "scripts/garlic/.git/objects/ab/cdef"}
	if !slices.Equal(git, want) {
		t.Errorf("--git sent\n  %v\nwant\n  %v", git, want)
	}

	// The ignore list is untouched by the flag: dist stays out either way.
	for _, rel := range rels(with) {
		if strings.Contains(rel, "/dist/") {
			t.Errorf("--git overrode the ignore list: %q", rel)
		}
	}
}

// bulbCensus feeds harvest, so it must never keep .git whatever plant sent.
func TestBulbCensusNeverKeepsGit(t *testing.T) {
	census, err := bulbCensus(semiBulb(t))
	if err != nil {
		t.Fatal(err)
	}
	for rel := range census {
		if strings.Contains(rel, "/.git/") {
			t.Errorf("harvest would compare against %q", rel)
		}
	}
}
