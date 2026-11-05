package github

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// PR is one GitHub pull request, trimmed to exactly the fields D4 names as read: number, title,
// html_url, state, draft, merged_at, head.ref, head.sha, base.ref, updated_at — plus
// head.repo.owner.login (F11, P108 Part 16 review).
type PR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	State   string `json:"state"` // "open" | "draft" | "merged" | "closed" — derived, see deriveState.
	HeadRef string `json:"headRef"`
	HeadSha string `json:"headSha"`
	// HeadRepoOwner is the owner of the repo HeadRef lives in — the SAME repo as this
	// PR's own base for an ordinary PR, but a DIFFERENT owner for one opened from a fork:
	// matching a snapshot entry by HeadRef alone, with no owner check, can badge a fork's PR from
	// a commonly-named branch ("main", "master", "patch-1") onto an unrelated local branch of the
	// same name. Empty when GitHub reports head.repo as null (the source repo/fork was deleted) —
	// callers must then treat it as "cannot confirm same-repo," not as a same-owner match.
	HeadRepoOwner string `json:"headRepoOwner"`
	BaseRef       string `json:"baseRef"`
	UpdatedAt     int64  `json:"updatedAt"` // Unix milliseconds.
	// NodeID, Draft, ReviewDecision and ChecksState are read by the sync queries only.
	NodeID         string `json:"nodeId,omitempty"`
	Draft          bool   `json:"draft,omitempty"`
	ReviewDecision string `json:"reviewDecision,omitempty"`
	ChecksState    string `json:"checksState,omitempty"`
	// CommentCount and Assignees are read by the sync queries only.
	CommentCount *int     `json:"commentCount,omitempty"`
	Assignees    []string `json:"assignees,omitempty"`
}

// rawPull is the GitHub REST response shape, decoded exactly as the API sends it — never exported,
// since PR (above) is this package's only public shape (D4's own "fields read" list, nothing more).
type rawPull struct {
	Number int     `json:"number"`
	Title  string  `json:"title"`
	URL    string  `json:"html_url"`
	State  string  `json:"state"`
	Draft  bool    `json:"draft"`
	Merged *string `json:"merged_at"`
	Head   struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
		// Repo is a pointer: GitHub reports head.repo as null once the source repo (a fork or
		// this repo itself) has been deleted (F11) — toPR must not assume it is always present.
		Repo *struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	UpdatedAt string `json:"updated_at"`
}

// deriveState is the derivation, verbatim — GitHub's own `state` field is only ever "open" or
// "closed"; "merged" and "draft" are never returned as `state` itself and must be derived from
// merged_at/draft:
//
//	merged_at != null            -> "merged"
//	state == "open" && draft     -> "draft"
//	state == "open"              -> "open"
//	otherwise                    -> "closed"
func deriveState(state string, draft bool, mergedAt *string) string {
	switch {
	case mergedAt != nil && *mergedAt != "":
		return "merged"
	case state == "open" && draft:
		return "draft"
	case state == "open":
		return "open"
	default:
		return "closed"
	}
}

func (p rawPull) toPR() PR {
	var updatedAtMillis int64
	if t, err := time.Parse(time.RFC3339, p.UpdatedAt); err == nil {
		updatedAtMillis = t.UnixMilli()
	}
	var headRepoOwner string
	if p.Head.Repo != nil {
		headRepoOwner = p.Head.Repo.Owner.Login
	}
	return PR{
		Number:        p.Number,
		Title:         p.Title,
		URL:           p.URL,
		State:         deriveState(p.State, p.Draft, p.Merged),
		HeadRef:       p.Head.Ref,
		HeadSha:       p.Head.Sha,
		HeadRepoOwner: headRepoOwner,
		BaseRef:       p.Base.Ref,
		UpdatedAt:     updatedAtMillis,
		Draft:         p.Draft && p.State == "open" && (p.Merged == nil || *p.Merged == ""),
	}
}

func toPRs(raw []rawPull) []PR {
	out := make([]PR, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.toPR())
	}
	return out
}

