package dbmcp

import (
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

type planIssuePayload struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type planPayload struct {
	EstimatedRowsRead *float64           `json:"estimatedRowsRead"`
	ThresholdRows     int                `json:"thresholdRows"`
	OverThreshold     bool               `json:"overThreshold"`
	Issues            []planIssuePayload `json:"issues"`
	IssuesOmitted     int                `json:"issuesOmitted"`
}

func planPayloadOf(p *ApprovalPlan) *planPayload {
	if p == nil {
		return nil
	}
	issues := make([]planIssuePayload, len(p.Issues))
	for i, iss := range p.Issues {
		issues[i] = planIssuePayload(iss)
	}
	return &planPayload{
		EstimatedRowsRead: p.EstimatedRowsRead, ThresholdRows: p.ThresholdRows,
		OverThreshold: p.OverThreshold, Issues: issues, IssuesOmitted: p.IssuesOmitted,
	}
}

type queryPayload struct {
	ConnectionID string `json:"connectionId"`
	Statement    string `json:"statement"`
	// Mode is the declared mode; Detected is what the statement looks like. Enforcement is set on
	// read items only.
	Mode        string       `json:"mode"`
	Detected    string       `json:"detected"`
	Enforcement string       `json:"enforcement"`
	Plan        *planPayload `json:"plan"`
}

type limitHitPayload struct {
	Limit    string  `json:"limit"`
	Measured float64 `json:"measured"`
	Max      float64 `json:"max"`
	Unit     string  `json:"unit"`
}

type exemptionPayload struct {
	ConnectionID string            `json:"connectionId"`
	Statement    string            `json:"statement"`
	Mode         string            `json:"mode"`
	Detected     string            `json:"detected"`
	Enforcement  string            `json:"enforcement"`
	Reason       string            `json:"reason"`
	Limits       []limitHitPayload `json:"limits"`
	Plan         *planPayload      `json:"plan"`
}

func exemptionPayloadOf(r ApprovalRequest) exemptionPayload {
	ex := r.Exemption
	if ex == nil {
		ex = &ApprovalExemption{}
	}
	limits := make([]limitHitPayload, len(ex.Hits))
	for i, h := range ex.Hits {
		limits[i] = limitHitPayload(h)
	}
	return exemptionPayload{
		ConnectionID: r.ConnectionID, Statement: r.Statement, Mode: r.Mode, Detected: string(r.Class),
		Enforcement: r.Enforcement, Reason: ex.Reason, Limits: limits, Plan: planPayloadOf(r.Plan),
	}
}

type responsePayload struct {
	ConnectionID string `json:"connectionId"`
	Statement    string `json:"statement"`
	IsError      bool   `json:"isError"`
	Bytes        int    `json:"bytes"`
	AlreadyRan   bool   `json:"alreadyRan"`
}

type kubePayload struct {
	Context   string `json:"context"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"`
	Statement string `json:"statement"`
}

type valuePayload struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Secret  bool   `json:"secret"`
	Saved   bool   `json:"saved"`
	FromVar string `json:"fromVar"`
}

type scriptPayload struct {
	ScriptID    string         `json:"scriptId"`
	Description string         `json:"description"`
	Command     string         `json:"command"`
	Cwd         string         `json:"cwd"`
	Params      []valuePayload `json:"params"`
	Env         []valuePayload `json:"env"`
}

func valuePayloads(in []ApprovalValue) []valuePayload {
	out := make([]valuePayload, len(in))
	for i, v := range in {
		out[i] = valuePayload(v)
	}
	return out
}

