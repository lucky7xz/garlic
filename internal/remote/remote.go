package remote

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lucky7xz/garlic/internal/domain"
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
		return plant(cfg, c, cmd.Address)
	case "harvest":
		return harvest(cfg, c, cmd.Address, true)
	case "status":
		return harvest(cfg, c, cmd.Address, false)
	case "wipe":
		return wipe(c, cmd.All)
	}
	return fmt.Errorf("unknown command %q", cmd.Verb)
}

// wipe clears garlic's own planting by default. Because a root can be shared
// with work garlic never planted, taking everything has to be asked for.
func wipe(c *conn, all bool) error {
	if all {
		if err := c.wipeAll(); err != nil {
			return err
		}
		fmt.Printf("wiped everything under %s\n", c.Describe())
		return nil
	}

	manifest, planted, err := c.readManifest()
	if err != nil {
		return err
	}
	if !planted {
		fmt.Printf("nothing planted at %s — leaving it alone\n", c.Describe())
		return nil
	}

	rels := sortedKeys(manifest)
	if err := c.wipePlanted(rels); err != nil {
		return err
	}

	fmt.Printf("wiped %d planted files from %s\n", len(rels), c.Describe())
	fmt.Printf("anything garlic did not plant is still there — `garlic wipe --all @ %s` takes the rest\n", c.remote.Name)
	return nil
}

// plant tops the remote up: it sends what the agent has not touched, and says
// so about everything it therefore left alone.
func plant(cfg domain.Config, c *conn, addr Address) error {
	opts := cfg.GetBoardOptions()
	bulb, err := findBulb(addr.Bulb, opts)
	if err != nil {
		return err
	}

	files, err := Select(addr, opts)
	if err != nil {
		return err
	}
	local, err := localCensus(files)
	if err != nil {
		return err
	}

	// A first plant has no baseline yet; it is the thing creating one.
	manifest, _, err := c.readManifest()
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = Manifest{}
	}

	remote, err := c.census()
	if err != nil {
		return err
	}

	moves := Classify(
		Manifest(scope(Census(manifest), addr, bulb.Extension)),
		local,
		scope(remote, addr, bulb.Extension),
	)
	p := PlantPlan(moves)

	if err := c.push(bulb, trimBulb(bulb, p.Push)); err != nil {
		return err
	}
	for _, rel := range p.Push {
		manifest[rel] = local[rel]
	}
	if err := c.writeManifest(manifest); err != nil {
		return err
	}

	report("plant", addr.String(), c.Describe(), p)
	return nil
}

// harvest collects what the agent produced. With apply false it is `status`:
// the same reckoning, printed and not acted on.
func harvest(cfg domain.Config, c *conn, addr Address, apply bool) error {
	manifest, planted, err := c.readManifest()
	if err != nil {
		return err
	}
	if !planted {
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
		// With no address, each bulb is reckoned against its own slice of the root.
		here := addr
		if here.Bulb == "" {
			here.Bulb = bulb.Name
		}

		p, taken, err := reckon(c, bulb, here, manifest, remote)
		if err != nil {
			return err
		}

		if apply {
			if err := collect(c, bulb, p); err != nil {
				return err
			}
			// Parking advances the baseline as much as collecting does. The
			// agent's version is now in your resource folder, so the conflict
			// has been handed over; leaving the baseline behind would deadlock
			// the file, since plant refuses to push while the remote has moved.
			for _, rel := range append(p.Take, p.Park...) {
				manifest[rel] = taken[rel]
			}
		}
		report(verbLabel(apply), here.String(), c.Describe(), p)
	}

	if apply {
		return c.writeManifest(manifest)
	}
	return nil
}

// reckon builds the three states for one bulb and reads them.
func reckon(c *conn, bulb domain.BoardOptions, addr Address, manifest Manifest, remote Census) (Plan, Census, error) {
	files, err := BulbFiles(bulb)
	if err != nil {
		return Plan{}, nil, err
	}
	all, err := localCensus(files)
	if err != nil {
		return Plan{}, nil, err
	}

	ext := bulb.Extension
	local := scope(all, addr, ext)
	planted := Manifest(scope(Census(manifest), addr, ext))
	there := scope(remote, addr, ext)

	moves := Classify(planted, local, there)

	// Only files that just appeared need their status tag read: everything else
	// either came off the board already or is not a candidate.
	var candidates []string
	for rel, move := range moves {
		if move == RemoteNew && strings.Count(rel, "/") == 2 && strings.HasSuffix(rel, ext) {
			candidates = append(candidates, rel)
		}
	}
	tags, err := c.statusTags(candidates)
	if err != nil {
		return Plan{}, nil, err
	}

	seen := visibility{
		Bulb:     bulb.Name,
		Ext:      ext,
		Statuses: bulb.Statuses,
		Tags:     tags,
		Remote:   there,
		Planted:  planted,
	}
	return HarvestPlan(moves, seen.allows), there, nil
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

func scope(census Census, addr Address, ext string) Census {
	out := Census{}
	for rel, hash := range census {
		if inScope(rel, addr, ext) {
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
		"Left": "left on remote (not something the board shows)",
		"Gone": "removed on remote (nothing deleted here)",
	},
	"status": {
		"Take":      "waiting to be collected",
		"Park":      "changed on both sides",
		"Left":      "on remote, not something the board shows",
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
