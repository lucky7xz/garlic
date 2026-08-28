package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetTags(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    Tags
	}{
		{
			"status only",
			"#statustag-inProgress\nsome notes\n",
			Tags{Status: "inProgress"},
		},
		{
			"hidden",
			"#garlic-hide\n#statustag-toDo\n",
			Tags{Status: "toDo", Hidden: true},
		},
		{
			// #AT is a convention for talking to an agent. Garlic ships it across
			// the wire like any other content and has no opinion about it.
			"agent tasks are not garlic's business",
			"#statustag-toDo\n- [ ] aggregate the logs #AT\n",
			Tags{Status: "toDo"},
		},
		{
			"nothing at all",
			"just prose\n",
			Tags{},
		},
		{
			"first status tag wins",
			"#statustag-toDo\n#statustag-onHold\n",
			Tags{Status: "toDo"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "p.md")
			if err := os.WriteFile(p, []byte(c.content), 0644); err != nil {
				t.Fatal(err)
			}
			if got := GetTags(p); got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestGetTagsOnMissingFile(t *testing.T) {
	if got := GetTags(filepath.Join(t.TempDir(), "nope.md")); got != (Tags{}) {
		t.Errorf("got %+v, want zero Tags", got)
	}
}
