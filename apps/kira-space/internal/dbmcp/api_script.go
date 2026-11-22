package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// apiScriptInputBytes bounds script.input, like the request body.
	apiScriptInputBytes = 1 << 20
	// transformBodyBytes is the masked body a transform reads; the agent cap applies to what it gets back.
	transformBodyBytes = agentapi.MaxScriptBodyBytes
	// transformInputBytes is the engine input budget of a transform: body text, raw json and headers.
	transformInputBytes = 8 << 20
	// apiScriptInputDisplay bounds the script input preview in the approval card.
	apiScriptInputDisplay = 2 << 10
	maxAPIResultBytes     = 512 << 10
)

const (
	apiScriptsOff   = "scripts are turned off in Kira Space's Studio MCP settings"
	apiScriptEmpty  = "script needs prepare, transform or both"
	apiScriptNoPrep = "script.input needs script.prepare"
)

type apiScriptArgs struct {
	Prepare   string          `json:"prepare,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Transform string          `json:"transform,omitempty"`
}

// apiScripts is what the handler holds from decode to delivery; the approval card shows these strings.
// The text the user approves is the text that runs: nothing edits it in between.
type apiScripts struct {
	Prepare, Transform string
	Input              json.RawMessage
	Changed            []string // set by prepare: the request fields it returned
	kept               *keptRun // nil unless a script mentions plain or secret
}

// apiOutcome is everything the final point needs: the raw masked response, and the transform output or failure.
type apiOutcome struct {
	Raw         agentapi.Result
	Transformed json.RawMessage // nil without a transform or on failure
	Failure     string          // transform failure text, "" otherwise
	// Inputs says what became of each declared input; empty without inputs.
	Inputs []agentapi.InputStatus
}

func newAPIScripts(a *apiScriptArgs) *apiScripts {
	in := bytes.TrimSpace(a.Input)
	if string(in) == "null" {
		in = nil
	}
	return &apiScripts{Prepare: a.Prepare, Transform: a.Transform, Input: in}
}

func (sc *apiScripts) hasTransform() bool { return sc != nil && sc.Transform != "" }

// preparedKeys is the canonical order of the fields a prepare script may return.
var preparedKeys = []string{"method", "url", "headers", "query", "body", "contentType", "form"}

// prepareAPI runs the prepare script on the agent's request and returns the merged request. A non-empty
// string is the refusal text; nothing was sent.
func (s *Server) prepareAPI(ctx context.Context, in agentapi.Input, sc *apiScripts) (agentapi.Input, string) {
	req, err := json.Marshal(struct {
		Method      string               `json:"method"`
		URL         string               `json:"url"`
		Headers     []agentapi.NameValue `json:"headers"`
		Query       []agentapi.NameValue `json:"query"`
		Body        string               `json:"body"`
		ContentType string               `json:"contentType"`
		Form        []agentapi.NameValue `json:"form"`
	}{in.Method, in.URL, nonNilNV(in.Headers), nonNilNV(in.Query), in.Body, in.ContentType, nonNilNV(in.Form)})
	if err != nil {
		s.log.Warn("dbmcp: prepare input", "scope", "dbmcp", "err", err)
		return in, "prepare script failed: internal error. Nothing was sent."
	}
	input := sc.Input
	if len(input) == 0 {
		input = json.RawMessage("null")
	}
	var merged agentapi.Input
	var changed []string
	_, failure := s.execScript(ctx, scriptRun{
		Stage: stagePrepare, Source: sc.Prepare,
		Bindings: []jsengine.Binding{{Name: "request", Value: req}, {Name: "input", Value: input}}, Vars: sc.kept,
		Validate: func(out jsengine.Result) string {
			var err error
			if merged, changed, err = applyPrepared(in, out); err != nil {
				return fmt.Sprintf("prepare script returned an invalid request: %s. Nothing was sent.", err)
			}
			return sc.kept.checkPrepared(in, merged, sc.Prepare)
		},
	}, func(e *jsengine.Error) string {
		return fmt.Sprintf("prepare script failed: %s. Nothing was sent.", scriptErrText(e))
	})
	if failure != "" {
		return in, failure
	}
	sc.Changed = changed
	// What prepare saved is what {{plain.x}} and {{secret.x}} in this request send.
	merged.Staged = sc.kept.stagedEntries()
	return merged, ""
}

func nonNilNV(in []agentapi.NameValue) []agentapi.NameValue {
	if in == nil {
		return []agentapi.NameValue{}
	}
	return in
}

// applyPrepared merges a prepare script's return value into the agent's request. Returned fields
// replace the agent's; absent ones keep them. changed lists the fields the script set.
func applyPrepared(in agentapi.Input, out jsengine.Result) (agentapi.Input, []string, error) {
	if out.Type != "object" {
		return in, nil, fmt.Errorf("the script must return an object, not %s", out.Type)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out.JSON, &fields); err != nil {
		return in, nil, errors.New("the script result is not an object")
	}
	for k := range fields {
		if !slices.Contains(preparedKeys, k) {
			return in, nil, fmt.Errorf("unknown key %q; allowed: %s", k, strings.Join(preparedKeys, ", "))
		}
	}
	var changed []string
	var fail error
	keep := func(err error) {
		if fail == nil {
			fail = err
		}
	}
	setString := func(key string, dst *string) {
		raw, ok := fields[key]
		if !ok {
			return
		}
		if err := json.Unmarshal(raw, dst); err != nil || bytes.TrimSpace(raw)[0] != '"' {
			keep(fmt.Errorf("%s must be a string", key))
			return
		}
		changed = append(changed, key)
	}
	setPairs := func(key string, dst *[]agentapi.NameValue) {
		raw, ok := fields[key]
		if !ok {
			return
		}
		pairs, err := decodePairs(key, raw)
		if err != nil {
			keep(err)
			return
		}
		*dst = pairs
		changed = append(changed, key)
	}
	setString("method", &in.Method)
	setString("url", &in.URL)
	setPairs("headers", &in.Headers)
	setPairs("query", &in.Query)
	setPairs("form", &in.Form)
	_, contentTypeSet := fields["contentType"]
	setString("contentType", &in.ContentType)
	if raw, ok := fields["body"]; ok {
		body, jsonBody, err := decodeBody(raw)
		if err != nil {
			keep(err)
		} else {
			in.Body = body
			changed = append(changed, "body")
			if jsonBody && !contentTypeSet && in.ContentType == "" && !hasContentTypeHeader(in.Headers) {
				in.ContentType = "application/json"
				changed = append(changed, "contentType")
			}
		}
	}
	if fail != nil {
		return in, nil, fail
	}
	if in.Body != "" && len(in.Form) > 0 {
		return in, nil, errors.New("body and form cannot both be set")
	}
	changed = canonicalOrder(changed)
	return in, changed, nil
}

func canonicalOrder(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range preparedKeys {
		if slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	return out
}

func hasContentTypeHeader(hs []agentapi.NameValue) bool {
	return slices.ContainsFunc(hs, func(h agentapi.NameValue) bool { return strings.EqualFold(h.Name, "Content-Type") })
}

// decodePairs reads an array of {name, value} string objects, naming the path of the first fault.
func decodePairs(key string, raw json.RawMessage) ([]agentapi.NameValue, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || bytes.TrimSpace(raw)[0] != '[' {
		return nil, fmt.Errorf("%s must be an array of {name, value}", key)
	}
	out := make([]agentapi.NameValue, 0, len(items))
	for i, item := range items {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(item, &m); err != nil || bytes.TrimSpace(item)[0] != '{' {
			return nil, fmt.Errorf("%s[%d] must be an object with name and value", key, i)
		}
		var nv agentapi.NameValue
		for k := range m {
			if k != "name" && k != "value" {
				return nil, fmt.Errorf("%s[%d] has unknown key %q", key, i, k)
			}
		}
		for _, f := range []struct {
			name string
			dst  *string
		}{{"name", &nv.Name}, {"value", &nv.Value}} {
			v, ok := m[f.name]
			if !ok || json.Unmarshal(v, f.dst) != nil || bytes.TrimSpace(v)[0] != '"' {
				return nil, fmt.Errorf("%s[%d].%s must be a string", key, i, f.name)
			}
		}
		out = append(out, nv)
	}
	return out, nil
}

// decodeBody takes a string as is, an object or array as its compact JSON text (the engine's slice, so
// big integers stay exact).
func decodeBody(raw json.RawMessage) (text string, isJSON bool, err error) {
	raw = bytes.TrimSpace(raw)
	switch raw[0] {
	case '"':
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", false, errors.New("body must be a string, object or array")
		}
		return text, false, nil
	case '{', '[':
		return string(raw), true, nil
	}
	return "", false, errors.New("body must be a string, object or array")
}

// transformAPI runs the transform on the masked response. The outcome carries the agent-bound output or
// the failure text; either way the request was already sent.
func (s *Server) transformAPI(ctx context.Context, plan agentapi.Planned, sc *apiScripts, raw agentapi.Result) apiOutcome {
	o := apiOutcome{Raw: raw}
	resp, err := responseBinding(raw)
	if err != nil {
		s.log.Warn("dbmcp: transform input", "scope", "dbmcp", "err", err)
		o.Failure = s.maskText(raw, fmt.Sprintf("transform script failed: internal error. The request was sent (HTTP %d); its response is not returned.", raw.Status))
		return o
	}
	reqB, _ := json.Marshal(struct {
		Method string `json:"method"`
		URL    string `json:"url"`
	}{plan.Method, plan.URL})
	bindings := []jsengine.Binding{{Name: "response", Value: resp}, {Name: "request", Value: reqB}}
	// Secret values the script saved leave its output too.
	mask := func(text string) string { return sc.kept.mask(s.maskText(raw, text)) }
	total := len(resp) + len(reqB)
	tail := fmt.Sprintf("The request was sent (HTTP %d); its response is not returned.", raw.Status)
	_, failure := s.execScript(ctx, scriptRun{
		Stage: stageTransform, Source: sc.Transform, Bindings: bindings, Vars: sc.kept,
		Limits: jsengine.Limits{InputBytes: transformInputBytes, OutputBytes: plan.MaxResponseBytes},
		Validate: func(out jsengine.Result) string {
			masked := json.RawMessage(mask(string(out.JSON)))
			if !json.Valid(masked) {
				return "the script result could not be masked; nothing was returned"
			}
			o.Transformed = masked
			return ""
		},
	}, func(e *jsengine.Error) string {
		var text string
		switch {
		case e.Kind == jsengine.KindInput && total > transformInputBytes:
			text = fmt.Sprintf("transform script failed: the response is too large for a script (%d bytes of input, limit %d). %s", total, transformInputBytes, tail)
		case e.Kind == jsengine.KindOutput:
			text = fmt.Sprintf("transform script failed: %s. %s Return less, or raise maxResponseBytes (max %d).", scriptErrText(e), tail, maxAPIResultBytes)
		default:
			text = fmt.Sprintf("transform script failed: %s. %s", scriptErrText(e), tail)
		}
		return mask(text)
	})
	o.Failure = failure
	return o
}

func (s *Server) maskText(raw agentapi.Result, text string) string {
	if raw.Mask == nil {
		return text
	}
	return raw.Mask(text)
}

// responseBinding is the transform's response global. json is the body as a raw JSON value when it is
// valid, complete and not binary, else null.
func responseBinding(r agentapi.Result) (json.RawMessage, error) {
	var parsed json.RawMessage
	if !r.Binary && !r.Truncated && json.Valid([]byte(r.Body)) {
		parsed = json.RawMessage(r.Body)
	}
	return json.Marshal(struct {
		Status      int                  `json:"status"`
		StatusText  string               `json:"statusText"`
		Headers     []agentapi.NameValue `json:"headers"`
		ContentType string               `json:"contentType"`
		Body        string               `json:"body"`
		JSON        json.RawMessage      `json:"json"`
		BodyBytes   int                  `json:"bodyBytes"`
		Truncated   bool                 `json:"truncated"`
		Binary      bool                 `json:"binary"`
		FinalURL    string               `json:"finalUrl"`
		Redirects   []agentapi.Redirect  `json:"redirects"`
	}{r.Status, r.StatusText, nonNilNV(r.Headers), r.ContentType, r.Body, parsed, r.BodyBytes, r.Truncated, r.Binary, r.FinalURL, r.Redirects})
}

// deliverAPI is the one point where the agent-bound result is final: the response review runs first,
// then the values the transform staged are saved. requested says the user asked for the review on the
// approval card.
func (s *Server) deliverAPI(ctx context.Context, plan agentapi.Planned, sc *apiScripts, o apiOutcome, requested bool) (*mcp.CallToolResult, any, error) {
	res, out, err := apiAgentResult(sc, o, nil, nil, nil)
	// Staged values are held until here; every way out but the final point drops them.
	defer sc.dropKept()
	// Read after the send: the user may change the setting while the request runs.
	if rv := s.apiReviewFor(plan, sc, requested); rv.On {
		if refusal := s.reviewAPI(ctx, plan, sc, o, rv, res, err); refusal != "" {
			return errResult(refusal)
		}
	}
	// Final point: the agent gets res, and the staged values are stored.
	if o.Failure == "" && sc != nil && ctx.Err() == nil {
		if saved, failed, deleted := sc.kept.commit(keptSourceOf(ctx, plan), plan.Origin); len(saved)+len(failed)+len(deleted) > 0 {
			return apiAgentResult(sc, o, saved, failed, deleted)
		}
	}
	return res, out, err
}

func (sc *apiScripts) dropKept() {
	if sc != nil {
		sc.kept.drop()
	}
}

// apiAgentResult is what the agent receives: the failure text, the transform's result with a summary of
// the values it kept, or the masked response as is.
func apiAgentResult(sc *apiScripts, o apiOutcome, saved []keptSummary, failed []keptFailure, deleted []string) (*mcp.CallToolResult, any, error) {
	if o.Failure != "" {
		return errResult(o.Failure)
	}
	extras := []struct {
		key string
		v   any
		n   int
	}{{"inputs", o.Inputs, len(o.Inputs)}, {"saved", saved, len(saved)}, {"notSaved", failed, len(failed)}, {"deleted", deleted, len(deleted)}}
	tail := ""
	for _, f := range extras {
		if f.n == 0 {
			continue
		}
		b, err := json.Marshal(f.v)
		if err != nil {
			continue
		}
		tail += `,"` + f.key + `":` + string(b)
	}
	if sc.hasTransform() {
		return textResult(`{"result":` + string(o.Transformed) + tail + `}`)
	}
	if tail == "" {
		return jsonResult(o.Raw)
	}
	body, err := json.Marshal(o.Raw)
	if err != nil {
		return nil, nil, fmt.Errorf("dbmcp: encode result: %w", err)
	}
	return textResult(string(body[:len(body)-1]) + tail + `}`)
}

// apiPrepareView is the prepare script as the approval card shows it.
type apiPrepareView struct {
	Source         string   `json:"source"`
	Hash           string   `json:"hash"`
	Changed        []string `json:"changed"`
	Input          string   `json:"input"`
	InputTruncated bool     `json:"inputTruncated"`
	// Writes are the values the prepare script saved: they interpolate in this request and are stored
	// after the final response.
	Writes []keepView `json:"writes"`
}

// keepView is a value a script saves (or a saved value it deletes) as the card shows it. Never a secret's value.
type keepView struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	Items            int    `json:"items"`
	Bytes            int    `json:"bytes"`
	Secret           bool   `json:"secret"`
	Preview          string `json:"preview"`
	PreviewTruncated bool   `json:"previewTruncated"`
	Deleted          bool   `json:"deleted"`
}

func keepViewsOf(in []HeldKeep) []keepView {
	out := make([]keepView, len(in))
	for i, h := range in {
		out[i] = keepView(h)
	}
	return out
}

// apiTransformView is the transform script as the approval card shows it.
type apiTransformView struct {
	Source string `json:"source"`
	Hash   string `json:"hash"`
	// MayKeep are the saved variables the transform may write or delete, with their sites in Source.
	MayKeep []keptMayKeep `json:"mayKeep"`
}

type apiScriptPayload struct {
	Prepare   *apiPrepareView   `json:"prepare"`
	Transform *apiTransformView `json:"transform"`
}

// apiScriptPayloadOf projects the scripts into the item payload; nil when there is none.
func apiScriptPayloadOf(sc *apiScripts) *apiScriptPayload {
	if sc == nil {
		return nil
	}
	out := &apiScriptPayload{}
	if sc.Prepare != "" {
		input, cut := capUTF8(string(sc.Input), apiScriptInputDisplay)
		out.Prepare = &apiPrepareView{
			Source: sc.Prepare, Hash: jsengine.SourceHash(sc.Prepare), Changed: append([]string{}, sc.Changed...),
			Input: input, InputTruncated: cut, Writes: keepViewsOf(sc.kept.stagedViews()),
		}
	}
	if sc.Transform != "" {
		out.Transform = &apiTransformView{Source: sc.Transform, Hash: jsengine.SourceHash(sc.Transform), MayKeep: []keptMayKeep{}}
		if sc.kept != nil {
			out.Transform.MayKeep = append(out.Transform.MayKeep, sc.kept.mayKeep...)
		}
	}
	return out
}
