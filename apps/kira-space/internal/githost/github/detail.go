package github

import (
	"context"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

var _ githost.ChangeRequestDetails = (*Provider)(nil)

const (
	detailActor   = `login ... on User{name}`
	detailComment = `id url body createdAt lastEditedAt author{` + detailActor + `}`
)

// detailQuery reads one pull request for the PR view. `last` and `commitCount` are aliases: the
// shared sync fields already select commits(last:1) for the rollup state.
const detailQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){` +
	syncPRFields + ` body createdAt mergedAt closedAt mergeable additions deletions changedFiles author{` + detailActor + `}
commitCount: commits{totalCount}
last: commits(last:1){nodes{commit{statusCheckRollup{contexts(first:100){totalCount nodes{__typename
... on CheckRun{name status conclusion detailsUrl isRequired(pullRequestNumber:$number) title}
... on StatusContext{context state targetUrl description isRequired(pullRequestNumber:$number)}}}}}}}
labels(first:50){nodes{name color}}
assignees(first:50){nodes{login name}}
reviewRequests(first:50){nodes{requestedReviewer{__typename ... on User{login name} ... on Team{slug name} ... on Bot{login} ... on Mannequin{login}}}}
latestOpinionatedReviews(first:50){nodes{state author{` + detailActor + `}}}
comments(last:100){totalCount nodes{` + detailComment + `}}
reviews(last:50){totalCount nodes{` + detailComment + ` state}}
reviewThreads(last:50){totalCount nodes{id path line originalLine isResolved isOutdated comments(first:30){totalCount nodes{` + detailComment + `}}}}
}} rateLimit{remaining resetAt cost}}`

type gqlActor struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

type gqlDetailComment struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Body         string    `json:"body"`
	CreatedAt    string    `json:"createdAt"`
	LastEditedAt string    `json:"lastEditedAt"`
	Author       *gqlActor `json:"author"`
	State        string    `json:"state"` // reviews only
}

type gqlCheckNode struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	Title      string `json:"title"`
	Context    string `json:"context"`
	State      string `json:"state"`
	TargetURL  string `json:"targetUrl"`
	Desc       string `json:"description"`
	IsRequired bool   `json:"isRequired"`
}

type gqlDetailPR struct {
	gqlPR
	Body         string    `json:"body"`
	CreatedAt    string    `json:"createdAt"`
	ClosedAt     string    `json:"closedAt"`
	MergedAt     string    `json:"mergedAt"`
	Mergeable    string    `json:"mergeable"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	ChangedFiles int       `json:"changedFiles"`
	Author       *gqlActor `json:"author"`
	CommitCount  struct {
		TotalCount int `json:"totalCount"`
	} `json:"commitCount"`
	Last struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						TotalCount int            `json:"totalCount"`
						Nodes      []gqlCheckNode `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"last"`
	Labels struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []gqlActor `json:"nodes"`
	} `json:"assignees"`
	ReviewRequests struct {
		Nodes []struct {
			Reviewer *struct {
				Typename string `json:"__typename"`
				Login    string `json:"login"`
				Slug     string `json:"slug"`
				Name     string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			State  string    `json:"state"`
			Author *gqlActor `json:"author"`
		} `json:"nodes"`
	} `json:"latestOpinionatedReviews"`
	Comments struct {
		TotalCount int                `json:"totalCount"`
		Nodes      []gqlDetailComment `json:"nodes"`
	} `json:"comments"`
	Reviews struct {
		TotalCount int                `json:"totalCount"`
		Nodes      []gqlDetailComment `json:"nodes"`
	} `json:"reviews"`
	ReviewThreads struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			ID           string `json:"id"`
			Path         string `json:"path"`
			Line         int    `json:"line"`
			OriginalLine int    `json:"originalLine"`
			IsResolved   bool   `json:"isResolved"`
			IsOutdated   bool   `json:"isOutdated"`
			Comments     struct {
				TotalCount int                `json:"totalCount"`
				Nodes      []gqlDetailComment `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
}

type detailData struct {
	Repository *struct {
		PullRequest *gqlDetailPR `json:"pullRequest"`
	} `json:"repository"`
	RateLimit gqlRateLimit `json:"rateLimit"`
}

// ParseChangeRequestLink reads a pull request link; issue and tree links miss.
func (p *Provider) ParseChangeRequestLink(ctx context.Context, raw string) (githost.Repo, int, bool) {
	l, ok := p.ParseBranchLink(ctx, raw)
	if !ok || l.Number <= 0 || l.Issue {
		return githost.Repo{}, 0, false
	}
	return l.Repo, l.Number, true
}

// ChangeRequestDetail reads one pull request with one GraphQL call.
func (p *Provider) ChangeRequestDetail(ctx context.Context, r githost.Repo, number int) (githost.ChangeRequestDetail, githost.Status) {
	repo := Repo(r)
	vars := []GraphQLVar{
		{Name: "owner", Str: repo.Owner}, {Name: "name", Str: repo.Name}, {Name: "number", Int: number, IsInt: true},
	}
	var data detailData
	status, _ := p.client.graphql(ctx, repo, detailQuery, vars, &data)
	if !status.OK() {
		return githost.ChangeRequestDetail{}, toStatus(withRateLimitReset(status, data.RateLimit))
	}
	if data.Repository == nil || data.Repository.PullRequest == nil {
		return githost.ChangeRequestDetail{}, toStatus(gone(repo.Host))
	}
	return mapDetail(data.Repository.PullRequest), toStatus(status)
}

func parseMillis(s string) int64 {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	return 0
}

func person(a *gqlActor) githost.Person {
	if a == nil {
		return githost.Person{Login: "ghost"}
	}
	return githost.Person{Login: a.Login, Name: a.Name}
}

func checkState(n gqlCheckNode) string {
	if n.Typename == "StatusContext" {
		switch n.State {
		case "SUCCESS":
			return githost.CheckSuccess
		case "FAILURE":
			return githost.CheckFailure
		case "ERROR":
			return githost.CheckError
		default:
			return githost.CheckPending
		}
	}
	if n.Status != "COMPLETED" {
		return githost.CheckPending
	}
	switch n.Conclusion {
	case "SUCCESS":
		return githost.CheckSuccess
	case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE", "ACTION_REQUIRED":
		return githost.CheckFailure
	case "SKIPPED":
		return githost.CheckSkipped
	default:
		return githost.CheckNeutral
	}
}

func reviewState(s string) string {
	switch s {
	case "APPROVED":
		return githost.ReviewerApproved
	case "CHANGES_REQUESTED":
		return githost.ReviewerChangesRequested
	case "COMMENTED", "DISMISSED":
		return githost.ReviewerCommented
	}
	return ""
}

func mapComment(c gqlDetailComment) githost.Comment {
	return githost.Comment{
		ID: c.ID, Author: person(c.Author), Body: c.Body, URL: c.URL,
		CreatedAt: parseMillis(c.CreatedAt), UpdatedAt: parseMillis(c.LastEditedAt),
	}
}

// mapDetail maps the GraphQL node onto the neutral detail. Pure.
func mapDetail(g *gqlDetailPR) githost.ChangeRequestDetail {
	d := githost.ChangeRequestDetail{
		ChangeRequest: toChangeRequests([]PR{g.toPR()})[0],
		Author:        person(g.Author),
		Body:          g.Body,
		CreatedAt:     parseMillis(g.CreatedAt),
		Stats: &githost.DiffStats{
			Commits: g.CommitCount.TotalCount, Files: g.ChangedFiles, Additions: g.Additions, Deletions: g.Deletions,
		},
	}
	// The detail's own assignees and threads replace the sync-only counts the shared fields carry.
	d.CommentCount, d.ChangeRequest.Assignees = nil, nil
	d.ClosedAt = parseMillis(g.MergedAt)
	if d.ClosedAt == 0 {
		d.ClosedAt = parseMillis(g.ClosedAt)
	}
	switch g.Mergeable {
	case "MERGEABLE":
		d.Mergeable = githost.MergeableYes
	case "CONFLICTING":
		d.Mergeable = githost.MergeableConflict
	}
	for _, l := range g.Labels.Nodes {
		d.Labels = append(d.Labels, githost.Label{Name: l.Name, Color: l.Color})
	}
	for _, a := range g.Assignees.Nodes {
		d.Assignees = append(d.Assignees, githost.Person{Login: a.Login, Name: a.Name})
	}

	if n := g.Last.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
		ctxs := n[0].Commit.StatusCheckRollup.Contexts
		d.ChecksTruncated = ctxs.TotalCount > len(ctxs.Nodes)
		for _, c := range ctxs.Nodes {
			ck := githost.Check{State: checkState(c), Required: c.IsRequired}
			if c.Typename == "StatusContext" {
				ck.Name, ck.URL, ck.Detail = c.Context, c.TargetURL, c.Desc
			} else {
				ck.Name, ck.URL, ck.Detail = c.Name, c.DetailsURL, c.Title
			}
			d.Checks = append(d.Checks, ck)
		}
	}

	d.Reviewers = mapReviewers(g)

	truncated := g.Comments.TotalCount > len(g.Comments.Nodes) ||
		g.Reviews.TotalCount > len(g.Reviews.Nodes) || g.ReviewThreads.TotalCount > len(g.ReviewThreads.Nodes)
	for _, c := range g.Comments.Nodes {
		d.Threads = append(d.Threads, githost.Thread{ID: c.ID, Kind: githost.ThreadComment, Comments: []githost.Comment{mapComment(c)}})
	}
	for _, rv := range g.Reviews.Nodes {
		st := rv.State
		if st == "PENDING" || (strings.TrimSpace(rv.Body) == "" && st != "APPROVED" && st != "CHANGES_REQUESTED") {
			continue
		}
		d.Threads = append(d.Threads, githost.Thread{
			ID: rv.ID, Kind: githost.ThreadReview, ReviewState: reviewState(st), Comments: []githost.Comment{mapComment(rv)},
		})
	}
	for _, t := range g.ReviewThreads.Nodes {
		th := githost.Thread{
			ID: t.ID, Kind: githost.ThreadCode, Path: t.Path, Line: t.Line, Resolved: t.IsResolved, Outdated: t.IsOutdated,
			CommentsTruncated: t.Comments.TotalCount > len(t.Comments.Nodes),
		}
		if th.Line == 0 {
			th.Line = t.OriginalLine
		}
		for _, c := range t.Comments.Nodes {
			th.Comments = append(th.Comments, mapComment(c))
		}
		d.Threads = append(d.Threads, th)
	}
	d.ThreadsTruncated = truncated
	d.Normalize()
	return d
}

// mapReviewers unions review requests (pending) with each reviewer's latest opinionated review; a
// review beats a pending request. Dismissed reviews drop.
func mapReviewers(g *gqlDetailPR) []githost.Reviewer {
	var out []githost.Reviewer
	idx := map[string]int{}
	for _, n := range g.LatestReviews.Nodes {
		st := reviewState(n.State)
		if st == "" || n.State == "DISMISSED" || n.Author == nil {
			continue
		}
		idx[n.Author.Login] = len(out)
		out = append(out, githost.Reviewer{Person: person(n.Author), State: st})
	}
	for _, n := range g.ReviewRequests.Nodes {
		rr := n.Reviewer
		if rr == nil {
			continue
		}
		rv := githost.Reviewer{State: githost.ReviewerPending}
		if rr.Typename == "Team" {
			rv.Team = true
			rv.Person = githost.Person{Login: rr.Slug, Name: rr.Name}
		} else {
			rv.Person = githost.Person{Login: rr.Login, Name: rr.Name}
		}
		if _, dup := idx[rv.Person.Login]; dup && !rv.Team {
			continue
		}
		out = append(out, rv)
	}
	return out
}
