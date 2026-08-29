package remote

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// The gates get harder as the blast radius widens, because the accident they
// guard is naming something broader than you meant -- `epics` when you wanted
// `epics/bioz`. Muscle memory from wiping a project must not carry you through
// wiping a bulb.
func TestGates(t *testing.T) {
	cases := []struct {
		name  string
		addr  Address
		files int
		want  []string // what each gate demands, "" for a bare yes/no
	}{
		{
			"a project asks once",
			Address{Bulb: "epics", Area: "bioz", Project: "mealprep"},
			10,
			[]string{"mealprep"},
		},
		{
			"an area confirms first, then names itself",
			Address{Bulb: "epics", Area: "bioz"},
			38,
			[]string{"", "bioz"},
		},
		{
			"a bulb adds the count, which cannot be typed without reading",
			Address{Bulb: "epics"},
			340,
			[]string{"", "epics", "340"},
		},
		{
			"the root asks four times, the last after everything is on screen",
			Address{},
			512,
			[]string{"", "berta", "512", ""},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gates(c.addr, "berta", c.files)

			var want []string
			for _, g := range got {
				want = append(want, g.want)
			}
			if !reflect.DeepEqual(want, c.want) {
				t.Errorf("gates demanded %q, want %q", want, c.want)
			}
			for _, g := range got {
				if g.prompt == "" {
					t.Error("a gate with no prompt cannot be answered")
				}
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	ask := gates(Address{Bulb: "epics"}, "berta", 340) // y -> epics -> 340

	cases := []struct {
		name  string
		typed string
		want  bool
	}{
		{"every answer right", "y\nepics\n340\n", true},
		{"yes spelled out", "yes\nepics\n340\n", true},
		{"declined at the first gate", "n\n", false},
		{"declined by saying nothing", "\n", false},
		{"the wrong name", "y\nepic\n340\n", false},
		{"the wrong count", "y\nepics\n34\n", false},
		{"surrounding whitespace is forgiven", "  y \n epics \n 340 \n", true},
		{"stdin ends early", "y\nepics\n", false},
		{"stdin is empty, as when piped", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := confirm(&out, strings.NewReader(c.typed), ask); got != c.want {
				t.Errorf("confirm = %v, want %v\nprompted:\n%s", got, c.want, out.String())
			}
		})
	}
}

// A prompt names the kind of thing it wants but never the value: being handed
// the answer turns a gate into a copy. The value you cannot know -- the file
// count -- comes from the summary printed above the gates, not from the prompt.
func TestConfirmPromptsNameWhatTheyWantButNotTheAnswer(t *testing.T) {
	var out bytes.Buffer
	confirm(&out, strings.NewReader("y\nepics\n340\n"), gates(Address{Bulb: "epics"}, "berta", 340))

	for _, want := range []string{"bulb name", "file count"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("prompts never ask for the %s:\n%s", want, out.String())
		}
	}
	for _, leak := range []string{"epics", "340"} {
		if strings.Contains(out.String(), leak) {
			t.Errorf("a prompt gave away %q, so it can be copied unread:\n%s", leak, out.String())
		}
	}
}

// The root's final gate exists to be a last look, so declining there must stop
// the wipe even though every earlier answer was correct.
func TestConfirmRootFinalGateCanStillRefuse(t *testing.T) {
	ask := gates(Address{}, "berta", 512)

	var out bytes.Buffer
	if confirm(&out, strings.NewReader("y\nberta\n512\nn\n"), ask) {
		t.Error("the final gate accepted a refusal")
	}
}
