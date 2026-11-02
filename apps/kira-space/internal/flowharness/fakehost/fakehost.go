// Package fakehost is an in-process second code host for flow tests. It speaks "merge requests" on
// forge.test, so a test proves nothing in the app hard-codes GitHub's words, URLs or CLI.
package fakehost

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// ID is the setting value and registry key of the fake host.
const ID = "forge"

// Host is the one host name the fake claims.
const Host = "forge.test"

// Forge is a githost.Provider, githost.ChangeRequestSync and githost.ChangeRequestReviews over
// in-memory data. Every call is recorded.
type Forge struct {
	mu    sync.Mutex
	crs   []githost.ChangeRequest
	calls []string
	// rev changes whenever the listing a Probe sees changes: every mutation but a checks state.
	rev      int
	account  string
	limited  time.Time
	offline  bool
	authors  map[int]string
	reviews  map[int][]githost.Review
	comments map[int][]string
	details  map[int]githost.ChangeRequestDetail
	posted   map[int][]PostedComment
}

var (
	_ githost.Provider          = (*Forge)(nil)
	_ githost.ChangeRequestSync = (*Forge)(nil)

	_ githost.ChangeRequestReviews  = (*Forge)(nil)
	_ githost.ChangeRequestDetails  = (*Forge)(nil)
	_ githost.ChangeRequestComments = (*Forge)(nil)
	_ githost.CLISetup              = (*Forge)(nil)
)

// New returns an empty fake host.
func New() *Forge {
	return &Forge{
		account: "forger", authors: map[int]string{}, reviews: map[int][]githost.Review{}, comments: map[int][]string{},
		details: map[int]githost.ChangeRequestDetail{}, posted: map[int][]PostedComment{},
	}
}

// AddChangeRequest registers cr. Set HeadOwner to the repo owner for the branch to badge.
func (f *Forge) AddChangeRequest(cr githost.ChangeRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.crs = append(f.crs, cr)
	f.rev++
}

// SetChangeRequest adds cr, or replaces the one with its number, as an update on the host does.
func (f *Forge) SetChangeRequest(cr githost.ChangeRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rev++
	for i := range f.crs {
		if f.crs[i].Number == cr.Number {
			f.crs[i] = cr
			return
		}
	}
	f.crs = append(f.crs, cr)
}

// RemoveChangeRequest deletes merge request number from the host.
func (f *Forge) RemoveChangeRequest(number int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rev++
	f.crs = slices.DeleteFunc(f.crs, func(cr githost.ChangeRequest) bool { return cr.Number == number })
}

// SetChecksState sets the CI state of merge request number. Like a real CI result, it does not
// change the host's listing, so a Probe still answers unchanged.
func (f *Forge) SetChecksState(number int, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.crs {
		if f.crs[i].Number == number {
			f.crs[i].ChecksState = state
		}
	}
}

// SetAccount changes the signed-in account name.
func (f *Forge) SetAccount(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.account = name
}

// SetRateLimited makes every host call fail as rate limited until until; the zero time clears it.
func (f *Forge) SetRateLimited(until time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limited = until
}

// SetHead moves merge request number's head, as a push to its branch would.
func (f *Forge) SetHead(number int, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.crs {
		if f.crs[i].Number == number {
			f.crs[i].HeadSha = sha
			f.rev++
		}
	}
}

// Calls returns every recorded call, in order, as "Method" or "Method:arg".
func (f *Forge) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// ResetCalls forgets every recorded call.
func (f *Forge) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// Count returns how many recorded calls start with prefix.
func (f *Forge) Count(prefix string) int {
	n := 0
	for _, c := range f.Calls() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *Forge) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}

func (f *Forge) ID() string { return ID }

func (f *Forge) Terms() githost.Terms {
	return githost.Terms{
		Label: "Forge", ChangeRequest: "Merge request", ChangeRequestNoun: "merge request",
		ChangeRequestShort: "MR", CLI: "Forge CLI",
	}
}

