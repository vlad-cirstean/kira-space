package issuetracker

import (
	"context"
	"sort"
	"strings"
)

// Caps on what an IssueDetail carries.
const (
	DetailMaxComments   = 100
	DetailMaxLabels     = 50
	DetailMaxComponents = 50
)

// IssueDetails reads one issue for the issue view. Optional: a tracker without it has no view.
type IssueDetails interface {
	// IssueDetail reads ref fresh (no result cache). Failures are the typed errors of errors.go.
	IssueDetail(ctx context.Context, ref Ref) (IssueDetail, error)
}

// Person is a user as a tracker names one.
type Person struct {
	ID   string `json:"id"`   // Jira accountId
	Name string `json:"name"` // display name
}

// Comment is one issue comment.
type Comment struct {
	ID        string `json:"id"`
	Author    Person `json:"author"`
	Body      string `json:"body"` // Markdown (converted by the provider)
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
	URL       string `json:"url,omitempty"` // https only
}

// IssueLink names another issue.
type IssueLink struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
}

// IssueDetail is everything the issue view shows.
type IssueDetail struct {
	Ref               Ref                  `json:"ref"` // URL filled from the signed-in site
	Title             string               `json:"title"`
	Status            string               `json:"status"`
	StatusCategory    string               `json:"statusCategory"` // new | indeterminate | done | ''
	Type              string               `json:"type,omitempty"`
	Priority          string               `json:"priority,omitempty"`
	Resolution        string               `json:"resolution,omitempty"`
	Assignee          *Person              `json:"assignee,omitempty"`
	Reporter          *Person              `json:"reporter,omitempty"`
	Labels            []string             `json:"labels"`
	Components        []string             `json:"components"`
	CreatedAt         int64                `json:"createdAt"`
	UpdatedAt         int64                `json:"updatedAt"`
	DueDate           string               `json:"dueDate,omitempty"` // YYYY-MM-DD as Jira stores it
	Parent            *IssueLink           `json:"parent,omitempty"`
	Description       string               `json:"description"` // Markdown
	Comments          []Comment            `json:"comments"`    // oldest first
	CommentsTotal     int                  `json:"commentsTotal"`
	CommentsTruncated bool                 `json:"commentsTruncated,omitempty"` // older ones not shown
	CommentsOmitted   bool                 `json:"commentsOmitted,omitempty"`   // none could be read
	Summary           ChangeRequestSummary `json:"summary"`
}

// Normalize makes d safe to send: empty slices for nil, https-only comment URLs, comments oldest first
// and capped to the newest DetailMaxComments, labels and components capped.
func (d *IssueDetail) Normalize() {
	if d.Labels == nil {
		d.Labels = []string{}
	}
	if d.Components == nil {
		d.Components = []string{}
	}
	if d.Summary.Providers == nil {
		d.Summary.Providers = []string{}
	}
	if len(d.Labels) > DetailMaxLabels {
		d.Labels = d.Labels[:DetailMaxLabels]
	}
	if len(d.Components) > DetailMaxComponents {
		d.Components = d.Components[:DetailMaxComponents]
	}
	if d.Comments == nil {
		d.Comments = []Comment{}
	}
	for i := range d.Comments {
		if !strings.HasPrefix(d.Comments[i].URL, "https://") {
			d.Comments[i].URL = ""
		}
	}
	sort.SliceStable(d.Comments, func(i, j int) bool {
		a, b := d.Comments[i], d.Comments[j]
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.ID < b.ID
	})
	if len(d.Comments) > DetailMaxComments {
		d.Comments = d.Comments[len(d.Comments)-DetailMaxComments:]
		d.CommentsTruncated = true
	}
	if d.CommentsTotal > len(d.Comments) {
		d.CommentsTruncated = true
	}
}
