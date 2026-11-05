package bridge

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// HostWindowKeyPrefix marks a host view window key: the PR view window, never restored.
const HostWindowKeyPrefix = "hostview-"

// IsHostViewKey reports whether key names a host view window.
func IsHostViewKey(key string) bool { return strings.HasPrefix(key, HostWindowKeyPrefix) }

const (
	maxHostViewURLBytes = 2048
	// hostViewBreakerFallback arms the breaker when the host gave no reset time; it mirrors gitsession's
	// hostBreakerFallback.
	hostViewBreakerFallback = 15 * time.Minute
)

// Host view target kinds.
const (
	HostViewKindChangeRequest = "changeRequest"
	HostViewKindIssue         = "issue"
)

// Host view change request result kinds.
const (
	HostViewOK          = "ok"
	HostViewUnavailable = "unavailable"
	HostViewGone        = "gone"
)

// hostViewWindowRows is the window table slice the service writes.
type hostViewWindowRows interface {
	List() ([]model.WindowRecord, error)
	Create(model.WindowRecord) error
	Delete(key string) error
}

// HostViewService opens and reads the PR and issue views: one window per change request or issue, never restored.
type HostViewService struct {
	Deps           appcore.Deps
	Windows        hostViewWindowRows
	Targets        *HostViewTargets
	postMu         sync.Mutex
	posting        map[string]bool
	Trackers       *issuetracker.Registry
	Now            func() time.Time
	OpenWindow     func(shell.WindowRecord)
	FocusWindow    func(key string) bool
	SetWindowTitle func(key, title string)

	openMu  sync.Mutex
	breaker hostViewBreaker
}

// HostViewOpenArgs is Open's request.
type HostViewOpenArgs struct {
	URL string `json:"url"`
}

// HostViewOpen is Open's result: Opened is false when an existing window was focused.
type HostViewOpen struct {
	Key    string `json:"key"`
	Opened bool   `json:"opened"`
}

// HostViewWindowArgs names a window.
type HostViewWindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// HostViewTarget is what a host view window shows.
type HostViewTarget struct {
	Kind     string `json:"kind"`
	URL      string `json:"url"` // '' for an issue on a site not yet known
	Provider string `json:"provider"`
	// Change request kind.
	Host   *githost.Info `json:"host,omitempty"`
	Repo   string        `json:"repo,omitempty"` // display "owner/name"
	Number int           `json:"number,omitempty"`
	// Issue kind.
	Tracker *issuetracker.Terms `json:"tracker,omitempty"`
	Key     string              `json:"key,omitempty"`
}

// HostViewIssueArgs is Issue's request: an issue key or link. Fresh forces a tracker read; otherwise a
// recent saved copy may answer, marked stale.
type HostViewIssueArgs struct {
	Ref   string `json:"ref"`
	Fresh bool   `json:"fresh"`
}

// HostViewIssue is Issue's result. Kind unavailable carries a Reason word.
type HostViewIssue struct {
	Kind   string                    `json:"kind"`
	Detail *issuetracker.IssueDetail `json:"detail,omitempty"`
	Reason string                    `json:"reason,omitempty"`
	// Compose is what the composer may do; nil when the tracker cannot post.
	Compose *HostViewCompose `json:"compose,omitempty"`
}

// HostViewChangeRequestArgs is ChangeRequest's request.
type HostViewChangeRequestArgs struct {
	URL string `json:"url"`
}

// HostViewChangeRequest is ChangeRequest's result. Kind unavailable carries the reason in Status.
type HostViewChangeRequest struct {
	Kind   string                       `json:"kind"`
	Detail *githost.ChangeRequestDetail `json:"detail,omitempty"`
	Status githost.Status               `json:"status"`
	// Compose is what the composer may do; nil when the provider cannot post.
	Compose *HostViewCompose `json:"compose,omitempty"`
}

func (s *HostViewService) hosts() *githost.Registry {
	if s.Deps.GitRegistry == nil {
		return nil
	}
	return s.Deps.GitRegistry.Hosts.Registry
}

