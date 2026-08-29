package remote

import (
	"testing"

	"github.com/lucky7xz/garlic/internal/domain"
)

// Harvest cannot validate an address against the local board the way plant
// does: the projects most worth collecting are the ones the agent created, and
// those exist only on the remote. So an address counts as naming something if
// any of the three states knows it -- and naming nothing in all three is a typo
// worth refusing, rather than a "nothing to do" that reads like a clean harvest.
func TestAddressNames(t *testing.T) {
	full := domain.BoardOptions{Name: "epics", Path: "/x/epics", Extension: ".md"}

	cases := []struct {
		name    string
		bulb    domain.BoardOptions
		addr    Address
		local   Census
		planted Manifest
		remote  Census
		want    bool
	}{
		{
			name:  "on the board here but never planted",
			bulb:  full,
			addr:  Address{Bulb: "epics", Area: "bioz", Project: "mealprep"},
			local: Census{"epics/bioz/mealprep.md": "A"},
			want:  true,
		},
		{
			// The case that matters: the agent made it, so it is nowhere else.
			name:   "only on the remote",
			bulb:   full,
			addr:   Address{Bulb: "epics", Area: "bioz", Project: "findings"},
			remote: Census{"epics/bioz/findings.md": "A"},
			want:   true,
		},
		{
			// Deleted both sides but still in the baseline: harvest has something
			// to say about it, so the address is not a typo.
			name:    "only in the manifest",
			bulb:    full,
			addr:    Address{Bulb: "epics", Area: "bioz", Project: "mealprep"},
			planted: Manifest{"epics/bioz/mealprep.md": "A"},
			want:    true,
		},
		{
			name:   "a mistyped project",
			bulb:   full,
			addr:   Address{Bulb: "epics", Area: "bioz", Project: "mealprepp"},
			local:  Census{"epics/bioz/mealprep.md": "A"},
			remote: Census{"epics/bioz/mealprep.md": "A"},
			want:   false,
		},
		{
			name:   "a mistyped area",
			bulb:   full,
			addr:   Address{Bulb: "epics", Area: "bioooz"},
			local:  Census{"epics/bioz/mealprep.md": "A"},
			remote: Census{"epics/bioz/mealprep.md": "A"},
			want:   false,
		},
		{
			name:  "an area with only resources under it still counts",
			bulb:  full,
			addr:  Address{Bulb: "epics", Area: "bioz"},
			local: Census{"epics/bioz/mealprep/notes.txt": "A"},
			want:  true,
		},
		{
			// On a semi bulb the folder is the project, so a third segment is
			// ignored -- naming any clove in the folder names the whole folder.
			name:  "semi bulb ignores the project segment",
			bulb:  domain.BoardOptions{Name: "scripts", Path: "/x/scripts", Extension: ".clove.md", WholeFolder: true},
			addr:  Address{Bulb: "scripts", Area: "garlic", Project: "whatever"},
			local: Census{"scripts/garlic/revise.clove.md": "A"},
			want:  true,
		},
		{
			name:  "ignored files do not make an address real",
			bulb:  domain.BoardOptions{Name: "scripts", Path: "/x/scripts", Extension: ".clove.md", WholeFolder: true, Ignore: []string{"dist"}},
			addr:  Address{Bulb: "scripts", Area: "drako"},
			local: Census{"scripts/drako/dist/bundle.js": "A"},
			want:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := addressNames(c.addr, c.bulb, c.local, Census(c.planted), c.remote)
			if got != c.want {
				t.Errorf("addressNames = %v, want %v", got, c.want)
			}
		})
	}
}
