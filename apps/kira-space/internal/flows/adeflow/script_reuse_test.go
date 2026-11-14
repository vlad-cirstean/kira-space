package adeflow_test

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type bearerAuth struct{ token string }

func (b bearerAuth) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	r.Header.Set("User-Agent", "claude-code/2.1.318 (cli)")
	return http.DefaultTransport.RoundTrip(r)
}

var mcpURLRe = regexp.MustCompile(`"url":"([^"]+)"`)

// J-X-005 and J-RUN-007: one Runs script is a prepare hook, an ADE-bound run, a schedule and an MCP
// tool. Each invocation records once with its own params; two runs on two branches at once keep
// separate state files.
func TestOneScriptEveryEntryPoint(t *testing.T) {
	clock := fakeclock.New(scheduleStart)
	f := newAutoFixture(t, "", false, flowharness.WithClock(clock))

	rec := t.TempDir()
	gate := filepath.Join(rec, "gate")
	journal := filepath.Join(rec, "journal")
	sched := targeted(f)
	sched.Params = map[string][]string{"who": {"cron"}}
	script := f.script(t, scripts.CustomScriptFields{
		Name: "record",
		Command: `printf '%s|%s\n' "$KIRA_PARAM_WHO" "${KIRA_BRANCH:-none}" >> ` + journal + "\n" +
			`pwd > ` + rec + `/cwd-$KIRA_PARAM_WHO` + "\n" +
			`while [ "$KIRA_PARAM_WAIT" = yes ] && [ ! -f ` + gate + ` ]; do sleep 0.1; done`,
		UseAdeDir: true,
		Params: []scripts.Param{
			{Name: "who", Type: scripts.ParamText, Required: true},
			{Name: "wait", Type: scripts.ParamText},
		},
		MCP:      &scripts.MCPAccess{Enabled: true, Run: scripts.MCPRunAllow, Description: "Records its caller."},
		Schedule: sched,
	})
	lines := func() []string {
		raw, err := os.ReadFile(journal)
		if err != nil {
			return nil
		}
		out := strings.Fields(string(raw))
		sort.Strings(out)
		return out
	}

	// 1. Prepare hook: a second task creates the docs branch, whose setup runs the script first.
	setPrepareRun(t, f.app, f.docs, runRef("record", map[string]model.ParamValues{"who": {"hook"}}))
	docsTask := createTask(t, f.app, "Docs", "flow", f.docs)
	docsBr := branchOf(t, board(t, f.app), docsTask.ID, f.docs)
	if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: docsTask.ID, BranchNames: map[string]string{docsBr.ID: "feat/docs-login"}}); err != nil {
		t.Fatalf("StartRun docs: %v", err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		s := branchOf(t, board(t, f.app), docsTask.ID, f.docs).Setup
		return s != nil && s.State == "ready"
	})
	if got := lines(); len(got) != 1 || got[0] != "hook|none" {
		t.Fatalf("after the prepare hook journal = %v, want one hook line", got)
	}

	// 2. ADE-bound runs on two branches at once: one script, two live runs, separate folders.
	startBound := func(who string, br adewire.Branch, win, term string) {
		t.Helper()
		args := scriptruns.RunArgs{
			ScriptID: script.ID, TaskID: f.task, BranchID: br.ID,
			Params: map[string][]string{"who": {who}, "wait": {"yes"}},
		}
		started := f.start(t, args)
		if started.Terminal == nil {
			t.Fatalf("Start %s = %+v, want a terminal launch", who, started)
		}
		if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
			TerminalID: term, WindowKey: win, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript,
			ScriptID: script.ID, ScriptLaunchToken: started.Terminal.Token,
		}); err != nil {
			t.Fatal(err)
		}
	}
	startBound("ade-api", f.apiBr, "w-api", "t-api")
	startBound("ade-web", f.webBr, "w-web", "t-web")
	testx.WaitUntil(t, waitFor, func() bool { return len(lines()) == 3 })
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		runs, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 50})
		if err != nil {
			return false
		}
		done := 0
		for _, r := range runs {
			if r.ScriptID == script.ID && r.Trigger == scriptruns.TriggerADE && r.State == "done" {
				done++
			}
		}
		return done == 2
	})
	cwd := func(who string) string {
		raw, err := os.ReadFile(filepath.Join(rec, "cwd-"+who))
		if err != nil {
			t.Fatalf("cwd-%s: %v", who, err)
		}
		return strings.TrimSpace(string(raw))
	}
	if cwd("ade-api") != realPath(t, f.apiBr.Worktree) || cwd("ade-web") != realPath(t, f.webBr.Worktree) || cwd("ade-api") == cwd("ade-web") {
		t.Fatalf("cwd api=%q web=%q, want each branch's own worktree", cwd("ade-api"), cwd("ade-web"))
	}

	// 3. Schedule: params ride in the schedule.
	tick(t, clock, 30*time.Second)
	scheduledRun(t, f, script.ID, 0, "done")

	// 4. MCP tool: the real server, a real client.
	st, err := f.app.W.DbMcp.SetEnabled(bridge.DbMcpSetEnabledArgs{Enabled: true})
	if err != nil || !st.Running {
		t.Fatalf("SetEnabled = %+v, %v", st, err)
	}
	m := mcpURLRe.FindStringSubmatch(st.Command)
	if m == nil {
		t.Fatalf("no server url in %q", st.Command)
	}
	token, ok, err := mcpauth.LoadHelperToken(mcpauth.HelperTokenPathNamed(f.app.SpaceHome, "mcp-db"))
	if err != nil || !ok {
		t.Fatalf("helper token: ok=%v err=%v", ok, err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-client", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: m[1], HTTPClient: &http.Client{Transport: bearerAuth{token}, Timeout: 30 * time.Second},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_script", Arguments: map[string]any{
		"scriptId": script.ID, "params": map[string][]string{"who": {"agent"}},
	}})
	if err != nil || res.IsError {
		t.Fatalf("run_script = %+v, %v", res, err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return len(lines()) == 5 })

	want := []string{"ade-api|feat/api-login", "ade-web|feat/web-login", "agent|none", "cron|feat/api-login", "hook|none"}
	if got := lines(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("journal = %v, want %v (each invocation once)", got, want)
	}
	runs, err := f.app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	triggers := map[scriptruns.Trigger]int{}
	for _, r := range runs {
		if r.ScriptID == script.ID {
			triggers[r.Trigger]++
		}
	}
	if triggers[scriptruns.TriggerADE] != 2 || triggers[scriptruns.TriggerScheduled] != 1 || triggers[scriptruns.TriggerMCP] != 1 {
		t.Fatalf("run triggers = %v, want ade 2, scheduled 1, mcp 1", triggers)
	}
}
