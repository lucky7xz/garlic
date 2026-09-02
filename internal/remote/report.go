package remote

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

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

// headingStyles colour the bucket headings, and only those: a file name has to
// survive being copied out of the terminal, and a report that is being read by
// something other than a person should read the same either way. lipgloss drops
// the styling by itself when stdout is not a terminal.
var headingStyles = map[string]lipgloss.Style{
	// The ordinary flow, in whichever direction the verb runs.
	"Take": lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	"Push": lipgloss.NewStyle().Foreground(lipgloss.Color("2")),

	// Both sides moved, so something is owed a decision.
	"Park":    lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	"Blocked": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),

	// Nothing to do, said out loud so it cannot be mistaken for silence.
	"Left":      lipgloss.NewStyle().Faint(true),
	"Gone":      lipgloss.NewStyle().Faint(true),
	"LocalGone": lipgloss.NewStyle().Faint(true),
}

// marks give each bucket a shape to find it by. The report is read by someone
// scanning for one of them -- what came back, what is contested -- rather than
// front to back, and a glyph is found faster than a sentence.
var marks = map[string]string{
	"Take":      "📥",
	"Push":      "📤",
	"Park":      "⚠️",
	"Blocked":   "✋",
	"Left":      "🧳",
	"Gone":      "🗑️",
	"LocalGone": "🕳️",
}

const (
	// garlicMark heads a report the way it heads the board.
	garlicMark = "🧄"

	// reportIndent is how far the file names sit from the margin, deepening by
	// bulbIndent when a report spans more than one bulb. gutter is the space
	// between two columns of names.
	reportIndent = 5
	bulbIndent   = 2
	gutter       = 2

	// fallbackWidth is what a report assumes when nobody is watching -- piped
	// into a file or a pager, it still has to wrap somewhere.
	fallbackWidth = 80

	// ruleFloor keeps the rules from shrinking to a dash beside a short header,
	// and ruleCeiling keeps them from crossing a very wide terminal end to end.
	ruleFloor   = 40
	ruleCeiling = 80
)

var (
	ruleStyle = lipgloss.NewStyle().Faint(true)
	bulbStyle = lipgloss.NewStyle().Bold(true)
)

// rule is the line that frames the report, as wide as the header it sits under
// and never wider than the terminal.
func rule(header, width int) string {
	length := max(header, ruleFloor)
	length = min(length, min(width, ruleCeiling))
	return ruleStyle.Render(strings.Repeat("─", length))
}

func report(verb, target, where string, p Plan) {
	fmt.Print(renderReport(verb, target, where, p))
}

func renderReport(verb, target, where string, p Plan) string {
	return renderReportAt(verb, target, where, p, terminalWidth())
}

// terminalWidth is the room the report has to lay names out in.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return fallbackWidth
	}
	return w
}

// renderReportAt is renderReport with the width handed in, which is the only
// way to test a layout that otherwise depends on whoever is looking at it.
func renderReportAt(verb, target, where string, p Plan, width int) string {
	var b strings.Builder

	if target == "" {
		target = "(everything)"
	}
	header := fmt.Sprintf("%s %s @ %s", garlicMark+" "+verb, target, where)
	fmt.Fprintf(&b, "%s\n%s\n", header, rule(lipgloss.Width(header), width))

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

	// Whether to split by bulb is decided once, for the whole report: a shape
	// that changed from bucket to bucket would read as a distinction rather
	// than as the absence of one.
	split := false
	seen := map[string]bool{}
	for _, bucket := range buckets {
		for _, rel := range bucket.paths {
			bulb, _, _ := strings.Cut(rel, "/")
			seen[bulb] = true
		}
	}
	if len(seen) > 1 {
		split = true
	}

	empty := true
	for _, bucket := range buckets {
		label, named := labels[verb][bucket.key]
		if len(bucket.paths) == 0 || !named {
			continue
		}
		empty = false

		heading := fmt.Sprintf("%s %s (%d)", marks[bucket.key], label, len(bucket.paths))
		fmt.Fprintf(&b, "\n  %s\n", headingStyles[bucket.key].Render(heading))
		if split {
			b.WriteString(byBulb(bucket.paths, width))
			continue
		}
		b.WriteString(columns(shorten(bucket.paths, target), reportIndent, width))
	}

	if empty {
		b.WriteString("\n  nothing to do\n")
	}

	b.WriteString(rule(lipgloss.Width(header), width) + "\n")
	return b.String()
}

