package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
	"github.com/lucky7xz/garlic/internal/remote"
	"golang.org/x/term"
)

type appState int

const (
	stateNormal appState = iota
	stateDeleting
	stateHiding
	stateInsertTyping
	stateInsertConfirm
	stateMoving
	stateRenaming
	stateHelp
	stateTooSmall
)

// Layout floors, derived from what the renderer actually needs rather than
// picked by feel. The board degrades gracefully as space shrinks (columns
// scroll horizontally via MaxVisible, statuses collapse to one via vertical
// focus), so the gate only has to catch sizes where nothing can be drawn.
//
//	minBoardWidth:  a single grid column is ColWidth+2 = 15 wide, but the
//	                header prefix ("🧄 [n/m] Workspace: ") is 20 columns, so it
//	                sets the floor. At exactly that width the prefix takes the
//	                whole line and the board name truncates away, which is the
//	                right thing to lose first.
//	minBoardHeight: header block (3) + one status block (title 1, bordered
//	                category header 3, separator 1, bordered project row 3,
//	                padding 1) + footer block (2) = 14.
const (
	minBoardWidth  = 20
	minBoardHeight = 14

	// garlicMark heads the board. It is fixed decoration, so unlike the planting
	// marks it can never change the header's width.
	garlicMark = "🧄"
)

// tooSmallToRender reports whether the board can be drawn at all at this size.
func tooSmallToRender(width, height int) bool {
	return width < minBoardWidth || height < minBoardHeight
}

// helpOverlayFloor is the size of the most compact help overlay there is, its
// single-column form. It is measured rather than declared, so it cannot drift
// out of step with the entry list.
func (m Model) helpOverlayFloor() (width, height int) {
	overlay := m.helpMenuCols(1)
	return lipgloss.Width(overlay), lipgloss.Height(overlay)
}

// fitsHelpOverlay reports whether the help overlay can be drawn without
// overflowing the terminal. Its floor is higher than the board's, so there is a
// band of sizes where the board is usable but help is not.
func (m Model) fitsHelpOverlay() bool {
	w, h := m.helpOverlayFloor()
	return m.TermWidth >= w && m.TermHeight >= h
}

type cursorState struct {
	Status   int
	Category int
	Project  int
	Offset   int
}

type Model struct {
	Boards       []domain.Board
	ActiveBoard  int
	MaxVisible   int
	ColWidth     int
	SelectedPath string
	ResourcePath string
	TermWidth    int
	TermHeight   int
	GridCursor   cursorState
	SavedCursors []cursorState
	UseAlt       bool
	AltModifier  string

	// Watcher
	UpdateChan  <-chan []domain.Board
	stopWatcher func()

	// Remotes is where a check can look; Planted is what the last one saw.
	// Garlic keeps no record of that: it is the answer to a question you asked
	// with `c`, it lives for the session and dies with it. It sits here rather
	// than on domain.Project because the watcher replaces Boards wholesale,
	// which would blink the marks out every time you saved a file.
	Remotes  []domain.Remote
	Planted  remote.Sighting
	Checking bool

	// Data state toggles
	ShowHidden   bool
	State        appState
	ActionTarget domain.Project
	DelInput     string
	InsertInput  string
	RenameInput  string
	ErrorMsg     string

	// Styles
	TitleStyle        lipgloss.Style
	HeaderStyle       lipgloss.Style
	CellStyle         lipgloss.Style
	EmptyCellStyle    lipgloss.Style
	SelectedCellStyle lipgloss.Style
	ResourceHintStyle lipgloss.Style
	PlantedHintStyle  lipgloss.Style
	HelpStyle         lipgloss.Style
	SeparatorStyle    lipgloss.Style
}

type RefreshMsg []domain.Board

func waitForUpdate(ch <-chan []domain.Board) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		boards, ok := <-ch
		if !ok {
			return nil
		}
		return RefreshMsg(boards)
	}
}

// checkMsg carries one check home. The error rides alongside a usable Sighting:
// one unreachable host must not discard what the others said.
type checkMsg struct {
	sighting remote.Sighting
	err      error
}

// checkRemotes asks every configured remote what it is holding. It runs as a
// tea.Cmd so that a slow or dead host cannot freeze the board.
func checkRemotes(remotes []domain.Remote) tea.Cmd {
	return func() tea.Msg {
		s, err := remote.Check(remotes)
		return checkMsg{sighting: s, err: err}
	}
}

