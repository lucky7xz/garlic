package remote

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Census is what a tree looks like right now: path relative to the root → sha256.
type Census map[string]string

// Manifest is what origin handed over: path → sha256 at the moment it was planted.
// It lives on the remote, so it is a baseline for both sides at once.
type Manifest map[string]string

// Movement says who moved a file since it was planted. Every plant and harvest
// rule is a reading of this one answer.
type Movement int

const (
	Still Movement = iota
	RemoteMoved
	LocalMoved
	BothMoved
	RemoteNew
	LocalNew
	RemoteGone
	LocalGone
)

func (m Movement) String() string {
	switch m {
	case Still:
		return "Still"
	case RemoteMoved:
		return "RemoteMoved"
	case LocalMoved:
		return "LocalMoved"
	case BothMoved:
		return "BothMoved"
	case RemoteNew:
		return "RemoteNew"
	case LocalNew:
		return "LocalNew"
	case RemoteGone:
		return "RemoteGone"
	case LocalGone:
		return "LocalGone"
	}
	return "unknown"
}

// Classify compares three states of every path it has seen. Without the manifest
// there are only two, and "the agent changed it" is indistinguishable from
// "you changed it" — which is the whole reason the manifest exists.
func Classify(manifest Manifest, local, remote Census) map[string]Movement {
	out := make(map[string]Movement)

	for _, p := range allPaths(manifest, local, remote) {
		base, planted := manifest[p]
		l, hasLocal := local[p]
		r, hasRemote := remote[p]

		switch {
		case !hasLocal && !hasRemote:
			out[p] = Still
		case !hasRemote:
			if planted {
				out[p] = RemoteGone
			} else {
				out[p] = LocalNew
			}
		case !hasLocal:
			switch {
			case !planted:
				out[p] = RemoteNew
			case r == base:
				// You deleted it and the agent left it alone: it stays deleted.
				out[p] = LocalGone
			default:
				// You deleted it, then the agent worked on it. That is new work,
				// not the file you threw away, so it is collectable again.
				out[p] = RemoteMoved
			}
		case l == r:
			out[p] = Still
		case !planted:
			// Both sides have it and they differ, with nothing to say who moved.
			out[p] = BothMoved
		case l == base:
			out[p] = RemoteMoved
		case r == base:
			out[p] = LocalMoved
		default:
			out[p] = BothMoved
		}
	}
	return out
}

func allPaths(manifest Manifest, local, remote Census) []string {
	seen := make(map[string]bool)
	for _, m := range []map[string]string{manifest, local, remote} {
		for p := range m {
			seen[p] = true
		}
	}
	return sortedKeys(seen)
}

// manifestFile is the on-disk shape. The hashes live under one table with
// quoted keys, so a filename like "running.md" stays one key instead of
// becoming nested tables.
type manifestFile struct {
	Files map[string]string `toml:"files"`
}

// ManifestName is where the manifest sits, inside the remote root, so that
// wiping the root takes the baseline with it.
const ManifestName = ".garlic-manifest.toml"

func (m Manifest) Encode() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("# Written by garlic. Hashes of what was planted, as planted.\n")
	if err := toml.NewEncoder(&buf).Encode(manifestFile{Files: m}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeManifest(data []byte) (Manifest, error) {
	var f manifestFile
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil, err
	}
	if f.Files == nil {
		f.Files = Manifest{}
	}
	return f.Files, nil
}

