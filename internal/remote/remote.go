package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lucky7xz/garlic/internal/domain"
	"github.com/lucky7xz/garlic/internal/filesystem"
	"golang.org/x/term"
)

// IsCommand reports whether args start with one of the remote verbs.
func IsCommand(arg string) bool {
	_, ok := verbs[arg]
	return ok
}

// Run executes "<verb> [address] @ <remote>".
func Run(cfg domain.Config, args []string) error {
	cmd, err := ParseCommand(args)
	if err != nil {
		return err
	}

	machine, err := cfg.FindRemote(cmd.Remote)
	if err != nil {
		return err
	}

	c, err := dial(machine)
	if err != nil {
		return err
	}

	switch cmd.Verb {
	case "plant":
		return plant(cfg, c, cmd.Address, cmd.Git)
	case "harvest":
		return harvest(cfg, c, cmd.Address, true)
	case "status":
		if err := harvest(cfg, c, cmd.Address, false); err != nil {
			return err
		}
		// status is the calm, read-only verb, so it is where garlic mentions the
		// one thing it cannot install for itself: the shell hook that makes tab
		// ask garlic. Only after the report, and only to a person -- a piped
		// status stays clean.
		if home, err := os.UserHomeDir(); err == nil && term.IsTerminal(int(os.Stdin.Fd())) {
			return OfferCompletion(os.Stdout, os.Stdin, home)
		}
		return nil
	case "wipe":
		return wipe(cfg, c, cmd.Address)
	}
	return fmt.Errorf("unknown command %q", cmd.Verb)
}

// wipe removes folders. An address names one -- a project, an area, a bulb --
// and no address means every bulb. What dies is everything in that folder,
// including work the agent left that you never harvested, because naming a path
// is itself the act of saying what you mean.
//
// Anything in the root that is not a bulb is never touched: garlic did not put
// it there. Clearing the root outright is `ssh <host> rm -rf <root>`, a command
// with no garlic knowledge in it, so garlic does not offer it.
func wipe(cfg domain.Config, c *conn, addr Address) error {
	opts := cfg.GetBoardOptions()
	if addr.Bulb != "" {
		bulb, err := findBulb(addr.Bulb, opts)
		if err != nil {
			return err
		}
		opts = []domain.BoardOptions{bulb}
	}

	remote, err := c.census()
	if err != nil {
		return err
	}
	found, err := c.readManifests()
	if err != nil {
		return err
	}

	doomed, loose := doomedFiles(opts, addr, remote, found)
	if len(doomed) == 0 {
		fmt.Printf("nothing at %s on %s — nothing to wipe\n", target(addr), c.Describe())
		return nil
	}

	fmt.Print(renderDoomed(addr, c.Describe(), doomed, loose, opts))

	// A wipe is answered by a person or not at all. Piping the answers in would
	// let a script reproduce the accident the gates exist to prevent.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("wipe needs a terminal to confirm")
	}
	if !confirm(os.Stdout, os.Stdin, gates(addr, c.remote.Name, len(doomed))) {
		fmt.Println("  nothing was wiped")
		return nil
	}

	// Files first, then the manifest, then the pruning. Both failure modes are
	// bad but not equally: writing the manifest first would leave entries missing
	// for files that still exist, and the next harvest would read those as new
	// and copy them back over yours. This way a failure leaves stale entries,
	// which blocks a re-send until you wipe again -- recoverable, and it destroys
	// nothing. Pruning last is what lets an emptied bulb folder vanish.
	if err := c.removeFiles(doomed); err != nil {
		return err
	}
	for _, bulb := range opts {
		if err := pruneManifest(c, bulb, addr, found[bulb.Name]); err != nil {
			return err
		}
	}
	if err := c.pruneDirs(doomed); err != nil {
		return err
	}

	fmt.Printf("wiped %d files from %s on %s\n", len(doomed), target(addr), c.Describe())
	return nil
}