func InitialModel(config domain.Config) Model {
	boardOpts := config.GetBoardOptions()
	var boards []domain.Board

	for _, opt := range boardOpts {
		boards = append(boards, filesystem.ScanBoard(opt))
	}

	updateChan, stopWatcher, _ := filesystem.WatchBoards(boardOpts)

	if len(boards) == 0 {
		boards = append(boards, domain.Board{
			Name:          "No Configured Roots",
			CategoryOrder: []string{},
			Grid:          make(map[string]map[string][]domain.Project),
			HiddenGrid:    make(map[string]map[string][]domain.Project),
			Statuses:      []string{},
		})
	}

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
		height = 24
	}

	initialState := stateNormal
	if tooSmallToRender(width, height) {
		initialState = stateTooSmall
	}

	return Model{
		Boards:       boards,
		ActiveBoard:  0,
		State:        initialState,
		TermWidth:    width,
		TermHeight:   height,
		SavedCursors: make([]cursorState, len(boards)),
		UpdateChan:   updateChan,
		stopWatcher:  stopWatcher,
		AltModifier:  config.AltModifier,
		Remotes:      config.Remotes,
	}
}

func (m Model) Init() tea.Cmd {
	return waitForUpdate(m.UpdateChan)
}

// StopWatcher tears down this model's filesystem watcher. The runner calls it
// once the TUI has quit, before the next lap builds a replacement.
func (m Model) StopWatcher() {
	if m.stopWatcher != nil {
		m.stopWatcher()
	}
}

// plantedOn names the remotes holding a project as of the last check, or nil.
// A project is planted exactly when its own path is a key in the manifest --
// plant always ships the project file itself, so this needs no prefix matching
// and no special case for the two bulb kinds.
func (m Model) plantedOn(board domain.Board, p domain.Project) []string {
	return m.Planted.On(remote.Rel(board.Opts, p.Path))
}

func (m Model) getSelectedProject() (domain.Project, bool) {
	currentBoard := m.Boards[m.ActiveBoard]
	if len(currentBoard.CategoryOrder) == 0 || len(currentBoard.Statuses) == 0 {
		return domain.Project{}, false
	}
	activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
	cat := currentBoard.CategoryOrder[m.GridCursor.Category]
	stat := currentBoard.Statuses[m.GridCursor.Status]
	projects := activeGrid[stat][cat]
	if len(projects) > 0 && m.GridCursor.Project < len(projects) {
		return projects[m.GridCursor.Project], true
	}
	return domain.Project{}, false
}

func (m Model) resourcePath(p domain.Project) string {
	currentBoard := m.Boards[m.ActiveBoard]
	name := strings.TrimSuffix(p.Name, currentBoard.Opts.Extension)
	path := filepath.Join(currentBoard.Opts.Path, p.Category, name)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return filepath.Join(currentBoard.Opts.Path, p.Category)
}

// boardJumpTarget reports the 1-based workspace a key requests, if any.
//
// alt+N is always accepted: it is the only modifier terminals reliably transmit
// alongside a digit. ctrl+1..9 has no distinct escape sequence at all, so
// honoring AltModifier alone would leave the binding dead for anyone who set it
// to "ctrl" -- even though ctrl+r, built the same way, does work. The configured
// modifier is accepted as well when it differs, costing nothing.
func (m Model) boardJumpTarget(key string) (int, bool) {
	digit, ok := strings.CutPrefix(key, "alt+")
	if !ok && m.AltModifier != "" && m.AltModifier != "alt" {
		digit, ok = strings.CutPrefix(key, m.AltModifier+"+")
	}
	if !ok || len(digit) != 1 || digit[0] < '1' || digit[0] > '9' {
		return 0, false
	}
	return int(digit[0] - '0'), true
}

// switchToBoard moves to a 0-based board index, parking the current cursor and
// restoring the one saved for the destination.
func (m *Model) switchToBoard(idx int) {
	// SavedCursors is sized once in InitialModel, but RefreshMsg replaces
	// m.Boards wholesale, so a watcher-driven change in board count can leave it
	// short. Grow it here rather than indexing past the end.
	for len(m.SavedCursors) < len(m.Boards) {
		m.SavedCursors = append(m.SavedCursors, cursorState{})
	}
	if idx < 0 || idx >= len(m.Boards) {
		return
	}
	m.SavedCursors[m.ActiveBoard] = m.GridCursor
	m.ActiveBoard = idx
	m.GridCursor = m.SavedCursors[m.ActiveBoard]
}

