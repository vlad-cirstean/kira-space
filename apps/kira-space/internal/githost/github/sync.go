package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// syncPageSize is the GraphQL page size of a Page walk; GitHub allows at most 100.
const syncPageSize = 100

// checksBatch is how many node ids one Checks call reads.
const checksBatch = 50

const syncPRFields = `id number title url state isDraft reviewDecision updatedAt headRefName headRefOid baseRefName headRepositoryOwner{login} comments{totalCount} reviews{totalCount} assignees(first:10){nodes{login}} commits(last:1){nodes{commit{statusCheckRollup{state}}}}`

const syncPageQuery = `query($owner:String!,$name:String!,$first:Int!,$after:String){repository(owner:$owner,name:$name){pullRequests(states:[OPEN,CLOSED,MERGED],orderBy:{field:UPDATED_AT,direction:DESC},first:$first,after:$after){pageInfo{endCursor hasNextPage} nodes{` + syncPRFields + `}}} rateLimit{remaining resetAt cost}}`

const syncChecksQuery = `query($ids:[ID!]!){nodes(ids:$ids){... on PullRequest{` + syncPRFields + `}} rateLimit{remaining resetAt cost}}`

type gqlPR struct {
	ID             string `json:"id"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	UpdatedAt      string `json:"updatedAt"`
	HeadRefName    string `json:"headRefName"`
	HeadRefOid     string `json:"headRefOid"`
	BaseRefName    string `json:"baseRefName"`
	HeadOwner      *struct {
		Login string `json:"login"`
	} `json:"headRepositoryOwner"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	Reviews struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviews"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type gqlRateLimit struct {
	Remaining int    `json:"remaining"`
	ResetAt   string `json:"resetAt"`
}

// toPR maps GraphQL's OPEN/CLOSED/MERGED and isDraft onto the REST-derived state vocabulary.
func (g gqlPR) toPR() PR {
	var updated int64
	if t, err := time.Parse(time.RFC3339, g.UpdatedAt); err == nil {
		updated = t.UnixMilli()
	}
	pr := PR{
		Number: g.Number, Title: g.Title, URL: g.URL, HeadRef: g.HeadRefName, HeadSha: g.HeadRefOid,
		BaseRef: g.BaseRefName, UpdatedAt: updated, NodeID: g.ID, ReviewDecision: g.ReviewDecision,
	}
	if g.HeadOwner != nil {
		pr.HeadRepoOwner = g.HeadOwner.Login
	}
	count := g.Comments.TotalCount + g.Reviews.TotalCount
	pr.CommentCount = &count
	pr.Assignees = make([]string, 0, len(g.Assignees.Nodes))
	for _, a := range g.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, a.Login)
	}
	switch g.State {
	case "MERGED":
		pr.State = "merged"
	case "OPEN":
		pr.Draft = g.IsDraft
		pr.State = "open"
		if g.IsDraft {
			pr.State = "draft"
		}
		if n := g.Commits.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
			pr.ChecksState = n[0].Commit.StatusCheckRollup.State
		}
	default:
		pr.State = "closed"
	}
	return pr
}

func (r gqlRateLimit) parse() (remaining int, resetAt time.Time) {
	t, _ := time.Parse(time.RFC3339, r.ResetAt)
	return r.Remaining, t
}

// SyncPage is one page of a Page walk.
type SyncPage struct {
	Items            []PR
	EndCursor        string
	HasNext          bool
	Remaining        int
	RateLimitResetAt time.Time
}

type syncPageData struct {
	Repository *struct {
		PullRequests struct {
			PageInfo struct {
				EndCursor   string `json:"endCursor"`
				HasNextPage bool   `json:"hasNextPage"`
			} `json:"pageInfo"`
			Nodes []*gqlPR `json:"nodes"`
		} `json:"pullRequests"`
	} `json:"repository"`
	RateLimit gqlRateLimit `json:"rateLimit"`
}

// gone is the Status of a repository GitHub does not show to the viewer.
func gone(host string) Status {
	return Status{Kind: KindForbidden, Host: host, Reason: "not found, or you cannot see it", Gone: true}
}

// PullsPage reads one page of every pull request, newest update first, closed and merged included.
func (c *Client) PullsPage(ctx context.Context, repo Repo, cursor string, first int) (SyncPage, Status) {
	if first <= 0 || first > syncPageSize {
		first = syncPageSize
	}
	vars := []GraphQLVar{
		{Name: "owner", Str: repo.Owner}, {Name: "name", Str: repo.Name}, {Name: "first", Int: first, IsInt: true},
	}
	if cursor != "" {
		vars = append(vars, GraphQLVar{Name: "after", Str: cursor})
	}
	var data syncPageData
	status, _ := c.graphql(ctx, repo, syncPageQuery, vars, &data)
	if !status.OK() {
		return SyncPage{}, withRateLimitReset(status, data.RateLimit)
	}
	if data.Repository == nil {
		return SyncPage{}, gone(repo.Host)
	}
	out := SyncPage{
		EndCursor: data.Repository.PullRequests.PageInfo.EndCursor,
		HasNext:   data.Repository.PullRequests.PageInfo.HasNextPage,
	}
	out.Remaining, out.RateLimitResetAt = data.RateLimit.parse()
	for _, n := range data.Repository.PullRequests.Nodes {
		if n != nil {
			out.Items = append(out.Items, n.toPR())
		}
	}
	return out, status
}

