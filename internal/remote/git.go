package remote

import (
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
)

// Garlic hands a repository over and lets git carry the commits back. Its own
// manifest is a file-granular stand-in for what git does properly -- a real
// named ancestor, a content-granular three-way merge -- so where a repository
// exists, garlic supplies the one thing git cannot guess: the address.

// gitBranch is where the agent's work lives on the remote. One branch per
// remote, always the same name, so nothing has to be chosen or remembered.
func gitBranch(remote string) string { return "garlic/" + remote }

// trackingRef is where that branch lands here: a remote-tracking ref rather than
// FETCH_HEAD, so it survives the next fetch and can be named afterwards.
func trackingRef(remote string) string { return "refs/remotes/" + remote + "/garlic" }

// repoAt reports whether a census shows a repository under this address. The
// census is a raw find, so .git paths are already in hand and the answer costs
// no round trip.
func repoAt(census Census, addr Address, bulb domain.BoardOptions) bool {
	for rel := range census {
		if !inScope(rel, addr, bulb.Extension, bulb.WholeFolder) {
			continue
		}
		for _, segment := range strings.Split(rel, "/") {
			if segment == ".git" {
				return true
			}
		}
	}
	return false
}

// repoAreas names the folders under an address that hold a repository. A bulb is
// a shelf: one folder can be a repo and its neighbour plain files, so harvest
// has to decide per folder rather than once for the whole address.
func repoAreas(census Census, addr Address, bulb domain.BoardOptions) []string {
	seen := map[string]bool{}
	var out []string

	for rel := range census {
		if !inScope(rel, addr, bulb.Extension, bulb.WholeFolder) {
			continue
		}
		parts := strings.Split(rel, "/")
		if len(parts) < 3 || seen[parts[1]] {
			continue
		}
		for _, segment := range parts[2:] {
			if segment == ".git" {
				seen[parts[1]] = true
				out = append(out, parts[1])
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// gitDir is the address as a path on the remote.
func (c *conn) gitDir(addr Address) string {
	parts := []string{c.root, addr.Bulb}
	if addr.Area != "" {
		parts = append(parts, addr.Area)
	}
	return path.Join(parts...)
}

// startBranch puts the agent's branch in place, without disturbing one that is
// already there -- re-running plant must never reset work the agent committed.
func (c *conn) startBranch(addr Address, branch string) error {
	script := fmt.Sprintf(
		"cd %s && (git rev-parse --verify --quiet %s >/dev/null || git checkout -q -b %s)",
		quote(c.gitDir(addr)), quote(branch), quote(branch))
	_, err := c.run(script, nil)
	return err
}

// uncommitted names what the agent left unstaged or untracked. Fetch cannot see
// it, and silence would read as "nothing to collect".
func (c *conn) uncommitted(addr Address) ([]string, error) {
	out, err := c.run(fmt.Sprintf("cd %s && git status --porcelain", quote(c.gitDir(addr))), nil)
	if err != nil {
		return nil, err
	}

	var dirty []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if len(line) > 3 {
			dirty = append(dirty, strings.TrimSpace(line[3:]))
		}
	}
	return dirty, nil
}

// fetch brings the agent's branch home. It runs in the local repository and
// touches nothing in the worktree -- which is exactly harvest's own stance:
// carry it across, leave the decision.
func (c *conn) fetch(local string, addr Address, remote string) error {
	url := c.remote.Host + ":" + c.gitDir(addr)
	refspec := "+" + gitBranch(remote) + ":" + trackingRef(remote)

	cmd := exec.Command("git", "-C", local, "fetch", url, refspec)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch %s: %w: %s", url, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// arrived lists what the fetched branch has that you do not, newest first.
func arrived(local, remote string) ([]string, error) {
	cmd := exec.Command("git", "-C", local, "log", "--oneline", "HEAD.."+trackingRef(remote))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var commits []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits, nil
}
