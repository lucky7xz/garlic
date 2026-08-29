package remote

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

// completeConfig is a workspace with both bulb kinds, so completion can be
// checked against the two different meanings of a category.
func completeConfig(t *testing.T) domain.Config {
	t.Helper()
	root := t.TempDir()

	write := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#statustag-toDo\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("epics/bioz/mealprep.md")
	write("epics/bioz/sleeplog.md")
	write("epics/work/report.md")
	write("epics/empty/notes.txt") // no tracked file: not an area you can plant
	write("scripts/garlic/revise.clove.md")
	write("scripts/drako/build.clove.md")

	return domain.Config{
		FullBulbs: []domain.BulbConfig{{
			Path:     filepath.Join(root, "epics"),
			Statuses: []string{"toDo"},
		}},
		SemiBulbs: []domain.BulbConfig{{
			Path:     filepath.Join(root, "scripts"),
			Statuses: []string{"toDo"},
		}},
		Remotes: []domain.Remote{
			{Name: "berta", Host: "berta@x", Root: "~/shara"},
			{Name: "bella", Host: "bella@y", Root: "~/shara"},
		},
	}
}

// Everything a completion needs is here on this machine -- which is also the
// only thing that can be planted. No ssh runs until you press enter.
func TestComplete(t *testing.T) {
	cfg := completeConfig(t)

	cases := []struct {
		name  string
		words []string
		want  []string
	}{
		{"the verbs", []string{""}, []string{"harvest", "init", "plant", "status", "wipe"}},
		{"a verb prefix", []string{"w"}, []string{"wipe"}},
		{"an ambiguous verb prefix", []string{"s"}, []string{"status"}},

		{"bulbs", []string{"plant", ""}, []string{"epics/", "scripts/"}},
		{"a bulb prefix", []string{"plant", "ep"}, []string{"epics/"}},

		{"areas of a full bulb", []string{"plant", "epics/"}, []string{"epics/bioz/", "epics/work/"}},
		{"an area prefix", []string{"harvest", "epics/b"}, []string{"epics/bioz/"}},
		{"projects of an area", []string{"wipe", "epics/bioz/"},
			[]string{"epics/bioz/mealprep", "epics/bioz/sleeplog"}},
		{"a project prefix", []string{"wipe", "epics/bioz/m"}, []string{"epics/bioz/mealprep"}},

		// A semi bulb's category is the project, so it is a leaf: no trailing
		// slash, and nothing below it to offer.
		{"a semi bulb stops at its folder", []string{"plant", "scripts/"},
			[]string{"scripts/drako", "scripts/garlic"}},
		{"nothing below a semi bulb folder", []string{"plant", "scripts/garlic/"}, nil},

		{"an area with nothing tracked is not offered", []string{"plant", "epics/e"}, nil},
		{"no such bulb", []string{"plant", "nope/"}, nil},
		{"deeper than an address goes", []string{"plant", "epics/bioz/mealprep/"}, nil},

		{"once an address is given, offer the remotes", []string{"plant", "epics", ""},
			[]string{"@bella", "@berta"}},
		{"the glued form", []string{"plant", "epics", "@b"}, []string{"@bella", "@berta"}},
		{"after a bare @", []string{"plant", "epics", "@", ""}, []string{"bella", "berta"}},
		{"a remote prefix after a bare @", []string{"plant", "epics", "@", "be"}, []string{"bella", "berta"}},

		// wipe and status take no address at all, so the remote comes straight
		// after the verb.
		{"wipe can go straight to a remote", []string{"wipe", "@b"}, []string{"@bella", "@berta"}},

		{"nothing left once the remote is named", []string{"plant", "epics", "@berta", ""}, nil},
		{"an unknown verb completes nothing", []string{"frobnicate", ""}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Complete(cfg, c.words)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Complete(%q)\n got %q\nwant %q", c.words, got, c.want)
			}
		})
	}
}

// The shell hands over the word under the cursor as the last element, empty when
// there is nothing typed yet. No words at all is the same as one empty one.
func TestCompleteWithNoWords(t *testing.T) {
	cfg := completeConfig(t)

	if got := Complete(cfg, nil); len(got) == 0 {
		t.Error("no words should still offer the verbs")
	}
}

