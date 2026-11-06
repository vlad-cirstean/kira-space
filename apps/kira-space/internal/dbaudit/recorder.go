// Package dbaudit records what agent sessions do through the Kira DB session server: one row per
// tool call and per approval, never row data, bound values or results (docs/ARCHITECTURE.md
// "Audit trail").
package dbaudit

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
)

const (
	maxStatementRunes = 4 << 10
	maxErrorRunes     = 1 << 10
	maxSummaryBytes   = 1 << 10
	maxReasonRunes    = 280
	internalErrorText = "internal error"
)

// summaryKeys are the only argument keys kept for tools that carry no statement. Request bodies,
// headers, documents, values and params never pass.
var summaryKeys = map[string]bool{
	"connectionId": true, "path": true, "mode": true, "clusterId": true, "namespace": true, "kind": true,
	"name": true, "pod": true, "container": true, "scriptId": true, "method": true, "url": true,
	"limit": true, "refresh": true, "tail": true,
}

// Store is the part of repos.DbAuditRepo the recorder writes through.
type Store interface {
	StartCall(model.DbAuditCall) error
	FinishCall(id, status string, endedMs int64, errText string) error
	SetStatement(id, statement string, length int, withheld, truncated bool) error
	RaiseApproval(model.DbAuditApproval) error
	ResolveApproval(id, outcome string, endedMs int64) error
}

var _ Store = (*repos.DbAuditRepo)(nil)

// Redactor masks a statement the way the connection's adapter prescribes; false when it cannot.
type Redactor interface {
	RedactStatement(connectionID, statement string) (string, bool)
}

// Deps wires a Recorder. Redactor, ConnName and Emit may be nil.
type Deps struct {
	Store    Store
	Redactor Redactor
	// ConnName resolves a connection's display name.
	ConnName func(connectionID string) string
	// Emit announces that taskID's trail changed.
	Emit func(taskID string)
	Now  func() time.Time
}

type callMeta struct {
	taskID, sessionID, stepID, runID, connectionID string
	// unredacted holds the statement until the connection is live and its adapter can mask it.
	unredacted string
}

// Recorder implements dbmcp.Auditor and dbmcp.ApprovalObserver. A write error is logged and never
// fails the tool call.
type Recorder struct {
	d Deps

	mu    sync.Mutex
	calls map[string]callMeta
	// approvals maps an open approval's request id to its call.
	approvals map[string]callMeta
}

var (
	_ dbmcp.Auditor          = (*Recorder)(nil)
	_ dbmcp.ApprovalObserver = (*Recorder)(nil)
)

// New returns a recorder over d.
func New(d Deps) *Recorder {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Recorder{d: d, calls: map[string]callMeta{}, approvals: map[string]callMeta{}}
}

func (r *Recorder) warn(what string, err error) {
	slog.Warn("db audit failed", "scope", "dbaudit", "step", what, "err", err)
}

func (r *Recorder) emit(taskID string) {
	if r.d.Emit != nil {
		r.d.Emit(taskID)
	}
}

// CallStarted opens the running row of a call.
func (r *Recorder) CallStarted(c dbmcp.AuditCall) {
	var args map[string]any
	_ = json.Unmarshal(c.Args, &args)
	str := func(k string) string { s, _ := args[k].(string); return s }
	connID := str("connectionId")
	pending := ""
	row := model.DbAuditCall{
		ID: c.ID, TaskID: c.Identity.TaskID, SessionID: c.Identity.SessionID, StepID: c.Identity.StepID,
		RunID: c.Identity.RunID, StartedMs: c.Started.UnixMilli(), Tool: c.Tool, ConnectionID: connID,
		Mode: str("mode"), Target: target(args),
	}
	if connID != "" && r.d.ConnName != nil {
		row.ConnectionName = r.d.ConnName(connID)
	}
	if c.Tool == "run_query" || c.Tool == "explain_query" {
		var ok bool
		row.Statement, row.StatementLen, ok, row.StatementTruncated = r.redactStatement(connID, str("sql"))
		if !ok && row.StatementLen > 0 {
			pending = str("sql")
		}
	} else {
		row.Summary = summarise(args)
	}
	if err := r.d.Store.StartCall(row); err != nil {
		r.warn("start call", err)
		return
	}
	r.mu.Lock()
	r.calls[c.ID] = callMeta{c.Identity.TaskID, c.Identity.SessionID, c.Identity.StepID, c.Identity.RunID, connID, pending}
	r.mu.Unlock()
	r.emit(row.TaskID)
}

