package azure

import (
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// rawPR is one pull request as the prFields projection prints it.
type rawPR struct {
	ID          int           `json:"id"`
	Title       string        `json:"title"`
	Status      string        `json:"status"` // active | completed | abandoned
	Draft       bool          `json:"draft"`
	Src         string        `json:"src"`
	Dst         string        `json:"dst"`
	Head        string        `json:"head"`
	Created     string        `json:"created"`
	Closed      string        `json:"closed"`
	Author      string        `json:"author"`
	Repo        string        `json:"repo"` // only in the search projection
	ForkRepo    string        `json:"forkRepo"`
	ForkProject string        `json:"forkProject"`
	Reviewers   []rawReviewer `json:"reviewers"`
	MergeStatus string        `json:"mergeStatus"` // succeeded | conflicts | ...
	// AutoComplete is who set auto-complete; "" when off.
	AutoComplete string `json:"autoComplete"`
}

type rawReviewer struct {
	Vote     int  `json:"vote"`
	Required bool `json:"required"`
	// Name, Login and Team are printed by the detail projection only.
	Name  string `json:"name"`
	Login string `json:"login"`
	Team  bool   `json:"team"`
}

// rawRESTPR is the same pull request as the REST API prints it, for `az devops invoke`, which has
// no projection.
type rawRESTPR struct {
	PullRequestID         int    `json:"pullRequestId"`
	Title                 string `json:"title"`
	Status                string `json:"status"`
	IsDraft               bool   `json:"isDraft"`
	SourceRefName         string `json:"sourceRefName"`
	TargetRefName         string `json:"targetRefName"`
	CreationDate          string `json:"creationDate"`
	ClosedDate            string `json:"closedDate"`
	LastMergeSourceCommit *struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeSourceCommit"`
	CreatedBy *struct {
		UniqueName string `json:"uniqueName"`
	} `json:"createdBy"`
	Reviewers []struct {
		Vote       int  `json:"vote"`
		IsRequired bool `json:"isRequired"`
	} `json:"reviewers"`
	MergeStatus       string `json:"mergeStatus"`
	AutoCompleteSetBy *struct {
		UniqueName string `json:"uniqueName"`
	} `json:"autoCompleteSetBy"`
	ForkSource *struct {
		Repository *struct {
			Name    string `json:"name"`
			Project *struct {
				Name string `json:"name"`
			} `json:"project"`
		} `json:"repository"`
	} `json:"forkSource"`
}

func (p rawRESTPR) raw() rawPR {
	r := rawPR{
		ID: p.PullRequestID, Title: p.Title, Status: p.Status, Draft: p.IsDraft,
		Src: p.SourceRefName, Dst: p.TargetRefName, Created: p.CreationDate, Closed: p.ClosedDate,
	}
	if p.LastMergeSourceCommit != nil {
		r.Head = p.LastMergeSourceCommit.CommitID
	}
	if p.CreatedBy != nil {
		r.Author = p.CreatedBy.UniqueName
	}
	r.MergeStatus = p.MergeStatus
	if p.AutoCompleteSetBy != nil {
		r.AutoComplete = p.AutoCompleteSetBy.UniqueName
	}
	for _, rv := range p.Reviewers {
		r.Reviewers = append(r.Reviewers, rawReviewer{Vote: rv.Vote, Required: rv.IsRequired})
	}
	if f := p.ForkSource; f != nil && f.Repository != nil {
		r.ForkRepo = f.Repository.Name
		if f.Repository.Project != nil {
			r.ForkProject = f.Repository.Project.Name
		}
	}
	return r
}

// rawPolicy is one policy evaluation as the policyQuery projection prints it.
type rawPolicy struct {
	Status string `json:"status"`
	Type   string `json:"type"`
}

func trimRef(ref string) string { return strings.TrimPrefix(ref, "refs/heads/") }

func parseTime(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// mapPR makes a neutral ChangeRequest. Azure keeps no update time, so UpdatedAt is the creation
// time; the web URL is composed, never the API url.
func mapPR(r githost.Repo, p rawPR) githost.ChangeRequest {
	cr := githost.ChangeRequest{
		Number: p.ID, Title: p.Title, HeadRef: trimRef(p.Src), HeadSha: p.Head, BaseRef: trimRef(p.Dst),
		UpdatedAt: parseTime(p.Created), NodeID: strconv.Itoa(p.ID), ReviewDecision: reviewDecision(p),
		URL: repoPath(r) + "/pullrequest/" + strconv.Itoa(p.ID),
	}
	cr.HeadOwner = r.Owner
	if p.ForkRepo != "" {
		org, _, _ := split(r)
		cr.HeadOwner = org + "/" + p.ForkProject + "/" + p.ForkRepo
	}
	switch p.Status {
	case "completed":
		cr.State = "merged"
	case "abandoned":
		cr.State = "closed"
	default:
		cr.State = "open"
		if p.Draft {
			cr.State, cr.Draft = "draft", true
		}
		switch p.MergeStatus {
		case "succeeded":
			cr.Mergeable = githost.MergeableYes
		case "conflicts":
			cr.Mergeable = githost.MergeableConflict
		}
		cr.AutoMerge = p.AutoComplete != ""
	}
	return cr
}

func mapPRs(r githost.Repo, raw []rawPR) []githost.ChangeRequest {
	out := make([]githost.ChangeRequest, 0, len(raw))
	for _, p := range raw {
		out = append(out, mapPR(r, p))
	}
	return out
}

// reviewDecision: a rejection or wait-for-author vote wins, then a required reviewer who has not
// voted, then any approval. Azure's votes are 10 approve, 5 approve with suggestions, 0 none, -5
// wait for author, -10 reject.
func reviewDecision(p rawPR) string {
	for _, v := range p.Reviewers {
		if v.Vote <= -5 {
			return "CHANGES_REQUESTED"
		}
	}
	for _, v := range p.Reviewers {
		if v.Required && v.Vote == 0 {
			return "REVIEW_REQUIRED"
		}
	}
	for _, v := range p.Reviewers {
		if v.Vote >= 5 {
			return "APPROVED"
		}
	}
	return ""
}

// checksState rolls up the Build and Status policy evaluations; other policies (reviewers,
// comment resolution, ...) are not checks.
func checksState(evals []rawPolicy) string {
	var rejected, broken, pending, approved bool
	for _, e := range evals {
		if e.Type != "Build" && e.Type != "Status" {
			continue
		}
		switch e.Status {
		case "rejected":
			rejected = true
		case "broken":
			broken = true
		case "queued", "running":
			pending = true
		case "approved":
			approved = true
		}
	}
	switch {
	case rejected:
		return "FAILURE"
	case broken:
		return "ERROR"
	case pending:
		return "PENDING"
	case approved:
		return "SUCCESS"
	}
	return ""
}
