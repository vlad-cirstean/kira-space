package taskfiles

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the README goldens")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from golden (run with -update):\n%s", name, got)
	}
	if n := strings.Count(got, "\n"); n > MaxReadmeLines {
		t.Fatalf("%d lines, cap is %d", n, MaxReadmeLines)
	}
}

func TestReadmeGoldens(t *testing.T) {
	golden(t, "readme-task.golden", Readme(ReadmeInput{TaskID: "t-42", Title: "Fix order totals", Step: "implement",
		Proofs: []ReadmeProof{{Slug: "orders", Name: "Orders add up", Steps: []ReadmeStep{{"totals", "table"}, {"flow", "diagram"}}}}}))
	var kids []ReadmeChild
	for i := 1; i <= 3; i++ {
		kids = append(kids, ReadmeChild{ID: fmt.Sprintf("c-%d", i), Title: fmt.Sprintf("Child %d", i), LatestHandover: "handovers/0002-review-to-implement.md"})
	}
	golden(t, "readme-orchestrator.golden", Readme(ReadmeInput{TaskID: "o-1", Title: "Epic", Proofs: nil, Children: kids}))
}

func TestReadmeCapHoldsForAnyTask(t *testing.T) {
	var proofs []ReadmeProof
	var kids []ReadmeChild
	for i := 0; i < 60; i++ {
		proofs = append(proofs, ReadmeProof{Slug: fmt.Sprintf("p%d", i), Name: strings.Repeat("n", 500), Steps: []ReadmeStep{{"s", "table"}}})
		kids = append(kids, ReadmeChild{ID: fmt.Sprintf("c%d", i), Title: strings.Repeat("t\n", 300)})
	}
	out := Readme(ReadmeInput{TaskID: "x", Title: strings.Repeat("T", 900), Proofs: proofs, Children: kids})
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > MaxReadmeLines {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines[5:] {
		if len(l) > maxReadmeLineBytes {
			t.Fatalf("index line of %d bytes", len(l))
		}
	}
	if !strings.Contains(out, "and 55 more under proofs/") || !strings.Contains(out, " more, see state.yaml chain") {
		t.Fatalf("missing more markers:\n%s", out)
	}
}
