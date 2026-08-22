package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lucky7xz/garlic/internal/domain"
)

func watchOpts(t *testing.T) []domain.BoardOptions {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "cat"), 0755); err != nil {
		t.Fatal(err)
	}
	return []domain.BoardOptions{{Path: dir, Name: "test", Extension: ".md", Statuses: []string{"todo"}}}
}

// The runner starts a fresh watcher every time it re-enters the TUI, so a
// watcher that outlives its stop leaks a goroutine per file opened.
func TestStopWatcherReleasesGoroutines(t *testing.T) {
	opts := watchOpts(t)

	// One warm-up round so fsnotify's own internal goroutines are already up and
	// not counted as growth below.
	if _, stop, err := WatchBoards(opts); err == nil {
		stop()
	}
	settle()
	before := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		_, stop, err := WatchBoards(opts)
		if err != nil {
			t.Fatalf("WatchBoards: %v", err)
		}
		stop()
	}
	settle()

	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("goroutines went %d -> %d over 10 stopped watchers", before, after)
	}
}

// A debounced rescan fires after the TUI has quit and nobody is reading the
// channel. The send must not park the goroutine forever.
func TestStopWatcherUnblocksPendingSend(t *testing.T) {
	opts := watchOpts(t)
	_, stop, err := WatchBoards(opts)
	if err != nil {
		t.Fatalf("WatchBoards: %v", err)
	}

	settle()
	before := runtime.NumGoroutine()

	// Touch a file to arm the debounce timer, then leave without ever reading.
	if err := os.WriteFile(filepath.Join(opts[0].Path, "cat", "p.md"), []byte("#statustag-todo"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // past the 150ms debounce: the send is now blocked

	stop()
	settle()

	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines went %d -> %d after stopping a watcher with a pending send", before, after)
	}
}

func settle() {
	for i := 0; i < 20; i++ {
		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
}