// doomedFiles is everything under the address, and which of it the manifest
// never knew about -- the agent's work, never harvested. Both lists are returned
// rather than one list and a count, so the summary can name the files it is
// warning you about instead of the first few of everything.
//
// The census is raw, so an ignore list shields nothing: wiping a folder takes
// what is in it.
func doomedFiles(opts []domain.BoardOptions, addr Address, remote Census, found map[string]Baseline) (doomed, loose []string) {
	for _, bulb := range opts {
		here := addr
		if here.Bulb == "" {
			here.Bulb = bulb.Name
		}

		for rel := range remote {
			if !inScope(rel, here, bulb.Extension, bulb.WholeFolder) {
				continue
			}
			doomed = append(doomed, rel)
			if _, ok := found[bulb.Name].Hashes[rel]; !ok {
				loose = append(loose, rel)
			}
		}
	}
	sort.Strings(doomed)
	sort.Strings(loose)
	return doomed, loose
}

// pruneManifest drops the wiped entries from a bulb's baseline, from both the
// hashes and the planting times. Leave them and Classify reads planted + local +
// no-remote as RemoteGone, which PlantPlan files under Gone -- so the replant
// would refuse to re-send, and the wipe would silently break the one workflow it
// exists for.
//
// inScope, not scope: scope also applies the ignore list, so adding `dist` to a
// bulb after planting would strand its entries where nothing could remove them.
func pruneManifest(c *conn, bulb domain.BoardOptions, addr Address, base Baseline) error {
	here := addr
	if here.Bulb == "" {
		here.Bulb = bulb.Name
	}

	kept := Baseline{Hashes: Manifest{}, Planted: Plantings{}}
	for rel, hash := range base.Hashes {
		if inScope(rel, here, bulb.Extension, bulb.WholeFolder) {
			continue
		}
		kept.Hashes[rel] = hash
		if at, ok := base.Planted[rel]; ok {
			kept.Planted[rel] = at
		}
	}

	// An emptied manifest is deleted rather than written blank, so that "no
	// manifest" keeps meaning "nothing was planted here".
	if len(kept.Hashes) == 0 {
		return c.dropManifest(bulb.Name)
	}
	return c.writeManifest(bulb.Name, kept)
}

// renderDoomed is the summary the gates are answered against. It splits the
// count because both the census and the manifest are in hand: seeing how much
// of this was never harvested is the last chance to notice you are about to
// throw away work.
func renderDoomed(addr Address, where string, doomed, loose []string, opts []domain.BoardOptions) string {
	const listed = 5

	var b strings.Builder

	fmt.Fprintf(&b, "wipe %s @ %s\n", target(addr), where)
	fmt.Fprintf(&b, "  %d files · %d you planted\n", len(doomed), len(doomed)-len(loose))

	if len(loose) > 0 {
		fmt.Fprintf(&b, "  %d never harvested:\n", len(loose))
		for _, rel := range loose[:min(listed, len(loose))] {
			fmt.Fprintf(&b, "    %s\n", rel)
		}
		if rest := len(loose) - listed; rest > 0 {
			fmt.Fprintf(&b, "    ... and %d more\n", rest)
		}
	}

	if names := belowNames(addr, doomed, opts); len(names) > 0 {
		fmt.Fprintf(&b, "  %s: %s\n", belowLabel(addr), strings.Join(names, ", "))
	}
	return b.String()
}

