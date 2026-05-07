package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameProject(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("Rename .md with resource folder", func(t *testing.T) {
		oldPath := filepath.Join(tmpDir, "old.md")
		_ = os.WriteFile(oldPath, []byte("#statustag-todo"), 0644)
		oldResDir := filepath.Join(tmpDir, "old")
		_ = os.Mkdir(oldResDir, 0755)

		if err := RenameProject(oldPath, "new"); err != nil {
			t.Fatalf("RenameProject failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmpDir, "new.md")); err != nil {
			t.Error("new file does not exist")
		}
		if _, err := os.Stat(filepath.Join(tmpDir, "new")); err != nil {
			t.Error("new resource dir does not exist")
		}
	})

	t.Run("Rename .clove.md without resource folder", func(t *testing.T) {
		oldPath := filepath.Join(tmpDir, "semi.clove.md")
		_ = os.WriteFile(oldPath, []byte("#statustag-todo"), 0644)

		if err := RenameProject(oldPath, "renamed"); err != nil {
			t.Fatalf("RenameProject failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmpDir, "renamed.clove.md")); err != nil {
			t.Error("new clove file does not exist")
		}
	})

	t.Run("Rename file but folder collision", func(t *testing.T) {
		oldPath := filepath.Join(tmpDir, "coll.md")
		_ = os.WriteFile(oldPath, []byte(""), 0644)
		_ = os.Mkdir(filepath.Join(tmpDir, "coll"), 0755)
		_ = os.Mkdir(filepath.Join(tmpDir, "target_dir"), 0755) // Collision for folder but not file

		if err := RenameProject(oldPath, "target_dir"); err != nil {
			t.Fatalf("RenameProject failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(tmpDir, "target_dir.md")); err != nil {
			t.Error("new file does not exist")
		}
		// Folder should NOT have been renamed because of collision
		if _, err := os.Stat(filepath.Join(tmpDir, "coll")); err != nil {
			t.Error("original folder should still exist")
		}
	})
}

func TestRenameProjectCollision(t *testing.T) {
	tmpDir := t.TempDir()

	oldPath := filepath.Join(tmpDir, "old.md")
	existingPath := filepath.Join(tmpDir, "exists.md")
	
	_ = os.WriteFile(oldPath, []byte(""), 0644)
	_ = os.WriteFile(existingPath, []byte(""), 0644)

	// Rename to existing should fail
	if err := RenameProject(oldPath, "exists"); err == nil {
		t.Error("expected error when renaming to existing file, got nil")
	}
}
