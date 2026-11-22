package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

// ApprovalTimeout bounds how long a prompt-mode query waits for a human answer before giving up.
const ApprovalTimeout = prompts.AnswerWait

// approvalWindow renders ApprovalTimeout for tool error texts.
func approvalWindow() string {
	m := int(ApprovalTimeout / time.Minute)
	if m == 1 {
		return "1 minute"
	}
	return strconv.Itoa(m) + " minutes"
}

// maxPendingApprovals caps the queue: an MCP client is a program and can call in a loop, and
// nothing else bounds how many run_query calls can be in-flight at once.
const maxPendingApprovals = 50

// Held result caps (P536). A held result lives in memory until the user answers, so both are bounded:
// 8 MiB is far above a 2000-row result of normal width and still parses in the webview; 64 MiB bounds the
// worst case of maxPendingApprovals results.
const (
	maxHeldResultBytes = 8 << 20
	maxHeldTotalBytes  = 64 << 20
)

// ApprovalOutcome is ApprovalBroker.Request's own vocabulary — never a Go error: a human decision
// is a value, not a failure.
type ApprovalOutcome int

const (
	ApprovalApproved ApprovalOutcome = iota
	ApprovalDenied
	ApprovalTimedOut
	ApprovalAbandoned // the server stopped, or the MCP client went away (ctx cancelled)
	ApprovalQueueFull // maxPendingApprovals already waiting; no human saw this request
	ApprovalTooLarge  // the held result is over the per-result cap; no human saw it
	ApprovalHeldFull  // held results already fill the total cap; no human saw this one
)

// String is the outcome's audit-trail name.
func (o ApprovalOutcome) String() string {
	switch o {
	case ApprovalApproved:
		return "approved"
	case ApprovalDenied:
		return "denied"
	case ApprovalTimedOut:
		return "timedOut"
	case ApprovalAbandoned:
		return "abandoned"
	case ApprovalQueueFull:
		return "queueFull"
	case ApprovalTooLarge:
		return "tooLarge"
	case ApprovalHeldFull:
		return "heldFull"
	}
	return "unknown"
}

// ApprovalObserver sees every request that passes RequestChoice. ApprovalRaised runs once the request is
// queued; ApprovalResolved runs once with the final outcome. A request refused before queueing (queue
// full, result too large) has no ApprovalRaised and a req.RequestID of "". Calls run outside the
// broker's lock.
type ApprovalObserver interface {
	ApprovalRaised(req ApprovalRequest)
	ApprovalResolved(req ApprovalRequest, outcome ApprovalOutcome)
}

type observerBox struct{ o ApprovalObserver }

// ApprovalChoice is what an approver ticked on the item. Zero for every kind but run_query's pre-run items.
type ApprovalChoice struct {
	ReviewResult bool `json:"reviewResult"`
}

type approvalResolution struct {
	outcome ApprovalOutcome
	choice  ApprovalChoice
	inputs  SuppliedInputs
}

// ApprovalInput is a value an approval asks the user to type. Needed says the item cannot be approved
// without it; a stored one may be sent to replace what Kira holds.
type ApprovalInput struct {
	Name   string
	Label  string
	Needed bool
}

// Limits on a typed value.
const maxInputBytes = 8 << 10

// SuppliedInputs are the values the approver typed, by input name, trimmed. They travel on the approve
// answer only and are zeroed by Wipe once Kira has stored them or the call ended. Go strings decoded from
// the answer JSON cannot be zeroed, so the wipe covers the copies Kira owns (stated in ARCHITECTURE).
type SuppliedInputs map[string]Supplied

// Supplied is one typed value. Once says the user chose to use it for this request only: it is never put in
// the kept store.
type Supplied struct {
	Value []byte
	Once  bool
}

// Take returns name's value and forgets it; the caller owns the bytes. ok is false when none was typed.
func (si SuppliedInputs) Take(name string) (Supplied, bool) {
	v, ok := si[name]
	delete(si, name)
	return v, ok
}

// Wipe zeroes every value still held.
func (si SuppliedInputs) Wipe() {
	for name, v := range si {
		clear(v.Value)
		delete(si, name)
	}
}