// Session is the position state that survives a trip out to the editor or file
// manager. The runner throws the whole Model away and rebuilds it every time it
// re-enters the TUI; this is what it carries across. It deliberately holds no
// Boards and no watcher handle, so keeping one between laps keeps nothing else
// alive.
type Session struct {
	ActiveBoard  int
	GridCursor   cursorState
	SavedCursors []cursorState
	ShowHidden   bool
	SelectedPath string
}

// Session snapshots where the cursor is, for Restore to re-establish once the
// boards have been rescanned.
func (m Model) Session() Session {
	s := Session{
		ActiveBoard:  m.ActiveBoard,
		GridCursor:   m.GridCursor,
		SavedCursors: append([]cursorState(nil), m.SavedCursors...),
		ShowHidden:   m.ShowHidden,
	}
	// Read from the grid rather than m.SelectedPath, which only the editor path
	// sets. Opening a resource folder with 'r' hands the file manager a directory
	// and leaves SelectedPath empty, but the cursor is still on a project and
	// that project is what we come back to.
	if p, ok := m.getSelectedProject(); ok {
		s.SelectedPath = p.Path
	}
	return s
}

// Restore puts the cursor back where a previous run left it. The boards have
// been rescanned since, and files may have been renamed, deleted, hidden or
// moved between statuses in the meantime, so nothing carried over is trusted:
// the path is looked up afresh and the indices are clamped.
func (m *Model) Restore(s Session) {
	if len(m.Boards) == 0 {
		return
	}
	m.ShowHidden = s.ShowHidden

	// copy handles a board list that grew or shrank since the snapshot: a short
	// source leaves the tail zeroed, a long one is truncated.
	m.SavedCursors = make([]cursorState, len(m.Boards))
	copy(m.SavedCursors, s.SavedCursors)

	if s.ActiveBoard >= 0 && s.ActiveBoard < len(m.Boards) {
		m.ActiveBoard = s.ActiveBoard
		m.GridCursor = s.GridCursor
	} else {
		// The workspace went away -- the config was edited between laps. Fall
		// back to the first board and the cursor last parked there.
		m.ActiveBoard = 0
		m.GridCursor = m.SavedCursors[0]
	}

	b := &m.Boards[m.ActiveBoard]
	if !m.seekPath(b, s.SelectedPath) {
		m.clampCursor(b)
	}
	m.RecalculateOffsets()
}

// seekPath moves the cursor onto the project with this path, reporting whether
// the board still holds it. Editing a file's #statustag moves it to a different
// status row, and following it there is what makes the cursor feel remembered
// rather than merely restored.
func (m *Model) seekPath(b *domain.Board, path string) bool {
	if path == "" {
		return false
	}
	grid := b.ActiveGrid(m.ShowHidden)
	for statusIdx, status := range b.Statuses {
		for catIdx, cat := range b.CategoryOrder {
			for projIdx, p := range grid[status][cat] {
				if p.Path == path {
					m.GridCursor.Status = statusIdx
					m.GridCursor.Category = catIdx
					m.GridCursor.Project = projIdx
					return true
				}
			}
		}
	}
	return false
}

