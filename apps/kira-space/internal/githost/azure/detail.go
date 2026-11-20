package azure

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"golang.org/x/sync/errgroup"
)

var _ githost.ChangeRequestDetails = (*Provider)(nil)

// Projections for the PR view, exported so the flow fake can recognise them.
const (
	// PRDetailQuery is prShowQuery's keys plus the detail ones; the reviewers key is the detail form.
	PRDetailQuery = `{` + prBaseFields + `,authorName:createdBy.displayName,description:description,` +
		"labels:labels[?active!=`false`].name," +
		`reviewers:reviewers[].{vote:vote,required:isRequired,name:displayName,login:uniqueName,team:isContainer}}`
	PolicyDetailQuery = `[].{status:status,type:configuration.type.displayName,name:configuration.settings.displayName,` +
		`statusName:configuration.settings.statusName,build:context.buildDefinitionName,blocking:configuration.isBlocking,` +
		`buildId:context.buildId}`
	// ThreadsQuery keeps the newest maxThreads threads so output stays under the 1 MiB cap.
	ThreadsQuery = `{total:length(value),threads:value[-200:].{id:id,status:status,deleted:isDeleted,` +
		`path:threadContext.filePath,right:threadContext.rightFileStart.line,left:threadContext.leftFileStart.line,` +
		`comments:comments[].{id:id,author:author.displayName,login:author.uniqueName,body:content,type:commentType,` +
		`deleted:isDeleted,published:publishedDate,updated:lastUpdatedDate}}}`
	// ThreadsRead is how many threads ThreadsQuery keeps.
	ThreadsRead = 200
)

