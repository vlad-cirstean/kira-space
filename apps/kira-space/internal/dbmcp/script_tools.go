package dbmcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ScriptControl is dbmcp's consumer-declared view of the script-run service for the script tools.
type ScriptControl interface {
	// Exposed lists the MCP-enabled normal scripts in panel order.
	Exposed() ([]ExposedScript, error)
	// Plan resolves the run without starting it; an error is the caller's fix to make.
	Plan(scriptID string, params map[string][]string) (ScriptPlan, error)
	// Start starts the planned run; a script changed since Plan carries a different hash and is refused.
	Start(scriptID string, params map[string][]string, hash string) (runID string, err error)
	// Run reads one run; false when there is none.
	Run(runID string) (ScriptRun, bool, error)
	// Wait returns when the run ends or ctx is done, then the current row.
	Wait(ctx context.Context, runID string) (ScriptRun, error)
	// Log reads a run's stored output.
	Log(runID string, q ScriptLogQuery) (ScriptLog, error)
}

// ExposedScript is a script an agent may run.
type ExposedScript struct {
	ID, Name, Description string
	// Run is the approval mode: "prompt" or "allow".
	Run    string
	Params []ScriptParamInfo
}

// ScriptParamInfo describes one parameter of an exposed script. A secret never shows a value.
type ScriptParamInfo struct {
	Name, Type string
	Options    []string
	Default    []string
	Required   bool
	Secret     bool
	// Saved says a secret has a saved value.
	Saved bool
}

// ScriptPlan is what a run would do. A secret value is always empty.
type ScriptPlan struct {
	Hash, Command, Cwd, Blocker string
	Missing                     []string
	Params, Env                 []ApprovalValue
}

// ScriptRun is one run as the tools show it.
type ScriptRun struct {
	ID, ScriptID, ScriptName, State, Trigger, Reason string
	ExitCode                                         *int
	StartedAt, FinishedAt                            *int64
	// Headless is false for a run in a terminal tab, whose output is not stored.
	Headless bool
}

// ScriptLogQuery picks stored lines: the newest Limit (FromEnd) or the oldest, with Contains as a filter.
type ScriptLogQuery struct {
	FromEnd  bool
	Limit    int
	Contains string
}

// ScriptLogLine is one stored line.
type ScriptLogLine struct {
	Seq    int    `json:"seq"`
	Stream string `json:"stream"`
	Text   string `json:"text"`
}

// ScriptLog is the picked lines, oldest first, with how many matched before the cap.
type ScriptLog struct {
	Lines     []ScriptLogLine
	Matched   int
	Truncated bool
}

const (
	scriptWaitDefault  = 25
	scriptWaitMax      = 50
	scriptLogsDefault  = 100
	scriptLogsMax      = 500
	scriptLogsMaxBytes = 64 << 10
	scriptNext         = "Call get_script_logs for output."
)

type runScriptArgs struct {
	ScriptID    string         `json:"scriptId"`
	Params      map[string]any `json:"params,omitempty"`
	WaitSeconds *int           `json:"waitSeconds,omitempty"`
}

type getScriptRunArgs struct {
	RunID       string `json:"runId"`
	WaitSeconds *int   `json:"waitSeconds,omitempty"`
}

type getScriptLogsArgs struct {
	RunID    string `json:"runId"`
	From     string `json:"from,omitempty"`
	Lines    *int   `json:"lines,omitempty"`
	Contains string `json:"contains,omitempty"`
}

type scriptParamView struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Default  []string `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Secret   bool     `json:"secret,omitempty"`
	Saved    bool     `json:"saved,omitempty"`
}

type scriptView struct {
	ScriptID    string            `json:"scriptId"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Approval    string            `json:"approval"`
	Params      []scriptParamView `json:"params"`
}

