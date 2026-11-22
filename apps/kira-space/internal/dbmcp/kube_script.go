package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A kube read's script has two stages. prepare runs before the card on the agent's args and returns a subset
// of the tool's own args, through the same clamps and required checks, so the card shows the final read.
// It never changes the cluster, the tool or the verb. transform runs after the read on the JSON the tool
// returns. Scripts read saved values through plain only: a cluster arg carries no credential, and a secret
// is never readable. A value prepare saves is only saved: kube args take no templates.
const (
	kubeScriptInputBytes  = maxHeldResultBytes
	kubeScriptOutputBytes = 4 << 20
	kubeNothingRead       = "Nothing was read."
)

type kubeScriptArgs struct {
	Prepare   string `json:"prepare,omitempty"`
	Transform string `json:"transform,omitempty"`
}

// kubeRequest is every arg a kube read takes, minus the cluster: one shape so prepare, the card, the
// statement and the read use the same values.
type kubeRequest struct {
	Kind, Name, Namespace, LabelSelector, Container string
	Previous, Timestamps                            bool
	Tail, MaxRows                                   int
}

// kubeCall is one data read as the handler decoded it.
type kubeCall struct {
	tool, op, clusterID string
	req                 kubeRequest
	script              *kubeScriptArgs
}

// kubePrepareKeys are the args a prepare script may return, per operation, in the order the card lists them.
var kubePrepareKeys = map[string][]string{
	kubeList:     {"kind", "namespace", "labelSelector", "maxRows"},
	kubeDescribe: {"kind", "namespace", "name"},
	kubeLogs:     {"namespace", "name", "container", "previous", "timestamps", "tail"},
}

// kubeRequired are the args a read cannot do without.
var kubeRequired = map[string][]string{
	kubeList:     {"kind"},
	kubeDescribe: {"kind", "name"},
	kubeLogs:     {"namespace", "name"},
}

// kubeNeeds is the refusal for a read missing a required arg; it names the op and the fix.
func kubeNeeds(op string) string {
	switch op {
	case kubeList:
		return "op list needs kind (a key from discover)"
	case kubeDescribe:
		return "op describe needs kind and name"
	}
	return "op logs needs namespace and name (the pod)"
}

// kubeClamp applies the tool's own limits.
func kubeClamp(op string, r kubeRequest) kubeRequest {
	switch op {
	case kubeList:
		r.MaxRows = clampOr(r.MaxRows, kubeListDefaultRows, kubeListMaxRows)
	case kubeLogs:
		r.Tail = clampOr(r.Tail, kubeLogsDefaultTail, kubeLogsMaxTail)
	}
	return r
}

// kubeFields is the request as the tool's own args, keyed by name, for the script and the card.
func kubeFields(op string, r kubeRequest) map[string]any {
	all := map[string]any{
		"kind": r.Kind, "name": r.Name, "namespace": r.Namespace, "labelSelector": r.LabelSelector,
		"container": r.Container, "previous": r.Previous, "timestamps": r.Timestamps,
		"tail": r.Tail, "maxRows": r.MaxRows,
	}
	out := make(map[string]any, len(kubePrepareKeys[op]))
	for _, k := range kubePrepareKeys[op] {
		out[k] = all[k]
	}
	return out
}

func (r kubeRequest) text(key string) string {
	switch key {
	case "kind":
		return r.Kind
	case "name":
		return r.Name
	case "namespace":
		return r.Namespace
	case "labelSelector":
		return r.LabelSelector
	case "container":
		return r.Container
	}
	return ""
}

// openKubeScripts validates the script argument, syntax-checks both stages and snapshots the kept variables
// they read, all before the cluster is touched. nil without a script. A non-empty string is the refusal text.
func (s *Server) openKubeScripts(ctx context.Context, tool string, a *kubeScriptArgs) (*apiScripts, string) {
	if a == nil {
		return nil, ""
	}
	if !s.scriptsOn() {
		return nil, apiScriptsOff + ". " + kubeNothingRead
	}
	if a.Prepare == "" && a.Transform == "" {
		return nil, apiScriptEmpty
	}
	for _, p := range []struct {
		stage scriptStage
		src   string
	}{{stagePrepare, a.Prepare}, {stageTransform, a.Transform}} {
		if p.src == "" {
			continue
		}
		if text := s.checkScript(ctx, p.stage, p.src, string(p.stage)+" script", kubeNothingRead); text != "" {
			return nil, text
		}
	}
	sc := &apiScripts{Prepare: a.Prepare, Transform: a.Transform}
	k, refusal := s.keptRunFor(keptScope(ctx), sc, tool, kubeNothingRead)
	if refusal != "" {
		return nil, refusal
	}
	sc.kept = k
	return sc, ""
}

// keptUses are the kept variables the scripts read, as they stood at plan time.
func (sc *apiScripts) keptUses() []agentapi.KeptUse {
	if sc == nil || sc.kept == nil {
		return nil
	}
	return sc.kept.uses
}

