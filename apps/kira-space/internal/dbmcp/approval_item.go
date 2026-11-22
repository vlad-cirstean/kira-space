package dbmcp

import (
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
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

// dbLocation is where a connection lives, split for the card header: the host, then the database or
// schema path (segments joined with " / ").
type dbLocation struct {
	Host string `json:"host"`
	Path string `json:"path"`
}

func dbLocationOf(t ApprovalTarget) dbLocation {
	loc := dbLocation{Host: t.Host, Path: t.Database}
	if len(t.Path) > 0 {
		segs := make([]string, len(t.Path))
		for i, s := range t.Path {
			segs[i] = s.Name
		}
		loc.Path = strings.Join(segs, " / ")
	}
	return loc
}

type queryPayload struct {
	dbLocation
	ConnectionID string `json:"connectionId"`
	Statement    string `json:"statement"`
	// Mode is the declared mode; Detected is what the statement looks like. Enforcement is set on
	// read items only.
	Mode        string       `json:"mode"`
	Detected    string       `json:"detected"`
	Enforcement string       `json:"enforcement"`
	Explain     bool         `json:"explain,omitempty"`
	Plan        *planPayload `json:"plan"`
	// ReviewOffered shows "review the result"; ReviewResult says the connection's settings already do.
	ReviewOffered bool `json:"reviewOffered"`
	ReviewResult  bool `json:"reviewResult"`
	// Params, Placeholders and Kept describe the kept variables bound to the statement. Names and previews
	// only: a secret has no preview and values never ride.
	Params       []string           `json:"params,omitempty"`
	Placeholders []string           `json:"placeholders,omitempty"`
	Kept         []agentapi.KeptUse `json:"kept,omitempty"`
	// Script is the result transform: its source, hash and the kept names it may write.
	Script *apiTransformView `json:"script"`
}

type limitHitPayload struct {
	Limit    string  `json:"limit"`
	Measured float64 `json:"measured"`
	Max      float64 `json:"max"`
	Unit     string  `json:"unit"`
}

type exemptionPayload struct {
	dbLocation
	ConnectionID  string             `json:"connectionId"`
	Statement     string             `json:"statement"`
	Mode          string             `json:"mode"`
	Detected      string             `json:"detected"`
	Enforcement   string             `json:"enforcement"`
	Reason        string             `json:"reason"`
	Limits        []limitHitPayload  `json:"limits"`
	Plan          *planPayload       `json:"plan"`
	ReviewOffered bool               `json:"reviewOffered"`
	ReviewResult  bool               `json:"reviewResult"`
	Params        []string           `json:"params,omitempty"`
	Placeholders  []string           `json:"placeholders,omitempty"`
	Kept          []agentapi.KeptUse `json:"kept,omitempty"`
	Script        *apiTransformView  `json:"script"`
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
		dbLocation:   dbLocationOf(r.Target),
		ConnectionID: r.ConnectionID, Statement: r.Statement, Mode: r.Mode, Detected: string(r.Class),
		Enforcement: r.Enforcement, Reason: ex.Reason, Limits: limits, Plan: planPayloadOf(r.Plan),
		ReviewOffered: r.OfferReview, ReviewResult: r.ReviewOn,
		Params: r.Params, Placeholders: r.Placeholders, Kept: r.Kept, Script: queryScriptView(r.QueryScript),
	}
}

// queryScriptView is the transform as the card shows it; nil without one.
func queryScriptView(sc *apiScripts) *apiTransformView {
	if !sc.hasTransform() {
		return nil
	}
	return apiScriptPayloadOf(sc).Transform
}

type responsePayload struct {
	dbLocation
	ConnectionID string `json:"connectionId"`
	Statement    string `json:"statement"`
	IsError      bool   `json:"isError"`
	Bytes        int    `json:"bytes"`
	// Scripted says a script produced the held text from the raw result; RawBytes sizes that raw result.
	Scripted   bool `json:"scripted"`
	RawBytes   int  `json:"rawBytes"`
	AlreadyRan bool `json:"alreadyRan"`
	// Mode is the declared mode; Requested says the approver asked for this review rather than a setting.
	Mode      string `json:"mode"`
	Requested bool   `json:"requested"`
	// Params, Placeholders and Kept repeat what the prompt bound to the statement, for the review's Request section.
	Params       []string           `json:"params,omitempty"`
	Placeholders []string           `json:"placeholders,omitempty"`
	Kept         []agentapi.KeptUse `json:"kept,omitempty"`
}

type kubePayload struct {
	Context   string `json:"context"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"`
	Statement string `json:"statement"`
	// ReviewOffered shows "review the result" on a list, describe or logs card; ReviewResult says the
	// cluster's settings already do.
	ReviewOffered bool `json:"reviewOffered"`
	ReviewResult  bool `json:"reviewResult"`
	// Tool, Request, Changed, Kept and Script exist on a list, describe or logs read; Changed names the args
	// the prepare script set.
	Tool    string              `json:"tool"`
	Request *kubeRequestPayload `json:"request"`
	Changed []string            `json:"changed"`
	Kept    []agentapi.KeptUse  `json:"kept,omitempty"`
	Script  *apiScriptPayload   `json:"script"`
}

