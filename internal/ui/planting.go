package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/remote"
)

// Everything the board says about planting lives on one line: the footer, which
// already changes with context. The header is left alone deliberately -- the
// view is centered as a block, so widening the title line would shift the whole
// board sideways the moment you pressed `g`.
//
//	never checked          ?: help • q: quit
//	checked, plain card    ?: help • q: quit • 🌱 checked 14:20
//	checked, planted card  🌱 planted on agent 3d ago • checked 14:20
//	check in flight        ?: help • q: quit • 🌱 checking…
//
// Two times, both named, because they answer different questions: how long the
// work has been over there, and how stale this picture of it is.

const footerSep = " • "

// shellHost is where alt+g goes: the one remote holding the project, or "" for
// here. Work garlic has not been asked about is work it only knows locally, so
// an unchecked board lands you on this machine rather than refusing -- and a
// board with no remotes configured at all still gets a shell.
//
// choose says the cursor is on work that lives in more than one place, which is
// the only case alt+g cannot answer by itself.
func (m Model) shellHost(board domain.Board, p domain.Project) (host string, choose bool) {
	switch hosts := m.plantedOn(board, p); len(hosts) {
	case 0:
		return "", false
	case 1:
		return hosts[0], false
	}
	return "", true
}

// shellCmd is that destination as a command: ssh into the planting when host
// names a remote, your own shell in the project's folder when it is empty.
//
// resourcePath applies the same rule on this side that remote.Shell applies on
// the other -- the resource folder when there is one, its area when there is
// not -- so both halves of alt+g land in the same place.
func (m Model) shellCmd(board domain.Board, p domain.Project, host string) (*exec.Cmd, error) {
	if host == "" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		cmd := exec.Command(shell)
		cmd.Dir = m.resourcePath(p)
		return cmd, nil
	}

	r, ok := m.findRemote(host)
	if !ok {
		// The name came from a manifest a configured remote handed us, so this
		// means the config changed under a check that is still on screen.
		return nil, fmt.Errorf("no remote named %q in your config", host)
	}
	return remote.Shell(r, remote.Rel(board.Opts, p.Path), board.Opts.Extension), nil
}

// session hands the terminal over. tea.ExecProcess is what makes this a visit
// rather than an exit: the board suspends, comes back when the shell ends, and
// the cursor is still on the card you left from -- unlike `r` and enter, which
// quit and let the runner take over.
func (m Model) session(board domain.Board, p domain.Project, host string) tea.Cmd {
	cmd, err := m.shellCmd(board, p, host)
	if err != nil {
		return func() tea.Msg { return sessionDoneMsg{err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return sessionDoneMsg{err}
	})
}

// findRemote resolves a name the way domain.Config.FindRemote does, against the
// list the model kept: the board holds remotes, not the config they came from.
func (m Model) findRemote(name string) (domain.Remote, bool) {
	for _, r := range m.Remotes {
		if r.Name == name {
			return r, true
		}
	}
	return domain.Remote{}, false
}

// connectMenu is the choice alt+g cannot make on its own: the remotes holding
// the project, and the copy on this machine. It borrows the help overlay's
// frame so the two read as the same kind of interruption, and needs no size
// floor of its own -- a handful of host names is narrower and shorter than the
// help the board already gates on.
//
// "here" is drawn as a row rather than stored in ConnectHosts, because a remote
// is free to be named "here" in anyone's config. That also keeps the slice the
// check handed us untouched: it is Sighting.Where's own memory, and appending
// to it could write past what the map thinks it lent out.
func (m Model) connectMenu() string {
	row := func(label string, i int) string {
		if i == m.ConnectCursor {
			return m.HeaderStyle.UnsetBorderStyle().Render("> " + label)
		}
		return "  " + label
	}

	rows := make([]string, 0, len(m.ConnectHosts)+1)
	for i, host := range m.ConnectHosts {
		rows = append(rows, row(host, i))
	}
	rows = append(rows, row("here", len(m.ConnectHosts)))

	content := lipgloss.JoinVertical(lipgloss.Left,
		m.HelpStyle.Render(plantedMark+" "+m.ActionTarget.Name+" is on"),
		"",
		lipgloss.JoinVertical(lipgloss.Left, rows...),
		"",
		m.HelpStyle.Render("enter: shell • esc: cancel"),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.SeparatorStyle.GetForeground()).
		Padding(1, 2).
		Render(content)
}

// checkedAt says when the last check happened, or that one is still out. Empty
// means nobody has asked yet, which is a different thing from having asked and
// found nothing.
func (m Model) checkedAt() string {
	switch {
	case m.Checking:
		return "checking…"
	case m.Planted.Checked():
		return "checked " + m.Planted.When.Format("15:04")
	}
	return ""
}

// plantedWhere names the remotes holding whatever the cursor is on, or "". The
// card only says that a project is planted; naming the hosts needs room, and
// the cursor already establishes which project is meant.
func (m Model) plantedWhere() string {
	p, ok := m.getSelectedProject()
	if !ok {
		return ""
	}

	board := m.Boards[m.ActiveBoard]
	hosts := m.plantedOn(board, p)
	if len(hosts) == 0 {
		return ""
	}

	where := "planted on " + strings.Join(hosts, ", ")
	if at := m.Planted.PlantedAt(remote.Rel(board.Opts, p.Path)); !at.IsZero() {
		where += " " + ago(m.Planted.When.Sub(at))
	}
	return where
}

// ago is the age of a planting, measured against the check that reported it.
// Days are the resolution that matters -- what has been sitting out there
// unharvested -- so anything shorter collapses to hours and minutes.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// areaPlanted reports whether a whole column went: every project in it is on a
// remote. The mark shows the granularity you planted at -- send one project and
// that project is marked, send the area and the column is marked instead. A
// column marked whenever anything under it went would erase the distinction.
//
// An empty category has not been planted; it has nothing to plant.
func (m Model) areaPlanted(board domain.Board, category string) bool {
	grid := board.ActiveGrid(m.ShowHidden)

	found := false
	for _, status := range board.Statuses {
		for _, p := range grid[status][category] {
			if len(m.plantedOn(board, p)) == 0 {
				return false
			}
			found = true
		}
	}
	return found
}

// idleFooter is the footer line when nothing more urgent is happening. It owns
// the help hint too, because the two share the slot.
func (m Model) idleFooter() string {
	hint := "?: help • q: quit"
	if !m.fitsHelpOverlay() {
		hint = "q: quit"
	}

	// The seedling belongs to the line rather than to each fact, so a planted
	// card reads "🌱 planted on agent • 14:20" instead of repeating the glyph.
	var facts []string
	where := m.plantedWhere()
	if where != "" {
		facts = append(facts, where)
	}
	if when := m.checkedAt(); when != "" {
		facts = append(facts, when)
	}
	if len(facts) == 0 {
		return m.HelpStyle.Render(hint)
	}

	group := m.PlantedHintStyle.Render(plantedMark + " " + strings.Join(facts, footerSep))

	// A planted card has something specific to say and would otherwise run long,
	// so it takes the line to itself.
	if where != "" {
		return group
	}

	line := m.HelpStyle.Render(hint+footerSep) + group
	if lipgloss.Width(line) > m.TermWidth {
		// What you pressed the key for outranks what you already know.
		return group
	}
	return line
}
