package azure

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

var _ githost.ChangeRequestLinks = (*Provider)(nil)

// repoPath is "https://dev.azure.com/{org}/{project}/_git/{repo}".
func repoPath(r githost.Repo) string {
	org, project, ok := split(r)
	if !ok {
		return "https://" + Host
	}
	return orgURL(org) + "/" + url.PathEscape(project) + "/_git/" + url.PathEscape(r.Name)
}

func (p *Provider) RepoURL(r githost.Repo) string { return repoPath(r) }

func (p *Provider) ChangeRequestURL(r githost.Repo, number int) string {
	return repoPath(r) + "/pullrequest/" + strconv.Itoa(number)
}

func (p *Provider) BranchURL(r githost.Repo, branch string) string {
	return repoPath(r) + "?version=GB" + url.QueryEscape(branch)
}

var pullRequestPath = regexp.MustCompile(`^/[^/]+/[^/]+/_git/[^/]+/pullrequest/[0-9]+$`)

func (p *Provider) IsChangeRequestURL(_ context.Context, u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Host == Host && pullRequestPath.MatchString(u.EscapedPath())
}

// webLink is a parsed Azure DevOps web URL: the repo (Name "" when the link names none), and what
// follows the marker segment.
type webLink struct {
	repo   githost.Repo
	marker string   // "_git" | "_workitems"
	rest   []string // after the repo name for _git, after "_workitems" for work items
	query  url.Values
}

// parseWeb reads https://dev.azure.com/{org}/{project}/{_git/{repo}|_workitems}/... and the
// {org}.visualstudio.com form, which may carry a collection segment before the project.
func parseWeb(raw string) (webLink, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return webLink{}, false
	}
	segs, ok := unescapeSegments(u.EscapedPath())
	if !ok {
		return webLink{}, false
	}
	var org string
	host := strings.ToLower(u.Hostname())
	if host == Host && u.Port() == "" {
		org, segs = segs[0], segs[1:]
	} else if o, legacy := legacyOrg(host); legacy && u.Port() == "" {
		org = o
		if len(segs) > 2 && (segs[2] == "_git" || segs[2] == "_workitems") {
			segs = segs[1:] // drop the collection
		}
	} else {
		return webLink{}, false
	}
	if len(segs) < 2 || segs[1] != "_git" && segs[1] != "_workitems" {
		return webLink{}, false
	}
	w := webLink{repo: githost.Repo{Host: Host, Owner: org + "/" + segs[0]}, marker: segs[1], query: u.Query()}
	if w.marker == "_git" {
		if len(segs) < 3 {
			return webLink{}, false
		}
		w.repo.Name, w.rest = strings.TrimSuffix(segs[2], ".git"), segs[3:]
	} else {
		w.rest = segs[2:]
	}
	return w, true
}

// pullRequestNumber is n in .../pullrequest/{n}[/].
func (w webLink) pullRequestNumber() (int, bool) {
	if w.marker != "_git" || len(w.rest) != 2 || w.rest[0] != "pullrequest" {
		return 0, false
	}
	return positive(w.rest[1])
}

// workItemNumber is n in .../_workitems/edit/{n}.
func (w webLink) workItemNumber() (int, bool) {
	if w.marker != "_workitems" || len(w.rest) != 2 || w.rest[0] != "edit" {
		return 0, false
	}
	return positive(w.rest[1])
}

func positive(s string) (int, bool) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0
}

func (p *Provider) ParseLink(raw string) (githost.Link, bool) {
	w, ok := parseWeb(raw)
	if !ok {
		return githost.Link{}, false
	}
	if n, ok := w.pullRequestNumber(); ok && w.repo.Name != "" {
		return githost.Link{Provider: ProviderID, Kind: githost.LinkChangeRequest, Ref: w.repo.Name + "!" + strconv.Itoa(n)}, true
	}
	if n, ok := w.workItemNumber(); ok {
		return githost.Link{Provider: ProviderID, Kind: githost.LinkIssue, Ref: "#" + strconv.Itoa(n)}, true
	}
	return githost.Link{}, false
}

// ParseBranchLink reads a pull request, branch (`?version=GB<branch>`) or work item link. The path
// query parameter names a file, not part of the branch.
func (p *Provider) ParseBranchLink(_ context.Context, raw string) (githost.BranchLink, bool) {
	w, ok := parseWeb(raw)
	if !ok {
		return githost.BranchLink{}, false
	}
	if n, ok := w.pullRequestNumber(); ok {
		return githost.BranchLink{Repo: w.repo, Number: n}, true
	}
	if n, ok := w.workItemNumber(); ok {
		return githost.BranchLink{Repo: w.repo, Number: n, Issue: true}, true
	}
	if w.marker == "_git" && len(w.rest) == 0 {
		if branch, ok := strings.CutPrefix(w.query.Get("version"), "GB"); ok && branch != "" {
			return githost.BranchLink{Repo: w.repo, TreePath: branch}, true
		}
	}
	return githost.BranchLink{}, false
}

func (p *Provider) ChangeRequestHead(ctx context.Context, r githost.Repo, number int) (githost.ChangeRequestHead, githost.Status) {
	return p.client.head(ctx, r, number)
}

// ChangeRequestRef is the merge preview ref: Azure serves no head ref for a pull request.
func (p *Provider) ChangeRequestRef(number int) string {
	return "refs/pull/" + strconv.Itoa(number) + "/merge"
}

// ParseChangeRequestLink reads a pull request link; work item and branch links miss.
func (p *Provider) ParseChangeRequestLink(ctx context.Context, raw string) (githost.Repo, int, bool) {
	l, ok := p.ParseBranchLink(ctx, raw)
	if !ok || l.Number <= 0 || l.Issue {
		return githost.Repo{}, 0, false
	}
	return l.Repo, l.Number, true
}