// answerOptions is the approve answer's options: the choice plus the typed inputs.
type answerOptions struct {
	ReviewResult bool              `json:"reviewResult"`
	Inputs       map[string]string `json:"inputs"`
	// InputModes says per typed input whether to keep it ("keep") or use it for this request only ("once").
	// Absent means once.
	InputModes map[string]string `json:"inputModes"`
}

// ApprovalActionResult is what Approve/Deny report — also never an error.
type ApprovalActionResult int

const (
	ApprovalActionResolved ApprovalActionResult = iota
	ApprovalActionAlreadyResolved
)

// ApprovalReason is why this request was raised — M2's permission gate, or an agent's exemption request.
// Both are the same question ("should this run?") asked for different reasons, so they share one
// queue and one dialog rather than competing for the user's attention (M3 §6.2).
type ApprovalReason string

const (
	ApprovalReasonPermission ApprovalReason = "permission"
	// ApprovalReasonExemption is an agent asking to lift limits for one statement.
	ApprovalReasonExemption ApprovalReason = "exemption"
)

// ApprovalStage says what the human is asked: whether a statement may run (query) or whether the
// result of a statement that already ran may reach the AI client (response).
type ApprovalStage string

const (
	ApprovalStageQuery    ApprovalStage = "query"
	ApprovalStageResponse ApprovalStage = "response"
)

// ApprovalResponse is the held result a response-stage request asks about. Text is the
// exact TextContent the client receives on Send; it stays out of queue snapshots (which fan out to
// every window) and is fetched by request id through ResponseFor.
type ApprovalResponse struct {
	Text    string
	IsError bool
	// Bytes is what the held-result caps count: Text, Raw and the script source.
	Bytes int
	// Requested says the approver asked for the review on the pre-run item, not a connection setting.
	Requested bool
	// Raw is the response the script read, set only when a script produced Text.
	Raw string
	// Script is the script that produced Text from Raw; nil without one.
	Script *HeldScript
	// Keep are the values the script will keep once the user sends the result: previews, never the
	// agent's text. Empty without kira.storeValue.
	Keep []HeldKeep
	// staged is the run that holds the values of Keep until the result is sent; only StagedSecret reads them.
	staged *keptRun
}

// HeldKeep is one value a held result will keep. Preview is empty for a secret.
type HeldKeep struct {
	Name             string
	Type             string
	Items, Bytes     int
	Secret           bool
	Preview          string
	PreviewTruncated bool
	// Blocked says the script read a one-time value, so this value is not kept.
	Blocked bool
}

// HeldScript is the script a held result came from.
type HeldScript struct {
	Source string
	// Hash is jsengine.SourceHash of Source.
	Hash string
}

// ApprovalPlanIssue is one Issue as the dialog renders it — plan evidence, never the whole tree.
type ApprovalPlanIssue struct {
	Severity string
	Code     string
	Message  string
}

// ApprovalPlan is the plan evidence the dialog shows for a heavy-query (or plan-carrying
// permission) request — never the whole tree (M3 §6.2). Issues is capped by the caller
// (bridge/dbmcp.go's own wire projection), with IssuesOmitted carrying the remainder.
type ApprovalPlan struct {
	EstimatedRowsRead *float64
	ThresholdRows     int
	OverThreshold     bool
	Issues            []ApprovalPlanIssue
	IssuesOmitted     int
}

// ApprovalTarget names the database a request would act on, so the dialog can show it prominently.
type ApprovalTarget struct {
	Color    string
	ReadOnly bool
	// Host is "host:port", "host", or "" (URI mode or file kinds).
	Host string
	// Database is the configured database (SQLite: the file path); "" when unset.
	Database string
	// Path is the decoded run_query/explain path; empty when the call passed none.
	Path []model.PathSegment
}

// targetOf projects a connection summary and the call's encoded path. A path that fails to decode
// yields an empty Path: the gate already accepted it, so this never fails.
func targetOf(summary model.ConnectionSummary, path string) ApprovalTarget {
	t := ApprovalTarget{Color: summary.Color, ReadOnly: summary.ReadOnly}
	if summary.Host != nil && *summary.Host != "" {
		t.Host = *summary.Host
		if summary.Port != nil {
			t.Host += ":" + strconv.Itoa(*summary.Port)
		}
	}
	if summary.Database != nil {
		t.Database = *summary.Database
	}
	if path != "" {
		if np, err := model.DecodePath(summary.ID, path); err == nil {
			t.Path = np.Segments
		}
	}
	return t
}

