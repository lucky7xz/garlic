package filesystem

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// UpdateProjectStatus replaces the existing status tag with a new one
func UpdateProjectStatus(path, newStatus string) error {
	// Basic path sanitization
	path = filepath.Clean(path)
	
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	
	var lines []string
	reStatus := regexp.MustCompile(`#statustag-\s*(\w+)`)
	scanner := bufio.NewScanner(file)
	
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if !found && reStatus.MatchString(line) {
			line = reStatus.ReplaceAllString(line, fmt.Sprintf("#statustag-%s", newStatus))
			found = true
		}
		lines = append(lines, line)
	}
	file.Close()

	if scanner.Err() != nil {
		return scanner.Err()
	}

	if !found {
		lines = append([]string{fmt.Sprintf("#statustag-%s", newStatus), ""}, lines...)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// CreateProject safely creates a new project file if it doesn't already exist
func CreateProject(path, status string) error {
	path = filepath.Clean(path)
	
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Check for existence to prevent overwrite
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("file already exists: %s", filepath.Base(path))
	}

	content := fmt.Sprintf("#statustag-%s\n\n", status)
	return os.WriteFile(path, []byte(content), 0644)
}

// DeleteProject safely removes a project file
func DeleteProject(path string) error {
	return os.Remove(filepath.Clean(path))
}

// RenameProject safely renames a project file and its resource folder if it exists
func RenameProject(oldPath, newNameWithoutExt string) error {
	oldPath = filepath.Clean(oldPath)
	dir := filepath.Dir(oldPath)

	// Determine extension (handling .clove.md)
	ext := ".md"
	if strings.HasSuffix(oldPath, ".clove.md") {
		ext = ".clove.md"
	}

	newPath := filepath.Join(dir, newNameWithoutExt+ext)

	// Check if target already exists
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("target file already exists: %s", newNameWithoutExt+ext)
	}

	// Identify resource folder before moving the file
	oldBase := strings.TrimSuffix(filepath.Base(oldPath), ext)
	oldResPath := filepath.Join(dir, oldBase)
	newResPath := filepath.Join(dir, newNameWithoutExt)

	// Rename the file
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}

	// Optionally rename resource folder
	if info, err := os.Stat(oldResPath); err == nil && info.IsDir() {
		// Only rename if the new folder name isn't already taken
		if _, err := os.Stat(newResPath); os.IsNotExist(err) {
			_ = os.Rename(oldResPath, newResPath)
		}
	}

	return nil
}
