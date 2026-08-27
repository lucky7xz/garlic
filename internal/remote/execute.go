package remote

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
)

// sshArgs builds the flags and destination for one ssh invocation.
func sshArgs(r domain.Remote) []string {
	var args []string
	if r.Port != 0 {
		args = append(args, "-p", fmt.Sprint(r.Port))
	}
	if r.IdentityFile != "" {
		args = append(args, "-i", r.IdentityFile)
	}
	return append(args, r.Host)
}

// rsyncTransport is rsync's -e argument, or "" when plain ssh will do.
func rsyncTransport(r domain.Remote) string {
	transport := "ssh"
	if r.Port != 0 {
		transport += fmt.Sprintf(" -p %d", r.Port)
	}
	if r.IdentityFile != "" {
		transport += " -i " + r.IdentityFile
	}
	if transport == "ssh" {
		return ""
	}
	return transport
}

// expandRemoteRoot resolves a leading ~ against the *remote's* home. The root
// belongs to the other machine, so it is never expanded locally.
func expandRemoteRoot(root, home string) string {
	switch {
	case root == "~":
		return home
	case strings.HasPrefix(root, "~/"):
		return path.Join(home, root[2:])
	}
	return path.Clean(root)
}

// localCensus hashes this side the same way sha256sum hashes the other.
func localCensus(files []File) (Census, error) {
	census := Census{}
	for _, f := range files {
		sum, err := hashFile(f.Local)
		if err != nil {
			return nil, err
		}
		census[f.Rel] = sum
	}
	return census, nil
}

func hashFile(p string) (string, error) {
	file, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// conn is a resolved remote: the machine, plus its root as an absolute path.
// The root is expanded by the remote's own shell, never by ours.
type conn struct {
	remote domain.Remote
	root   string
}

func dial(r domain.Remote) (*conn, error) {
	if r.Root == "" {
		return nil, fmt.Errorf("remote %q has no root", r.Name)
	}

	root := r.Root
	if strings.HasPrefix(root, "~") {
		home, err := sshRun(r, `printf %s "$HOME"`, nil)
		if err != nil {
			return nil, err
		}
		root = expandRemoteRoot(root, strings.TrimSpace(string(home)))
	}
	return &conn{remote: r, root: path.Clean(root)}, nil
}

// Describe names the remote the way error messages should.
func (c *conn) Describe() string { return c.remote.Host + ":" + c.root }

func sshRun(r domain.Remote, script string, stdin io.Reader) ([]byte, error) {
	cmd := exec.Command("ssh", append(sshArgs(r), script)...)
	cmd.Stdin = stdin

	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ssh %s: %w: %s", r.Host, err, strings.TrimSpace(errs.String()))
	}
	return out.Bytes(), nil
}

func (c *conn) run(script string, stdin io.Reader) ([]byte, error) {
	return sshRun(c.remote, script, stdin)
}

// census hashes the whole root in one round trip; scoping happens locally.
func (c *conn) census() (Census, error) {
	script := fmt.Sprintf(
		"cd %s 2>/dev/null || exit 0; find . -type f ! -name %s -exec sha256sum {} +",
		quote(c.root), quote(ManifestName))

	out, err := c.run(script, nil)
	if err != nil {
		return nil, err
	}
	return parseSums(out)
}

// readManifest reports whether the remote has a baseline at all. Without one,
// nothing can be said about who moved what.
func (c *conn) readManifest() (Manifest, bool, error) {
	out, err := c.run(fmt.Sprintf("cat %s 2>/dev/null || true", quote(c.manifestPath())), nil)
	if err != nil {
		return nil, false, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, false, nil
	}

	m, err := DecodeManifest(out)
	if err != nil {
		return nil, false, fmt.Errorf("manifest at %s is unreadable: %w", c.Describe(), err)
	}
	return m, true, nil
}

func (c *conn) writeManifest(m Manifest) error {
	data, err := m.Encode()
	if err != nil {
		return err
	}
	script := fmt.Sprintf("mkdir -p %s && cat > %s", quote(c.root), quote(c.manifestPath()))
	_, err = c.run(script, bytes.NewReader(data))
	return err
}

func (c *conn) manifestPath() string { return path.Join(c.root, ManifestName) }

// statusTags reads the status tag of files that appeared on the remote, which
// is the one thing a hash cannot tell us. Paths go over stdin so that a long
// list cannot overflow the command line.
func (c *conn) statusTags(rels []string) (map[string]string, error) {
	tags := map[string]string{}
	if len(rels) == 0 {
		return tags, nil
	}

	script := fmt.Sprintf(
		"cd %s && xargs -0 -r grep -m1 -H -oE '#statustag-[A-Za-z0-9_]+' -- 2>/dev/null || true",
		quote(c.root))

	out, err := c.run(script, strings.NewReader(strings.Join(rels, "\x00")+"\x00"))
	if err != nil {
		return nil, err
	}

	const marker = ":#statustag-"
	for line := range strings.SplitSeq(string(out), "\n") {
		if i := strings.Index(line, marker); i > 0 {
			tags[strings.TrimPrefix(line[:i], "./")] = line[i+len(marker):]
		}
	}
	return tags, nil
}

func (c *conn) wipe() error {
	_, err := c.run(fmt.Sprintf("rm -rf %s", quote(c.root)), nil)
	return err
}

// push sends files named relative to a bulb into that bulb's dir on the remote.
// rsync builds directories for the files it is given but not the destination
// root itself, so that has to exist first.
func (c *conn) push(bulb domain.BoardOptions, rels []string) error {
	if len(rels) == 0 {
		return nil
	}

	bulbRoot := path.Join(c.root, bulb.Name)
	if _, err := c.run("mkdir -p "+quote(bulbRoot), nil); err != nil {
		return err
	}
	return c.rsync(rels, bulb.Path+"/", c.remote.Host+":"+bulbRoot+"/")
}

// pull is the same transfer with the ends swapped.
func (c *conn) pull(bulb domain.BoardOptions, rels []string) error {
	src := c.remote.Host + ":" + path.Join(c.root, bulb.Name) + "/"
	return c.rsync(rels, src, bulb.Path+"/")
}

func (c *conn) rsync(rels []string, src, dest string) error {
	if len(rels) == 0 {
		return nil
	}

	args := []string{"-a", "--protect-args", "--files-from=-"}
	if transport := rsyncTransport(c.remote); transport != "" {
		args = append(args, "-e", transport)
	}
	args = append(args, src, dest)

	cmd := exec.Command("rsync", args...)
	cmd.Stdin = strings.NewReader(strings.Join(rels, "\n") + "\n")

	var errs bytes.Buffer
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(errs.String()))
	}
	return nil
}

// fetchOne copies a single remote file to an exact local path, which is how a
// contested file gets parked beside the one it disagrees with.
func (c *conn) fetchOne(rel, localDest string) error {
	if err := os.MkdirAll(filepath.Dir(localDest), 0755); err != nil {
		return err
	}

	args := []string{"-a", "--protect-args"}
	if transport := rsyncTransport(c.remote); transport != "" {
		args = append(args, "-e", transport)
	}
	args = append(args, c.remote.Host+":"+path.Join(c.root, rel), localDest)

	cmd := exec.Command("rsync", args...)
	var errs bytes.Buffer
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(errs.String()))
	}
	return nil
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