var remotePattern = regexp.MustCompile(`^(?:https://|git@)forge\.test[/:]([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

func (f *Forge) ParseRemote(remoteURL string) (githost.Repo, bool) {
	m := remotePattern.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if m == nil {
		return githost.Repo{}, false
	}
	return githost.Repo{Host: Host, Owner: m[1], Name: m[2]}, true
}

func (f *Forge) Claims(_ context.Context, host string) bool { return host == Host }

func (f *Forge) Status(context.Context, githost.Repo) githost.Status {
	return githost.Status{Kind: githost.KindOK, Host: Host, Account: f.accountName()}
}

func (f *Forge) Account(context.Context, githost.Repo) string { return f.accountName() }

func (f *Forge) accountName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.account
}

func (f *Forge) ForCommit(_ context.Context, _ githost.Repo, sha string) ([]githost.ChangeRequest, githost.Status) {
	f.record("ForCommit:" + sha)
	return f.match(func(cr githost.ChangeRequest) bool { return cr.HeadSha == sha })
}

func (f *Forge) ForBranch(_ context.Context, _ githost.Repo, branch string) ([]githost.ChangeRequest, githost.Status) {
	f.record("ForBranch:" + branch)
	return f.match(func(cr githost.ChangeRequest) bool { return cr.HeadRef == branch })
}

func (f *Forge) Open(_ context.Context, _ githost.Repo) ([]githost.ChangeRequest, githost.Status) {
	f.record("Open")
	return f.match(func(cr githost.ChangeRequest) bool { return cr.State == "open" || cr.State == "draft" })
}

// SetOffline makes every listing lookup answer unauthenticated, as a host the person signed out of.
func (f *Forge) SetOffline(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offline = v
}

func (f *Forge) match(keep func(githost.ChangeRequest) bool) ([]githost.ChangeRequest, githost.Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.offline {
		return nil, githost.Status{Kind: githost.KindUnauthenticated, Host: Host, Reason: "offline"}
	}
	var out []githost.ChangeRequest
	for _, cr := range f.crs {
		if keep(cr) {
			out = append(out, cr)
		}
	}
	return out, githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) ChangeRequestURL(r githost.Repo, number int) string {
	return fmt.Sprintf("%s/-/merge_requests/%d", f.RepoURL(r), number)
}

func (f *Forge) RepoURL(r githost.Repo) string {
	return fmt.Sprintf("https://%s/%s/%s", r.Host, r.Owner, r.Name)
}

func (f *Forge) BranchURL(r githost.Repo, branch string) string {
	return f.RepoURL(r) + "/-/tree/" + githost.RefPath(branch)
}

var mergeRequestPath = regexp.MustCompile(`^/[\w.-]+/[\w.-]+/-/merge_requests/[0-9]+$`)

func (f *Forge) IsChangeRequestURL(_ context.Context, u *url.URL) bool {
	return u.Scheme == "https" && u.Host == Host && mergeRequestPath.MatchString(u.Path)
}

var linkPattern = regexp.MustCompile(`^https://forge\.test/[^/\s]+/([^/\s]+)/-/(issues|merge_requests)/(\d+)`)

func (f *Forge) ParseLink(raw string) (githost.Link, bool) {
	m := linkPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return githost.Link{}, false
	}
	if m[2] == "merge_requests" {
		return githost.Link{Provider: ID, Kind: githost.LinkChangeRequest, Ref: m[1] + "!" + m[3]}, true
	}
	return githost.Link{Provider: ID, Kind: githost.LinkIssue, Ref: m[1] + "#" + m[3]}, true
}

// rateLimit is the failed status of a call made while SetRateLimited is in force.
func (f *Forge) rateLimit() (githost.Status, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.limited.IsZero() || !time.Now().Before(f.limited) {
		return githost.Status{}, false
	}
	return githost.Status{Kind: githost.KindForbidden, Host: Host, Reason: "rate limit exhausted", RateLimited: true, ResetAt: f.limited}, true
}

func nodeID(number int) string { return fmt.Sprintf("MR_%d", number) }

// withSyncFields fills the fields a sync read carries.
func withSyncFields(cr githost.ChangeRequest) githost.ChangeRequest {
	cr.NodeID = nodeID(cr.Number)
	cr.Draft = cr.State == "draft"
	return cr
}

