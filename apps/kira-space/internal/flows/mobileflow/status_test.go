package mobileflow_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P705 phone status. Contract mobile-status: tests/mobile/status.spec.ts replays the same shapes.
const statusScenario = "mobile-status"

var statusMask = flowharness.Mask("id", "taskId", "branchId", "createdAt", "startedAt", "finishedAt")

func TestPhoneStatus(t *testing.T) {
	f := newFixture(t)
	app := f.app
	p, _ := pairedPhone(t, f)
	stream := p.events()

	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "job", Color: "blue", Command: "echo secret-output; exit 3",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "status-run", WindowKey: desktopWindow, Cols: 80, Rows: 24,
		LaunchKind: terminal.LaunchKindScript, ScriptID: rec.ID,
	}); err != nil {
		t.Fatal(err)
	}

	var pushed any
	for {
		fr := stream.wait(t, bridge.ChannelScriptRunsChanged, waitFor)
		var run map[string]any
		if err := json.Unmarshal([]byte(fr.Data), &run); err != nil {
			t.Fatalf("frame %q: %v", fr.Data, err)
		}
		for _, key := range []string{"cwd", "command", "prompt", "terminalId", "outputTail"} {
			if hasKey(run, key) {
				t.Fatalf("pushed run leaks %q: %s", key, fr.Data)
			}
		}
		if run["state"] == "failed" {
			pushed = run
			break
		}
	}
	app.Contract(t, statusScenario, "sse:kira:scriptRuns:changed#failed", pushed, statusMask)

	var runs any
	testx.WaitUntil(t, waitFor, func() bool {
		r := p.get("/api/runs")
		if r.Status != http.StatusOK {
			return false
		}
		runs = r.raw(t)
		return hasKey(runs, "failed") || hasKey(runs, "reason")
	})
	for _, key := range []string{"cwd", "command", "prompt", "terminalId", "outputTail"} {
		if hasKey(runs, key) {
			t.Fatalf("/api/runs leaks %q: %v", key, runs)
		}
	}
	app.Contract(t, statusScenario, "http:GET /api/runs#failed", runs, statusMask)

	r := p.get("/api/computer")
	if r.Status != http.StatusOK {
		t.Fatalf("/api/computer: %d", r.Status)
	}
	computer := r.raw(t)
	for _, key := range []string{"cwd", "command", "host", "socket", "endpoint"} {
		if hasKey(computer, key) {
			t.Fatalf("/api/computer leaks %q: %v", key, computer)
		}
	}
	app.Contract(t, statusScenario, "http:GET /api/computer#docker", computer, flowharness.Mask("total", "running", "stopped", "state", "reason"))
}