// scriptRunView never carries output: logs only come from get_script_logs.
type scriptRunView struct {
	RunID      string `json:"runId"`
	OpID       string `json:"opId"`
	State      string `json:"state"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DurationMs int64  `json:"durationMs"`
	LogLines   int    `json:"logLines"`
	Next       string `json:"next"`
}

func noScriptResult(id string) (*mcp.CallToolResult, any, error) {
	return errResult(fmt.Sprintf("no script %q exposed to MCP", id))
}

func noRunResult(id string) (*mcp.CallToolResult, any, error) {
	return errResult(fmt.Sprintf("no run %q of a script exposed to MCP", id))
}

// exposedScript finds an exposed script by id; a hidden, smart or unknown one is not found alike.
func (s *Server) exposedScript(id string) (ExposedScript, bool, error) {
	list, err := s.cfg.Scripts.Exposed()
	if err != nil {
		return ExposedScript{}, false, err
	}
	for _, e := range list {
		if e.ID == id {
			return e, true, nil
		}
	}
	return ExposedScript{}, false, nil
}

// exposedRun reads a run of a script that is exposed now; any other run reads as unknown.
func (s *Server) exposedRun(runID string) (ScriptRun, bool, error) {
	run, ok, err := s.cfg.Scripts.Run(runID)
	if err != nil || !ok {
		return ScriptRun{}, false, err
	}
	_, exposed, err := s.exposedScript(run.ScriptID)
	if err != nil || !exposed {
		return ScriptRun{}, false, err
	}
	return run, true, nil
}

func (s *Server) listScripts(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	list, err := s.cfg.Scripts.Exposed()
	if err != nil {
		return toolError(err)
	}
	out := make([]scriptView, 0, len(list))
	for _, e := range list {
		v := scriptView{ScriptID: e.ID, Name: e.Name, Description: e.Description, Approval: e.Run, Params: make([]scriptParamView, 0, len(e.Params))}
		for _, p := range e.Params {
			v.Params = append(v.Params, scriptParamView(p))
		}
		out = append(out, v)
	}
	return jsonResult(map[string]any{"scripts": out})
}

// parseScriptParams reads the params argument: each value is a string or a list of strings.
func parseScriptParams(in map[string]any) (map[string][]string, string) {
	out := make(map[string][]string, len(in))
	for name, v := range in {
		switch t := v.(type) {
		case string:
			out[name] = []string{t}
		case []any:
			vals := make([]string, 0, len(t))
			for _, x := range t {
				str, ok := x.(string)
				if !ok {
					return nil, fmt.Sprintf("param %q must be a string or a list of strings", name)
				}
				vals = append(vals, str)
			}
			out[name] = vals
		default:
			return nil, fmt.Sprintf("param %q must be a string or a list of strings", name)
		}
	}
	return out, ""
}

// waitFor bounds a wait argument to 0..scriptWaitMax seconds; nil takes the default.
func waitFor(seconds *int) time.Duration {
	n := scriptWaitDefault
	if seconds != nil {
		n = min(max(*seconds, 0), scriptWaitMax)
	}
	return time.Duration(n) * time.Second
}

// planProblem says why a planned run cannot start, "" when it can.
func planProblem(name string, p ScriptPlan) string {
	switch {
	case p.Blocker != "":
		return fmt.Sprintf("script %q cannot run: %s", name, p.Blocker)
	case len(p.Missing) > 0:
		return fmt.Sprintf("script %q needs values for: %s", name, strings.Join(p.Missing, ", "))
	}
	return ""
}

// approveScript asks the user to approve the run and re-checks exposure once they answered: the
// wait can last two minutes, so a hidden script wins. A non-nil result stops the call.
func (s *Server) approveScript(ctx context.Context, es ExposedScript, plan ScriptPlan) (*mcp.CallToolResult, error) {
	outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
		Subject: approvals.KindScriptRun, ConnectionID: es.ID, ConnectionName: es.Name, Kind: "script", Class: adapters.ClassUnknown,
		Statement: plan.Command, Reason: ApprovalReasonPermission, Origin: originFrom(ctx),
		Script: &ApprovalScript{
			ScriptID: es.ID, Name: es.Name, Description: es.Description, Command: plan.Command, Cwd: plan.Cwd,
			Params: plan.Params, Env: plan.Env,
		},
	})
	if outcome != ApprovalApproved {
		res, _, err := approvalOutcomeError(outcome, fmt.Sprintf("run of script %q", es.Name))
		return res, err
	}
	_, still, err := s.exposedScript(es.ID)
	if err != nil {
		return nil, err
	}
	if !still {
		res, _, err := errResult(fmt.Sprintf("script %q is no longer exposed to MCP", es.Name))
		return res, err
	}
	return nil, nil
}

func (s *Server) runScript(ctx context.Context, req *mcp.CallToolRequest, a runScriptArgs) (*mcp.CallToolResult, any, error) {
	es, found, err := s.exposedScript(a.ScriptID)
	if err != nil {
		return toolError(err)
	}
	if !found {
		return noScriptResult(a.ScriptID)
	}
	params, bad := parseScriptParams(a.Params)
	if bad != "" {
		return errResult(bad)
	}
	plan, err := s.cfg.Scripts.Plan(es.ID, params)
	if err != nil {
		return toolError(err)
	}
	if problem := planProblem(es.Name, plan); problem != "" {
		return errResult(problem)
	}
	if es.Run != "allow" {
		if res, err := s.approveScript(ctx, es, plan); res != nil || err != nil {
			return res, nil, err
		}
	}
	runID, err := s.cfg.Scripts.Start(es.ID, params, plan.Hash)
	if err != nil {
		return toolError(err)
	}
	return s.waitedRun(ctx, runID, waitFor(a.WaitSeconds))
}

// waitedRun waits up to d for the run to end and answers its state.
func (s *Server) waitedRun(ctx context.Context, runID string, d time.Duration) (*mcp.CallToolResult, any, error) {
	wctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	run, err := s.cfg.Scripts.Wait(wctx, runID)
	if err != nil {
		return toolError(err)
	}
	return s.scriptRunResult(run)
}

func (s *Server) scriptRunResult(run ScriptRun) (*mcp.CallToolResult, any, error) {
	v := scriptRunView{RunID: run.ID, OpID: scriptRunPrefix + run.ID, State: run.State, ExitCode: run.ExitCode, Reason: run.Reason, Next: scriptNext}
	if run.StartedAt != nil {
		end := time.Now().UnixMilli()
		if run.FinishedAt != nil {
			end = *run.FinishedAt
		}
		v.DurationMs = max(end-*run.StartedAt, 0)
	}
	if run.Headless {
		log, err := s.cfg.Scripts.Log(run.ID, ScriptLogQuery{Limit: 1})
		if err != nil {
			return toolError(err)
		}
		v.LogLines = log.Matched
	}
	return jsonResult(v)
}

func (s *Server) getScriptRun(ctx context.Context, _ *mcp.CallToolRequest, a getScriptRunArgs) (*mcp.CallToolResult, any, error) {
	run, found, err := s.exposedRun(a.RunID)
	if err != nil {
		return toolError(err)
	}
	if !found {
		return noRunResult(a.RunID)
	}
	return s.waitedRun(ctx, run.ID, waitFor(a.WaitSeconds))
}

// capLogBytes drops lines from the far end until the text fits scriptLogsMaxBytes: the oldest for a
// tail, the newest for a head. It reports whether any line was dropped.
func capLogBytes(lines []ScriptLogLine, fromEnd bool) ([]ScriptLogLine, bool) {
	total := 0
	for _, l := range lines {
		total += len(l.Text)
	}
	dropped := false
	for total > scriptLogsMaxBytes && len(lines) > 1 {
		dropped = true
		if fromEnd {
			total -= len(lines[0].Text)
			lines = lines[1:]
		} else {
			total -= len(lines[len(lines)-1].Text)
			lines = lines[:len(lines)-1]
		}
	}
	return lines, dropped
}

func (s *Server) getScriptLogs(_ context.Context, _ *mcp.CallToolRequest, a getScriptLogsArgs) (*mcp.CallToolResult, any, error) {
	run, found, err := s.exposedRun(a.RunID)
	if err != nil {
		return toolError(err)
	}
	if !found {
		return noRunResult(a.RunID)
	}
	if !run.Headless {
		return jsonResult(map[string]any{"runId": run.ID, "state": run.State, "lines": []ScriptLogLine{}, "note": "This run ran in a terminal tab; its output is not stored."})
	}
	n := scriptLogsDefault
	if a.Lines != nil {
		n = min(max(*a.Lines, 1), scriptLogsMax)
	}
	q := ScriptLogQuery{FromEnd: a.From != "head", Limit: n, Contains: a.Contains}
	log, err := s.cfg.Scripts.Log(run.ID, q)
	if err != nil {
		return toolError(err)
	}
	lines, cut := capLogBytes(log.Lines, q.FromEnd)
	if lines == nil {
		lines = []ScriptLogLine{}
	}
	return jsonResult(map[string]any{
		"runId": run.ID, "state": run.State, "lines": lines, "matched": log.Matched, "returned": len(lines),
		"truncated": log.Truncated, "truncatedBytes": cut,
	})
}

// boundWait sets the 0..scriptWaitMax bounds of a waitSeconds property.
func boundWait(sch *jsonschema.Schema) *jsonschema.Schema {
	p := sch.Properties["waitSeconds"]
	p.Minimum, p.Maximum = jsonschema.Ptr(0.0), jsonschema.Ptr(float64(scriptWaitMax))
	return sch
}

// addScriptTools registers list_scripts, run_script, get_script_run and get_script_logs.
func (s *Server) addScriptTools(srv *mcp.Server, src llmprompts.Source) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_scripts", Description: llmprompts.DbmcpToolListScripts.Text(src, llmprompts.NoVars{}),
	}, withPanicRecovery(s, "list_scripts", s.listScripts))
	wait := map[string]any{"waitSeconds": llmprompts.LimitVars{Default: scriptWaitDefault, Max: scriptWaitMax}}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_script",
		Description: llmprompts.DbmcpToolRunScript.Text(src, llmprompts.NoVars{}),
		InputSchema: boundWait(llmprompts.RequireNonEmpty(llmprompts.InputSchema[runScriptArgs](src, llmprompts.DbmcpToolRunScript.ID(), wait), "scriptId")),
	}, withPanicRecovery(s, "run_script", s.runScript))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_script_run",
		Description: llmprompts.DbmcpToolGetScriptRun.Text(src, llmprompts.NoVars{}),
		InputSchema: boundWait(llmprompts.RequireNonEmpty(llmprompts.InputSchema[getScriptRunArgs](src, llmprompts.DbmcpToolGetScriptRun.ID(), wait), "runId")),
	}, withPanicRecovery(s, "get_script_run", s.getScriptRun))
	logs := llmprompts.RequireNonEmpty(llmprompts.InputSchema[getScriptLogsArgs](src, llmprompts.DbmcpToolGetScriptLogs.ID(),
		map[string]any{"lines": llmprompts.LimitVars{Default: scriptLogsDefault, Max: scriptLogsMax}}), "runId")
	logs.Properties["from"].Enum = []any{"tail", "head"}
	logs.Properties["lines"].Minimum, logs.Properties["lines"].Maximum = jsonschema.Ptr(1.0), jsonschema.Ptr(float64(scriptLogsMax))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_script_logs",
		Description: llmprompts.DbmcpToolGetScriptLogs.Text(src, llmprompts.NoVars{}),
		InputSchema: logs,
	}, withPanicRecovery(s, "get_script_logs", s.getScriptLogs))
}