// ApprovalRequest is one queued query — what the dialog renders. RequestID, EnqueuedAt and
// ExpiresAt are minted by Request, never by the caller.
type ApprovalRequest struct {
	RequestID      string
	ConnectionID   string
	ConnectionName string
	Kind           string
	// Class is what the statement looks like (the detected class); Mode is what the caller declared
	// (read, write or ddl) and decides the connection and the permission gate.
	Class adapters.OpClass
	Mode  string
	// Enforcement says what backs mode read on this kind: session, app or none. Read items only.
	Enforcement string
	Statement   string
	// Explain marks a plan request: the card shows the agent SQL as an explain.
	Explain bool
	Target  ApprovalTarget
	// Stage defaults to ApprovalStageQuery when empty.
	Stage ApprovalStage
	// Response is set only for ApprovalStageResponse.
	Response   *ApprovalResponse
	EnqueuedAt time.Time
	ExpiresAt  time.Time
	// Reason is "permission" for a mode's prompt verdict, "exemption" for a limit exemption request.
	Reason ApprovalReason
	// Plan is the query-plan evidence (M3 §6.1/§6.2), nil when no plan was made (the check is off, or
	// the statement was not explainable).
	Plan *ApprovalPlan
	// Cluster is set for a Kubernetes read; Target is then empty.
	Cluster *ApprovalCluster
	// Subject is the approvals kind key. Empty derives it: kube.read with a Cluster (kube.response at the
	// response stage), db.response at the response stage, else db.query.
	Subject string
	// Origin says which client asked; the tools copy it from the call's context.
	Origin ApprovalOrigin
	// Script is set for the script.run subject; Target is then empty.
	Script *ApprovalScript
	// API is set for the api.request subject; Target is then empty.
	API *agentapi.Planned
	// APIScript is the scripts of an api.request, shown beside the request.
	APIScript *apiScripts
	// APIReview is set for the api.response subject: what the item shows beside the held response.
	APIReview *apiReviewMeta
	// Params are the kept names bound to the statement's placeholders, Placeholders how each shows on the card
	// and Kept the entries as they stood when the statement was checked. Set on a db.query, db.exemption or
	// kube.read item.
	Params       []string
	Placeholders []string
	Kept         []agentapi.KeptUse
	// QueryScript is the result transform of a run_query, shown beside the statement.
	QueryScript *apiScripts
	// Exemption is set for the db.exemption subject.
	Exemption *ApprovalExemption
	// Inputs are the values the card asks the user to type (api.request); labels only, never values.
	Inputs []ApprovalInput
	// KeepMinutes is how long a typed value is kept when the user picks "keep"; shown on the card.
	KeepMinutes int
	// OfferReview marks a pre-run item that shows "review the result" to the approver; ReviewOn says the
	// connection's settings already review this call, so the choice is moot.
	OfferReview bool
	ReviewOn    bool
}

// ApprovalExemption is what an exemption request adds: the agent's reason and every limit the
// user is asked to lift for the one statement.
type ApprovalExemption struct {
	Reason string
	Hits   []LimitHit
}

// ApprovalOrigin is what the stateless HTTP transport can tell about the caller.
type ApprovalOrigin struct {
	Client string
	Cwd    string
	// SessionID, TaskID, StepID and RunID are set only for a call on the session server.
	SessionID, TaskID, StepID, RunID string
	// CallID is the audit row of the tool call this request was raised in; empty when not audited.
	CallID string
}

func (r ApprovalRequest) subject() string {
	switch {
	case r.Subject != "":
		return r.Subject
	case r.Cluster != nil && r.Stage == ApprovalStageResponse:
		return approvals.KindKubeResponse
	case r.Cluster != nil:
		return approvals.KindKubeRead
	case r.Stage == ApprovalStageResponse:
		return approvals.KindDbResponse
	}
	return approvals.KindDbQuery
}

func (r ApprovalRequest) itemID() string { return r.subject() + ":" + r.RequestID }

type approvalEntry struct {
	req    ApprovalRequest
	result chan approvalResolution // buffered 1; exactly one send over the entry's lifetime.
}

