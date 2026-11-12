package github

import (
	"os"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// GitHub rejects a whole request over an invalid document (differing-argument field merges,
// nullability conflicts), and fixture-fed tests never see that. Validate against GitHub's published
// schema; refresh testdata/github-schema.docs.graphql from
// https://docs.github.com/public/fpt/schema.docs.graphql.
func TestGraphQLDocumentsValidate(t *testing.T) {
	src, err := os.ReadFile("testdata/github-schema.docs.graphql")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "github-schema.docs.graphql", Input: string(src)})
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	for name, doc := range map[string]string{
		"detailQuery":         detailQuery,
		"syncPageQuery":       syncPageQuery,
		"syncChecksQuery":     syncChecksQuery,
		"replyThreadMutation": replyThreadMutation,
	} {
		t.Run(name, func(t *testing.T) {
			if _, errs := gqlparser.LoadQuery(schema, doc); len(errs) > 0 {
				t.Fatalf("invalid against GitHub schema: %v", errs)
			}
		})
	}
}