func (f *Forge) Page(_ context.Context, _ githost.Repo, cursor string, first int) (githost.ChangeRequestPage, githost.Status) {
	f.record("Page:" + cursor)
	if st, limited := f.rateLimit(); limited {
		return githost.ChangeRequestPage{}, st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	sorted := slices.Clone(f.crs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].UpdatedAt != sorted[j].UpdatedAt {
			return sorted[i].UpdatedAt > sorted[j].UpdatedAt
		}
		return sorted[i].Number > sorted[j].Number
	})
	start, _ := strconv.Atoi(cursor)
	start = min(max(start, 0), len(sorted))
	end := min(start+max(first, 1), len(sorted))
	page := githost.ChangeRequestPage{EndCursor: strconv.Itoa(end), HasNext: end < len(sorted)}
	for _, cr := range sorted[start:end] {
		page.Items = append(page.Items, withSyncFields(cr))
	}
	return page, githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) Probe(_ context.Context, _ githost.Repo, etag string) (bool, string, githost.Status) {
	if st, limited := f.rateLimit(); limited {
		f.record("Probe:limited")
		return false, "", st
	}
	f.mu.Lock()
	current := "r" + strconv.Itoa(f.rev)
	f.mu.Unlock()
	if etag == current {
		f.record("Probe:304")
		return false, current, githost.Status{Kind: githost.KindOK, Host: Host}
	}
	f.record("Probe:200")
	return true, current, githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) Checks(_ context.Context, _ githost.Repo, nodeIDs []string) ([]githost.ChangeRequest, githost.Status) {
	f.record("Checks:" + strconv.Itoa(len(nodeIDs)))
	if st, limited := f.rateLimit(); limited {
		return nil, st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []githost.ChangeRequest
	for _, cr := range f.crs {
		if slices.Contains(nodeIDs, nodeID(cr.Number)) {
			out = append(out, withSyncFields(cr))
		}
	}
	return out, githost.Status{Kind: githost.KindOK, Host: Host}
}

// SetAuthor sets the login that opened merge request number.
func (f *Forge) SetAuthor(number int, login string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authors[number] = login
}

// Reviews returns the reviews submitted on merge request number.
func (f *Forge) Reviews(number int) []githost.Review {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reviews[number])
}

// Comments returns the conversation comments posted on merge request number.
func (f *Forge) Comments(number int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.comments[number])
}

func (f *Forge) goneStatus() githost.Status {
	return githost.Status{Kind: githost.KindForbidden, Host: Host, Reason: "not found", Gone: true}
}

// find returns merge request number, under f.mu.
func (f *Forge) find(number int) (githost.ChangeRequest, bool) {
	for _, cr := range f.crs {
		if cr.Number == number {
			return cr, true
		}
	}
	return githost.ChangeRequest{}, false
}

