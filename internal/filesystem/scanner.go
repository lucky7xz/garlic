package filesystem

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
)

func ScanBoard(opts domain.BoardOptions) domain.Board {
	board := domain.Board{
		Name:          opts.Name,
		Grid:          make(map[string]map[string][]domain.Project),
		HiddenGrid:    make(map[string]map[string][]domain.Project),
		CategoryOrder: []string{},
		Statuses:      opts.Statuses,
		Opts:          opts,
	}

	for _, status := range opts.Statuses {
		board.Grid[status] = make(map[string][]domain.Project)
		board.HiddenGrid[status] = make(map[string][]domain.Project)
	}

	entries, err := os.ReadDir(opts.Path)
	if err != nil {
		return board
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue 
		}

		category := entry.Name()
		catPath := filepath.Join(opts.Path, category)
		
		files, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}

		matchedFilesCount := 0

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			
			if strings.HasSuffix(file.Name(), opts.Extension) {
				filePath := filepath.Join(catPath, file.Name())
				tag, isHidden := GetTags(filePath)
				
				if tag != "" && isAllowedStatus(tag, opts.Statuses) {
					p := domain.Project{
						Name:     file.Name(),
						Path:     filePath,
						Category: category,
						Status:   tag,
					}
					
					targetGrid := board.Grid
					if isHidden {
						targetGrid = board.HiddenGrid
					}
					
					if _, ok := targetGrid[tag][category]; !ok {
						targetGrid[tag][category] = []domain.Project{}
					}
					targetGrid[tag][category] = append(targetGrid[tag][category], p)
					matchedFilesCount++
				}
			}
		}

		if matchedFilesCount > 0 || opts.ShowEmptyCategories {
			board.CategoryOrder = append(board.CategoryOrder, category)
			
			for _, status := range opts.Statuses {
				if board.Grid[status] == nil {
					board.Grid[status] = make(map[string][]domain.Project)
				}
				if board.Grid[status][category] == nil {
					board.Grid[status][category] = make([]domain.Project, 0)
				}
				if board.HiddenGrid[status] == nil {
					board.HiddenGrid[status] = make(map[string][]domain.Project)
				}
				if board.HiddenGrid[status][category] == nil {
					board.HiddenGrid[status][category] = make([]domain.Project, 0)
				}
			}
		}
	}

	return board
}

func GetTags(filePath string) (string, bool) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", false
	}
	defer file.Close()

	reStatus := regexp.MustCompile(`#statustag-\s*(\w+)`)
	status := ""
	hidden := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "#garlic-hide" {
			hidden = true
		} else if m := reStatus.FindStringSubmatch(line); m != nil && len(m) >= 2 {
			if status == "" {
				status = m[1]
			}
		}
	}
	return status, hidden
}

func isAllowedStatus(s string, allowedStatuses []string) bool {
	for _, st := range allowedStatuses {
		if s == st {
			return true
		}
	}
	return false
}

func ToggleHiddenMarker(filepath string) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	found := false
	var newLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "#garlic-hide" {
			found = true
			continue 
		}
		newLines = append(newLines, line)
	}
	if !found {
		newLines = append([]string{"#garlic-hide"}, newLines...)
	}
	
	os.WriteFile(filepath, []byte(strings.Join(newLines, "\n")), 0644)
}