func (s *HostViewService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// hostViewLink is a change request link resolved against the registry.
type hostViewLink struct {
	p       githost.Provider
	details githost.ChangeRequestDetails
	repo    githost.Repo
	number  int
}

func (l hostViewLink) identity() string {
	return l.repoIdentity() + "|" + strconv.Itoa(l.number)
}

func (l hostViewLink) repoIdentity() string {
	return l.p.ID() + "|" + l.repo.Host + "|" + l.repo.Owner + "|" + l.repo.Name
}

func (s *HostViewService) parse(ctx context.Context, raw string) (hostViewLink, error) {
	hosts := s.hosts()
	raw = strings.TrimSpace(raw)
	if len(raw) <= maxHostViewURLBytes && hosts != nil {
		if p, d, repo, n, ok := hosts.ParseChangeRequestLink(ctx, raw); ok {
			return hostViewLink{p: p, details: d, repo: repo, number: n}, nil
		}
	}
	return hostViewLink{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitHost.view.notChangeRequest.body",
		map[string]string{"changeRequests": hosts.ChangeRequestViewLabels()})
}

// Open opens the view window for a change request link or an issue key or link, or focuses the one
// already open.
func (s *HostViewService) Open(ctx context.Context, args HostViewOpenArgs) (HostViewOpen, error) {
	raw := strings.TrimSpace(args.URL)
	if len(raw) <= maxHostViewURLBytes {
		if hosts := s.hosts(); hosts != nil {
			if p, d, repo, n, ok := hosts.ParseChangeRequestLink(ctx, raw); ok {
				return s.open(hostViewLink{p: p, details: d, repo: repo, number: n})
			}
		}
		if p, ref, ok := s.issueProvider(raw); ok {
			return s.openIssue(ctx, p, ref)
		}
	}
	return HostViewOpen{}, s.notViewable()
}

// issueProvider resolves raw to a tracker that can show an issue view.
func (s *HostViewService) issueProvider(raw string) (issuetracker.Provider, issuetracker.Ref, bool) {
	p, ref, ok := s.Trackers.ParseRef(raw)
	if !ok {
		return nil, issuetracker.Ref{}, false
	}
	if _, ok := p.(issuetracker.IssueDetails); !ok {
		return nil, issuetracker.Ref{}, false
	}
	return p, ref, true
}

func (s *HostViewService) notViewable() error {
	issues := s.Trackers.IssueViewLabels()
	if issues == "" {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitHost.view.notChangeRequest.body",
			map[string]string{"changeRequests": s.hosts().ChangeRequestViewLabels()})
	}
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.issueTracker.view.notViewable.body",
		map[string]string{"changeRequests": s.hosts().ChangeRequestViewLabels(), "issues": issues})
}

// OpenHostViewIssue opens the issue view for a key or link. It is a function, like
// OpenHostViewChangeRequest, so it is not bound.
func OpenHostViewIssue(ctx context.Context, s *HostViewService, raw string) (HostViewOpen, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) <= maxHostViewURLBytes {
		if p, ref, ok := s.issueProvider(raw); ok {
			return s.openIssue(ctx, p, ref)
		}
	}
	return HostViewOpen{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.issueTracker.view.notIssue.body",
		map[string]string{"issues": s.Trackers.IssueViewLabels()})
}

// openIssue identifies the issue by signed-in site and key, so a bare key and its link share a window.
func (s *HostViewService) openIssue(ctx context.Context, p issuetracker.Provider, ref issuetracker.Ref) (HostViewOpen, error) {
	site := ref.Site
	if site == "" {
		site = p.Status(ctx, false).Site
	}
	terms := p.Terms()
	target := HostViewTarget{Kind: HostViewKindIssue, Provider: p.ID(), Tracker: &terms, Key: ref.Key}
	if site != "" {
		target.URL = "https://" + site + "/browse/" + ref.Key
	}
	id := "issue|" + p.ID() + "|" + strings.ToLower(site) + "|" + ref.Key
	title := i18n.T("backend.space.issueTracker.view.window.title", map[string]string{"key": ref.Key, "label": terms.Label})
	out, err := s.openTarget(id, target, title)
	if err == nil && out.Opened {
		go s.prefetchIssue(ctx, p, ref)
	}
	return out, err
}

