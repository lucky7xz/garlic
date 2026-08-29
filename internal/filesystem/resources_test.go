package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasEntries(t *testing.T) {
	root := t.TempDir()

	mkdir := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	touch := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	empty := mkdir("empty")

	withFile := mkdir("withFile")
	touch(filepath.Join(withFile, "plan.pdf"))

	withDir := mkdir("withDir")
	mkdir(filepath.Join("withDir", "logs"))

	// A dotfile is still something you would see when you opened the folder.
	withDotfile := mkdir("withDotfile")
	touch(filepath.Join(withDotfile, ".keep"))

	plainFile := filepath.Join(root, "notes.md")
	touch(plainFile)

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"a folder with a file", withFile, true},
		{"a folder holding only a subfolder", withDir, true},
		{"a folder holding only a dotfile", withDotfile, true},
		{"an empty folder promises nothing", empty, false},
		{"no folder at all", filepath.Join(root, "nope"), false},
		{"a file, not a folder", plainFile, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasEntries(c.dir); got != c.want {
				t.Errorf("HasEntries(%q) = %v, want %v", c.dir, got, c.want)
			}
		})
	}
}
