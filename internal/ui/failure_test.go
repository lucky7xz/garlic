package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

// Hiding a file garlic cannot rewrite used to look exactly like hiding one it
// could: the key press did nothing and said nothing. The board is the only place
// that can report it.
func TestHidingAFileItCannotWriteSaysSo(t *testing.T) {
	board, root := resourceBoard(t)
	dir := filepath.Join(root, "epics", "bioz")

	m := modelAt(board, 100, 40)
	m.State = stateHiding
	m.ActionTarget = domain.Project{
		Name: "mealprep.md",
		Path: filepath.Join(dir, "mealprep.md"),
	}

	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	got := press(t, m, "y")

	if got.ErrorMsg == "" {
		t.Error("the key press failed silently")
	}
	if got.State != stateNormal {
		t.Error("the board was left stuck in the hiding state")
	}
}

// And the ordinary case still works, or the test above proves only that
// everything is broken.
func TestHidingAWritableFileSucceedsQuietly(t *testing.T) {
	board, root := resourceBoard(t)

	m := modelAt(board, 100, 40)
	m.State = stateHiding
	m.ActionTarget = domain.Project{
		Name: "mealprep.md",
		Path: filepath.Join(root, "epics", "bioz", "mealprep.md"),
	}

	got := press(t, m, "y")

	if got.ErrorMsg != "" {
		t.Errorf("reported %q on a file it could write", got.ErrorMsg)
	}

	body, err := os.ReadFile(m.ActionTarget.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "#garlic-hide") {
		t.Errorf("the marker never landed:\n%s", body)
	}
}
