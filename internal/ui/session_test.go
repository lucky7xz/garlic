package ui

import (
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

// restored simulates one lap of the runner loop: snapshot the model, rebuild a
// fresh one over newBoards as InitialModel would, then Restore into it.
func restored(m Model, newBoards []domain.Board) Model {
	s := m.Session()
	next := Model{
		Boards:       newBoards,
		TermWidth:    m.TermWidth,
		TermHeight:   m.TermHeight,
		SavedCursors: make([]cursorState, len(newBoards)),
	}
	next.Restore(s)
	return next
}

func TestRestoreKeepsWorkspaceAndCursor(t *testing.T) {
	m := multiBoardModel(3, "alt")
	m = press(t, m, "alt+2")
	m.GridCursor.Project = 2

	next := restored(m, boards(3))

	if next.ActiveBoard != 1 {
		t.Errorf("ActiveBoard = %d, want the workspace we left from (1)", next.ActiveBoard)
	}
	if next.GridCursor.Project != 2 {
		t.Errorf("GridCursor.Project = %d, want 2", next.GridCursor.Project)
	}
}

func TestRestoreKeepsHiddenView(t *testing.T) {
	m := multiBoardModel(2, "alt")
	m = press(t, m, "tab")
	if !m.ShowHidden {
		t.Fatal("tab did not enter hidden view")
	}

	if next := restored(m, boards(2)); !next.ShowHidden {
		t.Error("returning from an editor dropped out of hidden view")
	}
}

// The common case: the file's #statustag was edited, so the project sits in a
// different status row than the one the cursor recorded.
func TestRestoreFollowsProjectThatChangedStatus(t *testing.T) {
	before := board([]string{"todo", "done"}, []string{"cat"}, 2)
	m := Model{
		Boards:       []domain.Board{before},
		TermWidth:    80,
		TermHeight:   40,
		SavedCursors: make([]cursorState, 1),
	}
	m.GridCursor = cursorState{Status: 0, Category: 0, Project: 1}

	moved, ok := m.getSelectedProject()
	if !ok {
		t.Fatal("no project under the cursor to move")
	}

	// Rescan result: that project is now the only entry under "done".
	after := board([]string{"todo", "done"}, []string{"cat"}, 0)
	after.Grid["todo"]["cat"] = []domain.Project{before.Grid["todo"]["cat"][0]}
	moved.Status = "done"
	after.Grid["done"]["cat"] = []domain.Project{moved}

	next := restored(m, []domain.Board{after})

	if next.GridCursor.Status != 1 || next.GridCursor.Project != 0 {
		t.Errorf("cursor at status %d project %d, want the moved project at status 1 project 0",
			next.GridCursor.Status, next.GridCursor.Project)
	}
	got, ok := next.getSelectedProject()
	if !ok || got.Path != moved.Path {
		t.Errorf("selected %q, want the project we left on (%q)", got.Path, moved.Path)
	}
}

// Renamed or deleted from inside the file manager: the path is gone, so the
// saved indices are all there is to fall back on.
func TestRestoreFallsBackWhenProjectIsGone(t *testing.T) {
	m := multiBoardModel(2, "alt")
	m.GridCursor.Project = 1

	// One project per cell, so the recorded project index no longer exists either.
	shrunk := board([]string{"todo"}, []string{"cat"}, 1)
	shrunk.Grid["todo"]["cat"][0].Path = "/somewhere/else.md"

	next := restored(m, []domain.Board{shrunk, shrunk})

	if next.GridCursor.Project != 0 {
		t.Errorf("GridCursor.Project = %d, want it clamped to 0", next.GridCursor.Project)
	}
	if _, ok := next.getSelectedProject(); !ok {
		t.Error("cursor did not land on a real project")
	}
}

func TestRestoreClampsToFewerBoards(t *testing.T) {
	m := multiBoardModel(4, "alt")
	m = press(t, m, "alt+4")

	next := restored(m, boards(2))

	if next.ActiveBoard < 0 || next.ActiveBoard >= 2 {
		t.Errorf("ActiveBoard = %d, want a valid index into 2 boards", next.ActiveBoard)
	}
	if len(next.SavedCursors) != 2 {
		t.Errorf("SavedCursors len = %d, want 2", len(next.SavedCursors))
	}
}

func TestRestoreClampsCursorBeyondBoardShape(t *testing.T) {
	wide := board([]string{"a", "b", "c"}, []string{"x", "y", "z"}, 2)
	m := Model{
		Boards:       []domain.Board{wide},
		TermWidth:    80,
		TermHeight:   40,
		SavedCursors: make([]cursorState, 1),
	}
	m.GridCursor = cursorState{Status: 2, Category: 2, Project: 1}

	// The rescan found a much smaller board, and none of the old paths survive.
	narrow := board([]string{"a"}, []string{"x"}, 1)
	narrow.Grid["a"]["x"][0].Path = "/new.md"

	next := restored(m, []domain.Board{narrow})

	if next.GridCursor.Status != 0 || next.GridCursor.Category != 0 || next.GridCursor.Project != 0 {
		t.Errorf("cursor = %+v, want everything clamped to 0", next.GridCursor)
	}
	if _, ok := next.getSelectedProject(); !ok {
		t.Error("clamped cursor does not point at a project")
	}
}

// An empty board list is reachable only if InitialModel's placeholder is gone,
// but Restore indexes Boards and must not be the thing that panics.
func TestRestoreOnEmptyBoardsIsANoOp(t *testing.T) {
	m := multiBoardModel(2, "alt")
	next := Model{}
	next.Restore(m.Session())

	if next.ActiveBoard != 0 {
		t.Errorf("ActiveBoard = %d, want 0", next.ActiveBoard)
	}
}

func TestSessionRemembersProjectWhenOpeningAResourceFolder(t *testing.T) {
	m := multiBoardModel(2, "alt")
	m.GridCursor.Project = 1

	// 'r' hands the file manager a directory and leaves SelectedPath empty; the
	// board position still has to come back.
	m = press(t, m, "r")
	if m.ResourcePath == "" {
		t.Fatal("r did not set ResourcePath")
	}
	if m.SelectedPath != "" {
		t.Fatal("r unexpectedly set SelectedPath; this test no longer covers its case")
	}

	if got := m.Session().SelectedPath; got == "" {
		t.Error("Session lost the project under the cursor on the resource-folder path")
	}

	if next := restored(m, boards(2)); next.GridCursor.Project != 1 {
		t.Errorf("GridCursor.Project = %d, want 1", next.GridCursor.Project)
	}
}