// clampCursor pulls a cursor carried over from an earlier scan back inside the
// board it now points at.
func (m *Model) clampCursor(b *domain.Board) {
	if m.GridCursor.Status >= len(b.Statuses) {
		m.GridCursor.Status = len(b.Statuses) - 1
	}
	if m.GridCursor.Status < 0 {
		m.GridCursor.Status = 0
	}
	if m.GridCursor.Category >= len(b.CategoryOrder) {
		m.GridCursor.Category = len(b.CategoryOrder) - 1
	}
	if m.GridCursor.Category < 0 {
		m.GridCursor.Category = 0
	}
	m.clampProjectCursor(b)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	currentBoard := &m.Boards[m.ActiveBoard]

	switch msg := msg.(type) {
	case RefreshMsg:
		m.Boards = msg
		return m, waitForUpdate(m.UpdateChan)

	case checkMsg:
		m.Checking = false
		// Only a check that reached something replaces what is on screen; a total
		// failure leaves the previous answer alone and just says what went wrong.
		if msg.sighting.Checked() {
			m.Planted = msg.sighting
		}
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		if tooSmallToRender(m.TermWidth, m.TermHeight) {
			m.State = stateTooSmall
		} else if m.State == stateTooSmall {
			m.State = stateNormal
		}
		// A resize can shrink the terminal below what the help overlay needs
		// while the board itself still fits; drop back to the board.
		if m.State == stateHelp && !m.fitsHelpOverlay() {
			m.State = stateNormal
		}
	case tea.KeyMsg:
		m.ErrorMsg = ""
		if m.State == stateTooSmall {
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				return m, tea.Quit
			}
			return m, nil
		}
		// --- DELETE STATE OVERRIDE ---
		if m.State == stateDeleting {
			s := msg.String()
			if s == "esc" {
				m.State = stateNormal
			} else if s == "backspace" {
				if len(m.DelInput) > 0 {
					m.DelInput = m.DelInput[:len(m.DelInput)-1]
				}
			} else if s == "enter" {
				if m.DelInput == "delete" {
					if _, err := os.Stat(m.ActionTarget.Path); os.IsNotExist(err) {
						m.ErrorMsg = "Project file no longer exists"
						m.State = stateNormal
						return m, nil
					}
					if err := filesystem.DeleteProject(m.ActionTarget.Path); err == nil {
						m.Boards[m.ActiveBoard] = filesystem.ScanBoard(currentBoard.Opts)
						m.GridCursor.Status, m.GridCursor.Category, m.GridCursor.Project = 0, 0, 0
					} else {
						m.ErrorMsg = err.Error()
					}
					m.State = stateNormal
				} else {
					m.State = stateNormal
				}
			} else if len(s) == 1 {
				m.DelInput += s
			}
			return m, nil
		}

		// --- HIDE PROMPT STATE OVERRIDE ---
		if m.State == stateHiding {
			if msg.String() == "y" {
				if _, err := os.Stat(m.ActionTarget.Path); os.IsNotExist(err) {
					m.ErrorMsg = "Project file no longer exists"
				} else {
					filesystem.ToggleHiddenMarker(m.ActionTarget.Path)
					m.Boards[m.ActiveBoard] = filesystem.ScanBoard(currentBoard.Opts)
				}
			}
			m.State = stateNormal
			return m, nil
		}

		// --- INSERT TYPING STATE OVERRIDE ---
		if m.State == stateInsertTyping {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.State = stateNormal
				m.InsertInput = ""
			case "enter":
				if len(strings.TrimSpace(m.InsertInput)) > 0 {
					m.State = stateInsertConfirm
				} else {
					m.State = stateNormal
					m.InsertInput = ""
				}
			case "backspace":
				if len(m.InsertInput) > 0 {
					m.InsertInput = m.InsertInput[:len(m.InsertInput)-1]
				}
			case "space":
				m.InsertInput += " "
			default:
				s := msg.String()
				if len(s) == 1 && strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") {
					m.InsertInput += s
				}
			}
			return m, nil
		}

		// --- INSERT CONFIRM STATE OVERRIDE ---
		if m.State == stateInsertConfirm {
			if msg.String() == "y" {
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]

				newFileName := strings.TrimSpace(m.InsertInput)
				if !strings.HasSuffix(newFileName, currentBoard.Opts.Extension) {
					newFileName += currentBoard.Opts.Extension
				}

				targetDir := filepath.Join(currentBoard.Opts.Path, cat)
				if _, err := os.Stat(targetDir); os.IsNotExist(err) {
					m.ErrorMsg = "Category folder no longer exists"
				} else {
					targetPath := filepath.Join(targetDir, newFileName)
					if err := filesystem.CreateProject(targetPath, stat); err == nil {
						m.Boards[m.ActiveBoard] = filesystem.ScanBoard(currentBoard.Opts)
					} else {
						m.ErrorMsg = err.Error()
					}
				}
			}
			m.State = stateNormal
			m.InsertInput = ""
			return m, nil
		}

		// --- MOVE STATE OVERRIDE ---
		if m.State == stateMoving {
			switch msg.String() {
			case "esc", "q":
				m.State = stateNormal
			case "enter", " ":
				newStatus := currentBoard.Statuses[m.GridCursor.Status]
				if _, err := os.Stat(m.ActionTarget.Path); os.IsNotExist(err) {
					m.ErrorMsg = "Project file no longer exists"
				} else if err := filesystem.UpdateProjectStatus(m.ActionTarget.Path, newStatus); err == nil {
					m.Boards[m.ActiveBoard] = filesystem.ScanBoard(currentBoard.Opts)
				} else {
					m.ErrorMsg = err.Error()
				}
				m.State = stateNormal
			case "up", "k", "w":
				if m.GridCursor.Status > 0 {
					m.GridCursor.Status--
				}
			case "down", "j", "s":
				if m.GridCursor.Status < len(currentBoard.Statuses)-1 {
					m.GridCursor.Status++
				}
			}
			return m, nil
		}

		// --- RENAME STATE OVERRIDE ---
		if m.State == stateRenaming {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.State = stateNormal
				m.RenameInput = ""
			case "enter":
				newName := strings.TrimSpace(m.RenameInput)
				if len(newName) > 0 {
					if err := filesystem.RenameProject(m.ActionTarget.Path, newName); err == nil {
						m.Boards[m.ActiveBoard] = filesystem.ScanBoard(currentBoard.Opts)
					} else {
						m.ErrorMsg = err.Error()
					}
				}
				m.State = stateNormal
				m.RenameInput = ""
			case "backspace":
				if len(m.RenameInput) > 0 {
					m.RenameInput = m.RenameInput[:len(m.RenameInput)-1]
				}
			case "space":
				m.RenameInput += " "
			default:
				s := msg.String()
				if len(s) == 1 && strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") {
					m.RenameInput += s
				}
			}
			return m, nil
		}

		// --- HELP STATE OVERRIDE ---
		if m.State == stateHelp {
			if msg.String() == "q" || msg.String() == "esc" || msg.String() == "?" {
				m.State = stateNormal
			}
			return m, nil
		}

		// --- NORMAL NAVIGATION STATE ---
		modR := m.AltModifier + "+r"
		modEnter := m.AltModifier + "+enter"
		modSpace := m.AltModifier + "+ "

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "?":
			if m.fitsHelpOverlay() {
				m.State = stateHelp
			} else {
				w, h := m.helpOverlayFloor()
				m.ErrorMsg = fmt.Sprintf("Need %dx%d for help (have %dx%d)", w, h, m.TermWidth, m.TermHeight)
			}

		case "tab":
			m.ShowHidden = !m.ShowHidden
			m.GridCursor.Status, m.GridCursor.Category, m.GridCursor.Project = 0, 0, 0

		case "e":
			if p, ok := m.getSelectedProject(); ok {
				m.State, m.ActionTarget = stateRenaming, p
				m.RenameInput = strings.TrimSuffix(p.Name, currentBoard.Opts.Extension)
			}

		case "m":
			if p, ok := m.getSelectedProject(); ok {
				m.State, m.ActionTarget = stateMoving, p
			}

		case "u":
			if p, ok := m.getSelectedProject(); ok {
				m.State, m.ActionTarget = stateHiding, p
			}

		case "i":
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				m.State, m.InsertInput = stateInsertTyping, ""
			}

		case "delete":
			if p, ok := m.getSelectedProject(); ok {
				m.State, m.ActionTarget, m.DelInput = stateDeleting, p, ""
			}

		case "o":
			m.switchToBoard((m.ActiveBoard - 1 + len(m.Boards)) % len(m.Boards))

		case "p":
			m.switchToBoard((m.ActiveBoard + 1) % len(m.Boards))
		case "up", "k", "w":
			m.moveUp(currentBoard)
		case "down", "j", "s":
			m.moveDown(currentBoard)
		case "left", "h", "a":
			m.moveLeft(currentBoard)
		case "right", "l", "d":
			m.moveRight(currentBoard)
		case "r", modR:
			if p, ok := m.getSelectedProject(); ok {
				m.ResourcePath = m.resourcePath(p)
				if msg.String() == modR {
					m.UseAlt = true
				}
				return m, tea.Quit
			}

		case "c":
			if m.Checking {
				break
			}
			m.Checking = true
			return m, checkRemotes(m.Remotes)

		case "enter", " ", modEnter, modSpace:
			if p, ok := m.getSelectedProject(); ok {
				m.SelectedPath = p.Path
				if msg.String() == modEnter || msg.String() == modSpace {
					m.UseAlt = true
				}
				return m, tea.Quit
			}

		default:
			if n, ok := m.boardJumpTarget(msg.String()); ok {
				if n > len(m.Boards) {
					m.ErrorMsg = fmt.Sprintf("No workspace %d (have %d)", n, len(m.Boards))
				} else {
					m.switchToBoard(n - 1)
				}
			}
		}
	}

	m.RecalculateOffsets()
	return m, nil
}

