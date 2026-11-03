package issuetracker

import (
	"context"
	"sort"
	"strings"
)

// Caps on what an IssueDetail carries.
const (
	DetailMaxComments    = 100
	DetailMaxLabels      = 50
	DetailMaxComponents  = 50
	DetailMaxAttachments = 50
)

// IssueDetails reads one issue for the issue view. Optional: a tracker without it has no view.
type IssueDetails interface {
	// IssueDetail reads ref. fresh forces a read; otherwise a recent saved copy answers, marked Stale
	// once it is past the provider's fresh window. Failures are the typed errors of errors.go.
	IssueDetail(ctx context.Context, ref Ref, fresh bool) (IssueDetail, error)
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

// Attachment is a file on an issue. The bytes are not read: the tracker needs credentials Kira never
// touches, so the view links to URL instead.
type Attachment struct {
	ID        string  `json:"id"`
	Filename  string  `json:"filename"`
	MimeType  string  `json:"mimeType"`
	Size      int64   `json:"size"`
	CreatedAt int64   `json:"createdAt"`
	Author    *Person `json:"author,omitempty"`
	URL       string  `json:"url"` // https only
}

// IssueLink names another issue.
type IssueLink struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
}

// IssueDetail is everything the issue view shows.
type IssueDetail struct {
	Ref                  Ref                  `json:"ref"` // URL filled from the signed-in site
	Title                string               `json:"title"`
	Status               string               `json:"status"`
	StatusCategory       string               `json:"statusCategory"` // new | indeterminate | done | ''
	Type                 string               `json:"type,omitempty"`
	Priority             string               `json:"priority,omitempty"`
	Resolution           string               `json:"resolution,omitempty"`
	Assignee             *Person              `json:"assignee,omitempty"`
	Reporter             *Person              `json:"reporter,omitempty"`
	Labels               []string             `json:"labels"`
	Components           []string             `json:"components"`
	CreatedAt            int64                `json:"createdAt"`
	UpdatedAt            int64                `json:"updatedAt"`
	DueDate              string               `json:"dueDate,omitempty"` // YYYY-MM-DD as Jira stores it
	Parent               *IssueLink           `json:"parent,omitempty"`
	Description          string               `json:"description"` // Markdown
	Comments             []Comment            `json:"comments"`    // oldest first
	CommentsTotal        int                  `json:"commentsTotal"`
	CommentsTruncated    bool                 `json:"commentsTruncated,omitempty"`    // older ones not shown
	CommentsOmitted      bool                 `json:"commentsOmitted,omitempty"`      // none could be read
	Attachments          []Attachment         `json:"attachments"`                    // newest first
	AttachmentsTruncated bool                 `json:"attachmentsTruncated,omitempty"` // older ones not shown
	Summary              ChangeRequestSummary `json:"summary"`
	FetchedAt            int64                `json:"fetchedAt"`       // unix ms, when the tracker answered
	Stale                bool                 `json:"stale,omitempty"` // a saved copy past its fresh window
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
	d.normalizeAttachments()
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

// normalizeAttachments drops attachments without an https URL (nothing to open), orders the rest newest
// first and caps them.
func (d *IssueDetail) normalizeAttachments() {
	kept := make([]Attachment, 0, len(d.Attachments))
	for _, a := range d.Attachments {
		if strings.HasPrefix(a.URL, "https://") {
			kept = append(kept, a)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].CreatedAt != kept[j].CreatedAt {
			return kept[i].CreatedAt > kept[j].CreatedAt
		}
		return kept[i].ID < kept[j].ID
	})
	if len(kept) > DetailMaxAttachments {
		kept = kept[:DetailMaxAttachments]
		d.AttachmentsTruncated = true
	}
	d.Attachments = kept
}