// ApprovalBroker is the queue/deadline/injected-clock state machine for prompt-mode requests. Every
// pending request is one approvals.Item at once; Request blocks its own caller only and returns on its
// own answer, its deadline, or its ctx. The shared Queue (internal/approvals) carries the items to the
// windows and routes answers back through brokerOwner.
type ApprovalBroker struct {
	now func() time.Time
	// Queue is the app's approvals queue.
	Queue *approvals.Queue

	mu    sync.Mutex
	queue *notify.PendingQueue[*approvalEntry]
	// heldBytes totals the Response.Bytes of queued entries, under mu.
	heldBytes           int
	maxResult, maxTotal int
	// timeout is how long a request waits for an answer; ApprovalTimeout unless a test replaced it.
	timeout  time.Duration
	observer atomic.Pointer[observerBox]
}

// SetObserver wires the audit observer; call once at startup.
func (b *ApprovalBroker) SetObserver(o ApprovalObserver) { b.observer.Store(&observerBox{o}) }

// SubjectKey is the approvals kind key of the request.
func (r ApprovalRequest) SubjectKey() string { return r.subject() }

// NewApprovalBroker builds a broker over q and registers it as owner of its kinds; a nil q gets a
// private queue (tests).
func NewApprovalBroker(now func() time.Time, q *approvals.Queue) *ApprovalBroker {
	return newApprovalBrokerCaps(now, q, maxHeldResultBytes, maxHeldTotalBytes)
}

// newApprovalBrokerCaps is NewApprovalBroker with the held result caps injected (tests).
func newApprovalBrokerCaps(now func() time.Time, q *approvals.Queue, perResult, total int) *ApprovalBroker {
	if q == nil {
		q = approvals.New(nil)
	}
	b := &ApprovalBroker{now: now, Queue: q, queue: notify.NewPendingQueue[*approvalEntry](), maxResult: perResult, maxTotal: total, timeout: ApprovalTimeout}
	for _, kind := range []string{approvals.KindDbQuery, approvals.KindDbResponse, approvals.KindDbExemption, approvals.KindKubeRead, approvals.KindKubeResponse, approvals.KindScriptRun, approvals.KindAPIRequest, approvals.KindAPIResponse} {
		q.Register(kind, brokerOwner{b})
	}
	return b
}

// brokerOwner answers the broker's items.
type brokerOwner struct{ b *ApprovalBroker }

func (o brokerOwner) Answer(ref string, a approvals.Answer) error {
	if !a.Approve {
		// Typed values on a deny are discarded unread.
		if o.b.resolve(ref, ApprovalDenied, ApprovalChoice{}) == ApprovalActionAlreadyResolved {
			return ipcerr.NotFoundNamed("approval")
		}
		return nil
	}
	var opts answerOptions
	if len(a.Options) > 0 {
		dec := json.NewDecoder(bytes.NewReader(a.Options))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&opts); err != nil {
			return ipcerr.Contract("approval options are not understood")
		}
	}
	res, err := o.b.answer(ref, ApprovalChoice{ReviewResult: opts.ReviewResult}, opts.Inputs, opts.InputModes)
	if err != nil {
		return err
	}
	if res == ApprovalActionAlreadyResolved {
		return ipcerr.NotFoundNamed("approval")
	}
	return nil
}

// answer approves ref with choice and typed inputs. A choice or input the item does not take is a contract
// error and leaves the item pending; the check shares the lock with the lookup so a resolve cannot slip
// between them.
func (b *ApprovalBroker) answer(ref string, choice ApprovalChoice, typed, modes map[string]string) (ApprovalActionResult, error) {
	b.mu.Lock()
	entry, ok := b.entryLocked(ref)
	var supplied SuppliedInputs
	var err error
	if ok {
		supplied, err = checkAnswer(entry.req, choice, typed, modes)
	}
	b.mu.Unlock()
	if err != nil {
		return ApprovalActionResolved, err
	}
	return b.resolveWith(ref, ApprovalApproved, choice, supplied), nil
}

