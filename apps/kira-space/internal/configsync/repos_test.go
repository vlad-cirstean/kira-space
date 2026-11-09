package configsync

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
)

const sharedRepoYAML = `origin: https://example.com/acme/app.git
worktree:
  prepare_script: bun install
  prepare_timeout: 15m
integration_branches: [develop]
environments:
  - name: staging
    deployed_sha_script: curl -fsS https://staging.example.com/version
  - name: prod
    deployed_sha_run:
      script: deployed
review:
  base_candidates: [main, develop]
`

func TestRepoEntriesIgnoreLocalKeys(t *testing.T) {
	local := "path: /work/app\nnickname: App\ngraph:\n  page_size: 50\nworktree:\n  base_path: /work/trees\n  prepare_run:\n    script: prep\n"
	set, err := repoEntries([]byte(local))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(set.canon))
	for id := range set.canon {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if got := strings.Join(ids, ","); got != "worktree/prepare" {
		t.Fatalf("local ids = %q, want only worktree/prepare", got)
	}
	shared, err := repoEntries([]byte(sharedRepoYAML))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(shared.order, ","); got != "staging,prod" {
		t.Fatalf("environment order = %q", got)
	}
	for _, id := range []string{"worktree/prepare", "worktree/prepare_timeout", "integration_branches", "environments/staging", "environments/prod", "review/base_candidates"} {
		if _, ok := shared.canon[id]; !ok {
			t.Errorf("shared entry %s missing", id)
		}
	}
}

func TestApplyRepoEntries(t *testing.T) {
	local := `# my notes
path: /work/app
nickname: App
worktree:
  base_path: /work/trees
  prepare_run:
    script: prep
environments:
  - name: dev
    deployed_sha_script: echo dev
  - name: staging
    deployed_sha_script: echo old
graph:
  page_size: 50 # keep
`
	theirs, err := repoEntries([]byte(sharedRepoYAML))
	if err != nil {
		t.Fatal(err)
	}
	take := []string{"worktree/prepare", "worktree/prepare_timeout", "integration_branches", "environments/staging", "environments/prod", "review/base_candidates"}
	out, err := applyRepoEntries([]byte(local), theirs, take)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{"# my notes", "path: /work/app", "nickname: App", "base_path: /work/trees", "page_size: 50 # keep", "prepare_script: bun install", "prepare_timeout: 15m", "name: dev"} {
		if !strings.Contains(text, want) {
			t.Errorf("result lost %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "prepare_run") {
		t.Errorf("prepare_run must be unset when the shared unit sets prepare_script:\n%s", text)
	}
	if strings.Contains(text, "echo old") || !strings.Contains(text, "curl -fsS") {
		t.Errorf("staging not replaced:\n%s", text)
	}
	if strings.Index(text, "name: dev") > strings.Index(text, "name: staging") || strings.Index(text, "name: staging") > strings.Index(text, "name: prod") {
		t.Errorf("environment order must be local first, new ones after:\n%s", text)
	}

	gone, _ := repoEntries([]byte("origin: https://example.com/acme/app\n"))
	out, err = applyRepoEntries([]byte(local), gone, []string{"worktree/prepare", "environments/staging"})
	if err != nil {
		t.Fatal(err)
	}
	text = string(out)
	if strings.Contains(text, "prepare_run") || strings.Contains(text, "staging") || !strings.Contains(text, "base_path: /work/trees") || !strings.Contains(text, "name: dev") {
		t.Errorf("removal must drop only the taken entries:\n%s", text)
	}

	fresh, err := applyRepoEntries([]byte("path: /work/new\n"), theirs, take)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(fresh), "path: /work/new") {
		t.Errorf("new file must keep its seed first:\n%s", fresh)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestOriginClasses(t *testing.T) {
	path, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	s := New(Options{Home: t.TempDir(), Runner: gitclient.NewExecRunner(), GitPath: func(context.Context) (string, error) { return path, nil }})
	repo := func(urls ...string) string {
		dir := t.TempDir()
		gitIn(t, dir, "init", "-q")
		for i, u := range urls {
			if i == 0 {
				gitIn(t, dir, "remote", "add", "origin", u)
			} else {
				gitIn(t, dir, "config", "--add", "remote.origin.url", u)
			}
		}
		return dir
	}
	cases := []struct {
		name   string
		urls   []string
		keys   string
		single bool
	}{
		{"no origin", nil, "", false},
		{"scp url", []string{"git@Example.com:acme/app.git"}, "example.com/acme/app", true},
		{"same key twice", []string{"git@example.com:acme/app.git", "https://example.com/acme/app"}, "example.com/acme/app", true},
		{"two keys", []string{"git@example.com:acme/app.git", "https://other.example/acme/app"}, "example.com/acme/app,other.example/acme/app", false},
		{"local path", []string{"/srv/git/app.git"}, "", false},
		{"key next to local path", []string{"https://example.com/acme/app", "/srv/git/app.git"}, "example.com/acme/app", false},
	}
	for _, c := range cases {
		st, err := s.originsOf(context.Background(), repo(c.urls...))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := strings.Join(st.keys, ","); got != c.keys || st.single != c.single {
			t.Errorf("%s: keys %q single %v, want %q %v", c.name, got, st.single, c.keys, c.single)
		}
	}
}
