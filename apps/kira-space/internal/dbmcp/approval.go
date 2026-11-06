package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
)

// ApprovalTimeout bounds how long a prompt-mode query waits for a human answer before giving up.
const ApprovalTimeout = 10 * time.Minute

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

// ApprovalResponse is the held run_query result a response-stage request asks about. Text is the
// exact TextContent the client receives on Send; it stays out of queue snapshots (which fan out to
// every window) and is fetched by request id through ResponseFor.
type ApprovalResponse struct {
	Text    string
	IsError bool
	Bytes   int
	// Requested says the approver asked for the review on the pre-run item, not a connection setting.
	Requested bool
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
	// Path is the decoded run_query/explain_query path; empty when the call passed none.
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
	Target      ApprovalTarget
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
	// Subject is the approvals kind key. Empty derives it: kube.read with a Cluster, db.response
	// at the response stage, else db.query.
	Subject string
	// Origin says which client asked; the tools copy it from the call's context.
	Origin ApprovalOrigin
	// Script is set for the script.run subject; Target is then empty.
	Script *ApprovalScript
	// API is set for the api.request subject; Target is then empty.
	API *agentapi.Planned
	// Exemption is set for the db.exemption subject.
	Exemption *ApprovalExemption
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
	observer            atomic.Pointer[observerBox]
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
	b := &ApprovalBroker{now: now, Queue: q, queue: notify.NewPendingQueue[*approvalEntry](), maxResult: perResult, maxTotal: total}
	for _, kind := range []string{approvals.KindDbQuery, approvals.KindDbResponse, approvals.KindDbExemption, approvals.KindKubeRead, approvals.KindScriptRun, approvals.KindAPIRequest} {
		q.Register(kind, brokerOwner{b})
	}
	return b
}

// brokerOwner answers the broker's items.
type brokerOwner struct{ b *ApprovalBroker }

func (o brokerOwner) Answer(ref string, a approvals.Answer) error {
	if !a.Approve {
		if o.b.resolve(ref, ApprovalDenied, ApprovalChoice{}) == ApprovalActionAlreadyResolved {
			return ipcerr.NotFoundNamed("approval")
		}
		return nil
	}
	var choice ApprovalChoice
	if len(a.Options) > 0 {
		dec := json.NewDecoder(bytes.NewReader(a.Options))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&choice); err != nil {
			return ipcerr.Contract("approval options are not understood")
		}
	}
	res, err := o.b.answer(ref, choice)
	if err != nil {
		return err
	}
	if res == ApprovalActionAlreadyResolved {
		return ipcerr.NotFoundNamed("approval")
	}
	return nil
}

// answer approves ref with choice. A choice the item does not offer is a contract error and leaves the
// item pending; the check shares the lock with the lookup so a resolve cannot slip between them.
func (b *ApprovalBroker) answer(ref string, choice ApprovalChoice) (ApprovalActionResult, error) {
	if choice != (ApprovalChoice{}) {
		b.mu.Lock()
		entry, ok := b.entryLocked(ref)
		offered := ok && entry.req.OfferReview
		b.mu.Unlock()
		if ok && !offered {
			return ApprovalActionResolved, ipcerr.Contract("reviewResult is not offered for this approval")
		}
	}
	return b.resolve(ref, ApprovalApproved, choice), nil
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
	outcome, choice, queued := b.requestChoice(ctx, req)
	if box := b.observer.Load(); box != nil {
		box.o.ApprovalResolved(queued, outcome)
	}
	return outcome, choice
}

// requestChoice is RequestChoice's body; queued is the request as the queue saw it (RequestID minted),
// or req unchanged when it was refused before queueing.
func (b *ApprovalBroker) requestChoice(ctx context.Context, req ApprovalRequest) (ApprovalOutcome, ApprovalChoice, ApprovalRequest) {
	b.mu.Lock()
	if b.queue.Len() >= maxPendingApprovals {
		b.mu.Unlock()
		return ApprovalQueueFull, ApprovalChoice{}, req
	}
	if req.Response != nil {
		switch {
		case req.Response.Bytes > b.maxResult:
			b.mu.Unlock()
			return ApprovalTooLarge, ApprovalChoice{}, req
		case b.heldBytes+req.Response.Bytes > b.maxTotal:
			b.mu.Unlock()
			return ApprovalHeldFull, ApprovalChoice{}, req
		}
		b.heldBytes += req.Response.Bytes
	}
	now := b.now()
	if req.Stage == "" {
		req.Stage = ApprovalStageQuery
	}
	req.RequestID = ids.NewID()
	req.EnqueuedAt = now
	req.ExpiresAt = now.Add(ApprovalTimeout)
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
		return r.outcome, r.choice, entry.req
	case <-timer.C:
		if b.resolve(entry.req.RequestID, ApprovalTimedOut, ApprovalChoice{}) == ApprovalActionResolved {
			return ApprovalTimedOut, ApprovalChoice{}, entry.req
		}
		// Approve/Deny won the race in the instant between the timer firing and this goroutine
		// taking b.mu — take their answer instead of overriding it.
		r := <-entry.result
		return r.outcome, r.choice, entry.req
	case <-ctx.Done():
		if b.resolve(entry.req.RequestID, ApprovalAbandoned, ApprovalChoice{}) == ApprovalActionResolved {
			return ApprovalAbandoned, ApprovalChoice{}, entry.req
		}
		r := <-entry.result
		return r.outcome, r.choice, entry.req
	}
}

// SetHeldCaps replaces the held result caps; flow tests use it to reach them with small results.
func (b *ApprovalBroker) SetHeldCaps(perResult, total int) {
	b.mu.Lock()
	b.maxResult, b.maxTotal = perResult, total
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
	b.mu.Lock()
	entry, ok := b.queue.Remove(requestID)
	if !ok {
		b.mu.Unlock()
		return ApprovalActionAlreadyResolved
	}
	if entry.req.Response != nil {
		b.heldBytes -= entry.req.Response.Bytes
	}
	b.Queue.Remove(entry.req.itemID())
	b.mu.Unlock()

	entry.result <- approvalResolution{outcome: outcome, choice: choice}
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
