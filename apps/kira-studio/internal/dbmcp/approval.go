package dbmcp

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
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

// ApprovalOutcome is ApprovalBroker.Request's own vocabulary — never a Go error: a human decision
// is a value, not a failure.
type ApprovalOutcome int

const (
	ApprovalApproved ApprovalOutcome = iota
	ApprovalDenied
	ApprovalTimedOut
	ApprovalAbandoned // the server stopped, or the MCP client went away (ctx cancelled)
	ApprovalQueueFull // maxPendingApprovals already waiting; no human saw this request
)

// ApprovalActionResult is what Approve/Deny report — also never an error.
type ApprovalActionResult int

const (
	ApprovalActionResolved ApprovalActionResult = iota
	ApprovalActionAlreadyResolved
)

// ApprovalReason is why this request was raised — M2's permission gate, or M3's heavy-plan flag.
// Both are the same question ("should this run?") asked for different reasons, so they share one
// queue and one dialog rather than competing for the user's attention (M3 §6.2).
type ApprovalReason string

const (
	ApprovalReasonPermission ApprovalReason = "permission"
	ApprovalReasonHeavy      ApprovalReason = "heavy"
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
	// Reason is M2's own "permission" for every M2-era call site — M3's runQuery/explainQuery are
	// the first callers to pass ApprovalReasonHeavy or a non-nil Plan.
	Reason ApprovalReason
	// Plan is the heavy-query evidence (M3 §6.1/§6.2), nil for an ordinary permission prompt with
	// no plan attached (auto-force-explain off, or the statement was not explainable).
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
}

// ApprovalOrigin is what the stateless HTTP transport can tell about the caller.
type ApprovalOrigin struct {
	Client string
	Cwd    string
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
	result chan ApprovalOutcome // buffered 1; exactly one send over the entry's lifetime.
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
}

// NewApprovalBroker builds a broker over q and registers it as owner of its kinds; a nil q gets a
// private queue (tests).
func NewApprovalBroker(now func() time.Time, q *approvals.Queue) *ApprovalBroker {
	if q == nil {
		q = approvals.New(nil)
	}
	b := &ApprovalBroker{now: now, Queue: q, queue: notify.NewPendingQueue[*approvalEntry]()}
	for _, kind := range []string{approvals.KindDbQuery, approvals.KindDbResponse, approvals.KindKubeRead, approvals.KindScriptRun, approvals.KindAPIRequest} {
		q.Register(kind, brokerOwner{b})
	}
	return b
}

// brokerOwner answers the broker's items.
type brokerOwner struct{ b *ApprovalBroker }

func (o brokerOwner) Answer(ref string, a approvals.Answer) error {
	outcome := ApprovalDenied
	if a.Approve {
		outcome = ApprovalApproved
	}
	if o.b.resolve(ref, outcome) == ApprovalActionAlreadyResolved {
		return ipcerr.NotFoundNamed("approval")
	}
	return nil
}

// Request enqueues req (minting its RequestID, EnqueuedAt and ExpiresAt) and blocks the calling
// goroutine until it is approved, denied, its deadline elapses, or ctx is done — whichever comes
// first. Past maxPendingApprovals it returns ApprovalQueueFull immediately rather than queueing.
func (b *ApprovalBroker) Request(ctx context.Context, req ApprovalRequest) ApprovalOutcome {
	b.mu.Lock()
	if b.queue.Len() >= maxPendingApprovals {
		b.mu.Unlock()
		return ApprovalQueueFull
	}
	now := b.now()
	if req.Stage == "" {
		req.Stage = ApprovalStageQuery
	}
	req.RequestID = uuid.NewString()
	req.EnqueuedAt = now
	req.ExpiresAt = now.Add(ApprovalTimeout)
	entry := &approvalEntry{req: req, result: make(chan ApprovalOutcome, 1)}
	b.queue.Add(entry.req.RequestID, entry)
	// Under mu so a fast answer cannot Remove before this Put.
	b.Queue.Put(itemOf(entry.req))
	b.mu.Unlock()

	timer := time.NewTimer(time.Until(entry.req.ExpiresAt))
	defer timer.Stop()
	select {
	case outcome := <-entry.result:
		return outcome
	case <-timer.C:
		if b.resolve(entry.req.RequestID, ApprovalTimedOut) == ApprovalActionResolved {
			return ApprovalTimedOut
		}
		// Approve/Deny won the race in the instant between the timer firing and this goroutine
		// taking b.mu — take their answer instead of overriding it.
		return <-entry.result
	case <-ctx.Done():
		if b.resolve(entry.req.RequestID, ApprovalAbandoned) == ApprovalActionResolved {
			return ApprovalAbandoned
		}
		return <-entry.result
	}
}

// ResponseFor returns the held result of requestID, true only while that id is queued with stage
// response.
func (b *ApprovalBroker) ResponseFor(requestID string) (ApprovalResponse, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.queue.Snapshot() {
		if e.req.RequestID == requestID {
			if e.req.Stage != ApprovalStageResponse || e.req.Response == nil {
				return ApprovalResponse{}, false
			}
			return *e.req.Response, true
		}
	}
	return ApprovalResponse{}, false
}

// Approve resolves requestID as approved. Deny resolves it as denied. Both report
// ApprovalActionAlreadyResolved, never an error, for an unknown or already-resolved id —
// whichever window (or Request's own deadline/ctx branch) got there first wins, the other is a
// no-op.
func (b *ApprovalBroker) Approve(requestID string) ApprovalActionResult {
	return b.resolve(requestID, ApprovalApproved)
}

func (b *ApprovalBroker) Deny(requestID string) ApprovalActionResult {
	return b.resolve(requestID, ApprovalDenied)
}

// resolve is Approve/Deny's shared body, also called from Request's own deadline/ctx branches so
// every resolution path — human decision, timeout, abandonment — goes through the same
// remove-then-send sequence and can never double-send on entry.result.
func (b *ApprovalBroker) resolve(requestID string, outcome ApprovalOutcome) ApprovalActionResult {
	b.mu.Lock()
	entry, ok := b.queue.Remove(requestID)
	if !ok {
		b.mu.Unlock()
		return ApprovalActionAlreadyResolved
	}
	b.Queue.Remove(entry.req.itemID())
	b.mu.Unlock()

	entry.result <- outcome
	return ApprovalActionResolved
}

// AbandonAll resolves every pending entry as ApprovalAbandoned and empties the queue, leaving the
// broker usable: the DB MCP server can be toggled off and on again within one app run. Called on
// server stop and app quit so no run_query call is ever left blocked on a broker nobody will
// answer again.
func (b *ApprovalBroker) AbandonAll() {
	b.mu.Lock()
	entries := b.queue.Clear()
	for _, entry := range entries {
		b.Queue.Remove(entry.req.itemID())
	}
	b.mu.Unlock()

	for _, entry := range entries {
		entry.result <- ApprovalAbandoned
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
