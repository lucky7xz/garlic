package remote

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

func TestSSHArgs(t *testing.T) {
	cases := []struct {
		name   string
		remote domain.Remote
		want   []string
	}{
		{
			"bare destination",
			domain.Remote{Host: "you@1.2.3.4"},
			[]string{"you@1.2.3.4"},
		},
		{
			"port only",
			domain.Remote{Host: "you@1.2.3.4", Port: 2222},
			[]string{"-p", "2222", "you@1.2.3.4"},
		},
		{
			"identity only",
			domain.Remote{Host: "you@1.2.3.4", IdentityFile: "/home/you/.ssh/k"},
			[]string{"-i", "/home/you/.ssh/k", "you@1.2.3.4"},
		},
		{
			"both",
			domain.Remote{Host: "you@1.2.3.4", Port: 2222, IdentityFile: "/home/you/.ssh/k"},
			[]string{"-p", "2222", "-i", "/home/you/.ssh/k", "you@1.2.3.4"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sshArgs(c.remote); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRsyncTransport(t *testing.T) {
	if got := rsyncTransport(domain.Remote{Host: "you@1.2.3.4"}); got != "" {
		t.Errorf("plain remote should use rsync's default ssh, got %q", got)
	}

	got := rsyncTransport(domain.Remote{Host: "you@1.2.3.4", Port: 2222, IdentityFile: "/home/you/.ssh/k"})
	want := "ssh -p 2222 -i /home/you/.ssh/k"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// root is the remote's path, so ~ must expand against the remote's home,
// never this machine's.
func TestExpandRemoteRoot(t *testing.T) {
	cases := []struct{ root, home, want string }{
		{"~/shara", "/home/agent", "/home/agent/shara"},
		{"~", "/home/agent", "/home/agent"},
		{"/srv/work", "/home/agent", "/srv/work"},
		{"~/shara/", "/home/agent", "/home/agent/shara"},
		{"~notauser/x", "/home/agent", "~notauser/x"},
	}

	for _, c := range cases {
		if got := expandRemoteRoot(c.root, c.home); got != c.want {
			t.Errorf("expandRemoteRoot(%q, %q) = %q, want %q", c.root, c.home, got, c.want)
		}
	}
}

func TestLocalCensus(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	files := []File{
		{Rel: "epics/a.md", Local: write("a.md", "hello")},
		{Rel: "epics/b.md", Local: write("b.md", "")},
	}

	got, err := localCensus(files)
	if err != nil {
		t.Fatalf("localCensus failed: %v", err)
	}

	want := Census{
		// sha256 of "hello" and of the empty string
		"epics/a.md": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"epics/b.md": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLocalCensusReportsMissingFiles(t *testing.T) {
	files := []File{{Rel: "epics/gone.md", Local: filepath.Join(t.TempDir(), "gone.md")}}
	if _, err := localCensus(files); err == nil {
		t.Error("localCensus accepted a missing file, want error")
	}
}

// Pruning must run deepest-first, so an inner directory is gone before its
// parent is tried.
func TestParentDirs(t *testing.T) {
	got := parentDirs([]string{
		"epics/fitness/running.md",
		"epics/fitness/running/logs/day1.txt",
		"epics/fitness/running/plan.pdf",
		"toplevel.md",
	})

	want := []string{
		"epics/fitness/running/logs",
		"epics/fitness/running",
		"epics/fitness",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The destination may rely on POSIX ACLs to give two identities write access.
// rsync's -a implies --perms, and setting a mode recalculates the ACL mask,
// which silently revokes what was granted. Garlic compares content hashes and
// never times, so preserving modes buys nothing here.
func TestRsyncArgsDoNotPreservePerms(t *testing.T) {
	args := rsyncArgs(domain.Remote{Host: "you@host"})

	if !slices.Contains(args, "--no-perms") {
		t.Errorf("expected --no-perms, got %v", args)
	}
	if !slices.Contains(args, "--protect-args") {
		t.Errorf("expected --protect-args, got %v", args)
	}
	if slices.Contains(args, "-e") {
		t.Errorf("a plain remote needs no -e, got %v", args)
	}
}

func TestRsyncArgsCarryTransport(t *testing.T) {
	args := rsyncArgs(domain.Remote{Host: "you@host", Port: 2222})

	i := slices.Index(args, "-e")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("expected -e with a value, got %v", args)
	}
	if args[i+1] != "ssh -p 2222" {
		t.Errorf("got %q, want %q", args[i+1], "ssh -p 2222")
	}
}
