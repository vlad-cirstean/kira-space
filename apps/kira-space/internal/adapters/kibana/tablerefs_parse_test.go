package kibana

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func TestParseTablesKibana(t *testing.T) {
	path := model.NodePath{Segments: []model.PathSegment{{Kind: "collection", Name: "pathview"}}}
	cases := []struct {
		name, in string
		path     model.NodePath
		ref      string
		refusal  string
	}{
		{"request view wins", `{"dataView":"other"}`, path, "other", ""},
		{"path view", `{}`, path, "pathview", ""},
		{"no view", `{}`, model.NodePath{}, "", "parse error"},
		{"unknown field", `{"dataView":"a","bogus":1}`, path, "", "parse error"},
		{"not json", `x`, path, "", "parse error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parseTablesKibana(c.in, c.path)
			if c.refusal != "" {
				if !strings.HasPrefix(p.Refusal, c.refusal) {
					t.Fatalf("refusal = %q", p.Refusal)
				}
				return
			}
			if p.Refusal != "" || len(p.Refs) != 1 || p.Refs[0].Name != c.ref {
				t.Fatalf("got %+v", p)
			}
		})
	}
}
