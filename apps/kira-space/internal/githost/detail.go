package githost

import (
	"context"
	"sort"
	"strings"
)

// ChangeRequestDetails is optional: a provider that reads one change request for the PR view. Every
// call reports failure in Status, never an error.
type ChangeRequestDetails interface {
	// ParseChangeRequestLink reads repo and number from a change request web link (tabs, query and
	// fragment ignored). No host call beyond Claims.
	ParseChangeRequestLink(ctx context.Context, raw string) (Repo, int, bool)
	// ChangeRequestDetail reads number in r. A missing one is Status.Gone.
	ChangeRequestDetail(ctx context.Context, r Repo, number int) (ChangeRequestDetail, Status)
}

// Person is a host account.
type Person struct {
	Login string `json:"login"`          // GitHub login; Azure uniqueName
	Name  string `json:"name,omitempty"` // display name when known
}

// Label is a change request label.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // 6 hex digits, no '#'; "" when invalid or none
}

// Reviewer states.
const (
	ReviewerApproved         = "approved"
	ReviewerChangesRequested = "changesRequested"
	ReviewerRejected         = "rejected"
	ReviewerCommented        = "commented"
	ReviewerPending          = "pending"
)

// Reviewer is one requested or voting reviewer.
type Reviewer struct {
	Person   Person `json:"person"`
	Team     bool   `json:"team,omitempty"`
	State    string `json:"state"`
	Required bool   `json:"required,omitempty"`
}

// Check states.
const (
	CheckSuccess = "success"
	CheckFailure = "failure"
	CheckError   = "error"
	CheckPending = "pending"
	CheckSkipped = "skipped"
	CheckNeutral = "neutral"
)

// Check is one status check or build policy.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	URL      string `json:"url,omitempty"` // https only, else ""
	Detail   string `json:"detail,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// Comment is one read-only comment.
type Comment struct {
	ID        string `json:"id"`
	Author    Person `json:"author"`
	Body      string `json:"body"` // Markdown
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
	URL       string `json:"url,omitempty"`
}

// Thread kinds.
const (
	ThreadComment = "comment"
	ThreadReview  = "review"
	ThreadCode    = "code"
)

// Thread is one conversation entry: a comment, a review or a code discussion.
type Thread struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	ReviewState       string    `json:"reviewState,omitempty"` // review threads: a Reviewer state
	Path              string    `json:"path,omitempty"`
	Line              int       `json:"line,omitempty"`
	Resolved          bool      `json:"resolved,omitempty"`
	Outdated          bool      `json:"outdated,omitempty"`
	Comments          []Comment `json:"comments"`
	CommentsTruncated bool      `json:"commentsTruncated,omitempty"`
}

// DiffStats are the change request's size.
type DiffStats struct {
	Commits   int `json:"commits"`
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// ChangeRequestDetail is everything the PR view shows.
type ChangeRequestDetail struct {
	ChangeRequest
	Author           Person     `json:"author"`
	Body             string     `json:"body"`
	CreatedAt        int64      `json:"createdAt"`
	ClosedAt         int64      `json:"closedAt,omitempty"` // merge or close time
	Labels           []Label    `json:"labels"`
	Assignees        []Person   `json:"assignees"`
	Reviewers        []Reviewer `json:"reviewers"`
	Checks           []Check    `json:"checks"`
	ChecksTruncated  bool       `json:"checksTruncated,omitempty"`
	Threads          []Thread   `json:"threads"` // oldest first by first comment
	ThreadsTruncated bool       `json:"threadsTruncated,omitempty"`
	Stats            *DiffStats `json:"stats,omitempty"` // nil when the host gives none
}

// Caps shared by every provider. Past one the view keeps the newest and flags truncation.
const (
	DetailMaxComments       = 100 // conversation comments
	DetailMaxReviews        = 50
	DetailMaxCodeThreads    = 50
	DetailMaxThreadComments = 30
	DetailMaxChecks         = 100
	DetailMaxLabels         = 50
	DetailMaxPeople         = 50
)

// Normalize makes d safe to render: empty slices not nil, valid colours and https URLs only, threads
// ordered oldest first, caps applied.
func (d *ChangeRequestDetail) Normalize() {
	if len(d.Labels) > DetailMaxLabels {
		d.Labels = d.Labels[:DetailMaxLabels]
	}
	for i := range d.Labels {
		if !validHexColor(d.Labels[i].Color) {
			d.Labels[i].Color = ""
		}
	}
	d.Labels = orEmpty(d.Labels)
	d.Assignees = orEmpty(capFirst(d.Assignees, DetailMaxPeople))
	d.Reviewers = orEmpty(capFirst(d.Reviewers, DetailMaxPeople))
	if len(d.Checks) > DetailMaxChecks {
		d.Checks = d.Checks[:DetailMaxChecks]
		d.ChecksTruncated = true
	}
	for i := range d.Checks {
		d.Checks[i].URL = httpsOnly(d.Checks[i].URL)
	}
	d.Checks = orEmpty(d.Checks)

	for i := range d.Threads {
		t := &d.Threads[i]
		if len(t.Comments) > DetailMaxThreadComments {
			t.Comments = t.Comments[len(t.Comments)-DetailMaxThreadComments:]
			t.CommentsTruncated = true
		}
		for j := range t.Comments {
			t.Comments[j].URL = httpsOnly(t.Comments[j].URL)
		}
		t.Comments = orEmpty(t.Comments)
	}
	sort.SliceStable(d.Threads, func(i, j int) bool {
		a, b := firstCreated(d.Threads[i]), firstCreated(d.Threads[j])
		if a != b {
			return a < b
		}
		return d.Threads[i].ID < d.Threads[j].ID
	})
	caps := map[string]int{ThreadComment: DetailMaxComments, ThreadReview: DetailMaxReviews, ThreadCode: DetailMaxCodeThreads}
	left := map[string]int{}
	for _, t := range d.Threads {
		left[t.Kind]++
	}
	kept := make([]Thread, 0, len(d.Threads))
	for _, t := range d.Threads {
		if limit, ok := caps[t.Kind]; ok && left[t.Kind] > limit {
			left[t.Kind]--
			d.ThreadsTruncated = true
			continue
		}
		kept = append(kept, t)
	}
	d.Threads = orEmpty(kept)
}

func firstCreated(t Thread) int64 {
	if len(t.Comments) == 0 {
		return 0
	}
	return t.Comments[0].CreatedAt
}

func capFirst[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func validHexColor(c string) bool {
	if len(c) != 6 {
		return false
	}
	for _, r := range c {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func httpsOnly(u string) string {
	if strings.HasPrefix(u, "https://") {
		return u
	}
	return ""
}
