package gitsearch

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// conformanceRow mirrors conformance.spec.ts's own ConformanceRow shape exactly — one JSON file,
// two readers (docs/HISTORY.md (v1.3 G23)).
type conformanceRow struct {
	Name           string          `json:"name"`
	Query          conformanceQry  `json:"query"`
	Subject        string          `json:"subject"`
	Body           string          `json:"body"`
	AuthorName     string          `json:"authorName"`
	AuthorEmail    string          `json:"authorEmail"`
	CommitterName  string          `json:"committerName"`
	CommitterEmail string          `json:"committerEmail"`
	SHA            string          `json:"sha"`
	Expect         conformanceWant `json:"expect"`
}

type conformanceQry struct {
	Text          string `json:"text"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
	Regex         bool   `json:"regex"`
}

type conformanceWant struct {
	Supported bool     `json:"supported"`
	Fields    []string `json:"fields"`
}

type conformanceCorpus struct {
	Rows []conformanceRow `json:"rows"`
}

// corpusPath resolves packages/git-core/testdata/searchConformance.json relative to this source
// file's own location — stable regardless of the working directory `go test` is invoked from.
func corpusPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/gitsearch -> apps/kira-space -> apps -> <repo root> -> packages/git-core/testdata
	return filepath.Join(thisFile, "..", "..", "..", "..", "..", "packages", "git-core", "testdata", "searchConformance.json")
}

func loadCorpus(t *testing.T) conformanceCorpus {
	t.Helper()
	b, err := os.ReadFile(corpusPath(t))
	if err != nil {
		t.Fatalf("read searchConformance.json: %v", err)
	}
	var corpus conformanceCorpus
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatalf("unmarshal searchConformance.json: %v", err)
	}
	return corpus
}

func toQuery(q conformanceQry) Query {
	return Query{Text: q.Text, CaseSensitive: q.CaseSensitive, WholeWord: q.WholeWord, Regex: q.Regex}
}

func toFields(row conformanceRow) CommitFields {
	return CommitFields{
		SHA: row.SHA, Subject: row.Subject, Body: row.Body,
		AuthorName: row.AuthorName, AuthorEmail: row.AuthorEmail,
		CommitterName: row.CommitterName, CommitterEmail: row.CommitterEmail,
	}
}

// TestConformanceCorpus_NonEmpty guards against a silently-empty or unreadable corpus file
// passing this suite vacuously.
func TestConformanceCorpus_NonEmpty(t *testing.T) {
	t.Parallel()
	corpus := loadCorpus(t)
	if len(corpus.Rows) == 0 {
		t.Fatal("searchConformance.json has no rows")
	}
}

// TestConformanceCorpus_AgreesWithCompileAndMatchFields is G23's own exit-criterion item 2: every
// row in the shared corpus, run through Compile + MatchFields, agrees with the expectation
// conformance.spec.ts asserts against the exact same file on the TypeScript side.
func TestConformanceCorpus_AgreesWithCompileAndMatchFields(t *testing.T) {
	t.Parallel()
	corpus := loadCorpus(t)
	for _, row := range corpus.Rows {
		t.Run(row.Name, func(t *testing.T) {
			m, err := Compile(toQuery(row.Query))
			if !row.Expect.Supported {
				if !errors.Is(err, ErrUnsupportedPattern) {
					t.Fatalf("Compile error = %v, want ErrUnsupportedPattern", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Compile: unexpected error %v", err)
			}
			got := m.MatchFields(toFields(row))
			gotStrings := fieldStrings(got)
			want := row.Expect.Fields
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(gotStrings, want) {
				t.Fatalf("MatchFields = %v, want %v", gotStrings, want)
			}
		})
	}
}
