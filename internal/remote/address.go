package remote

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
)

// Address names a place on the board. An empty Bulb means the whole remote.
type Address struct {
	Bulb    string
	Area    string
	Project string
}

// Command is a parsed "<verb> [address] @ <remote>" invocation.
type Command struct {
	Verb    string
	Address Address
	Remote  string
	// Git sends the repository along with the work, so the agent can commit
	// against your real history. Only plant accepts it: harvest never collects
	// a .git, whatever put one there.
	Git bool
}

// How deep an address each verb accepts. plant and harvest write, so they insist
// on at least a bulb; status and wipe read the whole remote when given nothing.
var verbs = map[string]struct{ min, max int }{
	"plant":   {1, 3},
	"harvest": {1, 3},
	"status":  {0, 3},
	"wipe":    {0, 3},
}

// ParseCommand reads os.Args[1:].
func ParseCommand(args []string) (Command, error) {
	var cmd Command
	if len(args) == 0 {
		return cmd, fmt.Errorf("no command given")
	}

	cmd.Verb = args[0]
	limits, known := verbs[cmd.Verb]
	if !known {
		return cmd, fmt.Errorf("unknown command %q", cmd.Verb)
	}

	rest, remote, err := splitAt(args[1:])
	if err != nil {
		return cmd, err
	}
	cmd.Remote = remote

	// Flags are peeled wherever they sit, so `plant --git epics` and
	// `plant epics --git` both read the same.
	var address []string
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") {
			address = append(address, arg)
			continue
		}
		if arg != "--git" || cmd.Verb != "plant" {
			return cmd, fmt.Errorf("%s takes no %s", cmd.Verb, arg)
		}
		cmd.Git = true
	}
	rest = address

	if len(rest) > 1 {
		return cmd, fmt.Errorf("%s takes a single address", cmd.Verb)
	}

	var parts []string
	if len(rest) == 1 {
		parts = strings.Split(strings.TrimRight(rest[0], "/"), "/")
		// "." and ".." name nothing on any board, and wipe turns an address into
		// a list of files to delete -- so refusing beats matching nothing.
		for _, p := range parts {
			if p == "" || p == "." || p == ".." {
				return cmd, fmt.Errorf("bad segment %q in address %q", p, rest[0])
			}
		}
	}

	switch {
	case len(parts) < limits.min:
		return cmd, fmt.Errorf("%s needs an address, e.g. `garlic %s epics @ %s`", cmd.Verb, cmd.Verb, remote)
	case len(parts) > limits.max:
		return cmd, fmt.Errorf("address goes at most three deep: bulb/area/project")
	}

	for i, p := range parts {
		switch i {
		case 0:
			cmd.Address.Bulb = p
		case 1:
			cmd.Address.Area = p
		case 2:
			cmd.Address.Project = p
		}
	}
	return cmd, nil
}

// splitAt peels the trailing "@ <remote>" clause off, accepting "@ name" and "@name".
func splitAt(args []string) ([]string, string, error) {
	for i, a := range args {
		switch {
		case a == "@":
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("'@' needs a remote name")
			}
			if i+2 < len(args) {
				return nil, "", fmt.Errorf("unexpected argument after the remote name")
			}
			return args[:i], args[i+1], nil
		case strings.HasPrefix(a, "@"):
			if i+1 < len(args) {
				return nil, "", fmt.Errorf("unexpected argument after the remote name")
			}
			return args[:i], a[1:], nil
		}
	}
	return nil, "", fmt.Errorf("missing '@ <remote>'")
}

// File is one file in a transfer, named by its path relative to the remote root.
type File struct {
	Rel   string
	Local string
}

// Select turns an address into the files it covers. It fails when the address
// names nothing, because planting into thin air is a typo, not a no-op.
func Select(addr Address, opts []domain.BoardOptions, keepGit bool) ([]File, error) {
	bulb, err := findBulb(addr.Bulb, opts)
	if err != nil {
		return nil, err
	}

	all, err := BulbFiles(bulb, keepGit)
	if err != nil {
		return nil, err
	}
	if addr.Area != "" && !slices.ContainsFunc(all, func(f File) bool {
		return strings.HasPrefix(f.Rel, path.Join(bulb.Name, addr.Area)+"/")
	}) {
		return nil, fmt.Errorf("no area %q on bulb %q", addr.Area, addr.Bulb)
	}

	var files []File
	for _, f := range all {
		if inScope(f.Rel, addr, bulb.Extension, bulb.WholeFolder) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		if addr.Project != "" {
			return nil, fmt.Errorf("no project %q on the board at %s/%s", addr.Project, addr.Bulb, addr.Area)
		}
		return nil, fmt.Errorf("nothing on the board at %q", addr.Bulb)
	}
	return files, nil
}

