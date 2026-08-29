package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "running.md")

	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(path, []byte("replaced\n")); err != nil {
		t.Fatalf("replaceFile failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replaced\n" {
		t.Errorf("got %q, want %q", got, "replaced\n")
	}

	// Project files are read by editors and file managers, not just garlic, so
	// the mode must not come out as CreateTemp's 0600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode is %v, want 0644", info.Mode().Perm())
	}
}

// The whole point: a write that cannot complete must not cost you the original.
func TestReplaceFileLeavesTheOriginalWhenItCannotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "running.md")

	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// An unwritable directory is where the temp file cannot be created.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	if err := replaceFile(path, []byte("replaced\n")); err == nil {
		t.Fatal("replaceFile reported success where it could not write")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original\n" {
		t.Errorf("the original was damaged: got %q", got)
	}
}

// Nothing should be left lying beside the file it replaced.
func TestReplaceFileLeavesNoTemporaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "running.md")

	if err := os.WriteFile(path, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(path, []byte("replaced\n")); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "running.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only running.md", names)
	}
}