// parseSums reads `sha256sum` output into a Census. Coreutils writes
// "<hash><space><mode><name>" where mode is ' ' or '*', and flags a line whose
// name needed escaping with a leading backslash.
func parseSums(out []byte) (Census, error) {
	census := Census{}

	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		escaped := strings.HasPrefix(line, `\`)
		if escaped {
			line = line[1:]
		}

		sep := strings.IndexByte(line, ' ')
		if sep < 0 || sep+2 > len(line) || (line[sep+1] != ' ' && line[sep+1] != '*') {
			return nil, fmt.Errorf("cannot parse checksum line %q", line)
		}

		name := line[sep+2:]
		if escaped {
			name = unescapeName(name)
		}
		census[strings.TrimPrefix(name, "./")] = line[:sep]
	}
	return census, nil
}

func unescapeName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Plan is what a verb decided to do, in buckets that print as they read.
// Take and Push are the only ones that move bytes; the rest are things to say.
type Plan struct {
	Take      []string // harvest: overwrite local with the remote's version
	Park      []string // harvest: contested, so set the remote's version beside yours
	Push      []string // plant: send this local file over
	Left      []string // harvest: new on the remote, but not something the board shows
	Gone      []string // the agent deleted it — reported, never acted on
	Blocked   []string // plant: the agent has touched it, so it is not overwritten
	LocalGone []string // you deleted it locally; the remote copy is left alone
}

func (p Plan) Empty() bool {
	return len(p.Take) == 0 && len(p.Park) == 0 && len(p.Push) == 0 &&
		len(p.Left) == 0 && len(p.Gone) == 0 && len(p.Blocked) == 0 && len(p.LocalGone) == 0
}

// HarvestPlan collects the agent's work. It never deletes and never merges:
// a file you also changed is parked, not overwritten.
func HarvestPlan(moves map[string]Movement, visible func(string) bool) Plan {
	var plan Plan
	for _, p := range sortedKeys(moves) {
		switch moves[p] {
		case RemoteMoved:
			plan.Take = append(plan.Take, p)
		case RemoteNew:
			if visible(p) {
				plan.Take = append(plan.Take, p)
			} else {
				plan.Left = append(plan.Left, p)
			}
		case BothMoved:
			plan.Park = append(plan.Park, p)
		case RemoteGone:
			plan.Gone = append(plan.Gone, p)
		}
	}
	return plan
}

// PlantPlan tops the remote up. It is the mirror of HarvestPlan: anything the
// agent has touched — including deleted — is left exactly as the agent left it.
func PlantPlan(moves map[string]Movement) Plan {
	var plan Plan
	for _, p := range sortedKeys(moves) {
		switch moves[p] {
		case LocalMoved, LocalNew:
			plan.Push = append(plan.Push, p)
		case RemoteMoved, BothMoved:
			plan.Blocked = append(plan.Blocked, p)
		case RemoteGone:
			plan.Gone = append(plan.Gone, p)
		case LocalGone:
			plan.LocalGone = append(plan.LocalGone, p)
		}
	}
	return plan
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// inScope narrows a whole-root census to the address the user named. The remote
// is censused once and filtered here, so scoping costs no extra round trip.
func inScope(rel string, addr Address, ext string) bool {
	if addr.Bulb == "" {
		return true
	}

	parts := strings.Split(rel, "/")
	if len(parts) < 2 || parts[0] != addr.Bulb {
		return false
	}
	if addr.Area == "" {
		return true
	}
	if len(parts) < 3 || parts[1] != addr.Area {
		return false
	}
	if addr.Project == "" {
		return true
	}
	// A project is its file plus its resource folder, and nothing else.
	return parts[2] == addr.Project+ext || (parts[2] == addr.Project && len(parts) > 3)
}

// visibility answers whether a file that appeared on the remote is something
// the board would show, or a resource belonging to something it would show.
// Anything else is left where the agent put it.
type visibility struct {
	Bulb     string
	Ext      string
	Statuses []string
	Tags     map[string]string // project path → its status tag, read from the remote
	Remote   Census
	Planted  Manifest // anything in here came off the board to begin with
}

func (v visibility) allows(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) < 3 || parts[0] != v.Bulb {
		return false
	}

	if len(parts) == 3 {
		if _, planted := v.Planted[rel]; planted {
			return true
		}
		return strings.HasSuffix(parts[2], v.Ext) && slices.Contains(v.Statuses, v.Tags[rel])
	}

	// Deeper than a project file: it is a resource, and it travels only if the
	// project that owns the folder is itself on the board.
	owner := strings.Join(parts[:2], "/") + "/" + parts[2] + v.Ext
	if _, exists := v.Remote[owner]; !exists {
		return false
	}
	return v.allows(owner)
}
