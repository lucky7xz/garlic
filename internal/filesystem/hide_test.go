package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToggleHiddenMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "running.md")

	if err := os.WriteFile(path, []byte("#statustag-toDo\nnotes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	read := func() string {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	if err := ToggleHiddenMarker(path); err != nil {
		t.Fatalf("hiding failed: %v", err)
	}
	if !strings.Contains(read(), "#garlic-hide") {
		t.Errorf("marker was not added:\n%s", read())
	}
	if !strings.Contains(read(), "notes") {
		t.Errorf("the rest of the file was lost:\n%s", read())
	}

	if err := ToggleHiddenMarker(path); err != nil {
		t.Fatalf("unhiding failed: %v", err)
	}
	if strings.Contains(read(), "#garlic-hide") {
		t.Errorf("marker was not removed:\n%s", read())
	}
	if !strings.Contains(read(), "#statustag-toDo") {
		t.Errorf("the status tag was lost:\n%s", read())
	}
}

// A key press that appears to do nothing is worse than one that says why. The
// board has no other way to report this.
func TestToggleHiddenMarkerReportsWhatStoppedIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "running.md")

	if err := os.WriteFile(path, []byte("#statustag-toDo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// The rewrite lands beside the file, so an unwritable directory is what
	// stops it.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	if err := ToggleHiddenMarker(path); err == nil {
		t.Error("reported success on a file it could not rewrite")
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "#statustag-toDo\n" {
		t.Errorf("the original was damaged: %q", body)
	}
}

func TestToggleHiddenMarkerOnAMissingFile(t *testing.T) {
	if err := ToggleHiddenMarker(filepath.Join(t.TempDir(), "gone.md")); err == nil {
		t.Error("reported success for a file that does not exist")
	}
}