func (f *Forge) ReviewTarget(_ context.Context, _ githost.Repo, number int) (githost.ReviewTarget, githost.Status) {
	f.record("ReviewTarget:" + strconv.Itoa(number))
	if st, limited := f.rateLimit(); limited {
		return githost.ReviewTarget{}, st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cr, ok := f.find(number)
	if !ok {
		return githost.ReviewTarget{}, f.goneStatus()
	}
	return githost.ReviewTarget{Title: cr.Title, State: cr.State, HeadSha: cr.HeadSha, Author: f.authors[number]},
		githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) SubmitReview(_ context.Context, _ githost.Repo, number int, rv githost.Review) githost.Status {
	f.record("SubmitReview:" + strconv.Itoa(number))
	if st, limited := f.rateLimit(); limited {
		return st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.find(number); !ok {
		return f.goneStatus()
	}
	if rv.Event != githost.ReviewComment && strings.EqualFold(f.authors[number], f.account) {
		return githost.Status{Kind: githost.KindForbidden, Host: Host, Reason: "you cannot approve your own merge request"}
	}
	f.reviews[number] = append(f.reviews[number], rv)
	return githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) Comment(_ context.Context, _ githost.Repo, number int, body string) githost.Status {
	f.record("Comment:" + strconv.Itoa(number))
	if st, limited := f.rateLimit(); limited {
		return st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.find(number); !ok {
		return f.goneStatus()
	}
	f.comments[number] = append(f.comments[number], body)
	return githost.Status{Kind: githost.KindOK, Host: Host}
}

var detailLinkPattern = regexp.MustCompile(`^https://forge\.test/([\w.-]+)/([\w.-]+)/-/merge_requests/(\d+)/?(?:[?#].*)?$`)

// ParseChangeRequestLink reads a merge request link; issue links miss. It has no BranchLinks role, so
// the branch link text other flows assert stays unchanged.
func (f *Forge) ParseChangeRequestLink(_ context.Context, raw string) (githost.Repo, int, bool) {
	m := detailLinkPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return githost.Repo{}, 0, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil || n <= 0 {
		return githost.Repo{}, 0, false
	}
	return githost.Repo{Host: Host, Owner: m[1], Name: m[2]}, n, true
}

// SetDetail stores the detail ChangeRequestDetail answers for number. The merge request row and the
// author set with SetAuthor override the stored ChangeRequest and Author.
func (f *Forge) SetDetail(number int, d githost.ChangeRequestDetail) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.details[number] = d
}

func (f *Forge) ChangeRequestDetail(_ context.Context, _ githost.Repo, number int) (githost.ChangeRequestDetail, githost.Status) {
	f.record("detail:" + strconv.Itoa(number))
	if st, limited := f.rateLimit(); limited {
		return githost.ChangeRequestDetail{}, st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.offline {
		return githost.ChangeRequestDetail{}, githost.Status{Kind: githost.KindUnauthenticated, Host: Host, Reason: "offline"}
	}
	cr, ok := f.find(number)
	if !ok {
		return githost.ChangeRequestDetail{}, f.goneStatus()
	}
	d := f.details[number]
	d.ChangeRequest = cr
	if login, set := f.authors[number]; set {
		d.Author = githost.Person{Login: login}
	}
	d.Normalize()
	return d, githost.Status{Kind: githost.KindOK, Host: Host}
}

func (f *Forge) Setup() clisetup.Setup {
	return clisetup.Setup{
		DocsURL: "https://forge.test/cli",
		Steps: []clisetup.Step{
			{Purpose: clisetup.PurposeInstall, Command: "brew install forge"},
			{Purpose: clisetup.PurposeLogin, Command: "forge login"},
			{Purpose: clisetup.PurposeCheck, Command: "forge status"},
		},
	}
}

func (f *Forge) CLIStatus(ctx context.Context, _ bool) githost.Status {
	return f.Status(ctx, githost.Repo{Host: Host})
}

// PostedComment is one comment posted through the comment role.
type PostedComment struct {
	Body    string
	ReplyTo string
}

// Posted returns the comments posted on merge request number, in order.
func (f *Forge) Posted(number int) []PostedComment {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.posted[number])
}

func (f *Forge) ReplyKinds(githost.Repo) []string {
	return []string{githost.ThreadComment, githost.ThreadReview, githost.ThreadCode}
}

// PostChangeRequestComment records the comment and appends it to the detail overlay, so a refetch shows it.
func (f *Forge) PostChangeRequestComment(_ context.Context, _ githost.Repo, number int, c githost.NewComment) githost.Status {
	f.record("post:" + strconv.Itoa(number))
	if st, limited := f.rateLimit(); limited {
		return st
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.offline {
		return githost.Status{Kind: githost.KindUnauthenticated, Host: Host, Reason: "offline"}
	}
	if _, ok := f.find(number); !ok {
		return f.goneStatus()
	}
	d := f.details[number]
	threadAt := -1
	if c.ReplyTo != "" {
		for i, t := range d.Threads {
			if t.ID == c.ReplyTo {
				threadAt = i
			}
		}
		if threadAt < 0 {
			return githost.Status{Kind: githost.KindForbidden, Host: Host, Reason: "no such thread"}
		}
	}
	pc := PostedComment{Body: c.Body, ReplyTo: c.ReplyTo}
	f.posted[number] = append(f.posted[number], pc)
	id := "posted-" + strconv.Itoa(len(f.posted[number]))
	comment := githost.Comment{ID: id, Author: githost.Person{Login: f.account}, Body: c.Body, CreatedAt: time.Now().UnixMilli()}
	d.Threads = slices.Clone(d.Threads)
	if threadAt >= 0 {
		d.Threads[threadAt].Comments = append(slices.Clone(d.Threads[threadAt].Comments), comment)
	} else {
		d.Threads = append(d.Threads, githost.Thread{ID: id, Kind: githost.ThreadComment, Comments: []githost.Comment{comment}})
	}
	f.details[number] = d
	return githost.Status{Kind: githost.KindOK, Host: Host}
}
