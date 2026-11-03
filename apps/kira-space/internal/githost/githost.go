// Package githost is the provider-neutral contract for a repository's code host (GitHub, GitLab, ...).
// Callers depend on the narrowest role interface; each provider keeps its own credentials, CLI and
// API client. "Change request" is the neutral name for a pull or merge request.
package githost

import (
	"context"
	"net/url"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
)

// Repo is one hosted repository. Owner may be a nested group path for hosts that allow one.
type Repo struct{ Host, Owner, Name string }

// ChangeRequest is one pull or merge request.
type ChangeRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	State   string `json:"state"` // "open" | "draft" | "merged" | "closed"
	HeadRef string `json:"headRef"`
	HeadSha string `json:"headSha"`
	// HeadOwner owns the repo HeadRef lives in; "" when the source repo is gone.
	HeadOwner string `json:"headOwner"`
	BaseRef   string `json:"baseRef"`
	UpdatedAt int64  `json:"updatedAt"` // Unix milliseconds.
	// NodeID is the host's opaque id, for batched refreshes; "" when the host has none.
	NodeID string `json:"nodeId,omitempty"`
	// Draft marks an open change request not yet ready for review.
	Draft bool `json:"draft,omitempty"`
	// ReviewDecision is "" | APPROVED | CHANGES_REQUESTED | REVIEW_REQUIRED.
	ReviewDecision string `json:"reviewDecision,omitempty"`
	// ChecksState is "" | SUCCESS | FAILURE | PENDING | ERROR | EXPECTED.
	ChecksState string `json:"checksState,omitempty"`
}

// Status kinds.
const (
	KindOK              = "ok"
	KindNotFound        = "notFound"
	KindUnauthenticated = "unauthenticated"
	KindForbidden       = "forbidden"
)

// Status is a host call's actionability.
type Status struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"` // CLI path, when the host uses one.
	Version string `json:"version,omitempty"`
	Host    string `json:"host,omitempty"`
	Account string `json:"account,omitempty"`
	// Site is the account scope the CLI signed in to, for hosts that sign in per organization; "" otherwise.
	Site   string   `json:"site,omitempty"`
	Probed []string `json:"probed,omitempty"`
	Reason string   `json:"reason,omitempty"`
	// RateLimited arms the per-repo breaker.
	RateLimited bool `json:"-"`
	// ResetAt is when a rate limited call may be retried; zero when the host did not say.
	ResetAt time.Time `json:"-"`
	// Gone marks a repository the host reports as not found or not visible.
	Gone bool `json:"-"`
}

// OK reports whether s is a successful probe or call.
func (s Status) OK() bool { return s.Kind == KindOK }

// Link is a pasted issue or change-request link a provider recognised.
type Link struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"` // "changeRequest" | "issue"
	Ref      string `json:"ref"`  // display ref
}

// Link kinds.
const (
	LinkChangeRequest = "changeRequest"
	LinkIssue         = "issue"
)

// Provider is one code host.
type Provider interface {
	// ID is the setting value, e.g. "github".
	ID() string
	Terms() Terms
	Remotes
	Auth
	ChangeRequests
	URLs
}

// Remotes is repository and remote identity.
type Remotes interface {
	ParseRemote(remoteURL string) (Repo, bool)
	// Claims reports whether host belongs to this provider, for auto detection.
	Claims(ctx context.Context, host string) bool
}

// Auth is the host account the user is signed in as. A provider owns its credentials; Kira holds none.
type Auth interface {
	// Status is whether the CLI can act on r: installed, signed in to the account scope r lives in.
	Status(ctx context.Context, r Repo) Status
	// Account is the signed-in login for r's account scope; "" when unknown.
	Account(ctx context.Context, r Repo) string
}

// ChangeRequests reads pull or merge requests. Every call reports failure in Status, never an error.
type ChangeRequests interface {
	ForCommit(ctx context.Context, r Repo, sha string) ([]ChangeRequest, Status)
	// ForBranch includes closed ones, newest first.
	ForBranch(ctx context.Context, r Repo, branch string) ([]ChangeRequest, Status)
	// Open is the repo-wide open snapshot, bounded by the provider.
	Open(ctx context.Context, r Repo) ([]ChangeRequest, Status)
}

