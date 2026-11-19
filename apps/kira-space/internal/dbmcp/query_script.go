package dbmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/jsengine"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// run_query's script is a result transform only. The statement is literal agent text, so the user approves
// exactly what runs and the class is decided from it; a script that built SQL would run a statement the
// classifier, the table allow list and the user never saw. The script runs after execute, masking, column
// review and rendering, reads only the rendered (masked) result and cannot change the statement, mode, lane
// or verdict.
const (
	// queryScriptInputBytes is the rendered result a script may read; it equals the held-result cap.
	queryScriptInputBytes = maxHeldResultBytes
	// queryScriptOutputBytes caps what the script returns to the agent.
	queryScriptOutputBytes = 4 << 20
	queryScriptNothingRan  = "Nothing ran."
	// queryKeptNotAllowed: secret values and values the user typed belong to api_request; a SQL path would need
	// its own origin rule and result mask.
	queryKeptNotAllowed = "kept variable %q is secret or was typed by the user; those are used in api_request only, never in run_query. " + queryScriptNothingRan
)

// openQueryScript validates run_query's script argument, syntax-checks it and snapshots the kept variables it
// reads, all before the connection is touched. nil without a script. A non-empty string is the refusal text.
func (s *Server) openQueryScript(ctx context.Context, source string) (*apiScripts, string) {
	if source == "" {
		return nil, ""
	}
	if !s.scriptsOn() {
		return nil, apiScriptsOff
	}
	if text := s.checkScript(ctx, stageTransform, source, "script", queryScriptNothingRan); text != "" {
		return nil, text
	}
	// The transform shape carries the source, kept bindings, card view and hash; there is no prepare stage.
	sc := &apiScripts{Transform: source}
	k, refusal := s.keptRunFor(keptScope(ctx), sc, queryScriptNothingRan)
	if refusal != "" {
		return nil, refusal
	}
	// A read that cannot resolve now would fail after the statement ran, so it refuses here.
	if k != nil {
		for _, st := range k.sites[stageTransform] {
			if st.Op != "get" {
				continue
			}
			i := slices.IndexFunc(k.uses, func(u agentapi.KeptUse) bool { return u.Name == st.Name })
			if i < 0 {
				return nil, unknownKeptText(st.Name) + ". " + queryScriptNothingRan
			}
			if u := k.uses[i]; u.Secret || u.Source.Kind == keptvars.SourceUser {
				return nil, fmt.Sprintf(queryKeptNotAllowed, st.Name)
			}
		}
	}
	sc.kept = k
	return sc, ""
}

// queryScriptOutcome is what the final point needs: the rendered result the script read and its masked output
// or failure.
type queryScriptOutcome struct {
	// Raw is the rendered result the script read.
	Raw string
	// Output is the masked script result; nil on failure.
	Output  json.RawMessage
	Failure string
	Input   queryScriptInput
}

// queryScriptInput describes what the script read: metadata only, never a cell.
type queryScriptInput struct {
	Kind      string `json:"kind"`
	RowCount  int    `json:"rowCount"`
	Truncated bool   `json:"truncated"`
}

func queryScriptInputOf(raw string) queryScriptInput {
	var in queryScriptInput
	_ = json.Unmarshal([]byte(raw), &in)
	return in
}

// transformQuery runs the script on the rendered result. The query already ran either way.
func (s *Server) transformQuery(ctx context.Context, rv queryResolution, raw string) *queryScriptOutcome {
	sc := rv.script
	o := &queryScriptOutcome{Raw: raw, Input: queryScriptInputOf(raw)}
	if !json.Valid([]byte(raw)) {
		s.log.Warn("dbmcp: script input", "scope", "dbmcp")
		o.Failure = ranSuffix("script failed: internal error. The query ran; its result is not returned.", rv)
		return o
	}
	bindings := append([]jsengine.Binding{{Name: "result", Value: json.RawMessage(raw)}}, sc.kept.Bindings(stageTransform)...)
	mask := sc.kept.mask
	_, failure := s.execScript(ctx, scriptRun{
		Stage: stageTransform, Source: sc.Transform, Bindings: bindings, ConnectionID: rv.summary.ID,
		Limits: jsengine.Limits{InputBytes: queryScriptInputBytes, OutputBytes: queryScriptOutputBytes},
		Validate: func(out jsengine.Result) string {
			masked := json.RawMessage(mask(string(out.JSON)))
			if !json.Valid(masked) {
				return "the script result could not be masked; nothing was returned"
			}
			o.Output = masked
			return ""
		},
	}, func(e *jsengine.Error) string {
		var text string
		switch {
		case e.Kind == jsengine.KindInput && len(raw) > queryScriptInputBytes:
			text = fmt.Sprintf("script failed: the result is %.1f MB, over the %d MB script input limit. The query ran; its result is not returned. Ask for fewer rows or columns.", float64(len(raw))/(1<<20), queryScriptInputBytes>>20)
		case e.Kind == jsengine.KindOutput:
			text = fmt.Sprintf("script failed: %s. The query ran; its result is not returned. Return less.", scriptErrText(e))
		default:
			text = fmt.Sprintf("script failed: %s. The query ran; its result is not returned.", scriptErrText(e))
		}
		return ranSuffix(mask(text), rv)
	})
	o.Failure = failure
	return o
}

