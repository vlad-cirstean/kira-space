package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const specV1 = `# Spec

| Phase | Deliverable | Why here |
|---|---|---|
| **P0 Foundation: tooling** | Repo setup. **Done** | first |
| **P5.5 Half step** | Between | after P5 |

Prose that mentions a decision on P5.5 outside any table.

| **P58f Cutover** | Switch over, rather than dual-run | last |
| **P52-P57 Wails migration** | Seven steps | |
| **P16 Part 2: fixes** | Misc **Superseded by P20** | |

| # | Check | Result |
|---|---|---|
| 1 | P99 checked | ok |
`

const specV13 = `| Phase | Status upstream | Lands in this chapter as |
|---|---|---|
| P0 Foundation | Done | n/a |

| # | Deliverable | Depends on | Covers upstream |
|---|---|---|---|
| **G5** | Refs and tags | G2 | P7 |
`

const specV22 = `| Phase | Title | Status |
|---|---|---|
| P210 | Embedding search \| local model | Done. Shipped in full. |
| P212 Part 1 | Mobile web | Todo |

## P210, P212 Part 1 result

Chose X instead of Y.

### Detail

kept

## P52-P57 result

range body

` + "```" + `
## P999 result
` + "```" + `

## Other prose

Closing: reverted the cache.
`

func TestTableShapes(t *testing.T) {
	p := Parse("v1", specV1, "", nil, nil)
	var ids []string
	for _, ph := range p.Phases {
		ids = append(ids, ph.ID)
	}
	want := []string{"P0", "P5.5", "P58f", "P52-P57", "P16 Part 2"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if p.Phases[0].Title != "Foundation: tooling" || p.Phases[0].Status != "Done" {
		t.Errorf("P0 = %+v", p.Phases[0])
	}
	if p.Phases[4].Status != "Superseded by P20" {
		t.Errorf("status = %q", p.Phases[4].Status)
	}
	if len(p.Notes) != 1 || len(p.Phases[1].Mentions) != 1 {
		t.Errorf("notes = %v mentions = %v", p.Notes, p.Phases[1].Mentions)
	}

	q := Parse("v1.3", specV13, "", nil, nil)
	if len(q.Phases) != 1 || q.Phases[0].ID != "G5" || len(q.Phases[0].Cells) != 3 {
		t.Errorf("v1.3 phases = %+v", q.Phases)
	}
}

func TestResultSections(t *testing.T) {
	p := Parse("v2.2", specV22, "", nil, nil)
	if len(p.Phases) != 3 {
		t.Fatalf("phases = %d, want 3 (P52-P57 from its result heading)", len(p.Phases))
	}
	a, b, r := p.Phases[0], p.Phases[1], p.Phases[2]
	if a.Title != "Embedding search | local model" || a.Status != "Done." {
		t.Errorf("P210 = %+v", a)
	}
	if len(a.Results) != 1 || a.Results[0] != b.Results[0] || !strings.Contains(a.Results[0], "### Detail") {
		t.Errorf("comma heading did not file one body under both: %q / %q", a.Results, b.Results)
	}
	if r.ID != "P52-P57" || r.Title != "" || strings.Contains(r.Results[0], "other prose") {
		t.Errorf("range = %+v", r)
	}
	if !strings.Contains(r.Results[0], "## P999 result") {
		t.Errorf("fenced heading must stay in the body: %q", r.Results[0])
	}
	if len(p.Notes) != 1 || !strings.HasPrefix(p.Notes[0], "Closing:") {
		t.Errorf("notes = %v", p.Notes)
	}
}

func TestPlanResult(t *testing.T) {
	plans := map[string]string{
		"P131-part2-x.md": "# P131\n\n## Plan\n\nstuff\n\n## Result\n\nDone it.\n\n## Appendix\n\nno\n",
		"README.md":       "## Result\nignored",
	}
	p := Parse("v2.0", "| Phase | Deliverable |\n|---|---|\n| **P131 Part 2: x** | y |\n", "", nil, plans)
	if len(p.Phases) != 1 || len(p.Phases[0].Results) != 1 || p.Phases[0].Results[0] != "Done it." {
		t.Fatalf("phases = %+v", p.Phases)
	}
}

func TestBlocksSplice(t *testing.T) {
	dir := t.TempDir()
	aPath, bPath := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	aOrig := "<!-- chapter v1 -->\n## v1\nx\n<!-- /chapter v1 -->\n\n## Fixes\nfree text\n\n<!-- chapter v1.1 -->\n## v1.1\nz\n<!-- /chapter v1.1 -->\n"
	bOrig := "<!-- chapter v1.9 -->\n## v1.9\ny\n<!-- /chapter v1.9 -->\n"
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(aPath, aOrig)
	write(bPath, bOrig)
	got, err := readBlocks(dir)
	want := map[string]histBlock{
		"v1":   {aPath, "## v1\nx\n"},
		"v1.1": {aPath, "## v1.1\nz\n"},
		"v1.9": {bPath, "## v1.9\ny\n"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("readBlocks = %v, %v", got, err)
	}
	if err := spliceBlock(aPath, "v1.1", "## v1.1\nnew $1\n"); err != nil {
		t.Fatal(err)
	}
	spliced, _ := os.ReadFile(aPath)
	wantA := strings.Replace(aOrig, "z\n", "new $1\n", 1)
	bAfter, _ := os.ReadFile(bPath)
	if string(spliced) != wantA || string(bAfter) != bOrig {
		t.Errorf("splice result:\n%s", spliced)
	}
	if err := spliceBlock(aPath, "v1.1", "## v1.1\nz\n"); err != nil {
		t.Fatal(err)
	}
	if back, _ := os.ReadFile(aPath); string(back) != aOrig {
		t.Errorf("restore differs:\n%s", back)
	}
	cPath := filepath.Join(dir, "c.md")
	write(cPath, "<!-- chapter v1 -->\nd\n<!-- /chapter v1 -->\n")
	if _, err := readBlocks(dir); err == nil {
		t.Error("duplicate chapter across files accepted")
	}
	write(cPath, "<!-- chapter v2.0 -->\nd\n<!-- /chapter v2.1 -->\n")
	if _, err := readBlocks(dir); err == nil {
		t.Error("mismatched close marker accepted")
	}
}

func TestRepointText(t *testing.T) {
	commits := map[string]string{"v1": "aaaaaaaaaaaa", "v1.1": "bbbbbbbbbbbb"}
	in := "see docs/v1.1/plans/P22-slickgrid-plan.md §4.2 and docs/v1/SPEC.md:10-12. Glob docs/v1.1/plans/P1*.md"
	got, n, um := repointText("src/x.go", in, commits)
	want := "see docs/HISTORY.md (v1.1 P22) and docs/HISTORY.md (v1). Glob docs/v1.1/plans/P1*.md"
	if got != want || n != 2 || len(um) != 1 {
		t.Errorf("got %q n=%d unmatched=%v", got, n, um)
	}
	got, _, _ = repointText("scripts/a.sh", "# docs/v1/plans/P1-x.md", commits)
	if got != "# git show aaaaaaaaaaaa:docs/v1/plans/P1-x.md" {
		t.Errorf("operative = %q", got)
	}
	for p, want := range map[string]bool{
		"scripts/docshistory/parse_test.go":                                           true,
		"apps/kira-space/tests/claude/testdata/memory-eval/P285-corpus/manifest.json": true,
		"docs/history/06-v0.2.10.md":                                                  true,
		"src/x.go":                                                                    false,
	} {
		if skipRepoint(p) != want {
			t.Errorf("skipRepoint(%q) != %v", p, want)
		}
	}
}