// Completion reads the board, so a missing or unreadable bulb path must come
// back empty rather than take the shell down with it.
func TestCompleteSurvivesAMissingBulb(t *testing.T) {
	cfg := domain.Config{
		FullBulbs: []domain.BulbConfig{{Path: "/nonexistent/epics", Statuses: []string{"toDo"}}},
	}

	if got := Complete(cfg, []string{"plant", "epics/"}); got != nil {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestCompletionScript(t *testing.T) {
	got, err := CompletionScript("bash")
	if err != nil {
		t.Fatalf("CompletionScript(bash) failed: %v", err)
	}
	// The two halves that have to agree with Complete: it must call __complete,
	// and it must send the words up to the cursor rather than the whole line.
	for _, want := range []string{"__complete", "COMP_CWORD", "complete -F _garlic garlic"} {
		if !strings.Contains(got, want) {
			t.Errorf("script is missing %q:\n%s", want, got)
		}
	}

	if _, err := CompletionScript("fish"); err == nil {
		t.Error("CompletionScript(fish) succeeded, want an error naming what is shipped")
	}
}

// Installing writes the hook where bash-completion looks for it by name, so the
// shell loads it the first time you type `garlic` and no dotfile is touched.
func TestInstallCompletion(t *testing.T) {
	home := t.TempDir()

	path, err := InstallCompletion(home)
	if err != nil {
		t.Fatalf("InstallCompletion failed: %v", err)
	}

	want := filepath.Join(home, ".local", "share", "bash-completion", "completions", "garlic")
	if path != want {
		t.Errorf("wrote %q, want %q", path, want)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nothing at %q: %v", path, err)
	}
	if !strings.Contains(string(got), "complete -F _garlic garlic") {
		t.Errorf("the hook is not in the file:\n%s", got)
	}

	// Running it again must not fail: reinstalling after an upgrade is the
	// normal thing to do.
	if _, err := InstallCompletion(home); err != nil {
		t.Errorf("second install failed: %v", err)
	}
}

// `garlic init` is the moment garlic is being set up, so it is where the offer
// belongs -- but it only ever offers. Nothing is written to a shell's config,
// and saying no leaves the machine exactly as it was.
func TestOfferCompletion(t *testing.T) {
	installed := func(home string) bool {
		_, err := os.Stat(filepath.Join(home, ".local", "share", "bash-completion", "completions", "garlic"))
		return err == nil
	}

	t.Run("yes installs it", func(t *testing.T) {
		home := t.TempDir()
		var out bytes.Buffer

		if err := OfferCompletion(&out, strings.NewReader("y\n"), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}
		if !installed(home) {
			t.Error("said yes, nothing was written")
		}
		if !strings.Contains(out.String(), "new shell") {
			t.Errorf("never says it takes a new shell to take effect:\n%s", out.String())
		}
	})

	t.Run("no leaves the machine alone", func(t *testing.T) {
		home := t.TempDir()
		var out bytes.Buffer

		if err := OfferCompletion(&out, strings.NewReader("n\n"), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}
		if installed(home) {
			t.Error("said no, something was written anyway")
		}
	})

	t.Run("no answer is a no, not a hang", func(t *testing.T) {
		home := t.TempDir()
		var out bytes.Buffer

		if err := OfferCompletion(&out, strings.NewReader(""), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}
		if installed(home) {
			t.Error("no answer was taken as yes")
		}
	})

	// This rides on every status, so the ordinary case -- already installed --
	// has to print nothing at all.
	t.Run("already installed, so it says nothing", func(t *testing.T) {
		home := t.TempDir()
		if _, err := InstallCompletion(home); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		if err := OfferCompletion(&out, strings.NewReader(""), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}
		if out.String() != "" {
			t.Errorf("status would print this every time:\n%s", out.String())
		}
	})

	// Declining keeps no record, so the offer returns next time -- but it has to
	// leave you a way to do it by hand meanwhile.
	t.Run("declining names the manual way", func(t *testing.T) {
		home := t.TempDir()
		var out bytes.Buffer

		if err := OfferCompletion(&out, strings.NewReader("n\n"), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}
		if !strings.Contains(out.String(), "garlic completion bash") {
			t.Errorf("no way back is offered:\n%s", out.String())
		}
	})

	// What lands on disk has to be the script that was tested to work.
	t.Run("what is written is the tested script", func(t *testing.T) {
		home := t.TempDir()
		if err := OfferCompletion(&bytes.Buffer{}, strings.NewReader("y\n"), home); err != nil {
			t.Fatalf("OfferCompletion failed: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(home, ".local", "share", "bash-completion", "completions", "garlic"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != bashCompletion {
			t.Errorf("wrote something other than the completion script:\n%s", got)
		}
	})
}
