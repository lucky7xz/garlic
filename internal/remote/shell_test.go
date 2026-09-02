package remote

import (
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

// The script is the whole feature: nothing here runs ssh, it checks what would
// be handed to it.
func TestShellScript(t *testing.T) {
	cases := []struct {
		name string
		root string
		rel  string
		ext  string
		want string
	}{
		{
			"full bulb, absolute root",
			"/srv/work", "epics/bioz/mealprep.md", ".md",
			"cd '/srv/work/epics/bioz/mealprep' 2>/dev/null || cd '/srv/work/epics/bioz' || exit 1",
		},
		{
			"semi bulb lands in the project folder",
			"/srv/work", "scripts/garlic/release.clove.md", ".clove.md",
			"cd '/srv/work/scripts/garlic/release' 2>/dev/null || cd '/srv/work/scripts/garlic' || exit 1",
		},
		{
			"bare tilde is expanded over there",
			"~", "epics/bioz/mealprep.md", ".md",
			`cd "$HOME"/'epics/bioz/mealprep' 2>/dev/null || cd "$HOME"/'epics/bioz' || exit 1`,
		},
		{
			"tilde with a subdirectory",
			"~/work", "epics/bioz/mealprep.md", ".md",
			`cd "$HOME"/'work/epics/bioz/mealprep' 2>/dev/null || cd "$HOME"/'work/epics/bioz' || exit 1`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := Shell(domain.Remote{Host: "agent", Root: c.root}, c.rel, c.ext).Args
			script := args[len(args)-1]

			if !strings.HasPrefix(script, c.want) {
				t.Errorf("script is\n%s\nwant it to start with\n%s", script, c.want)
			}
			if !strings.HasSuffix(script, "exec ${SHELL:-/bin/sh} -l") {
				t.Errorf("script %q does not end in an interactive shell", script)
			}
		})
	}
}

// A quote in a folder name is the one thing that could break out of the script
// and run on the remote, so it stays inside the quoting.
func TestShellQuotesTheAwkwardName(t *testing.T) {
	args := Shell(domain.Remote{Host: "agent", Root: "/srv"}, "epics/it's/x.md", ".md").Args
	script := args[len(args)-1]

	if strings.Contains(script, `it's`) {
		t.Errorf("script %q let a quote through unescaped", script)
	}
	if !strings.Contains(script, `it'\''s`) {
		t.Errorf("script %q does not carry the escaped name", script)
	}
}

// ssh needs -t to give the far side a terminal, and the port and key belong to
// the remote, not to the script.
func TestShellCarriesTheConnectionFlags(t *testing.T) {
	r := domain.Remote{Host: "me@agent", Port: 2222, IdentityFile: "/home/me/.ssh/id_agent", Root: "/srv"}
	args := Shell(r, "epics/bioz/mealprep.md", ".md").Args

	want := []string{"ssh", "-t", "-p", "2222", "-i", "/home/me/.ssh/id_agent", "me@agent"}
	for i, w := range want {
		if i >= len(args) || args[i] != w {
			t.Fatalf("args are %q, want them to start with %q", args, want)
		}
	}
	if len(args) != len(want)+1 {
		t.Errorf("args are %q, want the script as the single remaining argument", args)
	}
}
