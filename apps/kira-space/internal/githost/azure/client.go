package azure

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"golang.org/x/sync/errgroup"
)

const (
	callTimeout = 30 * time.Second
	apiVersion  = "7.1"
	// listPage is az's own --top cap per page.
	listPage = 100
	// maxOpenPages bounds Open like GitHub's 300 rows.
	maxOpenPages = 3
	// maxCommitPRs bounds ForCommit like GitHub's per_page=10.
	maxCommitPRs = 10
	// checkWorkers bounds concurrent `az repos pr policy list` spawns.
	checkWorkers = 4
)

// argv builds one az call. Every call names its organization and passes --detect false: az would
// otherwise read org and project from the working directory's git remote.
func argv(sub []string, org string, extra ...string) []string {
	args := append([]string{}, sub...)
	args = append(args, extra...)
	return append(args, "--org", org, "--detect", "false", "--only-show-errors", "-o", "json")
}

// Projections keep output small under the 1 MiB toolexec cap.
const (
	prBaseFields = `id:pullRequestId,title:title,status:status,draft:isDraft,src:sourceRefName,dst:targetRefName,` +
		`head:lastMergeSourceCommit.commitId,created:creationDate,closed:closedDate,author:createdBy.uniqueName,` +
		`forkRepo:forkSource.repository.name,forkProject:forkSource.repository.project.name`
	prFields    = prBaseFields + `,reviewers:reviewers[].{vote:vote,required:isRequired}`
	prListQuery = `[].{` + prFields + `}`
	prShowQuery = `{` + prFields + `}`
	policyQuery = `[].{status:status,type:configuration.type.displayName}`
)

// Client is the az-facing surface of the provider.
type Client struct {
	discovery *Discovery
	runner    Runner
}

// NewClient constructs a Client over discovery and runner.
func NewClient(d *Discovery, r Runner) *Client { return &Client{discovery: d, runner: r} }

func invalidRepo(r githost.Repo) githost.Status {
	return githost.Status{Kind: githost.KindForbidden, Host: Host, Reason: "not an Azure Repos repository: " + r.Owner + "/" + r.Name}
}

// call runs one az command for r's organization under the auth gate, and returns stdout on success.
func (c *Client) call(ctx context.Context, org string, write bool, args []string) ([]byte, githost.Status) {
	st := c.discovery.Status(ctx, org)
	if !st.OK() {
		return nil, st
	}
	out, err := runWithin(ctx, c.runner, st.Path, callTimeout, args)
	if res := classify(org, err, ctx.Err(), write); !res.OK() {
		return nil, res
	}
	return out, st
}

// callJSON is call plus decoding stdout into out.
func (c *Client) callJSON(ctx context.Context, org string, write bool, args []string, out any) githost.Status {
	b, st := c.call(ctx, org, write, args)
	if !st.OK() {
		return st
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return status(org, githost.KindForbidden, "Azure DevOps returned an unreadable response")
		}
	}
	return st
}

// prList reads one page of `az repos pr list` for r.
func (c *Client) prList(ctx context.Context, r githost.Repo, extra ...string) ([]rawPR, githost.Status) {
	org, project, ok := split(r)
	if !ok {
		return nil, invalidRepo(r)
	}
	args := argv([]string{"repos", "pr", "list"}, orgURL(org),
		append([]string{"--project", project, "--repository", r.Name, "--query", prListQuery}, extra...)...)
	var raw []rawPR
	st := c.callJSON(ctx, orgURL(org), false, args, &raw)
	return raw, st
}

