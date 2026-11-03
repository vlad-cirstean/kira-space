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
