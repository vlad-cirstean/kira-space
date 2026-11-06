package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type scriptRunResult struct {
	RunID      string `json:"runId"`
	OpID       string `json:"opId"`
	State      string `json:"state"`
	ExitCode   *int   `json:"exitCode"`
	DurationMs int64  `json:"durationMs"`
	LogLines   int    `json:"logLines"`
}

type scriptLogsResult struct {
	Lines []struct {
		Seq    int    `json:"seq"`
		Stream string `json:"stream"`
		Text   string `json:"text"`
	} `json:"lines"`
	Matched        int  `json:"matched"`
	Returned       int  `json:"returned"`
	TruncatedBytes bool `json:"truncatedBytes"`
}

func (f *fixture) createScript(t *testing.T, fields scripts.CustomScriptFields, secrets map[string]*string) scripts.CustomScript {
	t.Helper()
	rec, err := f.app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: fields, SecretValues: secrets})
	if err != nil {
		t.Fatalf("create script %q: %v", fields.Name, err)
	}
	return rec
}

func allowMCP(description string) *scripts.MCPAccess {
	return &scripts.MCPAccess{Enabled: true, Run: scripts.MCPRunAllow, Description: description}
}

func callTool(t *testing.T, sess *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func callToolAsync(sess *mcp.ClientSession, name string, args map[string]any) chan callOutcome {
	ch := make(chan callOutcome, 1)
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		ch <- callOutcome{res, err}
	}()
	return ch
}

func decodeResult[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	var out T
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("result JSON %q: %v", text(res), err)
	}
	return out
}

func lineTexts(r scriptLogsResult) []string {
	out := make([]string, 0, len(r.Lines))
	for _, l := range r.Lines {
		out = append(out, l.Text)
	}
	return out
}

func equalStrings(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00") && len(a) == len(b)
}

