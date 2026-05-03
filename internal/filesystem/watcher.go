package filesystem

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lucky7xz/garlic/internal/domain"
)

// WatchBoards initializes an fsnotify watcher for all configured board roots and their categories.
// It returns a channel that emits updated Boards whenever a change is detected.
func WatchBoards(opts []domain.BoardOptions) (<-chan []domain.Board, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	updateChan := make(chan []domain.Board)

	// Initial watch setup: Watch roots and their immediate subdirectories (categories)
	for _, opt := range opts {
		_ = watcher.Add(opt.Path)
		entries, err := os.ReadDir(opt.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				_ = watcher.Add(filepath.Join(opt.Path, entry.Name()))
			}
		}
	}

	go func() {
		defer watcher.Close()
		
		var timer *time.Timer
		const debounceDuration = 150 * time.Millisecond

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				
				// Minimal: we only care about project files or category folders
				// If a new directory is created, watch it too
				if event.Has(fsnotify.Create) {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						_ = watcher.Add(event.Name)
					}
				}

				// Debounce: wait for the dust to settle
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounceDuration, func() {
					var boards []domain.Board
					for _, opt := range opts {
						boards = append(boards, ScanBoard(opt))
					}
					updateChan <- boards
				})

			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return updateChan, nil
}