func (m *Model) RecalculateOffsets() {
	currentBoard := m.Boards[m.ActiveBoard]
	m.ColWidth = 13
	m.MaxVisible = m.TermWidth / (m.ColWidth + 2)
	if m.MaxVisible < 1 {
		m.MaxVisible = 1
	}

	// Center-lock logic
	m.GridCursor.Offset = m.GridCursor.Category - (m.MaxVisible / 2)

	// Clamp
	numTotalColumns := len(currentBoard.CategoryOrder)
	if m.GridCursor.Offset > numTotalColumns-m.MaxVisible {
		m.GridCursor.Offset = numTotalColumns - m.MaxVisible
	}
	if m.GridCursor.Offset < 0 {
		m.GridCursor.Offset = 0
	}
}

func (m *Model) moveUp(b *domain.Board) {
	if len(b.CategoryOrder) == 0 || len(b.Statuses) == 0 {
		return
	}
	if m.GridCursor.Project > 0 {
		m.GridCursor.Project--
	} else if m.GridCursor.Status > 0 {
		m.GridCursor.Status--
		cat := b.CategoryOrder[m.GridCursor.Category]
		stat := b.Statuses[m.GridCursor.Status]
		projectsInCell := b.ActiveGrid(m.ShowHidden)[stat][cat]
		m.GridCursor.Project = len(projectsInCell) - 1
		if m.GridCursor.Project < 0 {
			m.GridCursor.Project = 0
		}
	}
}

