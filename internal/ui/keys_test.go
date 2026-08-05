package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucky7xz/garlic/internal/domain"
)

// boards builds n distinct single-cell boards.
func boards(n int) []domain.Board {
	var bs []domain.Board
	for i := 0; i < n; i++ {
		b := board([]string{"todo"}, []string{"cat"}, 3)
		b.Name = string(rune('a' + i))
		bs = append(bs, b)
	}
	return bs
}

func multiBoardModel(n int, altModifier string) Model {
	bs := boards(n)
	m := Model{
		Boards:       bs,
		TermWidth:    80,
		TermHeight:   40,
		SavedCursors: make([]cursorState, len(bs)),
		AltModifier:  altModifier,
	}
	ApplyTheme(domain.Theme{}, &m)
	m.RecalculateOffsets()
	return m
}

// press drives one key through Update and returns the resulting model. It
// builds the KeyMsg so that msg.String() round-trips to key; anything else
// would make these tests pass without exercising the real dispatch.
func press(t *testing.T, m Model, key string) Model {
	t.Helper()

	var msg tea.KeyMsg
	if runes := []rune(key); len(runes) == 1 {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: runes}
	} else if runes := []rune(key); len(runes) == 5 && key[:4] == "alt+" {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: runes[4:], Alt: true}
	}
	if msg.String() != key {
		t.Fatalf("press helper built %q for key %q", msg.String(), key)
	}

	next, _ := m.Update(msg)
	return next.(Model)
}

func TestBoardJumpTargetParsing(t *testing.T) {
	cases := []struct {
		altModifier string
		key         string
		want        int
		wantOK      bool
	}{
		{"alt", "alt+1", 1, true},
		{"alt", "alt+3", 3, true},
		{"alt", "alt+9", 9, true},

		// 0 is not a workspace number; workspaces are 1-based in the header.
		{"alt", "alt+0", 0, false},
		// Must not shadow the existing alternative-tool bindings.
		{"alt", "alt+r", 0, false},
		{"alt", "alt+enter", 0, false},
		{"alt", "alt+ ", 0, false},
		// Bare digits stay unbound.
		{"alt", "1", 0, false},
		{"alt", "alt+12", 0, false},
		{"alt", "", 0, false},

		// A configured modifier gains its own binding but never loses alt+N,
		// which is the only form terminals reliably deliver with a digit.
		{"ctrl", "alt+3", 3, true},
		{"ctrl", "ctrl+3", 3, true},
		{"ctrl", "ctrl+0", 0, false},
		{"ctrl", "ctrl+c", 0, false},

		// An empty modifier must not turn a bare "+1" into a jump.
		{"", "alt+2", 2, true},
		{"", "+2", 0, false},
	}

	for _, tc := range cases {
		m := Model{AltModifier: tc.altModifier}
		got, ok := m.boardJumpTarget(tc.key)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("AltModifier=%q key=%q: got (%d, %v), want (%d, %v)",
				tc.altModifier, tc.key, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestJumpToBoardByNumber(t *testing.T) {
	m := multiBoardModel(3, "alt")

	m = press(t, m, "alt+2")
	if m.ActiveBoard != 1 {
		t.Errorf("alt+2: ActiveBoard = %d, want 1", m.ActiveBoard)
	}

	m = press(t, m, "alt+3")
	if m.ActiveBoard != 2 {
		t.Errorf("alt+3: ActiveBoard = %d, want 2", m.ActiveBoard)
	}

	m = press(t, m, "alt+1")
	if m.ActiveBoard != 0 {
		t.Errorf("alt+1: ActiveBoard = %d, want 0", m.ActiveBoard)
	}
}

func TestJumpOutOfRangeReportsAndStays(t *testing.T) {
	m := multiBoardModel(3, "alt")
	m = press(t, m, "alt+2")

	m = press(t, m, "alt+7")
	if m.ActiveBoard != 1 {
		t.Errorf("alt+7 moved to board %d, want to stay on 1", m.ActiveBoard)
	}
	if m.ErrorMsg == "" {
		t.Error("alt+7 with 3 boards set no ErrorMsg")
	}
}

func TestJumpRestoresSavedCursor(t *testing.T) {
	m := multiBoardModel(3, "alt")

	// Park a distinctive cursor on board 1.
	m.GridCursor.Project = 2
	m = press(t, m, "alt+3")
	if m.GridCursor.Project != 0 {
		t.Errorf("board 3 opened with Project = %d, want its own 0", m.GridCursor.Project)
	}

	m = press(t, m, "alt+1")
	if m.GridCursor.Project != 2 {
		t.Errorf("returning to board 1 gave Project = %d, want the saved 2", m.GridCursor.Project)
	}
}

// o/p must still cycle and wrap after being rewritten onto switchToBoard.
func TestCycleBoardsStillWraps(t *testing.T) {
	m := multiBoardModel(3, "alt")

	for i, want := range []int{1, 2, 0} {
		m = press(t, m, "p")
		if m.ActiveBoard != want {
			t.Errorf("p #%d: ActiveBoard = %d, want %d", i+1, m.ActiveBoard, want)
		}
	}

	for i, want := range []int{2, 1, 0} {
		m = press(t, m, "o")
		if m.ActiveBoard != want {
			t.Errorf("o #%d: ActiveBoard = %d, want %d", i+1, m.ActiveBoard, want)
		}
	}
}

// A watcher refresh can grow the board list without resizing SavedCursors;
// switching must not index past the end.
func TestSwitchSurvivesBoardCountGrowth(t *testing.T) {
	m := multiBoardModel(2, "alt")
	m.Boards = boards(4) // as RefreshMsg does: replaces wholesale

	m = press(t, m, "alt+4")
	if m.ActiveBoard != 3 {
		t.Errorf("alt+4 after growth: ActiveBoard = %d, want 3", m.ActiveBoard)
	}
	if len(m.SavedCursors) < len(m.Boards) {
		t.Errorf("SavedCursors len %d < boards %d", len(m.SavedCursors), len(m.Boards))
	}
}