// settleStatement redacts a statement that could not be masked at the call's start (the connection
// was not live yet). Without a redactor even now, the statement is withheld.
func (r *Recorder) settleStatement(callID string, final bool) {
	r.mu.Lock()
	m, ok := r.calls[callID]
	raw := m.unredacted
	r.mu.Unlock()
	if !ok || raw == "" {
		return
	}
	out, length, masked, truncated := r.redactStatement(m.connectionID, raw)
	if !masked && !final {
		return
	}
	if err := r.d.Store.SetStatement(callID, out, length, !masked, truncated); err != nil {
		r.warn("set statement", err)
		return
	}
	r.mu.Lock()
	if m, ok := r.calls[callID]; ok {
		m.unredacted = ""
		r.calls[callID] = m
	}
	r.mu.Unlock()
}

// CallFinished closes a call's row. A stored error is a fixed text, never an adapter message.
func (r *Recorder) CallFinished(e dbmcp.AuditEnd) {
	r.settleStatement(e.ID, true)
	r.mu.Lock()
	m, ok := r.calls[e.ID]
	delete(r.calls, e.ID)
	r.mu.Unlock()
	if !ok {
		return
	}
	var errText string
	switch {
	case e.ErrCode != "":
		errText = dbmcp.AuditErrorText(e.ErrCode)
	case e.ErrText == internalErrorText:
		errText = internalErrorText
	case e.ErrText != "":
		errText = r.refusalText(m.connectionID, e.ErrText)
	}
	if err := r.d.Store.FinishCall(e.ID, e.Status, e.Ended.UnixMilli(), errText); err != nil {
		r.warn("finish call", err)
		return
	}
	r.emit(m.taskID)
}

// ApprovalRaised opens a pending approval row under its call.
func (r *Recorder) ApprovalRaised(req dbmcp.ApprovalRequest) {
	m, ok := r.metaFor(req)
	if !ok {
		return
	}
	r.settleStatement(req.Origin.CallID, false)
	row := r.approvalRow(m, req)
	row.ID, row.Outcome, row.StartedMs = req.RequestID, model.DbAuditPending, req.EnqueuedAt.UnixMilli()
	if err := r.d.Store.RaiseApproval(row); err != nil {
		r.warn("raise approval", err)
		return
	}
	r.mu.Lock()
	r.approvals[req.RequestID] = m
	r.mu.Unlock()
	r.emit(m.taskID)
}

// ApprovalResolved closes the approval's row; a request refused before queueing is stored already resolved.
func (r *Recorder) ApprovalResolved(req dbmcp.ApprovalRequest, outcome dbmcp.ApprovalOutcome) {
	now := r.d.Now().UnixMilli()
	if req.RequestID != "" {
		r.mu.Lock()
		m, ok := r.approvals[req.RequestID]
		delete(r.approvals, req.RequestID)
		r.mu.Unlock()
		if !ok {
			return
		}
		if err := r.d.Store.ResolveApproval(req.RequestID, outcome.String(), now); err != nil {
			r.warn("resolve approval", err)
			return
		}
		r.emit(m.taskID)
		return
	}
	m, ok := r.metaFor(req)
	if !ok {
		return
	}
	row := r.approvalRow(m, req)
	row.ID, row.Outcome, row.StartedMs, row.EndedMs = ids.NewID(), outcome.String(), now, &now
	if err := r.d.Store.RaiseApproval(row); err != nil {
		r.warn("insert resolved approval", err)
		return
	}
	r.emit(m.taskID)
}

