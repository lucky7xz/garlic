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
	stateTooSmall
)

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
	UpdateChan <-chan []domain.Board

	// Data state toggles
	ShowHidden   bool
	State        appState
	ActionTarget domain.Project
	DelInput     string
	InsertInput  string
	ErrorMsg     string

	// Styles
	TitleStyle        lipgloss.Style
	HeaderStyle       lipgloss.Style
	CellStyle         lipgloss.Style
	EmptyCellStyle    lipgloss.Style
	SelectedCellStyle lipgloss.Style
	ResourceHintStyle lipgloss.Style
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

func InitialModel(config domain.Config) Model {
	boardOpts := config.GetBoardOptions()
	var boards []domain.Board

	for _, opt := range boardOpts {
		boards = append(boards, filesystem.ScanBoard(opt))
	}

	updateChan, _ := filesystem.WatchBoards(boardOpts)

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
	if width < 70 || height < 15 {
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
		AltModifier:  config.AltModifier,
	}
}

func (m Model) Init() tea.Cmd {
	return waitForUpdate(m.UpdateChan)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	currentBoard := &m.Boards[m.ActiveBoard]

	switch msg := msg.(type) {
	case RefreshMsg:
		m.Boards = msg
		return m, waitForUpdate(m.UpdateChan)

	case tea.WindowSizeMsg:
		m.TermWidth = msg.Width
		m.TermHeight = msg.Height
		if m.TermWidth < 70 || m.TermHeight < 15 {
			m.State = stateTooSmall
		} else if m.State == stateTooSmall {
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

		// --- NORMAL NAVIGATION STATE ---
		modR := m.AltModifier + "+r"
		modEnter := m.AltModifier + "+enter"
		modSpace := m.AltModifier + "+ "

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab":
			m.ShowHidden = !m.ShowHidden
			m.GridCursor.Status, m.GridCursor.Category, m.GridCursor.Project = 0, 0, 0

		case "m":
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]
				projectsInCell := activeGrid[stat][cat]
				if len(projectsInCell) > 0 && m.GridCursor.Project < len(projectsInCell) {
					m.State = stateMoving
					m.ActionTarget = projectsInCell[m.GridCursor.Project]
				}
			}

		case "u":
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]
				activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
				projectsInCell := activeGrid[stat][cat]
				if len(projectsInCell) > 0 && m.GridCursor.Project < len(projectsInCell) {
					m.State = stateHiding
					m.ActionTarget = projectsInCell[m.GridCursor.Project]
				}
			}

		case "i":
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				m.State = stateInsertTyping
				m.InsertInput = ""
			}

		case "delete":
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]
				activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
				projectsInCell := activeGrid[stat][cat]
				if len(projectsInCell) > 0 && m.GridCursor.Project < len(projectsInCell) {
					m.State = stateDeleting
					m.ActionTarget = projectsInCell[m.GridCursor.Project]
					m.DelInput = ""
				}
			}

		case "o":
			m.SavedCursors[m.ActiveBoard] = m.GridCursor
			m.ActiveBoard = (m.ActiveBoard - 1 + len(m.Boards)) % len(m.Boards)
			m.GridCursor = m.SavedCursors[m.ActiveBoard]

		case "p":
			m.SavedCursors[m.ActiveBoard] = m.GridCursor
			m.ActiveBoard = (m.ActiveBoard + 1) % len(m.Boards)
			m.GridCursor = m.SavedCursors[m.ActiveBoard]
		case "up", "k", "w":
			m.moveUp(currentBoard)
		case "down", "j", "s":
			m.moveDown(currentBoard)
		case "left", "h", "a":
			m.moveLeft(currentBoard)
		case "right", "l", "d":
			m.moveRight(currentBoard)
		case "r", modR:
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]
				activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
				projectsInCell := activeGrid[stat][cat]
				if len(projectsInCell) > 0 && m.GridCursor.Project < len(projectsInCell) {
					p := projectsInCell[m.GridCursor.Project]
					baseName := strings.TrimSuffix(p.Name, currentBoard.Opts.Extension)
					hypotheticalPath := filepath.Join(currentBoard.Opts.Path, cat, baseName)

					if info, err := os.Stat(hypotheticalPath); err == nil && info.IsDir() {
						m.ResourcePath = hypotheticalPath
					} else {
						m.ResourcePath = filepath.Join(currentBoard.Opts.Path, cat)
					}
					if msg.String() == modR {
						m.UseAlt = true
					}
					return m, tea.Quit
				}
			}

		case "enter", " ", modEnter, modSpace:
			if len(currentBoard.CategoryOrder) > 0 && len(currentBoard.Statuses) > 0 {
				cat := currentBoard.CategoryOrder[m.GridCursor.Category]
				stat := currentBoard.Statuses[m.GridCursor.Status]
				activeGrid := currentBoard.ActiveGrid(m.ShowHidden)
				projectsInCell := activeGrid[stat][cat]
				if len(projectsInCell) > 0 && m.GridCursor.Project < len(projectsInCell) {
					m.SelectedPath = projectsInCell[m.GridCursor.Project].Path
					if msg.String() == modEnter || msg.String() == modSpace {
						m.UseAlt = true
					}
					return m, tea.Quit
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
		return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, "Terminal too small!")
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

	// Systematic Vertical Check: Estimate board height
	totalHeight := 8 // Headers + Footer + Padding
	for _, status := range currentBoard.Statuses {
		totalHeight += 5 // Status header + Category headers (3 lines) + Separator + Padding
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
	prefix := fmt.Sprintf("[%d/%d] Workspace: ", m.ActiveBoard+1, len(m.Boards))
	headerStr := activeTitleStyle.Render(prefix) + currentBoard.Name + activeTitleStyle.Render(viewMode) + "\n" + m.SeparatorStyle.Faint(true).Render(strings.Repeat("─", sepWidth)) + "\n"

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
			headerCells = append(headerCells, activeHeaderStyle.Render(truncate(category, contentWidth)))
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

					// Check for resource folder indicator
					resPath := filepath.Join(currentBoard.Opts.Path, category, name)
					hasResource := false
					if info, err := os.Stat(resPath); err == nil && info.IsDir() {
						hasResource = true
					}

					style := activeCellStyle
					if statusIdx == m.GridCursor.Status && catIdx == m.GridCursor.Category && i == m.GridCursor.Project {
						style = activeSelectedCellStyle
					}

					var cellContent string
					if hasResource {
						cellContent = truncate(name, contentWidth-1) + m.ResourceHintStyle.Render("*")
					} else {
						cellContent = truncate(name, contentWidth)
					}
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
		insertStyle := m.TitleStyle.Copy().Align(lipgloss.Center)
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
	} else {
		line1 := "arrows/hjkl: move • enter: select • r: resource dir • o/p: switch boards • tab: toggle view"
		line2 := "m: move • u: hide/unhide • i: insert • del: purge • q: quit"
		footerStr = m.HelpStyle.Align(lipgloss.Center).Render(line1+"\n"+line2+"\n* dedicated resources found") + "\n"
	}

	finalView := lipgloss.JoinVertical(lipgloss.Center, headerStr, gridStr, footerStr)

	// In focus mode, we force-anchor to the very top and remove centering
	if needsVerticalFocus {
		return finalView
	}

	return lipgloss.Place(m.TermWidth, m.TermHeight, lipgloss.Center, lipgloss.Center, finalView)
}

func truncate(s string, max int) string {
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
