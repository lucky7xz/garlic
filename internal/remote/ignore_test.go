package remote

import "testing"

func TestIgnored(t *testing.T) {
	cases := []struct {
		name     string
		rel      string
		patterns []string
		want     bool
	}{
		// .git never travels: rsync merges without deleting, so a harvested
		// refs/ or index would leave the repo pointing at the wrong commits.
		{"git dir at the root of a project", "scripts/garlic/.git/HEAD", nil, true},
		{"git dir nested deeper", "scripts/garlic/vendor/x/.git/config", nil, true},
		{"a file merely named .gitignore", "scripts/garlic/.gitignore", nil, false},
		{"a file merely named git", "scripts/garlic/git.md", nil, false},

		{"configured pattern", "scripts/drako/dist/drako", []string{"dist"}, true},
		{"pattern matches a segment, not a substring", "scripts/drako/distributed/x", []string{"dist"}, false},
		{"pattern deeper in the tree", "scripts/x/a/node_modules/b/c.js", []string{"node_modules"}, true},
		{"unmatched pattern", "scripts/drako/internal/main.go", []string{"dist"}, false},
		{"several patterns", "scripts/x/target/out", []string{"dist", "target"}, true},

		{"parked copies never travel", "epics/fitness/running/running.remote.md", nil, true},
		{"ordinary file", "scripts/garlic/main.go", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ignored(c.rel, c.patterns); got != c.want {
				t.Errorf("ignored(%q, %v) = %v, want %v", c.rel, c.patterns, got, c.want)
			}
		})
	}
}
