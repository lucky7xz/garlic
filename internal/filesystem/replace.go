package filesystem

import (
	"os"
	"path/filepath"
)

// replaceFile swaps a file's contents for new ones without putting the old ones
// at risk. os.WriteFile truncates before it writes, so a failure part-way
// through leaves a project file empty -- and a project file is the user's notes,
// not something garlic can regenerate.
//
// Writing beside the target and renaming over it is atomic on POSIX: either the
// old contents survive intact or the new ones are fully in place, never a
// half-written middle. The temp file is a sibling so the rename stays within one
// filesystem, where that guarantee holds.
func replaceFile(path string, data []byte) error {
	dir, name := filepath.Split(path)

	tmp, err := os.CreateTemp(dir, "."+name+".garlic-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has moved it away

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// CreateTemp makes the file 0600; project files are readable like any note.
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