// byBulb lays one bucket out under a heading per bulb. Only a report of the
// whole remote reaches here -- an address names one bulb by construction -- so
// there is nothing else to trim off the paths.
func byBulb(rels []string, width int) string {
	groups, order := group(rels)

	var b strings.Builder
	for _, bulb := range order {
		fmt.Fprintf(&b, "\n%s%s\n", strings.Repeat(" ", reportIndent), bulbStyle.Render(bulb))
		// The heading carries the bulb, so the names below it need not.
		b.WriteString(columns(shorten(groups[bulb], bulb), reportIndent+bulbIndent, width))
	}
	return b.String()
}

// group sorts paths into their bulbs, keeping the order the buckets arrived in
// so that two reports of the same remote read the same way.
func group(rels []string) (map[string][]string, []string) {
	groups := map[string][]string{}
	var order []string

	for _, rel := range rels {
		bulb, _, _ := strings.Cut(rel, "/")
		if _, seen := groups[bulb]; !seen {
			order = append(order, bulb)
		}
		groups[bulb] = append(groups[bulb], rel)
	}
	return groups, order
}

// shorten drops the address you asked about off the front of each path. The
// header line already names it, and repeating it down the left margin of every
// row is what turns a bulb-wide report into a wall.
//
// Both the address and its parent are tried, because a project address names a
// file without its extension: `epics/bioz/mealprep` is the prefix of everything
// in the resource folder, while the project file itself sits one level up.
// A path under neither is printed whole rather than mangled.
func shorten(rels []string, target string) []string {
	if target == "" || target == "(everything)" {
		return rels
	}

	prefixes := []string{target + "/"}
	if dir := path.Dir(target); dir != "." && dir != "/" {
		prefixes = append(prefixes, dir+"/")
	}

	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		short := rel
		for _, prefix := range prefixes {
			if trimmed, ok := strings.CutPrefix(rel, prefix); ok && trimmed != "" {
				short = trimmed
				break
			}
		}
		out = append(out, short)
	}
	return out
}

// columns lays names out in as many columns as the terminal allows, filling
// each column top to bottom so that a sorted list still reads downwards.
//
// Each column is measured on its own contents rather than on the longest name
// in the bucket. One `internal/remote/report.go` among a dozen short names
// would otherwise set the cell width for all of them and collapse the layout to
// a single column — which is the case worth laying out in the first place.
//
// One column is always possible: a name wider than the terminal overflows its
// row rather than being truncated, because a path you cannot read in full is a
// path you cannot act on.
func columns(names []string, indent, width int) string {
	if len(names) == 0 {
		return ""
	}

	avail := width - indent
	for cols := len(names); cols > 1; cols-- {
		rows := (len(names) + cols - 1) / cols
		// Dropping a column can leave the last one empty (7 names in 4 columns
		// is 2 rows, which 3 columns already covered); that layout was tried.
		if (cols-1)*rows >= len(names) {
			continue
		}
		if widths, total := layout(names, cols, rows); total <= avail {
			return draw(names, widths, rows, indent)
		}
	}

	widths, _ := layout(names, 1, len(names))
	return draw(names, widths, len(names), indent)
}

// layout measures each column of a candidate arrangement, and what the whole
// thing would cost. The last column carries no gutter: nothing follows it.
func layout(names []string, cols, rows int) (widths []int, total int) {
	widths = make([]int, cols)
	for c := range cols {
		for r := range rows {
			if i := c*rows + r; i < len(names) {
				if w := lipgloss.Width(names[i]); w > widths[c] {
					widths[c] = w
				}
			}
		}
		if c < cols-1 {
			widths[c] += gutter
		}
		total += widths[c]
	}
	return widths, total
}

func draw(names []string, widths []int, rows, indent int) string {
	var b strings.Builder
	pad := strings.Repeat(" ", indent)

	for r := range rows {
		line := pad
		for c := range widths {
			i := c*rows + r
			if i >= len(names) {
				continue
			}
			line += names[i]
			if next := (c+1)*rows + r; next < len(names) {
				line += strings.Repeat(" ", widths[c]-lipgloss.Width(names[i]))
			}
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return b.String()
}
