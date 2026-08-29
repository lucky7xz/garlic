package config

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/lucky7xz/garlic/internal/domain"
)

// Garlic stopped modelling the colours it never drew with, but an existing
// ~/.config/garlic/themes.toml still carries them -- EnsureUserFile only copies
// the bundled file when one is missing. Those files have to keep working, so
// this pins that unknown keys are ignored rather than rejected.
func TestThemesWithRetiredKeysStillLoad(t *testing.T) {
	old := `
[dracula]
Primary    = "#ff79c6"
Secondary  = "#bd93f9"
Background = "#282a36"
Foreground = "#f8f8f2"
Comment    = "#6272a4"
Success    = "#50fa7b"
Warning    = "#f1fa8c"
Error      = "#ff5555"
Info       = "#8be9fd"
Accent     = "#ff79c6"
Cyan       = "#8be9fd"
Green      = "#50fa7b"
Orange     = "#ffb86c"
Pink       = "#ff79c6"
Purple     = "#bd93f9"
Red        = "#ff5555"
Yellow     = "#f1fa8c"
`

	var themes domain.Themes
	if _, err := toml.Decode(old, &themes); err != nil {
		t.Fatalf("a theme file written for an older garlic no longer loads: %v", err)
	}

	got, ok := themes["dracula"]
	if !ok {
		t.Fatal("the theme did not decode at all")
	}
	for _, c := range []struct{ name, want, have string }{
		{"Primary", "#ff79c6", got.Primary},
		{"Secondary", "#bd93f9", got.Secondary},
		{"Foreground", "#f8f8f2", got.Foreground},
		{"Comment", "#6272a4", got.Comment},
		{"Warning", "#f1fa8c", got.Warning},
		{"Accent", "#ff79c6", got.Accent},
	} {
		if c.have != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.have, c.want)
		}
	}
}

// Every colour the bundled themes ship is one garlic actually draws with.
func TestBundledThemesCarryNothingUnused(t *testing.T) {
	raw, err := bootstrapFS.ReadFile("bootstrap/themes.toml")
	if err != nil {
		t.Fatal(err)
	}

	var themes domain.Themes
	meta, err := toml.Decode(string(raw), &themes)
	if err != nil {
		t.Fatalf("bundled themes do not decode: %v", err)
	}
	if left := meta.Undecoded(); len(left) > 0 {
		t.Errorf("bundled themes carry keys garlic never reads: %v", left)
	}
	if len(themes) == 0 {
		t.Fatal("no themes decoded, so this proves nothing")
	}
}