// withRateLimitReset carries GraphQL's reset time onto a rate limited status.
func withRateLimitReset(s Status, rl gqlRateLimit) Status {
	if isRateLimited(s) {
		_, s.ResetAt = rl.parse()
	}
	return s
}

// PullsByNodeID reads the current state of the pull requests with ids, in batches. A pull request
// GitHub no longer shows is absent from the result.
func (c *Client) PullsByNodeID(ctx context.Context, repo Repo, ids []string) ([]PR, Status) {
	var out []PR
	status := Status{Kind: KindOK, Host: repo.Host}
	for start := 0; start < len(ids); start += checksBatch {
		chunk := ids[start:min(start+checksBatch, len(ids))]
		vars := make([]GraphQLVar, 0, len(chunk))
		for _, id := range chunk {
			vars = append(vars, GraphQLVar{Name: "ids[]", Str: id})
		}
		var data struct {
			Nodes     []*gqlPR     `json:"nodes"`
			RateLimit gqlRateLimit `json:"rateLimit"`
		}
		s, _ := c.graphql(ctx, repo, syncChecksQuery, vars, &data)
		if !s.OK() {
			return out, withRateLimitReset(s, data.RateLimit)
		}
		for _, n := range data.Nodes {
			if n != nil && n.ID != "" {
				out = append(out, n.toPR())
			}
		}
	}
	return out, status
}

// Probe asks whether the repository's newest-updated pull request listing changed since etag. A
// conditional request answered 304 costs no rate limit.
func (c *Client) Probe(ctx context.Context, repo Repo, etag string) (changed bool, newETag string, s Status) {
	authStatus := c.discovery.Status(ctx, repo.Host)
	if !authStatus.OK() {
		return false, "", authStatus
	}
	path := fmt.Sprintf("repos/%s/pulls?state=all&sort=updated&direction=desc&per_page=1", repo.Path())
	res, runErr := c.runner.Run(ctx, authStatus.Path, Spec{Args: probeArgv(repo.Host, path, etag), Timeout: apiTimeout})
	if runErr != nil || ctx.Err() != nil {
		return false, "", classify(repo.Host, res, runErr, ctx.Err())
	}
	return parseProbe(repo.Host, res, etag)
}

// probeArgv is commonArgv with the response headers included and the validator sent.
func probeArgv(host, path, etag string) []string {
	argv := commonArgv(host, path)
	extra := []string{"-i"}
	if etag != "" {
		extra = append(extra, "-H", "If-None-Match: "+etag)
	}
	return append(argv[:len(argv)-1], append(extra, path)...)
}

// parseProbe reads `gh api -i`'s header block. gh exits non-zero on a 304 (and on any non-2xx), so
// the HTTP status line, not the exit code, decides.
func parseProbe(host string, res Result, sentETag string) (bool, string, Status) {
	code, headers, ok := parseHeaderBlock(res.Stdout)
	ok200 := Status{Kind: KindOK, Host: host}
	switch {
	case ok && code == 304:
		return false, sentETag, ok200
	case ok && code >= 200 && code < 300:
		return true, headers["etag"], ok200
	case ok && (code == 403 || code == 429) && rateLimitHeaders(headers):
		s := Status{Kind: KindForbidden, Host: host, Reason: graphqlRateLimited}
		s.ResetAt = headerResetAt(headers)
		return false, "", s
	}
	return false, "", classify(host, res, nil, nil)
}

// parseHeaderBlock splits a `gh api -i` stdout into the HTTP status code and the lower-cased
// headers; ok is false when stdout does not begin with a status line.
func parseHeaderBlock(stdout []byte) (code int, headers map[string]string, ok bool) {
	text := strings.ReplaceAll(string(stdout), "\r\n", "\n")
	block, _, _ := strings.Cut(text, "\n\n")
	lines := strings.Split(block, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "HTTP/") {
		return 0, nil, false
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 2 {
		return 0, nil, false
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, nil, false
	}
	headers = map[string]string{}
	for _, l := range lines[1:] {
		k, v, found := strings.Cut(l, ":")
		if found {
			headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return code, headers, true
}

func rateLimitHeaders(h map[string]string) bool {
	return h["x-ratelimit-remaining"] == "0" || h["retry-after"] != ""
}

// headerResetAt prefers retry-after (seconds) over x-ratelimit-reset (Unix seconds).
func headerResetAt(h map[string]string) time.Time {
	if v, err := strconv.Atoi(h["retry-after"]); err == nil && v >= 0 {
		return time.Now().Add(time.Duration(v) * time.Second)
	}
	if v, err := strconv.ParseInt(h["x-ratelimit-reset"], 10, 64); err == nil && v > 0 {
		return time.Unix(v, 0)
	}
	return time.Time{}
}