// kubeResponsePayload is the held result of a kube read, minus the result itself: the text is fetched
// by request id.
type kubeResponsePayload struct {
	Context   string `json:"context"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"`
	Statement string `json:"statement"`
	IsError   bool   `json:"isError"`
	Bytes     int    `json:"bytes"`
	// Requested says the approver asked for this review rather than a cluster or app setting.
	Requested bool `json:"requested"`
	// Scripted says a script produced the held text from the raw result; RawBytes sizes that raw result.
	Scripted bool `json:"scripted"`
	RawBytes int  `json:"rawBytes"`
	// Tool, Request and Changed repeat the prompt's final args and the ones the prepare script set, for the
	// review's Request section.
	Tool    string              `json:"tool"`
	Request *kubeRequestPayload `json:"request"`
	Changed []string            `json:"changed"`
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
		Origin: approvals.Origin{
			Client: r.Origin.Client, Cwd: r.Origin.Cwd, Session: r.Origin.SessionID,
			Task: r.Origin.TaskID, Step: r.Origin.StepID,
		},
		Reason: string(r.Reason), Title: promptTitle(r),
	}
	switch it.Kind {
	case approvals.KindKubeRead:
		c := r.Cluster
		it.Target = approvals.Target{Name: c.Context, Kind: "kubernetes", Location: kubeLocation(c)}
		it.Operation = approvals.Operation{Class: "read", Tone: "neutral"}
		p := kubePayload{
			Context: c.Context, Server: c.Server, Namespace: c.Namespace, Operation: c.Operation, Statement: r.Statement,
			ReviewOffered: r.OfferReview, ReviewResult: r.ReviewOn, Tool: c.Tool, Changed: []string{}, Kept: r.Kept,
			Script: apiScriptPayloadOf(c.Script),
		}
		if c.Operation != kubeDiscover {
			req := kubeRequestPayloadOf(c.Operation, c.Request)
			p.Request = &req
		}
		if c.Script != nil {
			p.Changed = append(p.Changed, c.Script.Changed...)
		}
		it.Payload = mustJSON(p)
	case approvals.KindKubeResponse:
		c := r.Cluster
		held := r.Response
		if held == nil {
			held = &ApprovalResponse{}
		}
		it.Reason = "preview"
		it.Target = approvals.Target{Name: c.Context, Kind: "kubernetes", Location: kubeLocation(c)}
		it.Operation = approvals.Operation{Class: "read", Tone: "neutral"}
		p := kubeResponsePayload{
			Context: c.Context, Server: c.Server, Namespace: c.Namespace, Operation: c.Operation, Statement: r.Statement,
			IsError: held.IsError, Bytes: len(held.Text), Requested: held.Requested,
			Scripted: held.Script != nil, RawBytes: len(held.Raw), Tool: c.Tool, Changed: []string{},
		}
		if c.Operation != kubeDiscover {
			req := kubeRequestPayloadOf(c.Operation, c.Request)
			p.Request = &req
		}
		if c.Script != nil {
			p.Changed = append(p.Changed, c.Script.Changed...)
		}
		it.Payload = mustJSON(p)
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
		it.Target, it.Operation, it.Payload = apiItemParts(r)
	case approvals.KindAPIResponse:
		it.Reason = "preview"
		it.Target, it.Operation = apiTargetOp(r.API)
		it.Payload = mustJSON(apiResponsePayloadOf(r))
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
			dbLocation:   dbLocationOf(r.Target),
			ConnectionID: r.ConnectionID, Statement: r.Statement,
			IsError: held.IsError, Bytes: len(held.Text), Scripted: held.Script != nil, RawBytes: len(held.Raw),
			AlreadyRan: r.Class != adapters.ClassRead,
			Mode:       r.Mode, Requested: held.Requested,
			Params: r.Params, Placeholders: r.Placeholders, Kept: r.Kept,
		})
	default:
		it.Target, it.Operation = dbTarget(r)
		it.Payload = mustJSON(queryPayload{
			dbLocation:   dbLocationOf(r.Target),
			ConnectionID: r.ConnectionID, Statement: r.Statement, Mode: r.Mode, Detected: string(r.Class),
			Enforcement: r.Enforcement, Explain: r.Explain, Plan: planPayloadOf(r.Plan),
			ReviewOffered: r.OfferReview, ReviewResult: r.ReviewOn,
			Params: r.Params, Placeholders: r.Placeholders, Kept: r.Kept, Script: queryScriptView(r.QueryScript),
		})
	}
	return it
}

func dbTarget(r ApprovalRequest) (approvals.Target, approvals.Operation) {
	t := r.Target
	loc := dbLocationOf(t)
	parts := make([]string, 0, 2)
	if loc.Host != "" {
		parts = append(parts, loc.Host)
	}
	if loc.Path != "" {
		parts = append(parts, loc.Path)
	}
	return approvals.Target{
			Name: r.ConnectionName, Kind: r.Kind, Color: t.Color, ReadOnly: t.ReadOnly, Location: strings.Join(parts, " · "),
		},
		approvals.Operation{Class: operationClass(r), Tone: toneOf(operationClass(r))}
}

// operationClass is the declared mode, or the detected class for an item that carries no mode. A masked
// browse is a read: the payload's mode names it, the class does not.
func operationClass(r ApprovalRequest) string {
	if r.Mode != "" && r.Mode != modeMasked {
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
	case req.Subject == approvals.KindAPIResponse && req.API != nil:
		return i18n.T("backend.prompt.dbMcpAPIResponse.title", map[string]string{"method": req.API.Method, "host": req.API.Host})
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
