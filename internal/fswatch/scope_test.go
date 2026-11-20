package fswatch

import (
	"path/filepath"
	"sort"
	"testing"
)

// scopeFixture builds a scope over a real tree (the walk reads it) with recording add/remove.
type scopeFixture struct {
	s       *scope
	root    string
	added   map[string]int
	removed []string
}

func newScopeFixture(t *testing.T, o Options, dirs ...string) *scopeFixture {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		mkdirs(t, filepath.Join(root, d))
	}
	o.Roots = append([]string{root}, o.Roots...)
	s, err := newScope(o)
	if err != nil {
		t.Fatal(err)
	}
	f := &scopeFixture{s: s, root: s.roots[0].caller, added: map[string]int{}}
	s.add = func(d string) error { f.added[d]++; return nil }
	s.remove = func(d string) { f.removed = append(f.removed, d) }
	if err := s.walkAll(); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *scopeFixture) path(rel string) string { return filepath.Join(f.root, rel) }

func (f *scopeFixture) watchedRels() []string {
	var out []string
	for d := range f.added {
		rel, _ := filepath.Rel(f.s.roots[0].disk, d)
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func TestScopeWalkRespectsDepthAndSkip(t *testing.T) {
	skip := func(p string) bool { return filepath.Base(p) == "skip" }
	f := newScopeFixture(t, Options{MaxDepth: 2, SkipDir: skip}, "a/b/c", "skip/x", "a/skip/y", "z")
	got := f.watchedRels()
	want := []string{".", "a", "a/b", "z"}
	if len(got) != len(want) {
		t.Fatalf("watched %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("watched %v, want %v", got, want)
		}
	}
}

func TestScopeDeliveryFilter(t *testing.T) {
	skip := func(p string) bool { return filepath.Base(p) == "skip" }
	f := newScopeFixture(t, Options{MaxDepth: 1, SkipDir: skip}, "a", "skip", "a/deep")
	cases := []struct {
		path string
		dir  bool
		op   Op
		want bool
	}{
		{"f", false, Write, true},         // parent is the root
		{"a/f", false, Write, true},       // parent watched
		{"skip", true, Create, true},      // skipped dir's own event: parent watched
		{"skip/f", false, Write, false},   // parent skipped
		{"a/deep", true, Create, true},    // own event, parent a watched
		{"a/deep/f", false, Write, false}, // beyond depth
	}
	for _, c := range cases {
		_, ok := f.s.admit(rawEvent{path: f.path(c.path), op: c.op, dir: c.dir})
		if ok != c.want {
			t.Errorf("admit(%s) delivered=%v, want %v", c.path, ok, c.want)
		}
	}
}

// A verdict holds across events (a repo made inside a watched folder keeps the folder watched) and
// is forgotten, with its subtree and backend watches, by Remove or Rename.
func TestScopeVerdictPersistsAndForgets(t *testing.T) {
	gitSeen := false
	skip := func(p string) bool { return gitSeen && filepath.Base(p) == "proj" }
	f := newScopeFixture(t, Options{MaxDepth: Unlimited, SkipDir: skip}, "proj/sub")
	gitSeen = true
	if _, ok := f.s.admit(rawEvent{path: f.path("proj/sub/f"), op: Write}); !ok {
		t.Fatal("decided-watched dir lost its verdict when SkipDir changed its mind")
	}
	f.removed = nil
	f.s.admit(rawEvent{path: f.path("proj"), op: Rename, dir: true})
	if len(f.removed) != 2 {
		t.Fatalf("removed watches %v, want proj and proj/sub", f.removed)
	}
	// Decided again on next contact, now with the new SkipDir answer.
	if _, ok := f.s.admit(rawEvent{path: f.path("proj/sub/f"), op: Write}); ok {
		t.Fatal("forgotten verdict was not decided again")
	}
}

func TestScopeNewDirIsWalkedAndAdded(t *testing.T) {
	f := newScopeFixture(t, Options{MaxDepth: Unlimited})
	mkdirs(t, f.path("n/m"))
	f.s.admit(rawEvent{path: f.path("n"), op: Create, dir: true})
	if f.added[filepath.Join(f.s.roots[0].disk, "n")] != 1 || f.added[filepath.Join(f.s.roots[0].disk, "n", "m")] != 1 {
		t.Fatalf("added %v, want n and n/m once", f.added)
	}
}

func TestScopeRescanRewalks(t *testing.T) {
	f := newScopeFixture(t, Options{MaxDepth: Unlimited}, "a")
	mkdirs(t, f.path("b")) // appeared with no event seen
	f.s.rescan()
	if f.added[filepath.Join(f.s.roots[0].disk, "b")] != 1 {
		t.Fatalf("rescan missed b: %v", f.added)
	}
}

func TestScopeDeepestRootAndCaseFold(t *testing.T) {
	f := newScopeFixture(t, Options{MaxDepth: Unlimited}, "sub")
	inner := f.path("sub")
	s, err := newScope(Options{Roots: []string{f.root, inner}, MaxDepth: Unlimited})
	if err != nil {
		t.Fatal(err)
	}
	s.add, s.remove = func(string) error { return nil }, func(string) {}
	ev, ok := s.admit(rawEvent{path: filepath.Join(s.roots[0].disk, "sub", "f"), op: Write})
	if !ok || ev.Path != filepath.Join(inner, "f") {
		t.Fatalf("got %+v, want path under the inner root spelling", ev)
	}
	if got := s.topRoots(); len(got) != 1 {
		t.Fatalf("topRoots %v, want only the outer root", got)
	}
	s.fold = true
	upper := []byte(s.roots[0].disk)
	for i := len(upper) - 1; i >= 0; i-- { // flip the case of the last letter
		if c := upper[i]; c >= 'a' && c <= 'z' {
			upper[i] = c - 32
			break
		}
	}
	if _, ok := s.admit(rawEvent{path: filepath.Join(string(upper), "g"), op: Write}); !ok {
		t.Fatal("case-folded root prefix did not match")
	}
	s.fold = false
	if _, ok := s.admit(rawEvent{path: filepath.Join(string(upper), "g"), op: Write}); ok && string(upper) != s.roots[0].disk {
		t.Fatal("case-folded root matched with folding off")
	}
}