// belowNames lists what sits one level under the address -- the bulbs, areas or
// projects about to go. Seeing six areas when you wanted one is what reveals the
// mistake the gates exist to catch.
func belowNames(addr Address, doomed []string, opts []domain.BoardOptions) []string {
	if addr.Project != "" {
		return nil
	}
	depth := map[bool]int{true: 0, false: 1}[addr.Bulb == ""]
	if addr.Area != "" {
		depth = 2
	}

	seen := map[string]bool{}
	var names []string
	for _, rel := range doomed {
		parts := strings.Split(rel, "/")
		if len(parts) <= depth {
			continue
		}
		name := strings.TrimSuffix(parts[depth], ".clove.md")
		name = strings.TrimSuffix(name, ".md")
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func belowLabel(addr Address) string {
	switch {
	case addr.Area != "":
		return "projects"
	case addr.Bulb != "":
		return "areas"
	}
	return "bulbs"
}

func target(addr Address) string {
	if addr.Bulb == "" {
		return "every bulb"
	}
	return addr.String()
}

// plant tops the remote up: it sends what the agent has not touched, and says
// so about everything it therefore left alone.
func plant(cfg domain.Config, c *conn, addr Address, withGit bool) error {
	opts := cfg.GetBoardOptions()
	bulb, err := findBulb(addr.Bulb, opts)
	if err != nil {
		return err
	}

	files, err := Select(addr, opts, withGit)
	if err != nil {
		return err
	}

	// #garlic-hide is the board's way of saying "not in play", and plant is the
	// only verb that honours it. Both sides are filtered, not just this one:
	// filtering only here would leave Classify seeing planted + no-local +
	// remote-present and calling it LocalGone, so plant would report a hidden
	// project as "gone from your side". It is not gone.
	hidden := hiddenUnder(bulb)
	kept := files[:0]
	for _, f := range files {
		if !isHidden(f.Rel, hidden) {
			kept = append(kept, f)
		}
	}
	files = kept

	local, err := localCensus(files)
	if err != nil {
		return err
	}

	// A first plant has no baseline yet; it is the thing creating one.
	found, err := c.readManifests()
	if err != nil {
		return err
	}
	base := found[bulb.Name]
	if base.Hashes == nil {
		base.Hashes = Manifest{}
	}
	if base.Planted == nil {
		base.Planted = Plantings{}
	}

	remote, err := c.census()
	if err != nil {
		return err
	}

	// Seeding a repository happens once. After that git is the channel, and
	// re-sending .git would push your refs over the agent's -- so this refuses
	// rather than doing it quietly.
	if withGit && repoAt(remote, addr, bulb) {
		return fmt.Errorf("%s already has a repository on %s — git is the channel now:\n  git fetch %s:%s %s",
			addr.String(), c.remote.Name, c.remote.Host, c.gitDir(addr), gitBranch(c.remote.Name))
	}

	// Manifest entries from before something was hidden are left alone: with
	// both sides filtered they read as Still, inert, exactly as an ignored
	// path's would.
	moves := Classify(
		Manifest(scope(Census(base.Hashes), addr, bulb)),
		local,
		visible(scope(remote, addr, bulb), hidden),
	)
	p := PlantPlan(moves)

	if err := c.push(bulb, trimBulb(bulb, p.Push)); err != nil {
		return err
	}
	// Planting is the only thing that stamps a time: it is the moment the work
	// went out. Harvest advances hashes but leaves the stamp, because collecting
	// does not change how long a thing has been over there.
	now := time.Now()
	for _, rel := range p.Push {
		base.Hashes[rel] = local[rel]
		base.Planted[rel] = now
	}
	if err := c.writeManifest(bulb.Name, base); err != nil {
		return err
	}

	report("plant", addr.String(), c.Describe(), p)

	if withGit {
		branch := gitBranch(c.remote.Name)
		if err := c.startBranch(addr, branch); err != nil {
			return err
		}
		fmt.Printf("\n  the repository went too. the agent commits on %s;\n", branch)
		fmt.Printf("  `garlic harvest %s @ %s` fetches those commits\n", addr.String(), c.remote.Name)
	}
	return nil
}

// harvest collects what the agent produced. With apply false it is `status`:
// the same reckoning, printed and not acted on.
func harvest(cfg domain.Config, c *conn, addr Address, apply bool) error {
	found, err := c.readManifests()
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return fmt.Errorf("no manifest at %s — nothing was planted here, or it was wiped", c.Describe())
	}

	remote, err := c.census()
	if err != nil {
		return err
	}

	opts := cfg.GetBoardOptions()
	if addr.Bulb != "" {
		bulb, err := findBulb(addr.Bulb, opts)
		if err != nil {
			return err
		}
		opts = []domain.BoardOptions{bulb}
	}

	for _, bulb := range opts {
		// With no address, each bulb is reckoned against its own slice of the
		// root. A bulb with no manifest needs no special case: its baseline is
		// empty, so nothing there was planted and everything reads as left alone.
		here := addr
		if here.Bulb == "" {
			here.Bulb = bulb.Name
		}
		base := found[bulb.Name]

		all, err := bulbCensus(bulb)
		if err != nil {
			return err
		}

		// Only an address the user actually typed can be a typo. Without one,
		// each bulb is simply reckoned against its own slice of the root, and
		// having nothing there is an ordinary answer.
		if addr.Area != "" && !addressNames(here, bulb, all, Census(base.Hashes), remote) {
			return fmt.Errorf("nothing at %q — not on the board here, not in the manifest, not on %s",
				here.String(), c.Describe())
		}

		// A repository is git's to move. Copying its working tree home would
		// flatten every commit into one and, worse, race with the merge you
		// would then have to do -- the same change arriving twice.
		if here.Area != "" && repoAt(remote, here, bulb) {
			if err := harvestRepo(c, bulb, here, apply); err != nil {
				return err
			}
			continue
		}

		p, taken := reckon(bulb, here, base.Hashes, remote, all)

		// Harvest compares hidden projects like any other -- it has to, or it
		// could never ask whether you still want their changes.
		hidden := hiddenUnder(bulb)
		var setAside []string
		for _, rel := range p.Take {
			if isHidden(rel, hidden) {
				setAside = append(setAside, rel)
			}
		}

		// status changes nothing, so it names them and asks nothing.
		takeHidden := false
		if len(setAside) > 0 {
			fmt.Print(renderSetAside(setAside))
			if apply {
				takeHidden = confirm(os.Stdout, os.Stdin,
					[]gate{{prompt: "still harvest them? [y/N] "}})
				if !takeHidden {
					p = withoutTakes(p, setAside)
				}
			}
		}

		if apply {
			if err := collect(c, bulb, p); err != nil {
				return err
			}
			// The agent's copy never carried the marker -- you hid yours after
			// planting -- so collecting it would return the project to the
			// visible board without saying so.
			if takeHidden {
				if err := rehide(bulb, setAside); err != nil {
					return err
				}
			}
			// Parking advances the baseline as much as collecting does. The
			// agent's version is now in your resource folder, so the conflict
			// has been handed over; leaving the baseline behind would deadlock
			// the file, since plant refuses to push while the remote has moved.
			for _, rel := range append(p.Take, p.Park...) {
				base.Hashes[rel] = taken[rel]
			}
			if err := c.writeManifest(bulb.Name, base); err != nil {
				return err
			}
		}
		report(verbLabel(apply), here.String(), c.Describe(), p)
	}
	return nil
}

// harvestRepo is harvest for a git repository: fetch the branch the agent works
// on and say what arrived. It merges nothing, which is the same stance harvest
// takes everywhere else -- carry it across, touch nothing of yours, leave the
// decision.
func harvestRepo(c *conn, bulb domain.BoardOptions, addr Address, apply bool) error {
	local := filepath.Join(bulb.Path, addr.Area)
	branch := gitBranch(c.remote.Name)

	fmt.Printf("%s %s @ %s\n", verbLabel(apply), addr.String(), c.Describe())
	fmt.Printf("  a git repository — commits travel, files do not\n")

	dirty, err := c.uncommitted(addr)
	if err != nil {
		return err
	}

	if !apply {
		fmt.Printf("\n  `garlic harvest %s @ %s` fetches %s\n", addr.String(), c.remote.Name, branch)
		reportDirty(dirty)
		return nil
	}

	if err := c.fetch(local, addr, c.remote.Name); err != nil {
		return err
	}
	commits, err := arrived(local, c.remote.Name)
	if err != nil {
		return err
	}

	fmt.Printf("\n  fetched %s → %s\n", branch, trackingRef(c.remote.Name))
	if len(commits) == 0 {
		fmt.Printf("  no commits you do not already have\n")
	} else {
		fmt.Printf("\n  %d commits\n", len(commits))
		for _, line := range commits {
			fmt.Printf("    %s\n", line)
		}
		fmt.Printf("\n  nothing merged, nothing of yours touched.\n")
		fmt.Printf("    git log %s\n", trackingRef(c.remote.Name))
		fmt.Printf("    git merge %s\n", trackingRef(c.remote.Name))
	}
	reportDirty(dirty)
	return nil
}

// reportDirty names what the agent left uncommitted. A fetch cannot see it, and
// saying nothing would read as "there was nothing to collect".
func reportDirty(dirty []string) {
	if len(dirty) == 0 {
		return
	}
	fmt.Printf("\n  %d files are uncommitted over there, so no fetch can bring them:\n", len(dirty))
	for i, rel := range dirty {
		if i == 5 {
			fmt.Printf("    ... and %d more\n", len(dirty)-i)
			break
		}
		fmt.Printf("    %s\n", rel)
	}
}

// bulbCensus is everything a bulb holds locally, hashed. The caller keeps it so
// that validating the address and reckoning against it share one walk.
//
// Never keeps .git: this feeds harvest, and a collected repository is the one
// thing that can corrupt yours.
func bulbCensus(bulb domain.BoardOptions) (Census, error) {
	files, err := BulbFiles(bulb, false)
	if err != nil {
		return nil, err
	}
	return localCensus(files)
}

// reckon builds the three states for one bulb and reads them. It asks the
// remote nothing: with the manifest deciding what may come home, a hash is all
// there is to compare.
func reckon(bulb domain.BoardOptions, addr Address, manifest Manifest, remote, all Census) (Plan, Census) {
	local := scope(all, addr, bulb)
	planted := Manifest(scope(Census(manifest), addr, bulb))
	there := scope(remote, addr, bulb)

	seen := visibility{
		Bulb:    bulb.Name,
		Planted: planted,
		Ignore:  bulb.Ignore,
	}
	return HarvestPlan(Classify(planted, local, there), seen.allows), there
}

func collect(c *conn, bulb domain.BoardOptions, p Plan) error {
	if err := c.pull(bulb, trimBulb(bulb, p.Take)); err != nil {
		return err
	}
	for _, rel := range p.Park {
		if err := c.fetchOne(rel, parkPath(bulb, rel)); err != nil {
			return err
		}
	}
	return nil
}

// addressNames reports whether an address matches anything at all, in any of
// the three states harvest already holds.
//
// Plant can validate against the board here, because it can only ever send what
// is here. Harvest cannot: the projects most worth collecting are the ones the
// agent created, which exist only on the remote. But an address that names
// nothing in any of the three is a typo, and letting it through would print
// "nothing to do" -- a clean harvest and a mistyped one reading identically.
func addressNames(addr Address, bulb domain.BoardOptions, states ...Census) bool {
	for _, state := range states {
		if len(scope(state, addr, bulb)) > 0 {
			return true
		}
	}
	return false
}

// renderSetAside names what harvest would collect into a project you have hidden.
func renderSetAside(rels []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n  hidden here, but the agent changed them (%d)\n", len(rels))
	for _, rel := range rels {
		fmt.Fprintf(&b, "    %s\n", rel)
	}
	fmt.Fprint(&b, "  ")
	return b.String()
}

// withoutTakes drops paths from Take, leaving the baseline un-advanced for them
// so the question returns next time rather than being silently settled.
func withoutTakes(p Plan, drop []string) Plan {
	gone := map[string]bool{}
	for _, rel := range drop {
		gone[rel] = true
	}

	kept := p.Take[:0]
	for _, rel := range p.Take {
		if !gone[rel] {
			kept = append(kept, rel)
		}
	}
	p.Take = kept
	return p
}

// rehide puts #garlic-hide back after a collect. The agent's copy never carried
// one -- you hid your copy after planting -- so taking it would otherwise return
// the project to the visible board without saying so.
func rehide(bulb domain.BoardOptions, rels []string) error {
	for _, rel := range rels {
		if !strings.HasSuffix(rel, bulb.Extension) {
			continue // a resource; only the project file carries the marker
		}
		local := filepath.Join(bulb.Path, strings.TrimPrefix(rel, bulb.Name+"/"))
		if filesystem.GetTags(local).Hidden {
			continue
		}
		if err := filesystem.ToggleHiddenMarker(local); err != nil {
			return err
		}
	}
	return nil
}

// visible drops the paths of hidden projects from a census.
func visible(census Census, hidden map[string]bool) Census {
	out := Census{}
	for rel, hash := range census {
		if !isHidden(rel, hidden) {
			out[rel] = hash
		}
	}
	return out
}

func scope(census Census, addr Address, bulb domain.BoardOptions) Census {
	out := Census{}
	for rel, hash := range census {
		if inScope(rel, addr, bulb.Extension, bulb.WholeFolder) && !ignored(rel, bulb.Ignore, false) {
			out[rel] = hash
		}
	}
	return out
}

// trimBulb drops the bulb name, leaving paths as rsync wants them on both ends.
func trimBulb(bulb domain.BoardOptions, rels []string) []string {
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		out = append(out, strings.TrimPrefix(rel, bulb.Name+"/"))
	}
	return out
}