// PullsForCommit is D4(a): `repos/{owner}/{repo}/commits/{sha}/pulls?per_page=10` — the one lookup
// that can answer "is THIS commit part of a pull request", including one opened from a fork (§9's
// own noted asymmetry with PullsForBranch, which cannot).
func (c *Client) PullsForCommit(ctx context.Context, repo Repo, sha string) ([]PR, Status) {
	var raw []rawPull
	// sha is client-supplied (only checked non-empty at gitrpc/gh.go) and reached this path
	// unescaped — `gh api` parses `path` as a URL before requesting it, so a sha containing
	// "?"/"#"/"../" reshapes the request path/query rather than 404ing. url.PathEscape keeps this
	// a single path segment regardless of content.
	path := fmt.Sprintf("repos/%s/commits/%s/pulls?per_page=10", repo.Path(), url.PathEscape(sha))
	status := c.get(ctx, repo, path, &raw)
	if !status.OK() {
		return nil, status
	}
	return toPRs(raw), status
}

// PullsForBranch is D4(b), upstream's own query verbatim:
// `repos/{owner}/{repo}/pulls?head={owner}:{branch}&state=all&sort=updated&direction=desc&per_page=5`
// — `state=all` is what lets this report a PR that has since closed, something no open-PR-only
// query could ever tell the reaper (F1's own "why two lookups" reasoning).
func (c *Client) PullsForBranch(ctx context.Context, repo Repo, branch string) ([]PR, Status) {
	var raw []rawPull
	// G30 round-1 architecture/security review, finding #6: branch is a real branch name — "+",
	// "%", "&", "#" are all legal in a ref — and reached this query value unescaped. An ordinary
	// branch like "feature/a+b" silently corrupted the query (decoded server-side as "feature/a
	// b"), so branch.resolveChangeRequest reported "no PR" for a branch that had one. url.QueryEscape both
	// halves of the "owner:branch" value; the literal ":" GitHub's own head= syntax needs stays
	// outside either escaped piece.
	path := fmt.Sprintf("repos/%s/pulls?head=%s:%s&state=all&sort=updated&direction=desc&per_page=5",
		repo.Path(), url.QueryEscape(repo.Owner), url.QueryEscape(branch))
	status := c.get(ctx, repo, path, &raw)
	if !status.OK() {
		return nil, status
	}
	return toPRs(raw), status
}

// maxSnapshotPages bounds D6's own repo-wide open-PR snapshot at 300 PRs (3 pages of 100) — D7's
// own rate-limit arithmetic assumes exactly this cap.
const maxSnapshotPages = 3

// OpenPulls is the repo-wide open-PR snapshot, 1-3 calls
// (`repos/{owner}/{repo}/pulls?state=open&sort=updated&direction=desc&per_page=100&page={1..3}`),
// stopping early the moment a page returns fewer than per_page elements — no `--paginate`
// (considered and rejected, a version-dependent output shape) and no `--jq` (a second expression
// language this package would then have to keep in step with the Go-side field list above).
func (c *Client) OpenPulls(ctx context.Context, repo Repo) ([]PR, Status) {
	const perPage = 100
	var all []PR
	for page := 1; page <= maxSnapshotPages; page++ {
		var raw []rawPull
		path := fmt.Sprintf("repos/%s/pulls?state=open&sort=updated&direction=desc&per_page=%d&page=%d",
			repo.Path(), perPage, page)
		status := c.get(ctx, repo, path, &raw)
		if !status.OK() {
			if page == 1 {
				return nil, status
			}
			// A later page failing after at least one page already succeeded is treated as "the
			// snapshot is what we got" rather than discarding everything already fetched — the
			// fail-open posture (a GitHub hiccup mid-snapshot never blocks git, and the caller
			// still gets a usable, if incomplete, snapshot).
			return all, Status{Kind: KindOK, Host: repo.Host}
		}
		all = append(all, toPRs(raw)...)
		if len(raw) < perPage {
			break
		}
	}
	return all, Status{Kind: KindOK, Host: repo.Host}
}
