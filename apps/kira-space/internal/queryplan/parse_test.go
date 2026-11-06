package queryplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// The five-dialect plan port, against the shared parity fixtures
// tests/fixtures/studio/explain-plans/*.{input,expected}.json — generated from the TypeScript side first,
// so this test is what gets pinned to them, never the reverse. A drift in either language's own
// parser fails a test in that language, on the same bytes.

const fixturesDir = "../../tests/fixtures/studio/explain-plans"

type pageSpec struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type fixtureInput struct {
	Kind               string     `json:"kind"`
	ThresholdRows      int        `json:"thresholdRows"`
	Pages              []pageSpec `json:"pages"`
	TruncatedFirstCell bool       `json:"truncatedFirstCell"`
}

func fixtureCases(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("read %s: %v", fixturesDir, err)
	}
	var out []string
	for _, e := range entries {
		const suffix = ".input.json"
		name := e.Name()
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			out = append(out, name[:len(name)-len(suffix)])
		}
	}
	sort.Strings(out)
	return out
}

func loadInput(t *testing.T, caseName string) fixtureInput {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixturesDir, caseName+".input.json"))
	if err != nil {
		t.Fatalf("read input for %s: %v", caseName, err)
	}
	var in fixtureInput
	if err := json.Unmarshal(b, &in); err != nil {
		t.Fatalf("parse input for %s: %v", caseName, err)
	}
	return in
}

func loadExpectedRaw(t *testing.T, caseName string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixturesDir, caseName+".expected.json"))
	if err != nil {
		t.Fatalf("read expected for %s: %v", caseName, err)
	}
	return b
}

// buildPages builds real page.Page values out of a fixture's own pages spec, via the same
// TabularPageBuilder the real adapter path uses — the parity test exercises the real page-parsing
// glue (page.IsNull/page.CellText/page.IsTruncated), not a shortcut straight to a dialect parser
// function.
func buildPages(t *testing.T, spec []pageSpec) []page.Page {
	t.Helper()
	out := make([]page.Page, 0, len(spec))
	for _, ps := range spec {
		cols := make([]page.ColumnDescriptor, len(ps.Columns))
		for i, name := range ps.Columns {
			cols[i] = page.ColumnDescriptor{Name: name, DataType: "text", TypeClass: page.TypeClassText, Nullable: true}
		}
		b := page.NewTabularPageBuilder(cols)
		for _, row := range ps.Rows {
			values := make([]*string, len(row))
			for i := range row {
				v := row[i]
				values[i] = &v
			}
			if err := b.AppendRow(values); err != nil {
				t.Fatalf("AppendRow: %v", err)
			}
		}
		out = append(out, b.Finish(page.UnpagedPosition(len(ps.Rows))))
	}
	return out
}

// buildTruncatedPage builds the one-page, one-oversized-cell shape every single-cell dialect's
// truncated-cell case needs, matching tests/fixtures/studio/explain-plans/loader.ts's own
// buildTruncatedPage exactly (the same MAX_CELL_BYTES-exceeding synthetic cell), so a real
// over-the-wire clip is what both languages' truncated-cell fixtures exercise.
func buildTruncatedPage(t *testing.T) []page.Page {
	t.Helper()
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{{Name: "QUERY PLAN", DataType: "text", TypeClass: page.TypeClassText, Nullable: true}})
	oversized := make([]byte, 0, page.MaxCellBytes+64)
	oversized = append(oversized, []byte(`[{"Plan":{"x":"`)...)
	for len(oversized) < page.MaxCellBytes+16 {
		oversized = append(oversized, 'y')
	}
	oversized = append(oversized, []byte(`"}}]`)...)
	v := string(oversized)
	if err := b.AppendRow([]*string{&v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	return []page.Page{b.Finish(page.UnpagedPosition(1))}
}

// sortMetricsByLabel is already this package's own rule for what parsers emit; normalize
// applies it recursively so a Plan built here (which — like the TypeScript port's own parser
// output — is already sorted by construction) is compared against expected.json's own sorted form
// consistently regardless of which side happens to change first.
func normalizeNode(n Node) Node {
	// append(nil, s...) with len(s) == 0 returns nil unchanged (no allocation happens) — silently
	// turning a genuinely non-nil empty Metrics slice back into nil (which marshals as `null`, not
	// `[]`) if written the shorter way. make+copy keeps a non-nil empty slice non-nil.
	metrics := make([]Metric, len(n.Metrics))
	copy(metrics, n.Metrics)
	n.Metrics = metrics
	sort.Slice(n.Metrics, func(i, j int) bool { return n.Metrics[i].Label < n.Metrics[j].Label })
	children := make([]Node, len(n.Children))
	for i, c := range n.Children {
		children[i] = normalizeNode(c)
	}
	n.Children = children
	return n
}

func normalizePlan(p Plan) Plan {
	p.Root = normalizeNode(p.Root)
	return p
}

func TestParityFixtures(t *testing.T) {
	for _, caseName := range fixtureCases(t) {
		t.Run(caseName, func(t *testing.T) {
			input := loadInput(t, caseName)

			if input.TruncatedFirstCell {
				pages := buildTruncatedPage(t)
				_, err := FromPages(input.Kind, pages, input.ThresholdRows)
				if !errors.Is(err, ErrTruncated) {
					t.Fatalf("FromPages(%s, truncated) error = %v, want ErrTruncated", input.Kind, err)
				}
				var expected struct {
					Truncated bool `json:"truncated"`
				}
				if err := json.Unmarshal(loadExpectedRaw(t, caseName), &expected); err != nil {
					t.Fatalf("parse expected: %v", err)
				}
				if !expected.Truncated {
					t.Fatalf("expected.json for %s does not say truncated:true", caseName)
				}
				return
			}

			pages := buildPages(t, input.Pages)
			plan, err := FromPages(input.Kind, pages, input.ThresholdRows)
			if err != nil {
				t.Fatalf("FromPages(%s): %v", input.Kind, err)
			}
			plan = normalizePlan(plan)

			gotJSON, err := json.Marshal(plan)
			if err != nil {
				t.Fatalf("marshal plan: %v", err)
			}
			var got any
			if err := json.Unmarshal(gotJSON, &got); err != nil {
				t.Fatalf("re-decode plan: %v", err)
			}

			var want any
			if err := json.Unmarshal(loadExpectedRaw(t, caseName), &want); err != nil {
				t.Fatalf("parse expected: %v", err)
			}

			gotCanon, _ := json.Marshal(got)
			wantCanon, _ := json.Marshal(want)
			if !bytes.Equal(gotCanon, wantCanon) {
				t.Fatalf("plan mismatch for %s\n got: %s\nwant: %s", caseName, gotCanon, wantCanon)
			}
		})
	}
}

// TestFormatJSNumberMatchesJSToString pins Number.prototype.toString's notation switches (P168
// Part 6 F19); the TS plan parsers render untyped metrics through String(v).
func TestFormatJSNumberMatchesJSToString(t *testing.T) {
	cases := map[float64]string{
		0: "0", 10: "10", -3: "-3", 1234567.5: "1234567.5", 0.00001: "0.00001", 0.000001: "0.000001",
		1e-7: "1e-7", 1.5e-7: "1.5e-7", 2e20: "200000000000000000000", 1e21: "1e+21", 1.5e21: "1.5e+21",
		123456789012345680000: "123456789012345680000", 0.5: "0.5", -1.25e-9: "-1.25e-9",
	}
	for in, want := range cases {
		if got := formatJSNumber(in); got != want {
			t.Errorf("formatJSNumber(%v) = %q, want %q", in, got, want)
		}
	}
}
