package remote

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
)

// Shell is the command that drops you into a planted project's folder on the
// remote holding it. It is the one place garlic runs ssh interactively: every
// other verb asks a question and comes back, this one hands over the terminal.
//
// rel names the project the way the manifest does, <bulb>/<area>/<file>, so the
// two candidate folders fall straight out of it. Which one exists is the
// remote's business, not ours -- asking first would cost a round trip to learn
// something the `cd` already finds out.
//
// The two bulb kinds converge here. On a full bulb the first candidate is the
// project's resource folder and the fallback is its area. On a semi bulb the
// first names nothing (the extension is buried in the file name, not the
// folder) and the fallback is the project folder itself, which is where you
// wanted to be anyway.
func Shell(r domain.Remote, rel, ext string) *exec.Cmd {
	script := fmt.Sprintf(
		"cd %s 2>/dev/null || cd %s || exit 1; exec ${SHELL:-/bin/sh} -l",
		remotePath(r.Root, strings.TrimSuffix(rel, ext)),
		remotePath(r.Root, path.Dir(rel)))

	args := append([]string{"-t"}, sshArgs(r)...)
	return exec.Command("ssh", append(args, script)...)
}

// remotePath is a shell expression naming <root>/<rel> over there.
//
// A leading ~ is left for the remote's own shell to expand, which is why this
// returns an expression rather than a path: expandRemoteRoot is the same rule,
// but it needs $HOME fetched over ssh first, and a connection should not pay for
// a round trip to learn a path the destination can work out itself. Everything
// after the tilde is quoted, so only the expansion we asked for happens.
func remotePath(root, rel string) string {
	switch {
	case root == "~":
		return `"$HOME"/` + quote(rel)
	case strings.HasPrefix(root, "~/"):
		return `"$HOME"/` + quote(path.Join(root[2:], rel))
	}
	return quote(path.Join(path.Clean(root), rel))
}