type rawPRDetail struct {
	rawPR
	AuthorName  string   `json:"authorName"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
}

type rawPolicyDetail struct {
	Status     string      `json:"status"`
	Type       string      `json:"type"`
	Name       string      `json:"name"`
	StatusName string      `json:"statusName"`
	Build      string      `json:"build"`
	Blocking   bool        `json:"blocking"`
	BuildID    json.Number `json:"buildId"`
}

// commentType and threadStatus read a REST enum that prints as a string, or as its number on older
// servers.
type (
	commentType  string
	threadStatus string
)

var (
	commentTypeNames  = []string{"unknown", "text", "codeChange", "system"}
	threadStatusNames = []string{"unknown", "active", "fixed", "wontFix", "closed", "byDesign", "pending"}
)

func enumName(b []byte, names []string) (string, error) {
	var s string
	if json.Unmarshal(b, &s) == nil {
		return s, nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return "", err
	}
	if n < 0 || n >= len(names) {
		return "unknown", nil
	}
	return names[n], nil
}

func (e *commentType) UnmarshalJSON(b []byte) error {
	s, err := enumName(b, commentTypeNames)
	*e = commentType(s)
	return err
}

func (e *threadStatus) UnmarshalJSON(b []byte) error {
	s, err := enumName(b, threadStatusNames)
	*e = threadStatus(s)
	return err
}

type rawThreadComment struct {
	ID        int         `json:"id"`
	Author    string      `json:"author"`
	Login     string      `json:"login"`
	Body      string      `json:"body"`
	Type      commentType `json:"type"`
	Deleted   bool        `json:"deleted"`
	Published string      `json:"published"`
	Updated   string      `json:"updated"`
}

type rawThread struct {
	ID       int                `json:"id"`
	Status   threadStatus       `json:"status"`
	Deleted  bool               `json:"deleted"`
	Path     string             `json:"path"`
	Right    int                `json:"right"`
	Left     int                `json:"left"`
	Comments []rawThreadComment `json:"comments"`
}

type rawThreads struct {
	Total   int         `json:"total"`
	Threads []rawThread `json:"threads"`
}

// ParseChangeRequestLink reads a pull request link; work item and branch links miss.
func (p *Provider) ParseChangeRequestLink(ctx context.Context, raw string) (githost.Repo, int, bool) {
	l, ok := p.ParseBranchLink(ctx, raw)
	if !ok || l.Number <= 0 || l.Issue {
		return githost.Repo{}, 0, false
	}
	return l.Repo, l.Number, true
}

// ChangeRequestDetail reads the pull request, its policy evaluations and its threads concurrently.
func (p *Provider) ChangeRequestDetail(ctx context.Context, r githost.Repo, number int) (githost.ChangeRequestDetail, githost.Status) {
	org, project, ok := split(r)
	if !ok {
		return githost.ChangeRequestDetail{}, invalidRepo(r)
	}
	c, o := p.client, orgURL(org)
	var (
		raw        rawPRDetail
		pol        []rawPolicyDetail
		th         rawThreads
		sShow, sPo githost.Status
		sTh        githost.Status
		g          errgroup.Group
	)
	g.Go(func() error {
		sShow = c.callJSON(ctx, o, false,
			argv([]string{"repos", "pr", "show"}, o, "--id", strconv.Itoa(number), "--query", PRDetailQuery), &raw)
		return nil
	})
	g.Go(func() error {
		sPo = c.callJSON(ctx, o, false,
			argv([]string{"repos", "pr", "policy", "list"}, o, "--id", strconv.Itoa(number), "--query", PolicyDetailQuery), &pol)
		return nil
	})
	g.Go(func() error {
		route := []string{"project=" + project, "repositoryId=" + r.Name, "pullRequestId=" + strconv.Itoa(number)}
		sTh = c.invoke(ctx, o, false, "GET", "pullRequestThreads", route, ThreadsQuery, nil, &th)
		return nil
	})
	_ = g.Wait()
	all := []githost.Status{sShow, sPo, sTh}
	for _, s := range all {
		if s.Gone {
			return githost.ChangeRequestDetail{}, s
		}
	}
	for _, s := range all {
		if !s.OK() {
			return githost.ChangeRequestDetail{}, s
		}
	}
	return mapDetail(r, raw, pol, th), sShow
}

func personOf(login, name string) githost.Person { return githost.Person{Login: login, Name: name} }

func voteState(v int) string {
	switch {
	case v >= 5:
		return githost.ReviewerApproved
	case v == -5:
		return githost.ReviewerChangesRequested
	case v <= -10:
		return githost.ReviewerRejected
	}
	return githost.ReviewerPending
}

func policyCheck(org, project string, e rawPolicyDetail) (githost.Check, bool) {
	if e.Type != "Build" && e.Type != "Status" {
		return githost.Check{}, false
	}
	c := githost.Check{Required: e.Blocking}
	for _, n := range []string{e.Name, e.StatusName, e.Build, e.Type} {
		if n != "" {
			c.Name = n
			break
		}
	}
	switch e.Status {
	case "approved":
		c.State = githost.CheckSuccess
	case "rejected":
		c.State = githost.CheckFailure
	case "broken":
		c.State = githost.CheckError
	case "queued", "running":
		c.State = githost.CheckPending
	default:
		c.State = githost.CheckSkipped
	}
	if id, err := e.BuildID.Int64(); err == nil && id > 0 {
		c.URL = orgURL(org) + "/" + url.PathEscape(project) + "/_build/results?buildId=" + strconv.FormatInt(id, 10)
	}
	return c, true
}

func resolvedStatus(s threadStatus) bool {
	switch s {
	case "fixed", "wontFix", "closed", "byDesign":
		return true
	}
	return false
}

// mapDetail maps the three az reads onto the neutral detail. Pure.
func mapDetail(r githost.Repo, raw rawPRDetail, pol []rawPolicyDetail, th rawThreads) githost.ChangeRequestDetail {
	org, project, _ := split(r)
	evals := make([]rawPolicy, 0, len(pol))
	for _, e := range pol {
		evals = append(evals, rawPolicy{Status: e.Status, Type: e.Type})
	}
	cr := mapPR(r, raw.rawPR)
	cr.ChecksState = checksState(evals)
	d := githost.ChangeRequestDetail{
		ChangeRequest: cr,
		Author:        personOf(raw.Author, raw.AuthorName),
		Body:          raw.Description,
		CreatedAt:     parseTime(raw.Created),
		ClosedAt:      parseTime(raw.Closed),
	}
	for _, l := range raw.Labels {
		d.Labels = append(d.Labels, githost.Label{Name: l})
	}
	for _, rv := range raw.Reviewers {
		d.Reviewers = append(d.Reviewers, githost.Reviewer{
			Person: personOf(rv.Login, rv.Name), Team: rv.Team, State: voteState(rv.Vote), Required: rv.Required,
		})
	}
	for _, e := range pol {
		if c, ok := policyCheck(org, project, e); ok {
			d.Checks = append(d.Checks, c)
		}
	}

	for _, t := range th.Threads {
		if t.Deleted {
			continue
		}
		out := githost.Thread{
			ID: strconv.Itoa(t.ID), Kind: githost.ThreadComment, Path: t.Path, Resolved: resolvedStatus(t.Status),
		}
		if t.Path != "" {
			out.Kind = githost.ThreadCode
			out.Line = t.Right
			if out.Line == 0 {
				out.Line = t.Left
			}
		}
		for _, c := range t.Comments {
			if c.Deleted || c.Type == "system" || c.Type == "codeChange" {
				continue
			}
			cm := githost.Comment{
				ID: strconv.Itoa(t.ID) + "/" + strconv.Itoa(c.ID), Author: personOf(c.Login, c.Author),
				Body: c.Body, CreatedAt: parseTime(c.Published),
			}
			if u := parseTime(c.Updated); u != cm.CreatedAt {
				cm.UpdatedAt = u
			}
			out.Comments = append(out.Comments, cm)
		}
		if len(out.Comments) > 0 {
			d.Threads = append(d.Threads, out)
		}
	}
	d.ThreadsTruncated = th.Total > ThreadsRead
	d.Normalize()
	return d
}