// itemOf projects a request into its approvals item. The statement text rides in the payload in full:
// a cap that elided the middle would let a hidden clause sit inside the omitted span.
func itemOf(r ApprovalRequest) approvals.Item {
	it := approvals.Item{
		ID: r.itemID(), Kind: r.subject(),
		CreatedAt: r.EnqueuedAt.UnixMilli(), ExpiresAt: r.ExpiresAt.UnixMilli(),
		Origin: approvals.Origin{Client: r.Origin.Client, Cwd: r.Origin.Cwd},
		Reason: string(r.Reason), Title: promptTitle(r),
	}
	switch it.Kind {
	case approvals.KindKubeRead:
		c := r.Cluster
		it.Target = approvals.Target{Name: c.Context, Kind: "kubernetes", Location: kubeLocation(c)}
		it.Operation = approvals.Operation{Class: "read", Tone: "neutral"}
		it.Payload = mustJSON(kubePayload{Context: c.Context, Server: c.Server, Namespace: c.Namespace, Operation: c.Operation, Statement: r.Statement})
	case approvals.KindScriptRun:
		sc := r.Script
		if sc == nil {
			sc = &ApprovalScript{}
		}
		it.Target = approvals.Target{Name: sc.Name, Kind: "script", Location: sc.Cwd}
		it.Operation = approvals.Operation{Class: "run", Tone: "caution"}
		it.Payload = mustJSON(scriptPayload{
			ScriptID: sc.ScriptID, Description: sc.Description, Command: sc.Command, Cwd: sc.Cwd,
			Params: valuePayloads(sc.Params), Env: valuePayloads(sc.Env),
		})
	case approvals.KindAPIRequest:
		it.Target, it.Operation, it.Payload = apiItemParts(r.API)
	case approvals.KindDbExemption:
		it.Target, it.Operation = dbTarget(r)
		it.Payload = mustJSON(exemptionPayloadOf(r))
	case approvals.KindDbResponse:
		it.Reason = "preview"
		it.Target, it.Operation = dbTarget(r)
		held := r.Response
		if held == nil {
			held = &ApprovalResponse{}
		}
		it.Payload = mustJSON(responsePayload{
			ConnectionID: r.ConnectionID, Statement: r.Statement,
			IsError: held.IsError, Bytes: held.Bytes, AlreadyRan: r.Class != adapters.ClassRead,
		})
	default:
		it.Target, it.Operation = dbTarget(r)
		it.Payload = mustJSON(queryPayload{
			ConnectionID: r.ConnectionID, Statement: r.Statement, Mode: r.Mode, Detected: string(r.Class),
			Enforcement: r.Enforcement, Plan: planPayloadOf(r.Plan),
		})
	}
	return it
}

func dbTarget(r ApprovalRequest) (approvals.Target, approvals.Operation) {
	t := r.Target
	parts := make([]string, 0, 2)
	if t.Host != "" {
		parts = append(parts, t.Host)
	}
	if len(t.Path) > 0 {
		segs := make([]string, len(t.Path))
		for i, s := range t.Path {
			segs[i] = s.Name
		}
		parts = append(parts, strings.Join(segs, " / "))
	} else if t.Database != "" {
		parts = append(parts, t.Database)
	}
	return approvals.Target{
			Name: r.ConnectionName, Kind: r.Kind, Color: t.Color, ReadOnly: t.ReadOnly, Location: strings.Join(parts, " · "),
		},
		approvals.Operation{Class: operationClass(r), Tone: toneOf(operationClass(r))}
}

// operationClass is the declared mode, or the detected class for an item that carries no mode.
func operationClass(r ApprovalRequest) string {
	if r.Mode != "" {
		return r.Mode
	}
	return string(r.Class)
}

func toneOf(class string) string {
	switch class {
	case string(adapters.ClassRead):
		return "neutral"
	case string(adapters.ClassWrite):
		return "caution"
	}
	return "danger"
}

func kubeLocation(c *ApprovalCluster) string {
	if c.Namespace == "" {
		return c.Server
	}
	return c.Server + " · " + c.Namespace
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Only plain strings, numbers and nil pointers go in; a failure is a programming error.
		panic("dbmcp: approval payload: " + err.Error())
	}
	return b
}

// promptTitle is the router title. It never carries the statement.
func promptTitle(req ApprovalRequest) string {
	named := req.ConnectionName != ""
	response := req.Stage == ApprovalStageResponse
	switch {
	case req.Subject == approvals.KindAPIRequest && req.API != nil:
		return i18n.T("backend.prompt.dbMcpAPI.title", map[string]string{"method": req.API.Method, "host": req.API.Host})
	case req.Subject == approvals.KindScriptRun:
		return i18n.T("backend.prompt.dbMcpScript.title", map[string]string{"name": req.ConnectionName})
	case req.Subject == approvals.KindDbExemption:
		return i18n.T("backend.prompt.dbMcpExemption.title", map[string]string{"name": req.ConnectionName})
	case response && named:
		return i18n.T("backend.prompt.dbMcpResponseNamed.title", map[string]string{"name": req.ConnectionName})
	case response:
		return i18n.T("backend.prompt.dbMcpResponse.title", nil)
	case named:
		return i18n.T("backend.prompt.dbMcpNamed.title", map[string]string{"name": req.ConnectionName})
	}
	return i18n.T("backend.prompt.dbMcp.title", nil)
}
