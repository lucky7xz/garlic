package remote

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// gate is one thing that must be typed before a wipe proceeds. An empty want is
// a bare yes/no; anything else must be matched exactly.
type gate struct {
	prompt string
	want   string
}

// gates decides what a wipe has to ask. The accident being guarded is naming
// something broader than you meant -- `epics` when you wanted `epics/bioz` --
// so the questions get harder as the address gets shallower, and their wording
// changes with it, because muscle memory from wiping a project must not carry
// through wiping a bulb.
//
// The file count is the useful one: it can only be typed by reading the summary,
// and it differs every time, so it can never become reflex.
func gates(addr Address, remote string, files int) []gate {
	count := strconv.Itoa(files)

	switch {
	case addr.Project != "":
		return []gate{
			{prompt: "type the project name to wipe it: ", want: addr.Project},
		}
	case addr.Area != "":
		return []gate{
			{prompt: "wipe this whole area? [y/N] "},
			{prompt: "type the area name: ", want: addr.Area},
		}
	case addr.Bulb != "":
		return []gate{
			{prompt: "wipe this whole bulb? [y/N] "},
			{prompt: "type the bulb name: ", want: addr.Bulb},
			{prompt: "type the file count: ", want: count},
		}
	default:
		// The last gate comes after everything is on screen: a deliberate look
		// back once you know exactly what you asked for.
		return []gate{
			{prompt: "wipe every bulb on this remote? [y/N] "},
			{prompt: "type the remote name: ", want: remote},
			{prompt: "type the file count: ", want: count},
			{prompt: "last chance. wipe it all? [y/N] "},
		}
	}
}

// confirm asks each gate in turn and reports whether every one was answered.
// Anything unexpected is a refusal -- including end of input, which is what a
// pipe or a cron job looks like, and neither should be able to wipe unattended.
func confirm(w io.Writer, r io.Reader, ask []gate) bool {
	in := bufio.NewScanner(r)

	for _, g := range ask {
		fmt.Fprint(w, g.prompt)
		if !in.Scan() {
			fmt.Fprintln(w, "\nno answer — nothing was wiped")
			return false
		}

		answer := strings.TrimSpace(in.Text())
		ok := answer == g.want
		if g.want == "" {
			ok = answer == "y" || answer == "yes"
		}
		if !ok {
			fmt.Fprintln(w, "nothing was wiped")
			return false
		}
	}
	return true
}
