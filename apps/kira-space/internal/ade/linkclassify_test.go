package ade

import (
	"net/url"
	"testing"
)

func TestParseLinkText(t *testing.T) {
	cases := []struct {
		in    string
		want  parsedLink
		errCd string
	}{
		{in: "https://a.example/x", want: parsedLink{URL: "https://a.example/x"}},
		{in: "[Runbook](https://wiki.example/runbook)", want: parsedLink{Title: "Runbook", URL: "https://wiki.example/runbook"}},
		{in: "Design notes https://a.example/d", want: parsedLink{Title: "Design notes", URL: "https://a.example/d"}},
		{in: "https://a.example/d - Design notes", want: parsedLink{Title: "Design notes", URL: "https://a.example/d"}},
		{in: "Notes: <https://a.example/d>", want: parsedLink{Title: "Notes", URL: "https://a.example/d"}},
		{in: "just some words", want: parsedLink{Words: "just some words"}},
		{in: "PAY-9", want: parsedLink{Words: "PAY-9"}},
		{in: "ftp://a.example/x", want: parsedLink{Words: "ftp://a.example/x"}},
		{in: "https://a.example https://b.example", errCd: linkErrOneLink},
	}
	for _, c := range cases {
		got, errCd := parseLinkText(c.in)
		if got != c.want || errCd != c.errCd {
			t.Errorf("parseLinkText(%q) = %+v, %q; want %+v, %q", c.in, got, errCd, c.want, c.errCd)
		}
	}
}

func TestIsDocURL(t *testing.T) {
	cases := []struct {
		raw      string
		codeHost bool
		want     bool
	}{
		{"https://docs.google.com/document/d/abc/edit", false, true},
		{"https://acme.notion.so/page", false, true},
		{"https://acme.sharepoint.com/x", false, true},
		{"https://proj.readthedocs.io/en/latest", false, true},
		{"https://acme.gitbook.io/x", false, true},
		{"https://paper.dropbox.com/doc/x", false, true},
		{"https://acme.atlassian.net/wiki/spaces/A", false, true},
		{"https://acme.atlassian.net/browse/PAY-1", false, false},
		{"https://confluence.example.com/x", false, true},
		{"https://forge.test/acme/api/wiki/Home", true, true},
		{"https://forge.test/acme/api/wiki/Home", false, false},
		{"https://a.example/guide.PDF", false, true},
		{"https://a.example/README.md", false, true},
		{"https://wiki.example/runbook", false, false},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.raw)
		if got := isDocURL(u, c.codeHost); got != c.want {
			t.Errorf("isDocURL(%q, %v) = %v; want %v", c.raw, c.codeHost, got, c.want)
		}
	}
}