// TestDbMcpScriptRunAndLogs: an agent runs MCP-enabled scripts and reads their stored output; a hidden
// or smart script reads as unknown; results carry no output; operations tools see only exposed runs.
// UI half: tests/ui/automations-mcp.spec.ts (Agent trigger label).
func TestDbMcpScriptRunAndLogs(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	hidden := f.createScript(t, scripts.CustomScriptFields{Name: "hidden", Command: "sleep 60"}, nil)
	f.createScript(t, scripts.CustomScriptFields{
		Name: "fails", Command: "printf 'one\\ntwo\\nERR three\\n'; exit 3", MCP: allowMCP("Prints three lines and fails."),
	}, nil)
	f.createScript(t, scripts.CustomScriptFields{Name: "slow", Command: "sleep 30", MCP: allowMCP("")}, nil)
	f.createScript(t, scripts.CustomScriptFields{Name: "smart", Kind: scripts.KindSmart, Command: "do it", Smart: &scripts.Smart{}}, nil)

	listed := callTool(t, sess, "list_scripts", nil)
	var list struct {
		Scripts []struct {
			ScriptID    string `json:"scriptId"`
			Description string `json:"description"`
			Approval    string `json:"approval"`
		} `json:"scripts"`
	}
	list = decodeResult[struct {
		Scripts []struct {
			ScriptID    string `json:"scriptId"`
			Description string `json:"description"`
			Approval    string `json:"approval"`
		} `json:"scripts"`
	}](t, listed)
	if len(list.Scripts) != 2 || list.Scripts[0].ScriptID != "fails" || list.Scripts[1].ScriptID != "slow" ||
		list.Scripts[0].Description != "Prints three lines and fails." || list.Scripts[0].Approval != "allow" {
		t.Fatalf("list_scripts = %s, want fails and slow only", text(listed))
	}

	unknown := callTool(t, sess, "run_script", map[string]any{"scriptId": "hidden"})
	missing := callTool(t, sess, "run_script", map[string]any{"scriptId": "nope"})
	if !unknown.IsError || !strings.Contains(text(unknown), `no script "hidden" exposed`) ||
		!strings.Contains(text(missing), `no script "nope" exposed`) {
		t.Fatalf("hidden = %q, unknown = %q, want the same refusal", text(unknown), text(missing))
	}
	if res := callTool(t, sess, "run_script", map[string]any{"scriptId": "smart"}); !res.IsError {
		t.Fatalf("run_script smart = %q, want a refusal", text(res))
	}

	res := callTool(t, sess, "run_script", map[string]any{"scriptId": "fails", "waitSeconds": 10})
	run := decodeResult[scriptRunResult](t, res)
	if run.State != "failed" || run.ExitCode == nil || *run.ExitCode != 3 || run.LogLines != 3 || run.OpID != "run:"+run.RunID {
		t.Fatalf("run_script fails = %s", text(res))
	}
	for _, leak := range []string{"ERR", "lastError", "outputTail", "two"} {
		if strings.Contains(text(res), leak) {
			t.Fatalf("run result leaks %q: %s", leak, text(res))
		}
	}
	runs, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
	if err != nil || len(runs) == 0 || runs[0].ID != run.RunID || runs[0].Trigger != scriptruns.TriggerMCP {
		t.Fatalf("runs = %+v (%v), want the agent run first with trigger mcp", runs, err)
	}

	tail := decodeResult[scriptLogsResult](t, callTool(t, sess, "get_script_logs", map[string]any{"runId": run.RunID, "lines": 2}))
	if !equalStrings(lineTexts(tail), []string{"two", "ERR three"}) || tail.Matched != 3 {
		t.Fatalf("tail 2 = %+v, want two, ERR three of 3", tail)
	}
	head := decodeResult[scriptLogsResult](t, callTool(t, sess, "get_script_logs", map[string]any{"runId": run.RunID, "from": "head", "lines": 1}))
	if !equalStrings(lineTexts(head), []string{"one"}) {
		t.Fatalf("head 1 = %+v, want one", head)
	}
	filtered := decodeResult[scriptLogsResult](t, callTool(t, sess, "get_script_logs", map[string]any{"runId": run.RunID, "contains": "ERR"}))
	if !equalStrings(lineTexts(filtered), []string{"ERR three"}) || filtered.Matched != 1 {
		t.Fatalf("contains ERR = %+v, want one matched line", filtered)
	}
	over, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "get_script_logs", Arguments: map[string]any{"runId": run.RunID, "lines": 501}})
	if err == nil && !over.IsError {
		t.Fatalf("lines 501 = %q, want the schema to refuse", text(over))
	}

	started := decodeResult[scriptRunResult](t, callTool(t, sess, "run_script", map[string]any{"scriptId": "slow", "waitSeconds": 1}))
	if started.State != "running" {
		t.Fatalf("run_script slow = %+v, want running after the wait", started)
	}
	entry := waitListed(t, sess, func(o listedOp) bool { return o.OpID == "run:"+started.RunID })
	if entry.Source != "mcp" || entry.Type != "scriptRun" || entry.Name != "slow" {
		t.Fatalf("listed run = %+v, want source mcp", entry)
	}
	if again := callTool(t, sess, "run_script", map[string]any{"scriptId": "slow", "waitSeconds": 0}); !again.IsError || !strings.Contains(text(again), "already running") {
		t.Fatalf("second run_script slow = %q, want the already-running refusal", text(again))
	}

	if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "hidden-run", WindowKey: "w1", Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: hidden.ID,
	}); err != nil {
		t.Fatal(err)
	}
	var hiddenRun string
	testx.WaitUntil(t, 20*time.Second, func() bool {
		all, _ := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
		for _, r := range all {
			if r.TerminalID == "hidden-run" && r.State == "running" {
				hiddenRun = r.ID
				return true
			}
		}
		return false
	})
	for _, o := range listOps(t, sess) {
		if o.OpID == "run:"+hiddenRun {
			t.Fatalf("list_operations leaks the run of a hidden script: %+v", o)
		}
	}
	if res := cancelOp(t, sess, "run:"+hiddenRun); !res.IsError || !strings.Contains(text(res), "no running operation") {
		t.Fatalf("cancel of a hidden script's run = %q, want the unknown-id text", text(res))
	}
	if res := callTool(t, sess, "get_script_run", map[string]any{"runId": hiddenRun}); !res.IsError {
		t.Fatalf("get_script_run of a hidden script's run = %q, want a refusal", text(res))
	}
	if err := f.app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: hiddenRun}); err != nil {
		t.Fatal(err)
	}

	if res := cancelOp(t, sess, "run:"+started.RunID); res.IsError || !strings.Contains(text(res), `"stopped":true`) {
		t.Fatalf("cancel_operation = %q", text(res))
	}
	done := decodeResult[scriptRunResult](t, callTool(t, sess, "get_script_run", map[string]any{"runId": started.RunID, "waitSeconds": 5}))
	if done.State != "cancelled" {
		t.Fatalf("get_script_run after cancel = %+v, want cancelled", done)
	}
}

