package ade

import (
	"context"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// linkclassify.go turns one line of user text into a link kind (Details tab). Recognition rules live
// here and nowhere else; the frontend only sends the text.

// Refusal causes parseLinkText and classifyLink report; links.go words them.
const (
	linkErrOneLink = "oneLink"
	linkErrNotLink = "notLink"
	// linkErrAlreadyUsed is EditLink's refusal when another row holds the new URL.
	linkErrAlreadyUsed = "alreadyLinked"
)

var markdownLinkRe = regexp.MustCompile(`^\[([^\]]*)\]\(([^()\s]+)\)$`)

var jiraKeyInURLRe = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

// parsedLink is a line split into title and link. URL is empty for a words-only line; then Words
// holds the whole text.
type parsedLink struct {
	Title, URL, Words string
}

func httpURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, false
	}
	return u, true
}

// parseLinkText reads `[title](url)`, `title url`, `url title` or a bare url. Exactly one http(s)
// link may appear; errCode is linkErrOneLink for more, "" otherwise.
func parseLinkText(text string) (p parsedLink, errCode string) {
	text = strings.TrimSpace(text)
	if m := markdownLinkRe.FindStringSubmatch(text); m != nil {
		if _, ok := httpURL(m[2]); ok {
			return parsedLink{Title: strings.TrimSpace(m[1]), URL: m[2]}, ""
		}
	}
	var rest []string
	for _, tok := range strings.Fields(text) {
		bare := strings.TrimSuffix(strings.TrimPrefix(tok, "<"), ">")
		if _, ok := httpURL(bare); ok {
			if p.URL != "" {
				return parsedLink{}, linkErrOneLink
			}
			p.URL = bare
			continue
		}
		rest = append(rest, tok)
	}
	if p.URL == "" {
		return parsedLink{Words: text}, ""
	}
	p.Title = strings.Trim(strings.Join(rest, " "), "-–—:| <>")
	return p, ""
}

// docHostSuffixes and docHosts name document sites; docExts name document files.
var (
	docHosts        = []string{"docs.google.com", "paper.dropbox.com"}
	docHostSuffixes = []string{".notion.so", ".notion.site", ".sharepoint.com", ".readthedocs.io", ".gitbook.io"}
	docExts         = []string{".md", ".mdx", ".pdf", ".doc", ".docx", ".rst", ".adoc", ".txt"}
)

// isDocURL applies the doc table. codeHost reports whether a code-host provider claims u's host.
func isDocURL(u *url.URL, codeHost bool) bool {
	host := strings.ToLower(u.Hostname())
	for _, h := range docHosts {
		if host == h {
			return true
		}
	}
	for _, s := range docHostSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	if strings.HasSuffix(host, ".atlassian.net") && strings.HasPrefix(u.Path, "/wiki/") {
		return true
	}
	if strings.HasPrefix(host, "confluence.") {
		return true
	}
	if codeHost {
		for _, seg := range strings.Split(u.Path, "/") {
			if seg == "wiki" {
				return true
			}
		}
	}
	ext := strings.ToLower(path.Ext(u.Path))
	for _, e := range docExts {
		if ext == e {
			return true
		}
	}
	return false
}

// Where a classified link files.
const (
	linkOriginNone    = ""
	linkOriginTracker = "tracker"
	linkOriginHost    = "host"
)

// linkClass is the recognised kind of a line.
type linkClass struct {
	Kind   string
	Ref    string
	Origin string
	URL    string
	Title  string // the given title, else "" (the caller names the row)
}

// classifyLink applies the recognition rules, first match wins. It returns linkErrNotLink for words
// that are no issue key.
func (b *TaskBoard) classifyLink(ctx context.Context, task model.AdeTask, p parsedLink) (linkClass, string) {
	if p.URL == "" {
		if _, ref, ok := b.deps.Trackers.ParseRef(p.Words); ok {
			return linkClass{Kind: model.AdeArtifactIssue, Ref: ref.Key, Origin: linkOriginTracker}, ""
		}
		return linkClass{}, linkErrNotLink
	}
	c := linkClass{Kind: model.AdeArtifactLink, URL: p.URL, Title: p.Title}
	u, _ := httpURL(p.URL)
	if _, ref, ok := b.deps.Trackers.ParseRef(p.URL); ok {
		c.Kind, c.Ref, c.Origin = model.AdeArtifactIssue, ref.Key, linkOriginTracker
		return c, ""
	}
	if task.JiraURL != "" {
		if ju, ok := httpURL(task.JiraURL); ok && ju.Host == u.Host {
			if key := jiraKeyInURLRe.FindString(u.Path + "?" + u.RawQuery); key != "" {
				c.Kind, c.Ref, c.Origin = model.AdeArtifactIssue, key, linkOriginTracker
				return c, ""
			}
		}
	}
	if l := b.hostLink(p.URL); l != nil {
		c.Kind, c.Ref, c.Origin = l.Kind, l.Ref, linkOriginHost
		return c, ""
	}
	if reg := b.hostRegistry(); reg != nil {
		if _, _, bl, ok := reg.ParseBranchLink(ctx, p.URL); ok && bl.Number == 0 && !bl.Issue && bl.TreePath != "" {
			c.Kind, c.Ref = artifactBranch, bl.TreePath
			return c, ""
		}
	}
	codeHost := false
	if reg := b.hostRegistry(); reg != nil {
		for _, prov := range reg.All() {
			if prov.Claims(ctx, u.Hostname()) {
				codeHost = true
				break
			}
		}
	}
	if isDocURL(u, codeHost) {
		c.Kind = model.AdeArtifactDoc
	}
	return c, ""
}
