package remote

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
)

// Complete answers what the shell should offer for a half-typed command. The
// shell hands over every word after "garlic", the last being the one under the
// cursor -- empty when nothing is typed there yet.
//
// Everything it needs is on this machine, which is also the only thing that can
// be planted, so pressing tab never opens an ssh connection. Nothing reaches the
// remote until you press enter.
//
// A harvest completed from here cannot offer projects the agent invented, since
// those exist only over there. Typing the address by hand still works: the
// grammar accepts an address the board has never seen.
func Complete(cfg domain.Config, words []string) []string {
	if len(words) == 0 {
		words = []string{""}
	}

	partial := words[len(words)-1]
	settled := words[:len(words)-1]

	if len(settled) == 0 {
		return matching(partial, append(sortedKeys(verbs), "init"))
	}
	if _, known := verbs[settled[0]]; !known {
		return nil
	}

	// "@ name" and "@name" both parse, so both get completed.
	if settled[len(settled)-1] == "@" {
		return matching(partial, remoteNames(cfg))
	}
	if strings.HasPrefix(partial, "@") {
		return matching(partial, at(remoteNames(cfg)))
	}
	for _, w := range settled[1:] {
		if strings.HasPrefix(w, "@") {
			return nil // the remote is already named; nothing follows it
		}
	}

	// An address is settled once a word sits between the verb and the cursor.
	if len(settled) > 1 {
		return matching(partial, at(remoteNames(cfg)))
	}
	return matching(partial, addresses(cfg, partial))
}

// addresses offers the next segment of an address, as a whole path so the shell
// replaces the word rather than appending to it. A segment with something below
// it ends in "/" so tabbing straight on keeps working.
func addresses(cfg domain.Config, partial string) []string {
	opts := cfg.GetBoardOptions()

	done := strings.Split(partial, "/")
	done = done[:len(done)-1] // the last piece is the prefix being completed

	switch len(done) {
	case 0:
		var names []string
		for _, bulb := range opts {
			names = append(names, bulb.Name+"/")
		}
		return names

	case 1:
		bulb, err := findBulb(done[0], opts)
		if err != nil {
			return nil
		}
		// On a semi bulb the folder is the project, so it is a leaf: there is
		// nothing below it to tab into.
		suffix := "/"
		if bulb.WholeFolder {
			suffix = ""
		}
		var names []string
		for _, category := range trackedCategories(filesystem.ScanBoard(bulb)) {
			names = append(names, bulb.Name+"/"+category+suffix)
		}
		return names

	case 2:
		bulb, err := findBulb(done[0], opts)
		if err != nil || bulb.WholeFolder {
			return nil
		}
		var names []string
		for _, p := range boardProjects(filesystem.ScanBoard(bulb)) {
			if p.Category == done[1] {
				names = append(names, bulb.Name+"/"+p.Category+"/"+strings.TrimSuffix(p.Name, bulb.Extension))
			}
		}
		return names
	}
	return nil
}

func remoteNames(cfg domain.Config) []string {
	var names []string
	for _, r := range cfg.Remotes {
		names = append(names, r.Name)
	}
	return names
}

func at(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, "@"+n)
	}
	return out
}

// matching keeps what the half-typed word could still become, in a stable order
// so the shell's list does not reshuffle between presses.
func matching(partial string, candidates []string) []string {
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, partial) {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// bashCompletion wires the shell to `garlic __complete`. It sends every word up
// to the cursor and takes back one candidate per line; errors are swallowed so a
// broken config cannot spill into the prompt.
//
// A candidate ending in "/" has something below it, so the space is withheld and
// tabbing straight on keeps working. Positions are homogeneous -- bulbs, then
// areas, then projects -- so the first candidate speaks for all of them.
const bashCompletion = `_garlic() {
    local IFS=$'\n'
    COMPREPLY=($(garlic __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null))
    [[ ${COMPREPLY[0]} == */ ]] && compopt -o nospace
    return 0
}
complete -F _garlic garlic
`

// CompletionScript is the shell snippet to eval, as `garlic completion bash`.
func CompletionScript(shell string) (string, error) {
	if shell == "bash" {
		return bashCompletion, nil
	}
	return "", fmt.Errorf("no completion for %q — garlic ships bash", shell)
}

// OfferCompletion asks whether to set tab completion up, and does it if so.
//
// Tab completion is the shell's doing, not garlic's: bash has to be told to ask
// garlic, and a program cannot reach into a running shell to say so. So it has
// to be written down once. This writes a file bash-completion loads by name,
// which is why nothing goes near your .bashrc.
//
// It only ever offers, and it says nothing at all once the file is there -- it
// runs on the back of every status, so the ordinary case has to be silent. A no
// leaves the machine exactly as it was, and comes back next time: remembering a
// refusal would mean keeping state on this machine, which is the one thing
// garlic has refused throughout.
func OfferCompletion(w io.Writer, r io.Reader, home string) error {
	path := completionPath(home)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	fmt.Fprintln(w, "\nTab completion lets `garlic plant ep<tab>` fill in your bulbs and projects.")
	fmt.Fprintln(w, "It reads only this machine — tab never opens an ssh connection.")
	if !confirm(w, r, []gate{{prompt: "set it up for bash? [y/N] "}}) {
		fmt.Fprintln(w, "skipped — `garlic completion bash` prints it if you change your mind")
		return nil
	}

	written, err := InstallCompletion(home)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s — open a new shell and tab works\n", written)
	return nil
}

// InstallCompletion writes the hook where bash-completion looks for it: a file
// named after the command, which the shell loads the first time you type it.
// That is why nothing goes in .bashrc -- bash finds this by name, on demand.
//
// It returns the path it wrote, and overwrites an existing one, since
// reinstalling after an upgrade is the ordinary thing to do.
func InstallCompletion(home string) (string, error) {
	path := completionPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(bashCompletion), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func completionPath(home string) string {
	return filepath.Join(home, ".local", "share", "bash-completion", "completions", "garlic")
}
