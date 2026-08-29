package filesystem

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lucky7xz/garlic/internal/domain"
)

// WatchBoards initializes an fsnotify watcher for all configured board roots and their categories.
// It returns a channel that emits updated Boards whenever a change is detected,
// and a stop func that tears the watcher down. Callers must stop a watcher they
// are done with: the runner builds a fresh one every time it re-enters the TUI,
// so leaving the old one running leaks a file descriptor and a goroutine per
// file opened.
func WatchBoards(opts []domain.BoardOptions) (<-chan []domain.Board, func(), error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, err
	}

	updateChan := make(chan []domain.Board)
	done := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(done) }) }

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
					// Once the TUI has quit nothing reads updateChan again, so an
					// unguarded send would park this goroutine forever.
					select {
					case updateChan <- boards:
					case <-done:
					}
				})

			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}

			case <-done:
				if timer != nil {
					timer.Stop()
				}
				return
			}
		}
	}()

	return updateChan, stop, nil
}