// BulbFiles is everything on a bulb that travels. Unlike Select it is content
// to return nothing.
//
// The two bulb kinds mean different things by "category". On a full bulb it is
// an area holding several projects, and a project is its file plus its resource
// folder. On a semi bulb the category is itself the project: a .clove.md marks
// the folder as in play, and the whole folder goes with it.
func BulbFiles(bulb domain.BoardOptions, keepGit bool) ([]File, error) {
	board := filesystem.ScanBoard(bulb)

	if bulb.WholeFolder {
		var files []File
		for _, category := range trackedCategories(board) {
			got, err := walkTree(filepath.Join(bulb.Path, category), bulb, keepGit)
			if err != nil {
				return nil, err
			}
			files = append(files, got...)
		}
		return files, nil
	}

	var files []File
	for _, p := range boardProjects(board) {
		base := strings.TrimSuffix(p.Name, bulb.Extension)
		files = append(files, File{Rel: Rel(bulb, p.Path), Local: p.Path})

		resources, err := walkTree(filepath.Join(filepath.Dir(p.Path), base), bulb, keepGit)
		if err != nil {
			return nil, err
		}
		files = append(files, resources...)
	}
	return files, nil
}

// trackedCategories names the folders holding at least one tracked file, in a
// stable order.
func trackedCategories(board domain.Board) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range boardProjects(board) {
		if !seen[p.Category] {
			seen[p.Category] = true
			out = append(out, p.Category)
		}
	}
	sort.Strings(out)
	return out
}

func findBulb(name string, opts []domain.BoardOptions) (domain.BoardOptions, error) {
	for _, o := range opts {
		if o.Name == name {
			return o, nil
		}
	}
	return domain.BoardOptions{}, fmt.Errorf("no bulb named %q", name)
}

// boardProjects flattens both grids; hidden projects are on the board too, so they ship.
func boardProjects(board domain.Board) []domain.Project {
	var out []domain.Project
	for _, grid := range []map[string]map[string][]domain.Project{board.Grid, board.HiddenGrid} {
		for _, byCategory := range grid {
			for _, projects := range byCategory {
				out = append(out, projects...)
			}
		}
	}
	return out
}

// walkTree collects every file under dir that is allowed to travel. Ignored
// directories are skipped whole, so a .git never gets walked at all.
func walkTree(dir string, bulb domain.BoardOptions, keepGit bool) ([]File, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var files []File
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := Rel(bulb, p)
		if ignored(rel, bulb.Ignore, keepGit) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			files = append(files, File{Rel: rel, Local: p})
		}
		return nil
	})
	return files, err
}

// Rel names a local path the way the remote will: <bulb>/<area>/...
func Rel(bulb domain.BoardOptions, local string) string {
	rel, err := filepath.Rel(bulb.Path, local)
	if err != nil {
		return ""
	}
	return path.Join(bulb.Name, filepath.ToSlash(rel))
}

func (a Address) String() string {
	parts := []string{}
	for _, p := range []string{a.Bulb, a.Area, a.Project} {
		if p == "" {
			break
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "/")
}

// isParked spots a copy garlic set down while resolving a conflict. Those are
// working notes for you, not content, so they never travel.
func isParked(name string) bool {
	return strings.Contains(name, ".remote.")
}

// ignored decides what never crosses, in either direction.
//
// .git is not a size rule but a correctness one: rsync merges without deleting,
// so a harvested refs/, HEAD or index would land on top of yours while your
// objects stayed — leaving branch pointers and worktree disagreeing. Patterns
// come from the bulb's `ignore` list and match whole path segments, never
// substrings, so "dist" cannot swallow "distributed".
// keepGit is set only by `plant --git`, which seeds a repository so the agent
// can commit against your history. Harvest passes false without exception:
// rsync merges without deleting, so a collected refs/ or index landing over your
// objects would leave branch pointers and worktree disagreeing.
func ignored(rel string, patterns []string, keepGit bool) bool {
	for _, segment := range strings.Split(rel, "/") {
		if segment == ".git" && !keepGit {
			return true
		}
		if isParked(segment) {
			return true
		}
		if slices.Contains(patterns, segment) {
			return true
		}
	}
	return false
}