func (m *Model) moveDown(b *domain.Board) {
	if len(b.CategoryOrder) == 0 || len(b.Statuses) == 0 {
		return
	}
	cat := b.CategoryOrder[m.GridCursor.Category]
	stat := b.Statuses[m.GridCursor.Status]
	projectsInCell := b.ActiveGrid(m.ShowHidden)[stat][cat]

	if m.GridCursor.Project < len(projectsInCell)-1 {
		m.GridCursor.Project++
	} else if m.GridCursor.Status < len(b.Statuses)-1 {
		m.GridCursor.Status++
		m.GridCursor.Project = 0
	}
}

func (m *Model) moveLeft(b *domain.Board) {
	if len(b.CategoryOrder) == 0 || len(b.Statuses) == 0 {
		return
	}
	originalCategory := m.GridCursor.Category
	for c := originalCategory - 1; c >= 0; c-- {
		cat := b.CategoryOrder[c]
		stat := b.Statuses[m.GridCursor.Status]
		if len(b.ActiveGrid(m.ShowHidden)[stat][cat]) >= 0 {
			m.GridCursor.Category = c
			m.clampProjectCursor(b)
			return
		}
	}
}

func (m *Model) moveRight(b *domain.Board) {
	if len(b.CategoryOrder) == 0 || len(b.Statuses) == 0 {
		return
	}
	originalCategory := m.GridCursor.Category
	for c := originalCategory + 1; c < len(b.CategoryOrder); c++ {
		cat := b.CategoryOrder[c]
		stat := b.Statuses[m.GridCursor.Status]
		if len(b.ActiveGrid(m.ShowHidden)[stat][cat]) >= 0 {
			m.GridCursor.Category = c
			m.clampProjectCursor(b)
			return
		}
	}
}

func (m *Model) clampProjectCursor(b *domain.Board) {
	if len(b.CategoryOrder) == 0 || len(b.Statuses) == 0 {
		return
	}
	cat := b.CategoryOrder[m.GridCursor.Category]
	stat := b.Statuses[m.GridCursor.Status]
	projectsInCell := b.ActiveGrid(m.ShowHidden)[stat][cat]

	if m.GridCursor.Project >= len(projectsInCell) {
		m.GridCursor.Project = len(projectsInCell) - 1
		if m.GridCursor.Project < 0 {
			m.GridCursor.Project = 0
		}
	}
}