// queryScriptEnvelope is what the agent receives when a script ran.
type queryScriptEnvelope struct {
	Kind         string           `json:"kind"`
	Output       json.RawMessage  `json:"output"`
	Input        queryScriptInput `json:"input"`
	Kept         []keptSummary    `json:"kept,omitempty"`
	KeptNotSaved []keptFailure    `json:"keptNotSaved,omitempty"`
}

// agentResult is the failure text, or the envelope with a summary of the values the script kept.
func (o *queryScriptOutcome) agentResult(saved []keptSummary, failed []keptFailure) (*mcp.CallToolResult, any, error) {
	if o.Failure != "" {
		return errResult(o.Failure)
	}
	return jsonResult(queryScriptEnvelope{Kind: "script", Output: o.Output, Input: o.Input, Kept: saved, KeptNotSaved: failed})
}

// heldOf is the review entry of a scripted result: the envelope the agent gets on Send, the rendered result the
// script read, the script and the values it will keep.
func (o *queryScriptOutcome) heldOf(rv queryResolution, res *mcp.CallToolResult, err error) ApprovalResponse {
	held := ApprovalResponse{IsError: true, Requested: rv.reviewRequested}
	if res != nil {
		held.IsError = res.IsError
		held.Text = resultText(res)
	} else if err != nil {
		held.Text = err.Error()
	}
	held.Raw = o.Raw
	held.Script = &HeldScript{Source: rv.script.Transform, Hash: jsengine.SourceHash(rv.script.Transform)}
	if o.Failure == "" {
		held.Keep = rv.script.kept.stagedViews()
	}
	held.Bytes = len(held.Text) + len(held.Raw) + len(held.Script.Source)
	return held
}

// keptSourceOfQuery is where a value kept by this query comes from. Request is display text: the statement as
// the connection's adapter would keep it in an audit trail, or its length when it cannot be redacted.
func (s *Server) keptSourceOfQuery(ctx context.Context, args runQueryArgs, rv queryResolution) keptvars.Source {
	statement := ""
	if r, ok := s.cfg.Query.(interface {
		RedactStatement(connectionID, statement string) (string, bool)
	}); ok {
		statement, _ = r.RedactStatement(args.ConnectionID, args.SQL)
	}
	if statement == "" {
		statement = fmt.Sprintf("statement withheld (%d characters)", len([]rune(args.SQL)))
	}
	req, _ := capUTF8(rv.summary.Name+": "+statement, 512)
	o := originFrom(ctx)
	return keptvars.Source{
		Kind: keptvars.SourceScript, MCP: "db", Tool: "run_query", Request: req,
		Client: o.Client, TaskID: o.TaskID, SessionID: o.SessionID,
	}
}

// deliverQuery is the one point where an executed query's agent-bound result is final: the review runs first
// when it is on, then the values the script staged are kept. Every way out but this point drops them.
func (s *Server) deliverQuery(ctx context.Context, args runQueryArgs, rv queryResolution, rendered maskset, res *mcp.CallToolResult, out any, err error, o *queryScriptOutcome) (*mcp.CallToolResult, any, error) {
	// Read after the run: the user may have changed the setting while the query ran.
	if rv.reviewRequested || s.reviewFor(rv.summary, rv.mode, rv.class) {
		if o == nil {
			return s.reviewResponse(ctx, args, rv, rendered, res, out, err)
		}
		if refusal := s.holdResponse(ctx, args, rv, rendered, o.heldOf(rv, res, err)); refusal != "" {
			return errResult(refusal)
		}
		if !s.scriptsOn() {
			return errResult(ranSuffix(fmt.Sprintf("query against %q ran, but scripts were turned off while its result was in review, so it was withheld.", rv.summary.Name), rv))
		}
	}
	if o != nil && o.Failure == "" && ctx.Err() == nil {
		if saved, failed := rv.script.kept.commit(s.keptSourceOfQuery(ctx, args, rv)); len(saved)+len(failed) > 0 {
			return o.agentResult(saved, failed)
		}
	}
	return res, out, err
}

// scriptHashOf is the hash an exemption binds; "" without a script.
func scriptHashOf(source string) string {
	if source == "" {
		return ""
	}
	return jsengine.SourceHash(source)
}