// checkAnswer validates what the approver sent against what the item offers. Errors name a label, never a value.
func checkAnswer(req ApprovalRequest, choice ApprovalChoice, typed, modes map[string]string) (SuppliedInputs, error) {
	if choice != (ApprovalChoice{}) && !req.OfferReview {
		return nil, ipcerr.Contract("reviewResult is not offered for this approval")
	}
	if len(typed) > 0 && len(req.Inputs) == 0 {
		return nil, ipcerr.Contract("this approval takes no inputs")
	}
	supplied := SuppliedInputs{}
	for name, raw := range typed {
		if !slices.ContainsFunc(req.Inputs, func(in ApprovalInput) bool { return in.Name == name }) {
			return nil, ipcerr.Contract("a value was sent for an input this approval does not ask for")
		}
		value, err := cleanInput(raw)
		if err != nil {
			supplied.Wipe()
			return nil, err
		}
		if value != nil {
			supplied[name] = Supplied{Value: value, Once: true}
		}
	}
	for name, mode := range modes {
		sv, typedOK := supplied[name]
		switch {
		case !slices.ContainsFunc(req.Inputs, func(in ApprovalInput) bool { return in.Name == name }):
			supplied.Wipe()
			return nil, ipcerr.Contract("a mode was sent for an input this approval does not ask for")
		case !typedOK:
			supplied.Wipe()
			return nil, ipcerr.Contract("a mode was sent for an input without a value")
		case mode != "once" && mode != "keep":
			supplied.Wipe()
			return nil, ipcerr.Contract("an input mode is once or keep")
		}
		sv.Once = mode == "once"
		supplied[name] = sv
	}
	for _, in := range req.Inputs {
		if _, ok := supplied[in.Name]; in.Needed && !ok {
			supplied.Wipe()
			return nil, ipcerr.Contract("enter %s to send", in.Label)
		}
	}
	return supplied, nil
}

// cleanInput trims a typed value and checks it is one line of valid text of bounded size. Empty after the
// trim is nil, which a needed input refuses and a stored one reads as "keep what is held".
func cleanInput(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return nil, nil
	case len(value) > maxInputBytes:
		return nil, ipcerr.Contract("a value can be at most 8 KiB")
	case !utf8.ValidString(value):
		return nil, ipcerr.Contract("a value must be valid text")
	case strings.IndexFunc(value, unicode.IsControl) >= 0:
		return nil, ipcerr.Contract("a value must be one line of text")
	}
	return []byte(value), nil
}

func (b *ApprovalBroker) entryLocked(requestID string) (*approvalEntry, bool) {
	for _, e := range b.queue.Snapshot() {
		if e.req.RequestID == requestID {
			return e, true
		}
	}
	return nil, false
}

// Request enqueues req (minting its RequestID, EnqueuedAt and ExpiresAt) and blocks the calling
// goroutine until it is approved, denied, its deadline elapses, or ctx is done — whichever comes
// first. Past maxPendingApprovals it returns ApprovalQueueFull immediately rather than queueing.
func (b *ApprovalBroker) Request(ctx context.Context, req ApprovalRequest) ApprovalOutcome {
	outcome, _ := b.RequestChoice(ctx, req)
	return outcome
}

// RequestChoice is Request plus what the approver ticked. A held result (req.Response) is checked
// against both caps under mu, the one place that sees every enqueue: over the per-result cap it
// returns ApprovalTooLarge, over the total ApprovalHeldFull, and no item is raised. Its bytes count
// until the entry resolves, whatever the path (answer, timeout, abandon).
func (b *ApprovalBroker) RequestChoice(ctx context.Context, req ApprovalRequest) (ApprovalOutcome, ApprovalChoice) {
	outcome, choice, inputs := b.RequestInputs(ctx, req)
	inputs.Wipe()
	return outcome, choice
}

// RequestInputs is RequestChoice plus the values the approver typed. Only an approved outcome carries
// inputs; the caller stores them and wipes what is left.
func (b *ApprovalBroker) RequestInputs(ctx context.Context, req ApprovalRequest) (ApprovalOutcome, ApprovalChoice, SuppliedInputs) {
	outcome, choice, inputs, queued := b.requestChoice(ctx, req)
	if box := b.observer.Load(); box != nil {
		box.o.ApprovalResolved(queued, outcome)
	}
	return outcome, choice, inputs
}

