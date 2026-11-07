package memory

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestMain(m *testing.M) { os.Exit(testx.RunWithTempHomes(m)) }

func newStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(filepath.Join(t.TempDir(), "memory.db"))
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func add(t *testing.T, s *Store, fact, reason string, kw ...string) commitResult {
	t.Helper()
	rev, err := s.Revision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.commitFact(context.Background(), commitInput{Revision: rev, Action: ActionAdd, Fact: fact,
		Reason: reason, Keywords: kw, Origin: OriginUser, Scope: ScopePersonal, Source: SourceUI, RequestID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func facts(ms []Memory) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Fact)
	}
	return out
}

func TestTriggersProtectHistory(t *testing.T) {
	s := newStore(t)
	r := add(t, s, "uses postgres", "ops decision")
	db, _ := s.conn()
	for name, q := range map[string]string{
		"immutable fact": `UPDATE memories SET fact = 'x' WHERE id = '` + r.ID + `'`,
		"delete":         `DELETE FROM memories WHERE id = '` + r.ID + `'`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("%s: expected abort", name)
		}
	}
	rev, _ := s.Revision(context.Background())
	if _, err := s.commitFact(context.Background(), commitInput{Revision: rev, Action: ActionUpdate, TargetID: r.ID,
		Fact: "uses postgres 16", Reason: "upgrade", Origin: OriginAgent, Scope: ScopePersonal, Source: SourceMCP, RequestID: "r2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE memories SET status = 'current' WHERE id = ?`, r.ID); err == nil {
		t.Error("a superseded memory must stay superseded")
	}
	var current int
	_ = db.QueryRow(`SELECT count(*) FROM memories WHERE lineage_id = ? AND status = 'current'`, r.LineageID).Scan(&current)
	if current != 1 {
		t.Errorf("current rows in lineage = %d, want 1", current)
	}
}

func TestStaleRevisionRejected(t *testing.T) {
	s := newStore(t)
	rev, _ := s.Revision(context.Background())
	add(t, s, "first fact", "why")
	_, err := s.commitFact(context.Background(), commitInput{Revision: rev, Action: ActionAdd, Fact: "second",
		Reason: "why", Origin: OriginUser, Scope: ScopePersonal, Source: SourceUI, RequestID: "r"})
	if err != errStale {
		t.Fatalf("err = %v, want errStale", err)
	}
}

func TestSearchRecall(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	add(t, s, "The deployment pipeline was deployed to staging", "release process")
	add(t, s, "Cluster runs on Kubernetes", "infra", "k8s", "kubernetes")
	add(t, s, "Prefers tabs over spaces", "style guide")
	old := add(t, s, "API port is 8080", "initial setup")
	rev, _ := s.Revision(ctx)
	if _, err := s.commitFact(ctx, commitInput{Revision: rev, Action: ActionUpdate, TargetID: old.ID, Fact: "API port is 9090",
		Reason: "moved", Origin: OriginUser, Scope: ScopePersonal, Source: SourceUI, RequestID: "r"}); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, q, want string }{
		{"prefix", "deplo", "The deployment pipeline was deployed to staging"},
		{"stem", "deploying", "The deployment pipeline was deployed to staging"},
		{"one of three terms", "tabs unrelated nonsense", "Prefers tabs over spaces"},
		{"keyword column", "k8s", "Cluster runs on Kubernetes"},
		{"stopword only", "the of and", ""},
	}
	for _, c := range cases {
		got, err := s.searchFTS(ctx, SearchArgs{Query: c.q})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.want != "" && !slices.Contains(facts(got), c.want) {
			t.Errorf("%s: %v missing %q", c.name, facts(got), c.want)
		}
	}

	cur, _ := s.searchFTS(ctx, SearchArgs{Query: "port"})
	if !slices.Equal(facts(cur), []string{"API port is 9090"}) {
		t.Errorf("current-only = %v", facts(cur))
	}
	all, _ := s.searchFTS(ctx, SearchArgs{Query: "port", IncludeHistory: true})
	if len(all) != 2 {
		t.Fatalf("with history = %v", facts(all))
	}
	for _, m := range all {
		if m.Historical != (m.Fact == "API port is 8080") {
			t.Errorf("historical flag wrong on %q", m.Fact)
		}
	}
}

func TestSearchNeverFailsOnFTSSyntax(t *testing.T) {
	s := newStore(t)
	add(t, s, "plain fact here", "reason")
	for _, q := range []string{`"`, `NEAR(a b)`, `a AND -b`, `col:x`, `*`, `fact"" OR "`, `^fact`, `(fact`} {
		if _, err := s.searchFTS(context.Background(), SearchArgs{Query: q}); err != nil {
			t.Errorf("query %q: %v", q, err)
		}
	}
}