// metaFor finds the call an approval belongs to; only session-server requests are audited.
func (r *Recorder) metaFor(req dbmcp.ApprovalRequest) (callMeta, bool) {
	if req.Origin.SessionID == "" {
		return callMeta{}, false
	}
	r.mu.Lock()
	m, ok := r.calls[req.Origin.CallID]
	r.mu.Unlock()
	if !ok {
		slog.Warn("db audit: approval without a recorded call", "scope", "dbaudit", "session", req.Origin.SessionID)
	}
	return m, ok
}

type exemptionJSON struct {
	Limits []limitJSON `json:"limits"`
	Reason string      `json:"reason"`
}

type limitJSON struct {
	Name      string `json:"name"`
	Threshold string `json:"threshold"`
}

func (r *Recorder) approvalRow(m callMeta, req dbmcp.ApprovalRequest) model.DbAuditApproval {
	reason := string(req.Reason)
	if reason == "" {
		reason = string(dbmcp.ApprovalReasonPermission)
	}
	stage := string(req.Stage)
	if stage == "" {
		stage = string(dbmcp.ApprovalStageQuery)
	}
	a := model.DbAuditApproval{
		CallID: req.Origin.CallID, TaskID: m.taskID, SessionID: m.sessionID, StepID: m.stepID, RunID: m.runID,
		Subject: req.SubjectKey(), Reason: reason, Stage: stage,
	}
	if req.Reason == dbmcp.ApprovalReasonExemption && req.Exemption != nil {
		ex := exemptionJSON{Reason: truncateRunes(oplog.RedactText(req.Exemption.Reason), maxReasonRunes)}
		for _, h := range req.Exemption.Hits {
			ex.Limits = append(ex.Limits, limitJSON{Name: h.Limit, Threshold: strings.TrimSpace(fmt.Sprintf("%g %s", h.Max, h.Unit))})
		}
		if b, err := json.Marshal(ex); err == nil {
			a.ExemptionJSON = string(b)
		}
	}
	return a
}

// redactStatement returns the statement as it may be stored, its original length in runes and
// whether it was withheld or cut.
func (r *Recorder) redactStatement(connectionID, statement string) (out string, length int, ok, truncated bool) {
	length = utf8.RuneCountInString(statement)
	if statement == "" {
		return "", 0, true, false
	}
	var masked string
	if r.d.Redactor != nil && connectionID != "" {
		masked, ok = r.d.Redactor.RedactStatement(connectionID, statement)
	}
	if !ok {
		return "", length, false, false
	}
	masked = oplog.RedactText(masked)
	if utf8.RuneCountInString(masked) > maxStatementRunes {
		masked, truncated = string([]rune(masked)[:maxStatementRunes]), true
	}
	return masked, length, true, truncated
}

// refusalText stores dbmcp's own refusal wording with literals masked: it can quote the statement.
func (r *Recorder) refusalText(connectionID, text string) string {
	masked, ok := "", false
	if r.d.Redactor != nil && connectionID != "" {
		masked, ok = r.d.Redactor.RedactStatement(connectionID, text)
	}
	if !ok {
		masked, ok = adapters.MaskSQLLiterals(text, adapters.PostgresDialect)
	}
	if !ok {
		return "refused (message not recorded)"
	}
	return truncateRunes(oplog.RedactText(masked), maxErrorRunes)
}

func target(args map[string]any) string {
	str := func(k string) string { s, _ := args[k].(string); return s }
	if p := str("path"); p != "" {
		return p
	}
	var parts []string
	for _, k := range []string{"clusterId", "namespace", "kind", "name", "pod", "scriptId"} {
		if v := str(k); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "/")
}

// summarise keeps allow-listed argument keys only; a url loses its credentials and query.
func summarise(args map[string]any) string {
	kept := map[string]any{}
	for k, v := range args {
		if !summaryKeys[k] {
			continue
		}
		if k == "url" {
			s, _ := v.(string)
			v = stripURL(s)
		}
		kept[k] = v
	}
	if len(kept) == 0 {
		return ""
	}
	b, err := json.Marshal(kept)
	if err != nil {
		return ""
	}
	return oplog.RedactJSON(b, maxSummaryBytes)
}

func stripURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return oplog.RedactText(u.String())
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