// requestChoice is RequestChoice's body; queued is the request as the queue saw it (RequestID minted),
// or req unchanged when it was refused before queueing.
func (b *ApprovalBroker) requestChoice(ctx context.Context, req ApprovalRequest) (ApprovalOutcome, ApprovalChoice, SuppliedInputs, ApprovalRequest) {
	b.mu.Lock()
	if b.queue.Len() >= maxPendingApprovals {
		b.mu.Unlock()
		return ApprovalQueueFull, ApprovalChoice{}, nil, req
	}
	if req.Response != nil {
		switch {
		case req.Response.Bytes > b.maxResult:
			b.mu.Unlock()
			return ApprovalTooLarge, ApprovalChoice{}, nil, req
		case b.heldBytes+req.Response.Bytes > b.maxTotal:
			b.mu.Unlock()
			return ApprovalHeldFull, ApprovalChoice{}, nil, req
		}
		b.heldBytes += req.Response.Bytes
	}
	now := b.now()
	if req.Stage == "" {
		req.Stage = ApprovalStageQuery
	}
	req.RequestID = ids.NewID()
	req.EnqueuedAt = now
	req.ExpiresAt = now.Add(b.timeout)
	entry := &approvalEntry{req: req, result: make(chan approvalResolution, 1)}
	b.queue.Add(entry.req.RequestID, entry)
	// Under mu so a fast answer cannot Remove before this Put.
	b.Queue.Put(itemOf(entry.req))
	b.mu.Unlock()
	if box := b.observer.Load(); box != nil {
		box.o.ApprovalRaised(entry.req)
	}

	timer := time.NewTimer(time.Until(entry.req.ExpiresAt))
	defer timer.Stop()
	select {
	case r := <-entry.result:
		return r.outcome, r.choice, r.inputs, entry.req
	case <-timer.C:
		if b.resolve(entry.req.RequestID, ApprovalTimedOut, ApprovalChoice{}) == ApprovalActionResolved {
			return ApprovalTimedOut, ApprovalChoice{}, nil, entry.req
		}
		// Approve/Deny won the race in the instant between the timer firing and this goroutine
		// taking b.mu — take their answer instead of overriding it.
		r := <-entry.result
		return r.outcome, r.choice, r.inputs, entry.req
	case <-ctx.Done():
		if b.resolve(entry.req.RequestID, ApprovalAbandoned, ApprovalChoice{}) == ApprovalActionResolved {
			return ApprovalAbandoned, ApprovalChoice{}, nil, entry.req
		}
		r := <-entry.result
		return r.outcome, r.choice, r.inputs, entry.req
	}
}

// SetHeldCaps replaces the held result caps; flow tests use it to reach them with small results.
func (b *ApprovalBroker) SetHeldCaps(perResult, total int) {
	b.mu.Lock()
	b.maxResult, b.maxTotal = perResult, total
	b.mu.Unlock()
}

// SetTimeout replaces how long a request waits for an answer; flow tests use it to reach the timeout
// without waiting ApprovalTimeout. Exemption records keep the constant.
func (b *ApprovalBroker) SetTimeout(d time.Duration) {
	b.mu.Lock()
	b.timeout = d
	b.mu.Unlock()
}

// ResponseFor returns the held result of requestID, true only while that id is queued with stage
// response.
func (b *ApprovalBroker) ResponseFor(requestID string) (ApprovalResponse, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entryLocked(requestID)
	if !ok || e.req.Stage != ApprovalStageResponse || e.req.Response == nil {
		return ApprovalResponse{}, false
	}
	return *e.req.Response, true
}

// StagedSecret returns the text of a secret value the held result of requestID will keep, for the user's
// reveal. false once the request resolved, or for a name that is not a staged secret.
func (b *ApprovalBroker) StagedSecret(requestID, name string) (string, bool) {
	b.mu.Lock()
	e, ok := b.entryLocked(requestID)
	var run *keptRun
	if ok && e.req.Stage == ApprovalStageResponse && e.req.Response != nil {
		run = e.req.Response.staged
	}
	b.mu.Unlock()
	return run.stagedSecret(name)
}

// Approve resolves requestID as approved. Deny resolves it as denied. Both report
// ApprovalActionAlreadyResolved, never an error, for an unknown or already-resolved id —
// whichever window (or Request's own deadline/ctx branch) got there first wins, the other is a
// no-op.
func (b *ApprovalBroker) Approve(requestID string) ApprovalActionResult {
	return b.resolve(requestID, ApprovalApproved, ApprovalChoice{})
}

