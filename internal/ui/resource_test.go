package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
)

// resourceBoard is a real bulb on disk: one project whose resource folder holds
// something, one whose folder is empty, one with no folder at all.
func resourceBoard(t *testing.T) (domain.Board, string) {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("epics/bioz/mealprep.md", "#statustag-toDo\n")
	write("epics/bioz/mealprep/log.csv", "1,2\n")

	write("epics/bioz/sleeplog.md", "#statustag-toDo\n")
	if err := os.MkdirAll(filepath.Join(root, "epics/bioz/sleeplog"), 0755); err != nil {
		t.Fatal(err)
	}

	write("epics/bioz/cardio.md", "#statustag-toDo\n")

	opts := domain.BoardOptions{
		Path:      filepath.Join(root, "epics"),
		Name:      "epics",
		Extension: ".md",
		Statuses:  []string{"toDo"},
	}
	return filesystem.ScanBoard(opts), root
}

// The mark has to mean "there is something to open". An empty folder looking
// exactly like a full one is the promise the board could not keep.
func TestResourceMarkOnlyForFolderWithSomethingInIt(t *testing.T) {
	board, _ := resourceBoard(t)
	got := modelAt(board, 100, 40).View()

	if !strings.Contains(got, "mealprep"+resourceMark) {
		t.Errorf("a folder with a file in it is unmarked:\n%s", got)
	}
	if strings.Contains(got, "sleeplog"+resourceMark) {
		t.Errorf("an empty folder is marked, so `r` opens onto nothing:\n%s", got)
	}
	if strings.Contains(got, "cardio"+resourceMark) {
		t.Errorf("a project with no folder at all is marked:\n%s", got)
	}
}

// The check runs per render rather than at scan time precisely so this works:
// the watcher does not follow resource folders that existed before launch.
func TestResourceMarkAppearsWithoutARescan(t *testing.T) {
	board, root := resourceBoard(t)
	m := modelAt(board, 100, 40)

	if strings.Contains(m.View(), "sleeplog"+resourceMark) {
		t.Fatal("marked before anything was put in the folder")
	}

	if err := os.WriteFile(filepath.Join(root, "epics/bioz/sleeplog/notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Same model, same board value -- only the filesystem moved.
	if !strings.Contains(m.View(), "sleeplog"+resourceMark) {
		t.Errorf("a file dropped in by hand never showed up:\n%s", m.View())
	}
}

// The grid is column-aligned, so the mark's width has to be what cell.go's
// arithmetic assumes.
func TestResourceMarkIsTwoColumns(t *testing.T) {
	if got := lipgloss.Width(resourceMark); got != 2 {
		t.Errorf("resourceMark measures %d columns, want 2", got)
	}
}