// parkPath is where a contested file's remote version lands. A project file goes
// inside its own resource folder: the scanner only reads the first level of an
// area, so a parked copy can never turn into a phantom project.
func parkPath(bulb domain.BoardOptions, rel string) string {
	parts := strings.Split(rel, "/")
	name := parts[len(parts)-1]

	if len(parts) == 3 && strings.HasSuffix(name, bulb.Extension) {
		base := strings.TrimSuffix(name, bulb.Extension)
		return filepath.Join(bulb.Path, parts[1], base, base+".remote"+bulb.Extension)
	}

	ext := filepath.Ext(name)
	parked := strings.TrimSuffix(name, ext) + ".remote" + ext
	return filepath.Join(bulb.Path, filepath.FromSlash(strings.Join(parts[1:len(parts)-1], "/")), parked)
}

func verbLabel(apply bool) string {
	if apply {
		return "harvest"
	}
	return "status"
}

// labels name each bucket in the voice of the verb that produced it. status
// reckons exactly as harvest does, so it shares the buckets but not the tense.
var labels = map[string]map[string]string{
	"plant": {
		"Push":      "pushed",
		"Blocked":   "left alone (the agent has changed these — harvest first)",
		"Gone":      "removed on remote (not re-sent)",
		"LocalGone": "gone from your side (the remote copy is untouched)",
	},
	"harvest": {
		"Take": "collected",
		"Park": "parked (changed on both sides — your file is untouched)",
		"Left": "left on remote (nothing here was planted)",
		"Gone": "removed on remote (nothing deleted here)",
	},
	"status": {
		"Take":      "waiting to be collected",
		"Park":      "changed on both sides",
		"Left":      "on remote, in nothing you planted",
		"Gone":      "removed on remote",
		"Push":      "not yet planted",
		"Blocked":   "changed by the agent",
		"LocalGone": "gone from your side",
	},
}

func report(verb, target, where string, p Plan) {
	fmt.Print(renderReport(verb, target, where, p))
}

func renderReport(verb, target, where string, p Plan) string {
	var b strings.Builder

	if target == "" {
		target = "(everything)"
	}
	fmt.Fprintf(&b, "%s %s @ %s\n", verb, target, where)

	buckets := []struct {
		key   string
		paths []string
	}{
		{"Take", p.Take},
		{"Push", p.Push},
		{"Park", p.Park},
		{"Blocked", p.Blocked},
		{"Left", p.Left},
		{"Gone", p.Gone},
		{"LocalGone", p.LocalGone},
	}

	empty := true
	for _, bucket := range buckets {
		label, named := labels[verb][bucket.key]
		if len(bucket.paths) == 0 || !named {
			continue
		}
		empty = false

		fmt.Fprintf(&b, "\n  %s (%d)\n", label, len(bucket.paths))
		for _, rel := range bucket.paths {
			fmt.Fprintf(&b, "    %s\n", rel)
		}
	}

	if empty {
		b.WriteString("\n  nothing to do\n")
	}
	return b.String()
}