// URLs builds and checks browser links. Composed server-side only.
type URLs interface {
	ChangeRequestURL(r Repo, number int) string
	RepoURL(r Repo) string
	BranchURL(r Repo, branch string) string
	// IsChangeRequestURL re-validates a composed URL right before the OS browser opens it.
	IsChangeRequestURL(ctx context.Context, u *url.URL) bool
	ParseLink(raw string) (Link, bool)
}

// ChangeRequestSync is optional: a provider that pages every change request by recency, probes for
// change cheaply and refreshes review and checks state in batches. Without it a repository keeps
// the per-branch lookups and the open snapshot only.
type ChangeRequestSync interface {
	// Page returns change requests, closed ones included, ordered by update time descending, after
	// cursor ("" is the newest).
	Page(ctx context.Context, r Repo, cursor string, first int) (ChangeRequestPage, Status)
	// Probe reports whether anything changed since etag ("" always reports a change). A probe costs
	// no rate limit when nothing changed.
	Probe(ctx context.Context, r Repo, etag string) (changed bool, newETag string, s Status)
	// Checks returns the current review and checks state of the change requests with nodeIDs. An id
	// missing from the result no longer exists or is no longer visible.
	Checks(ctx context.Context, r Repo, nodeIDs []string) ([]ChangeRequest, Status)
}

// Review events.
const (
	ReviewApprove        = "approve"
	ReviewComment        = "comment"
	ReviewRequestChanges = "requestChanges"
)

// MaxHostCommentBytes bounds a review or change request comment body. GitHub caps one at 65,536
// characters; a byte cap never exceeds that.
const MaxHostCommentBytes = 64 << 10

// Review is one submitted review. CommitSHA is the head it applies to.
type Review struct {
	Event     string
	Body      string // "" only for ReviewApprove
	CommitSHA string
}

// ReviewTarget is one change request's current review facts, read fresh.
type ReviewTarget struct {
	Title   string
	State   string // "open" | "draft" | "merged" | "closed"
	HeadSha string
	Author  string // host login; "" when the host has none (a deleted account)
}

// ChangeRequestReviews is optional: a provider that reviews change requests and comments on their
// conversation. Every call reports failure in Status, never an error.
type ChangeRequestReviews interface {
	ReviewTarget(ctx context.Context, r Repo, number int) (ReviewTarget, Status)
	SubmitReview(ctx context.Context, r Repo, number int, rv Review) Status
	Comment(ctx context.Context, r Repo, number int, body string) Status
}

// BranchLink is a pasted change-request or branch link. Number > 0 names a change request; else
// TreePath is the path after the branch marker, which may hold a file path after the branch.
type BranchLink struct {
	Repo     Repo
	Number   int
	TreePath string
	Issue    bool // an issue link: recognised, names no branch
}

// ChangeRequestHead is one change request's head, read fresh.
type ChangeRequestHead struct {
	ChangeRequest
	Author string // host login; "" for a deleted account
}

// BranchLinks is optional: a provider that resolves a pasted link to a branch. Every call reports
// failure in Status, never an error.
type BranchLinks interface {
	// ParseBranchLink makes no host call beyond Claims.
	ParseBranchLink(ctx context.Context, raw string) (BranchLink, bool)
	ChangeRequestHead(ctx context.Context, r Repo, number int) (ChangeRequestHead, Status)
	// ChangeRequestRef is the ref the base repo serves a change request's head at.
	ChangeRequestRef(number int) string
}

// ChangeRequestPage is one page of a ChangeRequestSync.Page walk.
type ChangeRequestPage struct {
	Items     []ChangeRequest
	EndCursor string
	HasNext   bool
	RateLimit RateLimit
}

// RateLimit is the host's remaining call budget; the zero value means unknown.
type RateLimit struct {
	Remaining int
	ResetAt   time.Time
}

// CLISetup is optional: a provider driven by a local CLI that Settings explains how to install and
// sign in to.
type CLISetup interface {
	Setup() clisetup.Setup
	// CLIStatus probes the CLI against the provider's primary account scope (GitHub: github.com; Azure: the
	// default organization). refresh drops the cached probe first.
	CLIStatus(ctx context.Context, refresh bool) Status
}
