package github

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

var linkNamePattern = regexp.MustCompile(`^[\w.-]+$`)

// pullTabs are the tabs GitHub serves under a pull request; true when the tab takes sub-paths
// (`files/a..b`, `commits/<sha>`, `changes/<sha>`).
var pullTabs = map[string]bool{"files": true, "commits": true, "checks": false, "changes": true}

// ParseBranchLink reads a pull, tree or issue link on a claimed host. Query, fragment and one trailing
// slash are ignored. A pull link may name a tab from pullTabs, with sub-paths where the tab takes them.
func (p *Provider) ParseBranchLink(ctx context.Context, raw string) (githost.BranchLink, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || !p.Claims(ctx, u.Host) {
		return githost.BranchLink{}, false
	}
	var segs []string
	for _, s := range strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/") {
		d, err := url.PathUnescape(s)
		if err != nil {
			return githost.BranchLink{}, false
		}
		segs = append(segs, d)
	}
	if n := len(segs); n > 0 && segs[n-1] == "" {
		segs = segs[:n-1]
	}
	if len(segs) < 4 {
		return githost.BranchLink{}, false
	}
	owner, name := segs[0], strings.TrimSuffix(segs[1], ".git")
	if !linkNamePattern.MatchString(owner) || !linkNamePattern.MatchString(name) {
		return githost.BranchLink{}, false
	}
	link := githost.BranchLink{Repo: githost.Repo{Host: u.Host, Owner: owner, Name: name}}
	rest := segs[3:]
	switch segs[2] {
	case "pull", "issues":
		n, err := strconv.Atoi(rest[0])
		if err != nil || n <= 0 || !digitsOnly(rest[0]) {
			return githost.BranchLink{}, false
		}
		if segs[2] == "issues" {
			if len(rest) != 1 {
				return githost.BranchLink{}, false
			}
			link.Issue = true
		} else if !validPullTail(rest[1:]) {
			return githost.BranchLink{}, false
		}
		link.Number = n
	case "tree":
		for _, s := range rest {
			if s == "" {
				return githost.BranchLink{}, false
			}
		}
		link.TreePath = strings.Join(rest, "/")
	default:
		return githost.BranchLink{}, false
	}
	return link, true
}

// validPullTail accepts nothing, or a known tab followed by non-empty sub-paths only where it takes them.
func validPullTail(tail []string) bool {
	if len(tail) == 0 {
		return true
	}
	deeper, known := pullTabs[tail[0]]
	if !known || (len(tail) > 1 && !deeper) {
		return false
	}
	for _, s := range tail[1:] {
		if s == "" {
			return false
		}
	}
	return true
}

func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

type rawPullHead struct {
	rawPull
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
}

// PullHead reads one pull request once: `repos/{owner}/{repo}/pulls/{n}`.
func (c *Client) PullHead(ctx context.Context, repo Repo, number int) (PR, string, Status) {
	var raw rawPullHead
	status := c.get(ctx, repo, fmt.Sprintf("repos/%s/pulls/%d", repo.Path(), number), &raw)
	if !status.OK() {
		return PR{}, "", status
	}
	var author string
	if raw.User != nil {
		author = raw.User.Login
	}
	return raw.toPR(), author, status
}

func (p *Provider) ChangeRequestHead(ctx context.Context, r githost.Repo, number int) (githost.ChangeRequestHead, githost.Status) {
	pr, author, s := p.client.PullHead(ctx, Repo(r), number)
	if !s.OK() {
		return githost.ChangeRequestHead{}, toStatus(s)
	}
	return githost.ChangeRequestHead{ChangeRequest: toChangeRequests([]PR{pr})[0], Author: author}, toStatus(s)
}

func (p *Provider) ChangeRequestRef(number int) string {
	return fmt.Sprintf("refs/pull/%d/head", number)
}
