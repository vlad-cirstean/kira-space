package memory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"golang.org/x/sync/semaphore"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

const (
	maxItems = 20
	// MaxItems is the most facts one store call takes; the MCP tool description states it.
	MaxItems = maxItems
	// MaxGroups is the most groups one store call takes; the MCP tool descriptions state it.
	MaxGroups         = maxGroups
	maxClarifications = 10
	maxClarifyLen     = 1000
	maxPipelines      = 2
	maxStaleRetries   = 2

	maxGroups      = 5
	maxGroupKeyLen = 300
	maxGroupTitle  = 200
	maxSessionLen  = 128
	maxSourceTag   = 32

	// DefaultContextLimit and MaxContextLimit bound the facts per group of a context read.
	DefaultContextLimit = 20
	MaxContextLimit     = 100
)

var (
	sourceTagPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	sessionPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

// ErrInvalid marks a request the caller can fix; its message is safe to show verbatim.
var ErrInvalid = errors.New("invalid memory request")

// Item is one fact to store with its provenance. Origin and Scope are required. SourceTag is
// required for origin external and forbidden otherwise; Project is required for scope project
// (the caller or the server fills a default) and forbidden otherwise.
type Item struct {
	Fact      string `json:"fact"`
	Reason    string `json:"reason"`
	Origin    string `json:"origin"`
	SourceTag string `json:"sourceTag,omitempty"`
	Scope     string `json:"scope"`
	Project   string `json:"project,omitempty"`
}

type Clarification struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type StoreRequest struct {
	Items          []Item
	Clarifications []Clarification
	// Session is the Claude Code session that wrote the facts; empty stores NULL.
	Session string
	// Groups the facts join, caller kinds only: task, document, thread. The session and request
	// groups are added by the service.
	Groups []GroupRef
	Source string // "mcp" | "ui" | "import"
	// SourceRef names the import file for Source "import"; it must be empty otherwise.
	SourceRef string
	// NoChallenge stores the items the gate accepts even when it challenges others: the source is an
	// outside one nobody can ask. Challenged items come back in Challenges with status "stored".
	NoChallenge bool
	// ImportPath is the import file's job-relative path (import mode only). It pins every committed
	// add or update to a source locator: the item's reason when it is one for this path, else the
	// bare "Stated in <path>". Gate and reconcile may reword reasons; provenance must not drift.
	ImportPath string
	// Progress, when set, is told the stage: "checking", "reconciling", "saving".
	Progress func(stage string)
}

type ItemChallenge struct {
	Index     int      `json:"index"`
	Fact      string   `json:"fact"`
	Questions []string `json:"questions"`
}

type FactOutcome struct {
	Fact   string `json:"fact"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	Action string `json:"action"` // add | update | noop | failed
	// ID is the version id; LineageID is the fact id agents cite.
	ID         string `json:"id"`
	LineageID  string `json:"lineageId"`
	Version    int    `json:"version"`
	PreviousID string `json:"previousId"`
	Why        string `json:"why"`
	Error      string `json:"error"`
}

type StoreResult struct {
	Status     string          `json:"status"` // "stored" | "challenged"
	Challenges []ItemChallenge `json:"challenges"`
	Outcomes   []FactOutcome   `json:"outcomes"`
}

// Service is the one entry point for both callers: the stdio MCP server and Kira Space's bridge.
type Service struct {
	store  *Store
	runner Runner
	sem    *semaphore.Weighted
	opts   ServiceOptions

	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	backfilling atomic.Bool
}

// ServiceOptions are NewService's optional collaborators; the zero value is keyword-only search.
type ServiceOptions struct {
	Embedder Embedder
	// OnChange runs after a store call commits something.
	OnChange func()
	// OnSemantic runs when embedding progress changes (a backfill batch landed).
	OnSemantic func()
	// Prompts resolves the gate and reconcile prompts; nil means the registry defaults.
	Prompts llmprompts.Source
}

func NewService(store *Store, runner Runner, opts ServiceOptions) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	if opts.Prompts == nil {
		opts.Prompts = llmprompts.Defaults()
	}
	return &Service{store: store, runner: runner, sem: semaphore.NewWeighted(maxPipelines), opts: opts, ctx: ctx, cancel: cancel}
}

// Close stops the backfill and closes the embedder when it has a Close method. It does not close
// the store.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
	if c, ok := s.opts.Embedder.(io.Closer); ok {
		_ = c.Close()
	}
}

// ValidSession reports whether s is usable as a session id.
func ValidSession(s string) bool { return sessionPattern.MatchString(s) }

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func validateItem(i int, it Item) error {
	if n := utf8.RuneCountInString(it.Fact); n < 1 || n > MaxFactLen {
		return invalid("item %d: fact must be 1 to %d characters", i, MaxFactLen)
	}
	if n := utf8.RuneCountInString(it.Reason); n < 1 || n > MaxReasonLen {
		return invalid("item %d: reason must be 1 to %d characters", i, MaxReasonLen)
	}
	return checkProvenance(fmt.Sprintf("item %d", i), it)
}

// CheckProvenance validates the provenance fields alone (a job's, a doc's) with the rules every
// stored item obeys. The error is ErrInvalid.
func CheckProvenance(origin, sourceTag, scope, project string) error {
	return checkProvenance("provenance", Item{Origin: origin, SourceTag: sourceTag, Scope: scope, Project: project})
}

func checkProvenance(label string, it Item) error {
	switch it.Origin {
	case OriginUser, OriginAgent:
		if it.SourceTag != "" {
			return invalid("%s: sourceTag is only for origin %q", label, OriginExternal)
		}
	case OriginExternal:
		if !sourceTagPattern.MatchString(it.SourceTag) {
			return invalid("%s: origin %q needs a sourceTag of 1 to %d characters from a-z, 0-9 and -", label, OriginExternal, maxSourceTag)
		}
	default:
		return invalid("%s: origin must be %q, %q or %q", label, OriginUser, OriginAgent, OriginExternal)
	}
	switch it.Scope {
	case ScopeProject:
		if it.Project == "" {
			return invalid("%s: scope %q needs a project (the project could not be detected)", label, ScopeProject)
		}
		if err := checkProject(it.Project); err != nil {
			return invalid("%s: %s", label, strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": "))
		}
	case ScopeCompany, ScopePersonal:
		if it.Project != "" {
			return invalid("%s: project is only for scope %q", label, ScopeProject)
		}
	default:
		return invalid("%s: scope must be %q, %q or %q", label, ScopeCompany, ScopeProject, ScopePersonal)
	}
	return nil
}

// CheckJobGroups validates the caller groups of an ingest job. One of the store's group slots is the
// file's own document group.
func CheckJobGroups(groups []GroupRef) error {
	if len(groups) > maxGroups-1 {
		return invalid("send at most %d groups; each file also joins its own document group", maxGroups-1)
	}
	return validateGroups(groups)
}

func validateGroups(groups []GroupRef) error {
	if len(groups) > maxGroups {
		return invalid("send at most %d groups", maxGroups)
	}
	for i, g := range groups {
		switch g.Kind {
		case GroupTask, GroupDocument, GroupThread:
		default:
			return invalid("group %d: kind must be %q, %q or %q", i, GroupTask, GroupDocument, GroupThread)
		}
		if n := utf8.RuneCountInString(g.Key); n < 1 || n > maxGroupKeyLen {
			return invalid("group %d: key must be 1 to %d characters", i, maxGroupKeyLen)
		}
		if utf8.RuneCountInString(g.Title) > maxGroupTitle {
			return invalid("group %d: title must be at most %d characters", i, maxGroupTitle)
		}
	}
	return nil
}

func validateRequest(req StoreRequest) error {
	if n := len(req.Items); n < 1 || n > maxItems {
		return invalid("send 1 to %d items", maxItems)
	}
	for i, it := range req.Items {
		if err := validateItem(i, it); err != nil {
			return err
		}
	}
	if len(req.Clarifications) > maxClarifications {
		return invalid("send at most %d clarifications", maxClarifications)
	}
	for _, c := range req.Clarifications {
		if utf8.RuneCountInString(c.Question) > maxClarifyLen || utf8.RuneCountInString(c.Answer) > maxClarifyLen {
			return invalid("clarifications must be at most %d characters", maxClarifyLen)
		}
	}
	if req.Session != "" && !sessionPattern.MatchString(req.Session) {
		return invalid("session must be 1 to %d characters from A-Z, a-z, 0-9 and ._:-", maxSessionLen)
	}
	if err := validateGroups(req.Groups); err != nil {
		return err
	}
	switch req.Source {
	case SourceMCP, SourceUI:
		if req.SourceRef != "" {
			return invalid("source %q takes no source ref", req.Source)
		}
	case SourceImport:
		if n := utf8.RuneCountInString(req.SourceRef); n < 1 || n > 64 {
			return invalid("source %q needs a source ref of 1 to 64 characters", req.Source)
		}
	default:
		return invalid("unknown source %q", req.Source)
	}
	return nil
}

// normalizeItems trims and lowercases each sourceTag and trims the project, on a copy.
func normalizeItems(in []Item) []Item {
	out := make([]Item, len(in))
	for i, it := range in {
		it.SourceTag = strings.ToLower(strings.TrimSpace(it.SourceTag))
		it.Project = strings.TrimSpace(it.Project)
		out[i] = it
	}
	return out
}

func (s *Service) progress(req StoreRequest, stage string) {
	if req.Progress != nil {
		req.Progress(stage)
	}
}

// decide runs the read-only front of the pipeline: gate, then candidate lookup and reconcile. The
// caller holds the semaphore. A challenge returns early with Status "challenged" and no works,
// unless the request is NoChallenge: then the accepted items go on.
func (s *Service) decide(ctx context.Context, req StoreRequest) (works []*work, rev int64, res StoreResult, err error) {
	s.progress(req, "checking")
	verdicts, err := s.runGate(ctx, req)
	if err != nil {
		return nil, 0, StoreResult{}, err
	}
	res = StoreResult{Challenges: []ItemChallenge{}, Outcomes: []FactOutcome{}}
	for _, v := range verdicts {
		if v.Verdict == "challenge" {
			res.Challenges = append(res.Challenges, ItemChallenge{Index: v.Index, Fact: req.Items[v.Index].Fact, Questions: v.Questions})
		}
	}
	if len(res.Challenges) > 0 && !req.NoChallenge {
		res.Status = "challenged"
		return nil, 0, res, nil
	}
	res.Status = "stored"

	for _, v := range verdicts {
		it := req.Items[v.Index]
		pinned := importReason(req, it)
		for _, f := range v.Facts {
			works = append(works, &work{Fact: f.Fact, Kind: f.Kind, Reason: cmp.Or(pinned, f.Reason), pinned: pinned, Keywords: f.Keywords,
				Origin: it.Origin, SourceTag: it.SourceTag, Scope: it.Scope, Project: it.Project})
		}
	}

	s.progress(req, "reconciling")
	if rev, err = s.store.Revision(ctx); err != nil {
		return nil, 0, StoreResult{}, err
	}
	var pending []*work
	for _, w := range works {
		need, err := s.prepare(ctx, w)
		if err != nil {
			return nil, 0, StoreResult{}, err
		}
		if need {
			pending = append(pending, w)
		}
	}
	if err := s.reconcile(ctx, pending); err != nil {
		return nil, 0, StoreResult{}, err
	}
	return works, rev, res, nil
}

// importReason is the pinned reason of an import-mode item (see StoreRequest.ImportPath), "" outside it.
func importReason(req StoreRequest, it Item) string {
	if req.Source != SourceImport || req.ImportPath == "" {
		return ""
	}
	if _, ok := memfmt.ParseLocator(it.Reason, req.ImportPath); ok {
		return it.Reason
	}
	return memfmt.Locator(req.ImportPath, "")
}

// Store gates, reconciles and commits. A challenge stores nothing. A runner error fails the whole
// request; per-fact failures land in Outcomes.
func (s *Service) Store(ctx context.Context, req StoreRequest) (StoreResult, error) {
	req.Items = normalizeItems(req.Items)
	if err := validateRequest(req); err != nil {
		return StoreResult{}, err
	}
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return StoreResult{}, err
	}
	defer s.sem.Release(1)

	works, rev, res, err := s.decide(ctx, req)
	if err != nil || res.Status == "challenged" {
		return res, err
	}

	s.progress(req, "saving")
	requestID := ids.NewID()
	groups := requestGroups(req, requestID)
	committed := false
	for i := range works {
		out := s.commit(ctx, req, requestID, groups, works[i:], &rev)
		committed = committed || out.Action != ActionFailed
		res.Outcomes = append(res.Outcomes, out)
	}
	if committed {
		if s.opts.OnChange != nil {
			s.opts.OnChange()
		}
		s.kickBackfill()
	}
	return res, nil
}

// requestGroups are the groups every fact of a call joins: its session, the call itself, and the
// caller's own groups.
func requestGroups(req StoreRequest, requestID string) []GroupRef {
	groups := make([]GroupRef, 0, len(req.Groups)+2)
	if req.Session != "" {
		groups = append(groups, GroupRef{Kind: GroupSession, Key: req.Session})
	}
	groups = append(groups, GroupRef{Kind: GroupRequest, Key: requestID})
	return append(groups, req.Groups...)
}

// commit applies the first work's decision. When the store moved underneath, it re-decides every
// work not yet committed, since each was decided against the older revision.
func (s *Service) commit(ctx context.Context, req StoreRequest, requestID string, groups []GroupRef, rest []*work, rev *int64) FactOutcome {
	w := rest[0]
	fail := func(err error) FactOutcome {
		return FactOutcome{Fact: w.Fact, Kind: w.kind(), Reason: w.Reason, Action: ActionFailed, Error: err.Error()}
	}
	for attempt := 0; ; attempt++ {
		if w.dec.Err != nil {
			return fail(w.dec.Err)
		}
		vec, model := s.vectorFor(ctx, w)
		if w.pinned != "" && (w.dec.Action == ActionAdd || w.dec.Action == ActionUpdate) {
			w.dec.Reason = w.pinned
		}
		r, err := s.store.commitFact(ctx, commitInput{
			Revision: *rev, Action: w.dec.Action, TargetID: w.dec.Target, Fact: w.dec.Fact, Kind: w.kind(), Reason: w.dec.Reason,
			Keywords: w.Keywords, Origin: w.Origin, SourceTag: w.SourceTag, Scope: w.Scope, Project: w.Project,
			SessionID: req.Session, Source: req.Source, SourceRef: req.SourceRef, RequestID: requestID, Why: w.dec.Why,
			Groups: groups, Vec: vec, Model: model,
		})
		if err == nil {
			*rev = r.Revision
			return FactOutcome{Fact: w.dec.factOr(w.Fact), Kind: w.kind(), Reason: w.dec.reasonOr(w.Reason), Action: w.dec.Action,
				ID: r.ID, LineageID: r.LineageID, Version: r.Version, PreviousID: r.PreviousID, Why: w.dec.Why}
		}
		if !errors.Is(err, errStale) {
			return fail(err)
		}
		if attempt == maxStaleRetries {
			return fail(errStale)
		}
		if err := s.redecide(ctx, rest, rev); err != nil {
			return fail(err)
		}
	}
}

// redecide refreshes the revision and reruns candidate retrieval and one reconcile for every work given.
func (s *Service) redecide(ctx context.Context, works []*work, rev *int64) error {
	var err error
	if *rev, err = s.store.Revision(ctx); err != nil {
		return err
	}
	var pending []*work
	for _, w := range works {
		need, err := s.prepare(ctx, w)
		if err != nil {
			return err
		}
		if need {
			pending = append(pending, w)
		}
	}
	return s.reconcile(ctx, pending)
}

func (d decision) factOr(fallback string) string {
	if d.Fact != "" {
		return d.Fact
	}
	return fallback
}

func (d decision) reasonOr(fallback string) string {
	if d.Reason != "" {
		return d.Reason
	}
	return fallback
}

func (s *Service) Recent(ctx context.Context, limit int) ([]Memory, error) {
	return s.store.Recent(ctx, limit)
}

func (s *Service) History(ctx context.Context, id string) (History, error) {
	return s.store.Lineage(ctx, id)
}

// Context returns a fact with the other facts stored in its groups. id is a fact id or version id;
// limit caps the facts per group (0 means DefaultContextLimit).
func (s *Service) Context(ctx context.Context, id string, limit int) (Context, error) {
	switch {
	case limit <= 0:
		limit = DefaultContextLimit
	case limit > MaxContextLimit:
		limit = MaxContextLimit
	}
	return s.store.Context(ctx, id, limit)
}

// Projects lists the project keys of current facts.
func (s *Service) Projects(ctx context.Context) ([]string, error) { return s.store.Projects(ctx) }

// ImportProvenance reads the job and file an import-mode store call belongs to.
func (s *Service) ImportProvenance(ctx context.Context, ref string) (ImportProvenance, error) {
	return s.store.ImportProvenance(ctx, ref)
}