// TestDbMcpScriptApproval: a prompt-mode script waits for the user, the item carries the command,
// params, masked secret and client origin, a changed script is refused, and the log never holds the
// secret value. UI half: tests/ui/dbmcp-approval-script.spec.ts.
func TestDbMcpScriptApproval(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	const secret = "s3cr3t-val-91"
	f.createScript(t, scripts.CustomScriptFields{
		Name: "ask", Command: `echo "region=$KIRA_PARAM_REGION token=$KIRA_PARAM_TOKEN"`,
		Params: []scripts.Param{
			{Name: "region", Type: "select", Options: []string{"eu", "us"}, Default: []string{"eu"}},
			{Name: "token", Type: "text", Secret: true},
		},
		MCP: &scripts.MCPAccess{Enabled: true, Run: scripts.MCPRunPrompt, Description: "Echoes the region and a token."},
	}, map[string]*string{"token": ptr(secret)})

	args := map[string]any{"scriptId": "ask", "params": map[string]any{"region": "eu"}, "waitSeconds": 10}
	if res := callTool(t, sess, "run_script", map[string]any{"scriptId": "ask", "params": map[string]any{"token": "x"}}); !res.IsError || !strings.Contains(text(res), "token") {
		t.Fatalf("run_script with a secret param = %q, want a refusal naming it", text(res))
	}
	if len(f.list(t)) != 0 {
		t.Fatal("a refused secret param raised an approval")
	}

	pending := callToolAsync(sess, "run_script", args)
	req := f.pending(t)
	it := req.Item
	if it.Kind != approvals.KindScriptRun || it.Origin.Client != "Claude Code 2.1.318" || it.Target.Name != "ask" {
		t.Fatalf("pending = %+v, want a script.run item from Claude Code 2.1.318", it)
	}
	var pl struct {
		Command string
		Env     []struct {
			Name, Value   string
			Secret, Saved bool
		}
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	var tokenOK bool
	for _, e := range pl.Env {
		if e.Name == "KIRA_PARAM_TOKEN" {
			tokenOK = e.Secret && e.Saved && e.Value == ""
		}
	}
	if !strings.Contains(pl.Command, "KIRA_PARAM_REGION") || !tokenOK {
		t.Fatalf("script payload = %s, want the command and a masked saved token", it.Payload)
	}
	// Contract dbmcp-approval-script: tests/ui/dbmcp-approval-script.spec.ts renders this item.
	f.app.Contract(t, "dbmcp-approval-script", "ApprovalsService.List", []approvals.Item{it}, flowharness.Mask("createdAt", "expiresAt", "cwd", "location"))
	f.app.Contract(t, "dbmcp-approval-script", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{it.ID}, Approve: false})

	f.answer(t, false, req.RequestID)
	out := wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), "denied by the user") {
		t.Fatalf("denied run_script = %q, want a denial", text(out))
	}

	pending = callToolAsync(sess, "run_script", args)
	req = f.pending(t)
	view, err := f.app.W.CustomScripts.File(bridge.CustomScriptsFileArgs{File: "ask.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(view.Text, "region=", "area=", 1)
	if edited == view.Text {
		t.Fatalf("file lacks the command text:\n%s", view.Text)
	}
	if _, err := f.app.W.CustomScripts.SaveFile(bridge.CustomScriptsSaveFileArgs{File: "ask.yaml", Text: edited, BaseHash: view.Hash}); err != nil {
		t.Fatal(err)
	}
	f.answer(t, true, req.RequestID)
	out = wait(t, pending)
	if !out.IsError || !strings.Contains(text(out), "changed since the preview") {
		t.Fatalf("run_script after an edit = %q, want the changed-since-preview refusal", text(out))
	}

	pending = callToolAsync(sess, "run_script", args)
	f.answer(t, true, f.pending(t).RequestID)
	res := wait(t, pending)
	run := decodeResult[scriptRunResult](t, res)
	if run.State != "done" {
		t.Fatalf("approved run = %s, want done", text(res))
	}
	logs := callTool(t, sess, "get_script_logs", map[string]any{"runId": run.RunID})
	if strings.Contains(text(logs), secret) || !strings.Contains(text(logs), "area=eu token="+scripts.SecretMask) {
		t.Fatalf("logs = %s, want the masked token and never the value", text(logs))
	}
}