// prepareKube runs the prepare script on the agent's request and returns the final one. A non-empty string
// is the refusal text; nothing was read.
func (s *Server) prepareKube(ctx context.Context, call kubeCall, sc *apiScripts) (kubeRequest, string) {
	in := call.req
	req, err := json.Marshal(kubeFields(call.op, in))
	if err != nil {
		s.log.Warn("dbmcp: prepare input", "scope", "dbmcp", "err", err)
		return in, "prepare script failed: internal error. " + kubeNothingRead
	}
	var merged kubeRequest
	var changed []string
	_, failure := s.execScript(ctx, scriptRun{
		Stage: stagePrepare, Source: sc.Prepare,
		Bindings: []jsengine.Binding{{Name: "request", Value: req}}, Vars: sc.kept,
		Validate: func(out jsengine.Result) string {
			var err error
			if merged, changed, err = applyKubePrepared(call.op, in, out); err != nil {
				return fmt.Sprintf("prepare script returned an invalid request: %s. %s", err, kubeNothingRead)
			}
			return ""
		},
	}, func(e *jsengine.Error) string {
		return fmt.Sprintf("prepare script failed: %s. %s", scriptErrText(e), kubeNothingRead)
	})
	if failure != "" {
		return in, failure
	}
	sc.Changed = changed
	return merged, ""
}

// applyKubePrepared merges a prepare script's return value into the agent's request. Returned args replace
// the agent's; absent ones keep them. changed lists the args the script set, in card order.
func applyKubePrepared(op string, in kubeRequest, out jsengine.Result) (kubeRequest, []string, error) {
	if out.Type != "object" {
		return in, nil, fmt.Errorf("the script must return an object, not %s", out.Type)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out.JSON, &fields); err != nil {
		return in, nil, errors.New("the script result is not an object")
	}
	allowed := kubePrepareKeys[op]
	for k := range fields {
		if !slices.Contains(allowed, k) {
			return in, nil, fmt.Errorf("unknown key %q; allowed: %s", k, strings.Join(allowed, ", "))
		}
	}
	var changed []string
	for _, key := range allowed {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if err := setKubeField(&in, key, bytes.TrimSpace(raw)); err != nil {
			return in, nil, err
		}
		changed = append(changed, key)
	}
	for _, key := range kubeRequired[op] {
		if strings.TrimSpace(in.text(key)) == "" {
			return in, nil, fmt.Errorf("%s cannot be empty", key)
		}
	}
	return kubeClamp(op, in), changed, nil
}

// setKubeField decodes one prepare-script value into the request field named key.
func setKubeField(in *kubeRequest, key string, raw json.RawMessage) error {
	switch key {
	case "previous", "timestamps":
		if string(raw) != "true" && string(raw) != "false" {
			return fmt.Errorf("%s must be true or false", key)
		}
		v := string(raw) == "true"
		if key == "previous" {
			in.Previous = v
		} else {
			in.Timestamps = v
		}
	case "tail", "maxRows":
		var v float64
		if json.Unmarshal(raw, &v) != nil || raw[0] == '"' || v != math.Trunc(v) || math.Abs(v) > 1<<31 {
			return fmt.Errorf("%s must be a whole number", key)
		}
		if key == "tail" {
			in.Tail = int(v)
		} else {
			in.MaxRows = int(v)
		}
	default:
		var v string
		if json.Unmarshal(raw, &v) != nil || raw[0] != '"' {
			return fmt.Errorf("%s must be a string", key)
		}
		switch key {
		case "kind":
			in.Kind = v
		case "name":
			in.Name = v
		case "namespace":
			in.Namespace = v
		case "labelSelector":
			in.LabelSelector = v
		case "container":
			in.Container = v
		}
	}
	return nil
}

// kubeScriptInput describes what the transform read: metadata only, never a row or a line.
type kubeScriptInput struct {
	RowCount  int  `json:"rowCount"`
	Truncated bool `json:"truncated"`
}

func kubeScriptInputOf(op, raw string) kubeScriptInput {
	var r struct {
		RowCount          int               `json:"rowCount"`
		Lines             []json.RawMessage `json:"lines"`
		Truncated         bool              `json:"truncated"`
		ManifestTruncated bool              `json:"manifestTruncated"`
	}
	_ = json.Unmarshal([]byte(raw), &r)
	in := kubeScriptInput{Truncated: r.Truncated || r.ManifestTruncated}
	switch op {
	case kubeList:
		in.RowCount = r.RowCount
	case kubeLogs:
		in.RowCount = len(r.Lines)
	}
	return in
}

// kubeScriptOutcome is what the final point needs: the JSON the script read and its masked output or failure.
type kubeScriptOutcome struct {
	Raw     string
	Output  json.RawMessage // nil on failure
	Failure string
	Input   kubeScriptInput
	sc      *apiScripts
}