func (b *ApprovalBroker) Deny(requestID string) ApprovalActionResult {
	return b.resolve(requestID, ApprovalDenied, ApprovalChoice{})
}

// resolve is Approve/Deny's shared body, also called from Request's own deadline/ctx branches so
// every resolution path — human decision, timeout, abandonment — goes through the same
// remove-then-send sequence and can never double-send on entry.result.
func (b *ApprovalBroker) resolve(requestID string, outcome ApprovalOutcome, choice ApprovalChoice) ApprovalActionResult {
	return b.resolveWith(requestID, outcome, choice, nil)
}

// resolveWith is resolve carrying typed inputs to the waiting request. An already resolved id wipes them.
func (b *ApprovalBroker) resolveWith(requestID string, outcome ApprovalOutcome, choice ApprovalChoice, inputs SuppliedInputs) ApprovalActionResult {
	b.mu.Lock()
	entry, ok := b.queue.Remove(requestID)
	if !ok {
		b.mu.Unlock()
		inputs.Wipe()
		return ApprovalActionAlreadyResolved
	}
	if entry.req.Response != nil {
		b.heldBytes -= entry.req.Response.Bytes
	}
	b.Queue.Remove(entry.req.itemID())
	b.mu.Unlock()

	entry.result <- approvalResolution{outcome: outcome, choice: choice, inputs: inputs}
	return ApprovalActionResolved
}

// AbandonAll resolves every pending entry as ApprovalAbandoned and empties the queue, leaving the
// broker usable: the DB MCP server can be toggled off and on again within one app run. Called on
// server stop and app quit so no run_query call is ever left blocked on a broker nobody will
// answer again.
func (b *ApprovalBroker) AbandonAll() {
	b.mu.Lock()
	entries := b.queue.Clear()
	b.heldBytes = 0
	for _, entry := range entries {
		b.Queue.Remove(entry.req.itemID())
	}
	b.mu.Unlock()

	for _, entry := range entries {
		entry.result <- approvalResolution{outcome: ApprovalAbandoned}
	}
}

// AbandonExternal abandons the pending entries of external clients, leaving agent-session ones queued.
func (b *ApprovalBroker) AbandonExternal() {
	b.abandonWhere(func(o ApprovalOrigin) bool { return o.SessionID == "" })
}

// AbandonSession abandons the pending entries of one agent session.
func (b *ApprovalBroker) AbandonSession(sessionID string) {
	b.abandonWhere(func(o ApprovalOrigin) bool { return o.SessionID == sessionID })
}

// AbandonSessions abandons the pending entries of every agent session.
func (b *ApprovalBroker) AbandonSessions() {
	b.abandonWhere(func(o ApprovalOrigin) bool { return o.SessionID != "" })
}

func (b *ApprovalBroker) abandonWhere(match func(ApprovalOrigin) bool) {
	b.mu.Lock()
	var entries []*approvalEntry
	for _, e := range b.queue.Snapshot() {
		if !match(e.req.Origin) {
			continue
		}
		if _, ok := b.queue.Remove(e.req.RequestID); !ok {
			continue
		}
		if e.req.Response != nil {
			b.heldBytes -= e.req.Response.Bytes
		}
		b.Queue.Remove(e.req.itemID())
		entries = append(entries, e)
	}
	b.mu.Unlock()

	for _, entry := range entries {
		entry.result <- approvalResolution{outcome: ApprovalAbandoned}
	}
}

// ApprovalCluster names the Kubernetes read a request would run; nil on a database request.
type ApprovalCluster struct {
	Context   string
	Server    string
	Namespace string // "" means all namespaces, or none for discovery
	// Operation is the permission group: discover, list, describe or logs.
	Operation string
	// Tool is the MCP tool name; Request the final args of a list, describe or logs read (after prepare).
	Tool    string
	Request kubeRequest
	// Script is the scripts of the read, shown beside the statement; nil without one.
	Script *apiScripts
}

// ApprovalValue is one param or env entry of a script request. A secret's Value is always "".
type ApprovalValue struct {
	Name    string
	Value   string
	Secret  bool
	Saved   bool
	FromVar string
}

// ApprovalScript names the script run a request would start.
type ApprovalScript struct {
	ScriptID    string
	Name        string
	Description string
	Command     string
	Cwd         string
	Params      []ApprovalValue
	Env         []ApprovalValue
}
