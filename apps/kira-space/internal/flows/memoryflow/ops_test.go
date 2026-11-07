package memoryflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of the memory MCP rows of the Operations panel; UI half: tests/ui/operations-memory.spec.ts.
// Contract space-ops-memory.

const (
	opsMemScenario = "space-ops-memory"
	opsPushWithin  = 3 * time.Second
)

var opsMemMask = flowharness.Mask("id", "startedAt", "durationMs", "subject")

// opsSession starts a real memory-mcp subprocess in dir, as Claude Code does, and returns it with
// its client.
func opsSession(t *testing.T, app *flowharness.App, dir string) (*mcp.ClientSession, *exec.Cmd) {
	t.Helper()
	cmd := exec.Command(app.W.Memory.McpStatus().Executable, "memory-mcp")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_CODE_SESSION_ID="+flowSession)
	sess, err := mcp.NewClient(&mcp.Implementation{Name: "flow", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess, cmd
}

func opsMemRows(t *testing.T, app *flowharness.App) []oplog.Record {
	t.Helper()
	rows, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var out []oplog.Record
	for _, r := range rows {
		if r.Module == oplog.ModuleMemory {
			out = append(out, r)
		}
	}
	return out
}

// waitMemRow waits for the memory row of kind to reach status in the app and returns it.
func waitMemRow(t *testing.T, app *flowharness.App, kind, status string) oplog.Record {
	t.Helper()
	var got oplog.Record
	testx.WaitUntil(t, opsPushWithin, func() bool {
		for _, r := range opsMemRows(t, app) {
			if r.Kind == kind && r.Status == status {
				got = r
				return true
			}
		}
		return false
	})
	return got
}

func callRaw(t *testing.T, sess *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return res
}

func holdGate(t *testing.T, app *flowharness.App) (release func()) {
	t.Helper()
	flag := filepath.Join(t.TempDir(), "release")
	app.Scenario(fakeagent.Scenario{Prompts: []fakeagent.PromptRule{
		{System: gateSystem, WaitFile: flag, Emit: testdata(t, "gate-accept.json")},
		{System: reconcileSystem, Emit: testdata(t, "reconcile.json")},
	}})
	var once bool
	release = func() {
		if once {
			return
		}
		once = true
		if err := os.WriteFile(flag, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(release)
	return release
}

var opsStoreArgs = map[string]any{"items": []map[string]string{
	{"fact": "billing runs postgres", "reason": "thread", "origin": "agent", "scope": "company"},
	{"fact": "staging deploys at 09:00", "reason": "thread", "origin": "agent", "scope": "company"},
}}

func ownerFiles(t *testing.T, app *flowharness.App) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(app.SpaceHome, "op-owners", "*.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMemoryMcpOps(t *testing.T) {
	t.Run("ok error orphaned", func(t *testing.T) {
		app := flowharness.New(t)
		release := holdGate(t, app)
		sess, cmd := opsSession(t, app, app.NewRepo("proj").Dir)

		mark := app.Events.Mark()
		res := callRaw(t, sess, "search_memories", map[string]any{"query": "see https://u:tok@host/x"})
		if res.IsError {
			t.Fatalf("search_memories failed: %+v", res.Content)
		}
		ok := waitMemRow(t, app, "search_memories", oplog.StatusOK)
		if ok.Category != oplog.CategoryMCP || ok.Trigger != oplog.TriggerMCP || ok.Background || ok.Cancellable ||
			ok.Subject == "" || !strings.Contains(ok.ID, ".") || ok.Command == nil ||
			strings.Contains(*ok.Command, "tok@") || !strings.Contains(*ok.Command, "***") {
			t.Fatalf("ok row = %+v", ok)
		}
		app.Events.WaitAfter(t, mark, bridge.ChannelOpUpdate, func(e flowharness.Event) bool {
			var r oplog.Record
			e.Decode(t, &r)
			return r.ID == ok.ID && r.Status == oplog.StatusOK
		}, opsPushWithin)
		app.Contract(t, opsMemScenario, "record#ok", ok, opsMemMask)

		bad := callRaw(t, sess, "memory_history", map[string]any{"id": "kg_missing"})
		if !bad.IsError {
			t.Fatalf("memory_history of an unknown id = %+v, want an error result", bad)
		}
		failed := waitMemRow(t, app, "memory_history", oplog.StatusError)
		if failed.Error == nil || *failed.Error == "" {
			t.Fatalf("error row = %+v, want the tool text", failed)
		}
		app.Contract(t, opsMemScenario, "record#error", failed, opsMemMask)

		go func() { _, _ = sess.CallTool(ctx, &mcp.CallToolParams{Name: "store_memory", Arguments: opsStoreArgs}) }()
		running := waitMemRow(t, app, "store_memory", oplog.StatusRunning)
		if running.Cancellable || running.DurationMs != nil {
			t.Fatalf("running row = %+v, want not cancellable and open", running)
		}
		app.Contract(t, opsMemScenario, "record#running", running, opsMemMask)
		if len(ownerFiles(t, app)) != 1 {
			t.Fatalf("owner files = %v, want one", ownerFiles(t, app))
		}

		if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		orphaned := waitMemRow(t, app, "store_memory", oplog.StatusError)
		if orphaned.ID != running.ID || orphaned.Error == nil || *orphaned.Error != oplog.ForeignStoppedMsg {
			t.Fatalf("orphaned row = %+v, want %s ended with %q", orphaned, running.ID, oplog.ForeignStoppedMsg)
		}
		testx.WaitUntil(t, opsPushWithin, func() bool { return len(ownerFiles(t, app)) == 0 })
		app.Contract(t, opsMemScenario, "record#orphaned", orphaned, opsMemMask)
		release()
	})

	t.Run("app restart keeps a live row", func(t *testing.T) {
		app := flowharness.New(t)
		release := holdGate(t, app)
		sess, _ := opsSession(t, app, app.NewRepo("proj").Dir)
		go func() { _, _ = sess.CallTool(ctx, &mcp.CallToolParams{Name: "store_memory", Arguments: opsStoreArgs}) }()
		waitMemRow(t, app, "store_memory", oplog.StatusRunning)

		app.Restart()
		rows := opsMemRows(t, app)
		if len(rows) != 1 || rows[0].Status != oplog.StatusRunning {
			t.Fatalf("rows after restart = %+v, want the one still running", rows)
		}
		release()
		waitMemRow(t, app, "store_memory", oplog.StatusOK)
	})

	t.Run("no kira.db", func(t *testing.T) {
		app := flowharness.New(t)
		home := t.TempDir()
		t.Setenv("KIRA_SPACE_HOME", home)
		sess, _ := opsSession(t, app, app.Work)
		callMCP(t, sess, "search_memories", map[string]any{"query": "anything"})
		if _, err := os.Stat(filepath.Join(home, "kira.db")); !os.IsNotExist(err) {
			t.Fatalf("kira.db stat = %v, want it absent", err)
		}
	})

	t.Run("import mode writes no foreground row", func(t *testing.T) {
		app := importApp(t)
		awaiting := createJob(t, app, importDir(t))
		if err := app.W.MemoryImport.Start(bridge.MemoryImportStartArgs{ID: awaiting.Job.ID}); err != nil {
			t.Fatal(err)
		}
		waitJob(t, app, awaiting.Job.ID, "done")
		time.Sleep(2500 * time.Millisecond) // two watcher ticks
		if rows := opsMemRows(t, app); len(rows) != 0 {
			t.Fatalf("import rows = %+v, want none", rows)
		}
	})
}