func (c *Client) forBranch(ctx context.Context, r githost.Repo, branch string) ([]githost.ChangeRequest, githost.Status) {
	raw, st := c.prList(ctx, r, "--source-branch", branch, "--status", "all", "--top", "5")
	if !st.OK() {
		return nil, st
	}
	crs := mapPRs(r, raw)
	out := crs[:0]
	for _, cr := range crs {
		if cr.HeadOwner == r.Owner { // a fork's pull request is not this repo's branch
			out = append(out, cr)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out, st
}

// openPRs is the active snapshot, bounded at maxOpenPages pages.
func (c *Client) openPRs(ctx context.Context, r githost.Repo) ([]rawPR, githost.Status) {
	var all []rawPR
	var st githost.Status
	for page := range maxOpenPages {
		var raw []rawPR
		raw, st = c.prList(ctx, r, "--status", "active", "--top", strconv.Itoa(listPage), "--skip", strconv.Itoa(page*listPage))
		if !st.OK() {
			return nil, st
		}
		all = append(all, raw...)
		if len(raw) < listPage {
			break
		}
	}
	return all, st
}

func (c *Client) open(ctx context.Context, r githost.Repo) ([]githost.ChangeRequest, githost.Status) {
	raw, st := c.openPRs(ctx, r)
	if !st.OK() {
		return nil, st
	}
	return mapPRs(r, raw), st
}

// forCommit finds pull requests holding sha as a source or merge commit with the pull request
// query REST resource, which has no az subcommand.
func (c *Client) forCommit(ctx context.Context, r githost.Repo, sha string) ([]githost.ChangeRequest, githost.Status) {
	org, project, ok := split(r)
	if !ok {
		return nil, invalidRepo(r)
	}
	body := map[string]any{"queries": []map[string]any{
		{"type": "commit", "items": []string{sha}},
		{"type": "lastMergeCommit", "items": []string{sha}},
	}}
	var res struct {
		Results []map[string][]rawRESTPR `json:"results"`
	}
	st := c.invoke(ctx, orgURL(org), false, "POST", "pullRequestQuery", []string{"project=" + project, "repositoryId=" + r.Name}, "", body, &res)
	if !st.OK() {
		return nil, st
	}
	seen := map[int]bool{}
	var out []githost.ChangeRequest
	for _, m := range res.Results {
		for _, prs := range m {
			for _, p := range prs {
				if seen[p.PullRequestID] {
					continue
				}
				seen[p.PullRequestID] = true
				out = append(out, mapPR(r, p.raw()))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	if len(out) > maxCommitPRs {
		out = out[:maxCommitPRs]
	}
	return out, st
}

// invoke calls a git-area REST resource with `az devops invoke`. A non-nil body travels in a private
// temp file removed after the call; query, when set, projects the response (az caps stdout at 1 MiB).
func (c *Client) invoke(ctx context.Context, org string, write bool, method, resource string, route []string, query string, body, out any) githost.Status {
	extra := append(append([]string{"--area", "git", "--resource", resource, "--route-parameters"}, route...),
		"--http-method", method)
	if body != nil {
		f, err := os.CreateTemp("", "kira-az-*.json")
		if err != nil {
			return status(org, githost.KindForbidden, "could not write the request body: "+err.Error())
		}
		defer os.Remove(f.Name())
		if err := json.NewEncoder(f).Encode(body); err != nil {
			f.Close()
			return status(org, githost.KindForbidden, "could not write the request body: "+err.Error())
		}
		if err := f.Close(); err != nil {
			return status(org, githost.KindForbidden, "could not write the request body: "+err.Error())
		}
		extra = append(extra, "--in-file", f.Name())
	}
	extra = append(extra, "--api-version", apiVersion)
	if query != "" {
		extra = append(extra, "--query", query)
	}
	return c.callJSON(ctx, org, write, argv([]string{"devops", "invoke"}, org, extra...), out)
}

// prShow reads one pull request by id.
func (c *Client) prShow(ctx context.Context, r githost.Repo, number int) (rawPR, githost.Status) {
	org, _, ok := split(r)
	if !ok {
		return rawPR{}, invalidRepo(r)
	}
	var raw rawPR
	st := c.callJSON(ctx, orgURL(org), false,
		argv([]string{"repos", "pr", "show"}, orgURL(org), "--id", strconv.Itoa(number), "--query", prShowQuery), &raw)
	return raw, st
}

func (c *Client) head(ctx context.Context, r githost.Repo, number int) (githost.ChangeRequestHead, githost.Status) {
	raw, st := c.prShow(ctx, r, number)
	if !st.OK() {
		return githost.ChangeRequestHead{}, st
	}
	return githost.ChangeRequestHead{ChangeRequest: mapPR(r, raw), Author: raw.Author}, st
}

// policies reads one pull request's policy evaluations.
func (c *Client) policies(ctx context.Context, org string, number int) ([]rawPolicy, githost.Status) {
	var raw []rawPolicy
	st := c.callJSON(ctx, org, false,
		argv([]string{"repos", "pr", "policy", "list"}, org, "--id", strconv.Itoa(number), "--query", policyQuery), &raw)
	return raw, st
}

// checks refreshes the pull requests with ids: active ones carry their policy rollup, one that left
// active is read by id, and a gone one is left out.
func (c *Client) checks(ctx context.Context, r githost.Repo, ids []int) ([]githost.ChangeRequest, githost.Status) {
	org, _, ok := split(r)
	if !ok {
		return nil, invalidRepo(r)
	}
	active, st := c.openPRs(ctx, r)
	if !st.OK() {
		return nil, st
	}
	byID := make(map[int]rawPR, len(active))
	for _, p := range active {
		byID[p.ID] = p
	}
	out := make([]githost.ChangeRequest, len(ids))
	found := make([]bool, len(ids))
	var g errgroup.Group
	g.SetLimit(checkWorkers)
	var mu sync.Mutex
	var failed githost.Status
	fail := func(s githost.Status) {
		mu.Lock()
		defer mu.Unlock()
		if failed.Kind == "" {
			failed = s
		}
	}
	for i, id := range ids {
		g.Go(func() error {
			if p, isActive := byID[id]; isActive {
				pol, s := c.policies(ctx, orgURL(org), id)
				if !s.OK() {
					fail(s)
					return nil
				}
				cr := mapPR(r, p)
				cr.ChecksState = checksState(pol)
				out[i], found[i] = cr, true
				return nil
			}
			p, s := c.prShow(ctx, r, id)
			switch {
			case s.Gone:
			case !s.OK():
				fail(s)
			default:
				out[i], found[i] = mapPR(r, p), true
			}
			return nil
		})
	}
	_ = g.Wait()
	if failed.Kind != "" {
		return nil, failed
	}
	res := make([]githost.ChangeRequest, 0, len(ids))
	for i := range ids {
		if found[i] {
			res = append(res, out[i])
		}
	}
	return res, githost.Status{Kind: githost.KindOK, Host: Host, Site: orgURL(org)}
}
