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
				tags := GetTags(filePath)
				tag := tags.Status
				
				if tag != "" && isAllowedStatus(tag, opts.Statuses) {
					p := domain.Project{
						Name:      file.Name(),
						Path:      filePath,
						Category:  category,
						Status:    tag,
						AgentTask: tags.AgentTask,
					}
					
					targetGrid := board.Grid
					if tags.Hidden {
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

// Tags is the state a project file carries in its own content. Everything the
// board knows about a project comes from here, which is also why it survives a
// trip to another machine: it is just bytes in the file.
type Tags struct {
	Status string
	Hidden bool
	// AgentTask is true while at least one bare #AT remains. The agent flips
	// each one to #AT-done as it finishes, and the board mark disappears.
	AgentTask bool
}

var (
	reStatus = regexp.MustCompile(`#statustag-\s*(\w+)`)
	// Matched whole so that #ATTENTION and #AT-done are not mistaken for a
	// bare #AT. RE2 has no lookahead, so compare the match instead.
	reAgentTask = regexp.MustCompile(`#AT[-\w]*`)
)

func GetTags(filePath string) Tags {
	file, err := os.Open(filePath)
	if err != nil {
		return Tags{}
	}
	defer file.Close()

	var tags Tags
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "#garlic-hide" {
			tags.Hidden = true
		}
		if m := reStatus.FindStringSubmatch(line); len(m) >= 2 && tags.Status == "" {
			tags.Status = m[1]
		}
		for _, m := range reAgentTask.FindAllString(line, -1) {
			if m == "#AT" {
				tags.AgentTask = true
			}
		}
	}
	return tags
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
