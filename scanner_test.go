package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanBoard(t *testing.T) {
	tmpDir := t.TempDir()
	
	workDir := filepath.Join(tmpDir, "work") // Will have files
	emptyDir := filepath.Join(tmpDir, "empty") // Will be completely empty
	os.Mkdir(workDir, 0755)
	os.Mkdir(emptyDir, 0755)

	writeTestFile(t, filepath.Join(workDir, "task1.md"), "#statustag-toDo\nTask 1")
	writeTestFile(t, filepath.Join(workDir, "task2.md"), "#garlic-hide\n#statustag-inProgress\nTask 2")
	writeTestFile(t, filepath.Join(workDir, "task3.txt"), "#statustag-toDo\nTask 3") // Wrong extension

	statuses := []string{"toDo", "inProgress"}

	t.Run("Full Bulb (Shows Empty, matches .md)", func(t *testing.T) {
		opts := BoardOptions{
			Path:                tmpDir,
			Name:                "TestFull",
			Extension:           ".md",
			Statuses:            statuses,
			ShowEmptyCategories: true,
		}

		board := ScanBoard(opts)

		// Expecting both 'work' AND 'empty' to be in CategoryOrder
		if len(board.CategoryOrder) != 2 {
			t.Fatalf("expected 2 categories, got %d. Categories: %v", len(board.CategoryOrder), board.CategoryOrder)
		}

		// task1 is normal, task2 is hidden. 
		workToDo := len(board.Grid["toDo"]["work"])  // task 1
		workInProgress := len(board.Grid["inProgress"]["work"]) // 0
		hiddenInProgress := len(board.HiddenGrid["inProgress"]["work"]) // task 2
		
		if workToDo != 1 || workInProgress != 0 || hiddenInProgress != 1 {
			t.Errorf("expected 1 toDo(Grid), 0 inProgress(Grid), 1 inProgress(Hidden). Got: %d, %d, %d", workToDo, workInProgress, hiddenInProgress)
		}
	})
}

// helper to easily dump text into files for our tests
func writeTestFile(t *testing.T, path string, content string) {
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}
