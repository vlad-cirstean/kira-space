package gitremote

import "testing"

func TestKey(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"https://github.com/KiraTheCat/Kira-Studio.git", "github.com/kirathecat/kira-studio", true},
		{"http://github.com/acme/billing", "github.com/acme/billing", true},
		{"ssh://git@github.com:22/acme/billing.git", "github.com/acme/billing", true},
		{"git@github.com:acme/billing.git", "github.com/acme/billing", true},
		{"github.com:acme/billing", "github.com/acme/billing", true},
		{"git://example.org/acme/billing.git", "example.org/acme/billing", true},
		{"https://github.com/acme/billing/", "github.com/acme/billing", true},
		{"https://GitHub.COM/acme/billing", "github.com/acme/billing", true},
		{"https://gitlab.com/group/sub/deep/repo.git", "gitlab.com/group/sub/deep/repo", true},
		{"https://oauth2:ghp_secret@github.com/acme/billing.git", "github.com/acme/billing", true},
		{"https://user:pw@host.example:8443/a/b", "host.example/a/b", true},
		{"/srv/git/billing.git", "", false},
		{"./billing", "", false},
		{"file:///srv/git/billing.git", "", false},
		{"~/billing", "", false},
		{"https://github.com", "", false},
		{"https://github.com/", "", false},
		{"https://github.com/a/../b", "", false},
		{"not a url", "", false},
		{"", "", false},
		{"billing", "", false},
	}
	for _, c := range cases {
		got, ok := Key(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("Key(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