// transformKube runs the transform on the tool's JSON. The read already happened either way.
func (s *Server) transformKube(ctx context.Context, call kubeCall, req kubeRequest, sc *apiScripts, raw string) *kubeScriptOutcome {
	o := &kubeScriptOutcome{Raw: raw, Input: kubeScriptInputOf(call.op, raw), sc: sc}
	reqJSON, err := json.Marshal(kubeFields(call.op, req))
	if err != nil || !json.Valid([]byte(raw)) {
		s.log.Warn("dbmcp: script input", "scope", "dbmcp")
		o.Failure = "script failed: internal error. The read ran; its result is not returned."
		return o
	}
	bindings := []jsengine.Binding{{Name: "result", Value: json.RawMessage(raw)}, {Name: "request", Value: reqJSON}}
	mask := sc.kept.mask
	_, failure := s.execScript(ctx, scriptRun{
		Stage: stageTransform, Source: sc.Transform, Bindings: bindings, Vars: sc.kept,
		Limits: jsengine.Limits{InputBytes: kubeScriptInputBytes, OutputBytes: kubeScriptOutputBytes},
		Validate: func(out jsengine.Result) string {
			masked := json.RawMessage(mask(string(out.JSON)))
			if !json.Valid(masked) {
				return "the script result could not be masked; nothing was returned"
			}
			o.Output = masked
			return ""
		},
	}, func(e *jsengine.Error) string {
		const tail = "The read ran; its result is not returned."
		var text string
		switch {
		case e.Kind == jsengine.KindInput && len(raw) > kubeScriptInputBytes:
			text = fmt.Sprintf("script failed: the result is %.1f MB, over the %d MB script input limit. %s Ask for fewer rows or a smaller tail.", float64(len(raw))/(1<<20), kubeScriptInputBytes>>20, tail)
		case e.Kind == jsengine.KindOutput:
			text = fmt.Sprintf("script failed: %s. %s Return less.", scriptErrText(e), tail)
		default:
			text = fmt.Sprintf("script failed: %s. %s", scriptErrText(e), tail)
		}
		return mask(text)
	})
	o.Failure = failure
	return o
}

// kubeScriptEnvelope is what the agent receives when a script ran.
type kubeScriptEnvelope struct {
	Output   json.RawMessage `json:"output"`
	Input    kubeScriptInput `json:"input"`
	Saved    []keptSummary   `json:"saved,omitempty"`
	NotSaved []keptFailure   `json:"notSaved,omitempty"`
	Deleted  []string        `json:"deleted,omitempty"`
}

// agentResult is the failure text, or the envelope with a summary of the values the script kept.
func (o *kubeScriptOutcome) agentResult(saved []keptSummary, failed []keptFailure, deleted []string) (*mcp.CallToolResult, any, error) {
	if o.Failure != "" {
		return errResult(o.Failure)
	}
	return jsonResult(kubeScriptEnvelope{Output: o.Output, Input: o.Input, Saved: saved, NotSaved: failed, Deleted: deleted})
}

// heldOf is the review entry of a scripted result: the envelope the agent gets on Send, the JSON the script
// read, the script and the values it will keep.
func (o *kubeScriptOutcome) heldOf(res *mcp.CallToolResult, requested bool) ApprovalResponse {
	held := ApprovalResponse{IsError: res.IsError, Text: resultText(res), Requested: requested, Raw: o.Raw}
	held.Script = &HeldScript{Source: o.sc.Transform, Hash: jsengine.SourceHash(o.sc.Transform)}
	if o.Failure == "" {
		held.Keep = o.sc.kept.stagedViews()
		held.staged = o.sc.kept
	}
	held.Bytes = len(held.Text) + len(held.Raw) + len(held.Script.Source)
	return held
}

// keptSourceOfKube is where a value kept by this read comes from. Request is display text: the cluster
// context and the kubectl command the read stands for.
func keptSourceOfKube(ctx context.Context, c kubernetes.Cluster, tool, statement string) keptvars.Source {
	req, _ := capUTF8(c.Context+": "+statement, 512)
	o := originFrom(ctx)
	return keptvars.Source{
		Kind: keptvars.SourceScript, MCP: "kube", Tool: tool, Request: req,
		Client: o.Client, TaskID: o.TaskID, SessionID: o.SessionID,
	}
}

// kubeRequestPayload is the final request as the card shows it.
type kubeRequestPayload struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	LabelSelector string `json:"labelSelector"`
	Pod           string `json:"pod"`
	Container     string `json:"container"`
	Previous      bool   `json:"previous"`
	Timestamps    bool   `json:"timestamps"`
	Tail          int    `json:"tail"`
	MaxRows       int    `json:"maxRows"`
}

// kubeRequestPayloadOf maps a pod's name to the payload's pod for logs, and to name for any other read.
func kubeRequestPayloadOf(op string, r kubeRequest) kubeRequestPayload {
	p := kubeRequestPayload{
		Kind: r.Kind, Namespace: r.Namespace, LabelSelector: r.LabelSelector, Container: r.Container,
		Previous: r.Previous, Timestamps: r.Timestamps, Tail: r.Tail, MaxRows: r.MaxRows,
	}
	if op == kubeLogs {
		p.Pod = r.Name
	} else {
		p.Name = r.Name
	}
	return p
}
