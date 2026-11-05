package configfs

import (
	"errors"
	"testing"
)

func TestEditYAML(t *testing.T) {
	src := "# top\npath: /r\ngraph:\n  page_size: 5000 # keep\n  scope: all\nreview: {}\n"
	got, err := EditYAML([]byte(src), []NodeSet{
		{Path: []string{"graph", "page_size"}, Value: 3000},
		{Path: []string{"stash", "show_in_graph"}, Value: false},
		{Path: []string{"review", "base_candidates"}, Value: []string{"main", "dev"}},
		{Path: []string{"worktree", "prepare_script"}, Value: "bun install\nbun run gen\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "# top\npath: /r\ngraph:\n  page_size: 3000 # keep\n  scope: all\nreview:\n  base_candidates:\n    - main\n    - dev\nstash:\n  show_in_graph: false\nworktree:\n  prepare_script: |\n    bun install\n    bun run gen\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if _, err := EditYAML([]byte("path: /r\n"), []NodeSet{{Path: []string{"path", "x"}, Value: 1}}); !errors.Is(err, ErrNotMapping) {
		t.Errorf("scalar parent: err = %v, want ErrNotMapping", err)
	}
}

func TestEditYAMLUnset(t *testing.T) {
	cases := []struct {
		name, src, want string
		path            []string
	}{
		{"leaf", "a: 1\nb: 2 # keep\n", "b: 2 # keep\n", []string{"a"}},
		{"nested prunes empty parents", "x: 1\nw:\n  p:\n    q: 1\n", "x: 1\n", []string{"w", "p", "q"}},
		{"sibling keeps parent", "w:\n  p: 1\n  q: 2\n", "w:\n  q: 2\n", []string{"w", "p"}},
		{"parent with head comment stays", "# why\nw:\n  p: 1\n", "# why\nw: {}\n", []string{"w", "p"}},
		{"file header moves to the next key", "# top\na: 1\nb: 2\n", "# top\nb: 2\n", []string{"a"}},
		{"missing path is a no-op", "a: 1\n", "a: 1\n", []string{"w", "p"}},
		{"path through a scalar is a no-op", "a: 1\n", "a: 1\n", []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := EditYAML([]byte(c.src), []NodeSet{{Path: c.path, Unset: true}})
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}
