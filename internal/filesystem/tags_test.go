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
			"an outstanding agent task",
			"#statustag-toDo\n- [ ] aggregate the logs #AT\n",
			Tags{Status: "toDo", AgentTask: true},
		},
		{
			"the agent flipped it",
			"#statustag-toDo\n- [x] aggregate the logs #AT-done\n",
			Tags{Status: "toDo"},
		},
		{
			"one done, one still outstanding",
			"#statustag-toDo\n- [x] first #AT-done\n- [ ] second #AT\n",
			Tags{Status: "toDo", AgentTask: true},
		},
		{
			"every task flipped",
			"#statustag-toDo\n- [x] first #AT-done\n- [x] second #AT-done\n",
			Tags{Status: "toDo"},
		},
		{
			"#AT must not match a longer word",
			"#statustag-toDo\n#ATTENTION please\n",
			Tags{Status: "toDo"},
		},
		{
			"#AT at end of a sentence",
			"#statustag-toDo\nplease do this #AT.\n",
			Tags{Status: "toDo", AgentTask: true},
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
