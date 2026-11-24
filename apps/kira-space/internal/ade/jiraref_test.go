package ade

import "testing"

func TestParseJiraRef(t *testing.T) {
	cases := []struct {
		in         string
		key, site  string
		wantParsed bool
	}{
		{"KIRA-12", "KIRA-12", "", true},
		{" KIRA-12 ", "KIRA-12", "", true},
		{"https://acme.atlassian.net/browse/KIRA-12", "KIRA-12", "acme.atlassian.net", true},
		{"https://acme.atlassian.net/browse/KIRA-12?focusedCommentId=5", "KIRA-12", "acme.atlassian.net", true},
		{"https://Acme.atlassian.net/jira/software/projects/KIRA/boards/1/issues/KIRA-7", "KIRA-7", "acme.atlassian.net", true},
		{"https://acme.atlassian.net/jira/software/projects/KIRA/boards/1?selectedIssue=KIRA-9", "KIRA-9", "acme.atlassian.net", true},
		{"kira-12", "", "", false},
		{"KIRA12", "", "", false},
		{"https://github.com/o/r/issues/12", "", "", false},
		{"https://acme.atlassian.net/browse/kira-12", "", "", false},
		{"ftp://acme.atlassian.net/browse/KIRA-12", "", "", false},
		{"hello KIRA-12", "", "", false},
	}
	for _, c := range cases {
		ref, ok := ParseJiraRef(c.in)
		if ok != c.wantParsed || ref.Key != c.key || ref.Site != c.site {
			t.Errorf("ParseJiraRef(%q) = %+v, %v; want key %q site %q parsed %v", c.in, ref, ok, c.key, c.site, c.wantParsed)
		}
	}
}
