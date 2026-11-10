package github

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

func TestParseBranchLink(t *testing.T) {
	p := &Provider{client: NewClient(NewDiscovery(fakeLocator{path: "/gh", found: true}, okRunner(), &fakeClock{}), okRunner())}
	p.client.discovery.Status(context.Background(), "ghes.example.com")
	repo := githost.Repo{Host: "github.com", Owner: "acme", Name: "api"}
	cases := []struct {
		name string
		raw  string
		ok   bool
		want githost.BranchLink
	}{
		{"pull", "https://github.com/acme/api/pull/12", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull files", "https://github.com/acme/api/pull/12/files", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull query fragment", "https://github.com/acme/api/pull/12/commits?x=1#a", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull trailing slash", "https://github.com/acme/api/pull/12/", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull changes", "https://github.com/acme/api/pull/12/changes", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull changes sha", "https://github.com/acme/api/pull/12/changes/abc123", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull files range", "https://github.com/acme/api/pull/12/files/abc..def", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull files trailing slash", "https://github.com/acme/api/pull/12/files/", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull commits sha", "https://github.com/acme/api/pull/12/commits/abc123", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"pull checks deeper", "https://github.com/acme/api/pull/12/checks/9", false, githost.BranchLink{}},
		{"pull unknown suffix", "https://github.com/acme/api/pull/12/foo", false, githost.BranchLink{}},
		{"tree nested", "https://github.com/acme/api/tree/feat/x/src", true, githost.BranchLink{Repo: repo, TreePath: "feat/x/src"}},
		{"tree escapes", "https://github.com/acme/api/tree/feat%2Fx", true, githost.BranchLink{Repo: repo, TreePath: "feat/x"}},
		{"tree no ref", "https://github.com/acme/api/tree/", false, githost.BranchLink{}},
		{"issue trailing slash", "https://github.com/acme/api/issues/5/", true, githost.BranchLink{Repo: repo, Number: 5, Issue: true}},
		{"tree trailing slash", "https://github.com/acme/api/tree/main/", true, githost.BranchLink{Repo: repo, TreePath: "main"}},
		{"issue", "https://github.com/acme/api/issues/5", true, githost.BranchLink{Repo: repo, Number: 5, Issue: true}},
		{"http", "http://github.com/acme/api/pull/12", false, githost.BranchLink{}},
		{"unclaimed host", "https://example.com/acme/api/pull/12", false, githost.BranchLink{}},
		{"ghes host", "https://ghes.example.com/acme/api/pull/3", true, githost.BranchLink{Repo: githost.Repo{Host: "ghes.example.com", Owner: "acme", Name: "api"}, Number: 3}},
		{"git suffix", "https://github.com/acme/api.git/pull/12", true, githost.BranchLink{Repo: repo, Number: 12}},
		{"extra owner segment", "https://github.com/a/b/c/pull/12", false, githost.BranchLink{}},
		{"zero number", "https://github.com/acme/api/pull/0", false, githost.BranchLink{}},
	}
	for _, c := range cases {
		got, ok := p.ParseBranchLink(context.Background(), c.raw)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