// prefetchIssue starts the tracker read while the new window boots, so its first Issue call finds the
// result saved. The read outlives ctx; its error is dropped, the window's own call reports it.
func (s *HostViewService) prefetchIssue(ctx context.Context, p issuetracker.Provider, ref issuetracker.Ref) {
	_, _ = p.(issuetracker.IssueDetails).IssueDetail(context.WithoutCancel(ctx), ref, true)
}

// OpenHostViewChangeRequest opens the PR view for repo r and change request n of provider p. It is a
// function, not a method, because its arguments are not wire types and the bindings would reject it.
func OpenHostViewChangeRequest(s *HostViewService, p githost.Provider, r githost.Repo, n int) (HostViewOpen, error) {
	d, ok := p.(githost.ChangeRequestDetails)
	if !ok {
		return HostViewOpen{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitHost.view.unavailable.body", nil)
	}
	return s.open(hostViewLink{p: p, details: d, repo: r, number: n})
}

func (s *HostViewService) open(l hostViewLink) (HostViewOpen, error) {
	terms := l.p.Terms()
	info := githost.Describe(l.p)
	target := HostViewTarget{
		Kind: HostViewKindChangeRequest, URL: l.p.ChangeRequestURL(l.repo, l.number), Provider: l.p.ID(),
		Host: &info, Repo: l.repo.Owner + "/" + l.repo.Name, Number: l.number,
	}
	title := i18n.T("backend.space.gitHost.view.window.title", map[string]string{
		"short": terms.ChangeRequestShort, "number": strconv.Itoa(l.number), "repo": target.Repo,
	})
	return s.openTarget(l.identity(), target, title)
}

// openTarget focuses the window of identity, or creates one showing target.
func (s *HostViewService) openTarget(id string, target HostViewTarget, title string) (HostViewOpen, error) {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	if s.OpenWindow == nil || s.FocusWindow == nil || s.Windows == nil || s.Targets == nil {
		return HostViewOpen{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitHost.view.unavailable.body", nil)
	}
	if key, ok := s.Targets.KeyFor(id); ok {
		if s.FocusWindow(key) {
			return HostViewOpen{Key: key}, nil
		}
		s.Targets.Forget(key)
		if err := s.Windows.Delete(key); err != nil {
			slog.Warn("hostview: drop stale window row", "scope", "hostview", "key", key, "err", err)
		}
	}
	records, err := s.Windows.List()
	if err != nil {
		return HostViewOpen{}, ipcerr.InternalErr(err)
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	key := HostWindowKeyPrefix + ids.NewID()
	if err := s.Windows.Create(model.WindowRecord{Key: key, Order: order}); err != nil {
		return HostViewOpen{}, ipcerr.InternalErr(err)
	}
	s.Targets.Put(key, id, target)
	s.OpenWindow(shell.WindowRecord{Key: key, Order: order})
	if s.SetWindowTitle != nil {
		s.SetWindowTitle(key, title)
	}
	return HostViewOpen{Key: key, Opened: true}, nil
}

// Issue reads the issue detail. A tracker failure is data (Kind), not an error: the window shows the
// reason and offers a retry. acli gives no rate-limit signal and every read is bounded, so no breaker.
func (s *HostViewService) Issue(ctx context.Context, args HostViewIssueArgs) (HostViewIssue, error) {
	raw := strings.TrimSpace(args.Ref)
	p, ref, ok := s.issueProvider(raw)
	if !ok || len(raw) > maxHostViewURLBytes {
		return HostViewIssue{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.issueTracker.view.notIssue.body",
			map[string]string{"issues": s.Trackers.IssueViewLabels()})
	}
	d, err := p.(issuetracker.IssueDetails).IssueDetail(ctx, ref, args.Fresh)
	switch {
	case err == nil:
		out := HostViewIssue{Kind: HostViewOK, Detail: &d}
		if _, ok := p.(issuetracker.IssueComments); ok {
			out.Compose = issueCompose()
		}
		return out, nil
	case errors.Is(err, issuetracker.ErrNotFound):
		return HostViewIssue{Kind: HostViewGone, Reason: issuetracker.ReasonNotFound}, nil
	case ctx.Err() != nil:
		return HostViewIssue{Kind: HostViewUnavailable, Reason: issuetracker.ReasonTimeout}, nil
	}
	return HostViewIssue{Kind: HostViewUnavailable, Reason: issuetracker.Reason(err)}, nil
}

// WindowTarget returns what the window with this key shows, null when it is no host view window.
func (s *HostViewService) WindowTarget(_ context.Context, args HostViewWindowArgs) (*HostViewTarget, error) {
	if err := validateAdeItemID(args.WindowKey, "windowKey"); err != nil {
		return nil, err
	}
	if s.Targets == nil {
		return nil, nil
	}
	return s.Targets.Get(args.WindowKey), nil
}

// ChangeRequest reads the change request detail. A host failure is data (Kind), not an error: the
// window shows the reason and offers a retry. A rate limit arms a per-repository breaker.
func (s *HostViewService) ChangeRequest(ctx context.Context, args HostViewChangeRequestArgs) (HostViewChangeRequest, error) {
	l, err := s.parse(ctx, args.URL)
	if err != nil {
		return HostViewChangeRequest{}, err
	}
	key := l.repoIdentity()
	if st, armed := s.breaker.armed(key, s.now()); armed {
		return HostViewChangeRequest{Kind: HostViewUnavailable, Status: st}, nil
	}
	d, st := l.details.ChangeRequestDetail(ctx, l.repo, l.number)
	switch {
	case ctx.Err() != nil:
		return HostViewChangeRequest{Kind: HostViewUnavailable, Status: githost.Status{Kind: githost.KindNotFound, Reason: "cancelled"}}, nil
	case st.OK():
		out := HostViewChangeRequest{Kind: HostViewOK, Detail: &d, Status: st}
		if w, ok := l.p.(githost.ChangeRequestComments); ok {
			out.Compose = changeRequestCompose(w.ReplyKinds(l.repo))
		}
		return out, nil
	case st.Gone:
		return HostViewChangeRequest{Kind: HostViewGone, Status: st}, nil
	case st.RateLimited:
		until := st.ResetAt
		if until.IsZero() {
			until = s.now().Add(hostViewBreakerFallback)
		}
		s.breaker.arm(key, until, st)
	}
	return HostViewChangeRequest{Kind: HostViewUnavailable, Status: st}, nil
}

// HostViewTargets maps window keys to what they show. It is not a service: Forget is not bound.
type HostViewTargets struct {
	mu         sync.Mutex
	byKey      map[string]hostViewTargetEntry
	byIdentity map[string]string
}

type hostViewTargetEntry struct {
	identity string
	target   HostViewTarget
}

func (t *HostViewTargets) Put(key, identity string, target HostViewTarget) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byKey == nil {
		t.byKey, t.byIdentity = map[string]hostViewTargetEntry{}, map[string]string{}
	}
	t.byKey[key] = hostViewTargetEntry{identity: identity, target: target}
	t.byIdentity[identity] = key
}

func (t *HostViewTargets) Get(key string) *HostViewTarget {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.byKey[key]
	if !ok {
		return nil
	}
	target := e.target
	return &target
}

func (t *HostViewTargets) KeyFor(identity string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key, ok := t.byIdentity[identity]
	return key, ok
}

// Forget drops key's target; a window close calls it.
func (t *HostViewTargets) Forget(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.byKey[key]; ok {
		delete(t.byKey, key)
		if t.byIdentity[e.identity] == key {
			delete(t.byIdentity, e.identity)
		}
	}
}

// hostViewBreaker stops calls to one repository's host while it is rate limited.
type hostViewBreaker struct {
	mu    sync.Mutex
	until map[string]hostViewBreakerEntry
}

type hostViewBreakerEntry struct {
	until  time.Time
	status githost.Status
}

func (b *hostViewBreaker) arm(key string, until time.Time, st githost.Status) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.until == nil {
		b.until = map[string]hostViewBreakerEntry{}
	}
	b.until[key] = hostViewBreakerEntry{until: until, status: st}
}

func (b *hostViewBreaker) armed(key string, now time.Time) (githost.Status, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.until[key]
	if !ok {
		return githost.Status{}, false
	}
	if !now.Before(e.until) {
		delete(b.until, key)
		return githost.Status{}, false
	}
	return e.status, true
}