func (m Model) View() string {
	if m.State == stateTooSmall {
		// Kept to three short lines so the notice itself fits the sizes it reports on.
		msg := fmt.Sprintf("Terminal too small\nneed %dx%d\nhave %dx%d",
			minBoardWidth, minBoardHeight, m.TermWidth, m.TermHeight)
		return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, msg)
	}
	currentBoard := m.Boards[m.ActiveBoard]
	activeGrid := currentBoard.ActiveGrid(m.ShowHidden)

	numColumns := len(currentBoard.CategoryOrder)
	if numColumns == 0 {
		msg := "No Areas found for Board: " + currentBoard.Name + "\n\nTip: run `garlic init` to scaffold a demo file system."
		return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, msg)
	}
	if len(currentBoard.Statuses) == 0 {
		return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, "No Statuses defined for Board: "+currentBoard.Name)
	}

	numTotalColumns := len(currentBoard.CategoryOrder)
	endIdx := m.GridCursor.Offset + m.MaxVisible
	if endIdx > numTotalColumns {
		endIdx = numTotalColumns
	}
	visibleCategories := currentBoard.CategoryOrder[m.GridCursor.Offset:endIdx]
	sepWidth := (m.ColWidth + 2) * len(visibleCategories)

	// Systematic Vertical Check: count the lines this view is about to emit.
	// This has to match the render below exactly. If it undercounts, focus mode
	// stays off, the view falls through to the centered Place() at the bottom of
	// this function, and content taller than the terminal is clipped at *both*
	// ends -- which silently eats the "Status:" labels.
	totalHeight := 5 // Header block (title + separator + blank) + footer block (hint + blank)
	for _, status := range currentBoard.Statuses {
		totalHeight += 6 // Status title + category headers (3, bordered) + separator + trailing padding
		maxRows := 0
		for _, cat := range currentBoard.CategoryOrder {
			if count := len(activeGrid[status][cat]); count > maxRows {
				maxRows = count
			}
		}
		if maxRows == 0 {
			maxRows = 1
		}
		totalHeight += maxRows * 3 // Each bordered project row is 3 lines tall
	}

	needsVerticalFocus := totalHeight > m.TermHeight

	activeHeaderStyle := m.HeaderStyle.Width(m.ColWidth)
	activeCellStyle := m.CellStyle.Width(m.ColWidth)
	activeSelectedCellStyle := m.SelectedCellStyle.Width(m.ColWidth)
	activeEmptyCellStyle := m.EmptyCellStyle.Width(m.ColWidth)
	activeTitleStyle := m.TitleStyle
	contentWidth := m.ColWidth - 2

	// Flexible Header
	viewMode := ""
	if m.ShowHidden {
		viewMode = " [HIDDEN]"
		activeHeaderStyle = activeHeaderStyle.Faint(true).Bold(false)
		activeSelectedCellStyle = activeSelectedCellStyle.Faint(true).Bold(false)
		activeTitleStyle = activeTitleStyle.Faint(true).Bold(false)
	}
	prefix := fmt.Sprintf("%s [%d/%d] Workspace: ", garlicMark, m.ActiveBoard+1, len(m.Boards))
	// The board name is arbitrary length, so it -- not the grid -- is what pushes
	// the header past the terminal edge. Trim it to whatever the prefix leaves.
	boardName := truncate(currentBoard.Name, m.TermWidth-lipgloss.Width(prefix)-lipgloss.Width(viewMode))
	headerStr := activeTitleStyle.Render(prefix) + boardName + activeTitleStyle.Render(viewMode) + "\n" + m.SeparatorStyle.Faint(true).Render(strings.Repeat("─", sepWidth)) + "\n"

	// One mark per fact: a fully planted area is marked on its column, and its
	// projects then say nothing, since the column already said it.
	areaMark := make(map[string]bool, len(visibleCategories))
	for _, category := range visibleCategories {
		areaMark[category] = m.areaPlanted(currentBoard, category)
	}

	var gridRows []string
	for statusIdx, status := range currentBoard.Statuses {
		// Vertical camera: Only show active status area if board is too tall
		if needsVerticalFocus && statusIdx != m.GridCursor.Status {
			continue
		}

		// Count total projects in this status across all categories
		totalInStatus := 0
		for _, catProjects := range activeGrid[status] {
			totalInStatus += len(catProjects)
		}

		gridRows = append(gridRows, activeTitleStyle.Render(fmt.Sprintf("Status: %s (%d)", status, totalInStatus)))

		var headerCells []string
		for _, category := range visibleCategories {
			head := projectCell(category, false, areaMark[category],
				contentWidth, m.ResourceHintStyle, m.PlantedHintStyle)
			headerCells = append(headerCells, activeHeaderStyle.Render(head))
		}
		gridRows = append(gridRows, lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
		gridRows = append(gridRows, m.SeparatorStyle.Faint(true).Render(strings.Repeat("─", sepWidth)))

		maxRows := 0
		for _, category := range visibleCategories {
			if count := len(activeGrid[status][category]); count > maxRows {
				maxRows = count
			}
		}
		if maxRows == 0 {
			maxRows = 1
		}

		for i := 0; i < maxRows; i++ {
			var rowCells []string
			for visIdx, category := range visibleCategories {
				catIdx := visIdx + m.GridCursor.Offset
				projects := activeGrid[status][category]
				if i < len(projects) {
					p := projects[i]
					name := p.Name
					name = strings.TrimSuffix(name, ".clove.md")
					name = strings.TrimSuffix(name, ".md")

					// Asked every render rather than at scan time on purpose: the
					// watcher does not follow resource folders that existed before
					// launch, so a file dropped in through a file manager would
					// leave a scan-time answer stale.
					resPath := filepath.Join(currentBoard.Opts.Path, category, name)
					hasResource := filesystem.HasEntries(resPath)

					style := activeCellStyle
					if statusIdx == m.GridCursor.Status && catIdx == m.GridCursor.Category && i == m.GridCursor.Project {
						style = activeSelectedCellStyle
					}

					planted := !areaMark[category] && len(m.plantedOn(currentBoard, p)) > 0
					cellContent := projectCell(name, hasResource, planted, contentWidth, m.ResourceHintStyle, m.PlantedHintStyle)
					rowCells = append(rowCells, style.Render(cellContent))
				} else {
					style := activeEmptyCellStyle
					if statusIdx == m.GridCursor.Status && catIdx == m.GridCursor.Category && i == m.GridCursor.Project {
						style = activeSelectedCellStyle
					}
					rowCells = append(rowCells, style.Render(""))
				}
			}
			gridRows = append(gridRows, lipgloss.JoinHorizontal(lipgloss.Top, rowCells...))
		}
		gridRows = append(gridRows, "") // Padding block
	}

	gridStr := lipgloss.JoinVertical(lipgloss.Left, gridRows...) // Explicitly left align internally

	var footerStr string
	if m.ErrorMsg != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Align(lipgloss.Center)
		footerStr = errorStyle.Render("ERROR: "+m.ErrorMsg) + "\n"
	} else if m.State == stateDeleting {
		dangerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Align(lipgloss.Center)
		footerStr = dangerStyle.Render(fmt.Sprintf("WARNING! Type 'delete' to PERMANENTLY ERASE %s [%s]", m.ActionTarget.Name, m.DelInput)) + "\n"
	} else if m.State == stateInsertTyping {
		cat := currentBoard.CategoryOrder[m.GridCursor.Category]
		insertStyle := m.TitleStyle.Align(lipgloss.Center)
		footerStr = insertStyle.Render(fmt.Sprintf("CREATE IN %s: %s_", cat, m.InsertInput)) + "\n"
	} else if m.State == stateInsertConfirm {
		newFileName := strings.TrimSpace(m.InsertInput)
		if !strings.HasSuffix(newFileName, currentBoard.Opts.Extension) {
			newFileName += currentBoard.Opts.Extension
		}
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Align(lipgloss.Center)
		footerStr = warningStyle.Render(fmt.Sprintf("CONFIRM: Create '%s'? (y/*)", newFileName)) + "\n"
	} else if m.State == stateHiding {
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Align(lipgloss.Center)
		verb := "HIDE"
		if m.ShowHidden {
			verb = "UNHIDE"
		}
		footerStr = warningStyle.Render(fmt.Sprintf("%s %s? (y/*)", verb, m.ActionTarget.Name)) + "\n"
	} else if m.State == stateMoving {
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true).Align(lipgloss.Center)
		newStatus := currentBoard.Statuses[m.GridCursor.Status]
		footerStr = warningStyle.Render(fmt.Sprintf("MOVING %s TO %s... (enter: drop • esc: cancel)", m.ActionTarget.Name, newStatus)) + "\n"
	} else if m.State == stateRenaming {
		insertStyle := m.TitleStyle.Align(lipgloss.Center)
		footerStr = insertStyle.Render(fmt.Sprintf("RENAME: %s_", m.RenameInput)) + "\n"
	} else {
		footerStr = m.idleFooter() + "\n"
	}

	finalView := lipgloss.JoinVertical(lipgloss.Center, headerStr, gridStr, footerStr)

	if m.State == stateHelp {
		overlay := m.helpMenu()
		return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, overlay, lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("0")))
	}

	// In focus mode, we force-anchor to the very top and remove centering
	if needsVerticalFocus {
		return finalView
	}

	return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, finalView)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if max < 3 {
		if len(s) > max {
			return s[:max]
		}
		return s
	}
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
