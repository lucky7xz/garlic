package remote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// The whole design rests on this: fetching the agent's branch brings its commits
// across with their messages, and touches nothing of yours. Two real repositories
// on the filesystem -- no ssh, no remote machine.
func TestFetchBringsCommitsWithoutTouchingYours(t *testing.T) {
	root := t.TempDir()
	mine := filepath.Join(root, "mine")
	theirs := filepath.Join(root, "theirs")

	// What you had when you planted.
	if err := os.MkdirAll(mine, 0755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main", mine)
	if err := os.WriteFile(filepath.Join(mine, "main.go"), []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, mine, "add", "-A")
	git(t, mine, "commit", "-qm", "base")

	// plant --git copies the tree, .git included.
	git(t, root, "clone", "-q", mine, theirs)

	// The agent works on the branch garlic owns.
	git(t, theirs, "checkout", "-q", "-b", gitBranch("agent"))
	for i, msg := range []string{"add retry to the fetch loop", "cover the timeout path"} {
		body := []byte(strings.Repeat("v", i+2) + "\n")
		if err := os.WriteFile(filepath.Join(theirs, "main.go"), body, 0644); err != nil {
			t.Fatal(err)
		}
		git(t, theirs, "commit", "-qam", msg)
	}

	before := git(t, mine, "rev-parse", "HEAD")

	// Exactly the refspec conn.fetch builds, with a local path for a URL.
	git(t, mine, "fetch", theirs, "+"+gitBranch("agent")+":"+trackingRef("agent"))

	commits, err := arrived(mine, "agent")
	if err != nil {
		t.Fatalf("arrived failed: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits, want 2: %v", len(commits), commits)
	}
	if !strings.Contains(commits[0], "cover the timeout path") {
		t.Errorf("newest commit lost its message: %q", commits[0])
	}

	// Harvest's promise: nothing of yours moved.
	if now := git(t, mine, "rev-parse", "HEAD"); now != before {
		t.Error("HEAD moved")
	}
	body, err := os.ReadFile(filepath.Join(mine, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "v1\n" {
		t.Errorf("the worktree was touched: %q", body)
	}
	if dirty := git(t, mine, "status", "--porcelain"); dirty != "" {
		t.Errorf("the worktree is dirty:\n%s", dirty)
	}

	// And the payoff of sending .git: a shared ancestor, so this merges cleanly
	// rather than as unrelated history.
	if out, err := exec.Command("git", "-C", mine, "merge-base", "--is-ancestor",
		"HEAD", trackingRef("agent")).CombinedOutput(); err != nil {
		t.Errorf("no shared ancestor, so the merge would not be clean: %v %s", err, out)
	}
}

func TestGitNaming(t *testing.T) {
	if got := gitBranch("agent"); got != "garlic/agent" {
		t.Errorf("gitBranch = %q", got)
	}
	if got := trackingRef("agent"); got != "refs/remotes/agent/garlic" {
		t.Errorf("trackingRef = %q", got)
	}
}

// Detection has to come off the census garlic already has, and must not mistake
// a neighbouring area's repository for this one's.
func TestRepoAt(t *testing.T) {
	bulb := domain.BoardOptions{Name: "scripts", Extension: ".clove.md", WholeFolder: true}
	census := Census{
		"scripts/garlic/main.go":   "A",
		"scripts/garlic/.git/HEAD": "B",
		"scripts/drako/build.sh":   "C",
	}

	if !repoAt(census, Address{Bulb: "scripts", Area: "garlic"}, bulb) {
		t.Error("missed the repository under scripts/garlic")
	}
	if repoAt(census, Address{Bulb: "scripts", Area: "drako"}, bulb) {
		t.Error("found a repository under scripts/drako, which has none")
	}
	if repoAt(Census{"scripts/garlic/main.go": "A"}, Address{Bulb: "scripts", Area: "garlic"}, bulb) {
		t.Error("found a repository in a census with no .git at all")
	}
}

// A bulb is a shelf: one folder can be a repository and its neighbour plain
// files, so harvest decides per folder. Deciding once for the whole address is
// what made `harvest scripts` file-copy repos it should have fetched.
func TestRepoAreas(t *testing.T) {
	semi := domain.BoardOptions{Name: "scripts", Extension: ".clove.md", WholeFolder: true}
	census := Census{
		"scripts/garlic/main.go":        "A",
		"scripts/garlic/.git/HEAD":      "B",
		"scripts/drako/.git/refs/heads": "C",
		"scripts/notes/todo.clove.md":   "D",
	}

	got := repoAreas(census, Address{Bulb: "scripts"}, semi)
	if len(got) != 2 || got[0] != "drako" || got[1] != "garlic" {
		t.Errorf("repoAreas = %v, want [drako garlic] at bulb level", got)
	}

	// Addressed at one folder, only that one is in scope.
	got = repoAreas(census, Address{Bulb: "scripts", Area: "garlic"}, semi)
	if len(got) != 1 || got[0] != "garlic" {
		t.Errorf("repoAreas(scripts/garlic) = %v, want [garlic]", got)
	}

	// A folder with no repository is never named.
	got = repoAreas(census, Address{Bulb: "scripts", Area: "notes"}, semi)
	if len(got) != 0 {
		t.Errorf("repoAreas(scripts/notes) = %v, want none", got)
	}

	// A bulb whose .git sits at the bulb root is not a folder repo.
	got = repoAreas(Census{"scripts/.git/HEAD": "A"}, Address{Bulb: "scripts"}, semi)
	if len(got) != 0 {
		t.Errorf("repoAreas = %v, want none: a bulb is not a repository", got)
	}
}

// The repo folders drop out of the file comparison, so their working trees are
// never copied home alongside the commits.
func TestRepoAreasAreExcludedFromTheFileComparison(t *testing.T) {
	census := Census{
		"scripts/garlic/main.go":      "A",
		"scripts/notes/todo.clove.md": "B",
	}
	skip := map[string]bool{"scripts/garlic/": true}

	got := visible(census, skip)
	if _, ok := got["scripts/garlic/main.go"]; ok {
		t.Error("a repository's working tree would be copied home as well as fetched")
	}
	if _, ok := got["scripts/notes/todo.clove.md"]; !ok {
		t.Error("a plain folder beside a repository was dropped too")
	}
}
