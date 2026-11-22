package dbmcpflow

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-script-failures-api.spec.ts (contract e2e-script-failures-api): a
// prepare or transform that times out, runs out of memory or stack, throws or returns too much gives the
// agent and the op row the same text, sends the request exactly as often as the stage allows, keeps
// nothing, and frees its run slot.

const (
	scriptFailuresAPIKey = "e2e-script-failures-api"
	// ropeSource compares two 512 MiB ropes: a hard exit of the worker, not a catchable error.
	ropeSource = "let s = 'a'; for (let i = 0; i < 29; i++) s = s + s; return s === s + 'x'"
	stackSrc   = "function f() { return f() + 1 } return f()"
)

func TestE2EScriptFailuresAPI(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "never")
	u := usersURL(srv.URL)
	opts := f.e2eOpts(srv.URL, flowharness.Mask("id", "startedAt", "endedAt", "durationMs"))

	type row struct {
		key, stage, src string
		script          map[string]any
		maxBytes        int
		sent            int // requests this row adds
		contains        []string
		lacks           []string
	}
	rows := []row{
		{key: "prepareTimeout", stage: "jsPrepare", src: "while(true){}", script: map[string]any{"prepare": "while(true){}"}, contains: []string{"prepare script failed:", "timeout", "Nothing was sent."}},
		{key: "prepareMemory", stage: "jsPrepare", src: ropeSource, script: map[string]any{"prepare": ropeSource}, contains: []string{"prepare script failed:", "memory", "Nothing was sent."}, lacks: []string{"line"}},
		{key: "prepareStack", stage: "jsPrepare", src: stackSrc, script: map[string]any{"prepare": stackSrc}, contains: []string{"prepare script failed:", "stack", "Nothing was sent."}},
		{key: "transformThrow", stage: "jsTransform", src: `throw new Error("boom")`, script: map[string]any{"transform": `throw new Error("boom")`}, sent: 1, contains: []string{"transform script failed:", "script runtime error at line 1", "boom", "was sent (HTTP 200)"}},
		{key: "transformMemory", stage: "jsTransform", src: ropeSource, script: map[string]any{"transform": ropeSource}, sent: 1, contains: []string{"transform script failed:", "memory", "was sent (HTTP 200)"}, lacks: []string{"line"}},
		{key: "transformOutput", stage: "jsTransform", src: "return 'x'.repeat(600000)", script: map[string]any{"transform": "return 'x'.repeat(600000)"}, maxBytes: 512 << 10, sent: 1, contains: []string{"transform script failed:", "Return less", "was sent (HTTP 200)"}},
	}
	sent := 0
	var failed []oplog.Record
	for _, r := range rows {
		t.Run(r.key, func(t *testing.T) {
			args := scriptArgs(envID, "GET", u, r.script)
			if r.maxBytes > 0 {
				args["maxResponseBytes"] = r.maxBytes
			}
			res := callTool(t, sess, "api_request", args)
			msg := text(res)
			sent += r.sent
			if !res.IsError {
				t.Fatalf("result %q, want an error", msg)
			}
			for _, w := range r.contains {
				if !strings.Contains(msg, w) {
					t.Errorf("text %q lacks %q", msg, w)
				}
			}
			for _, w := range r.lacks {
				if strings.Contains(msg, w) {
					t.Errorf("text %q holds %q", msg, w)
				}
			}
			if got := len(srv.Requests()); got != sent {
				t.Errorf("server saw %d requests, want %d", got, sent)
			}
			op := f.scriptOpRow(t, r.stage, r.src)
			if op.Status != "error" || op.Error == nil || *op.Error != msg {
				t.Errorf("op row error = %v, want the tool text %q", op.Error, msg)
			}
			if names := f.keptNames(externalScope); len(names) != 0 {
				t.Errorf("kept = %v, want nothing", names)
			}
			if n := f.queueLen(t); n != 0 {
				t.Errorf("queue = %d, want 0", n)
			}
			failed = append(failed, op)
			f.app.Contract(t, scriptFailuresAPIKey, "failureText:"+r.key, msg, opts...)
		})
	}
	f.app.Contract(t, scriptFailuresAPIKey, "OpsService.Recent#failures", failed, opts...)

	t.Run("a failed transform under review is held and Send gives the failure text", func(t *testing.T) {
		f.setReviewApp(t, true)
		f.setAPI(t, true, "never")
		const src = `throw new Error("boom review")`
		before := len(srv.Requests())
		h := f.startHeld(t, sess, scriptArgs(envID, "GET", u, map[string]any{"transform": src}))
		if !h.held.IsError || !strings.Contains(h.held.Text, "transform script failed:") || !strings.Contains(h.held.Raw, "a@x.test") || len(h.held.Keep) != 0 {
			t.Fatalf("pending = %+v, want the failure text beside the raw response", h.held)
		}
		if p := f.responseItem(t, h.req.Item); !p.IsError || !p.Scripted {
			t.Fatalf("payload = %+v, want a scripted error item", p)
		}
		f.app.Contract(t, scriptFailuresAPIKey, "ApprovalsService.List#failed", []approvals.Item{h.req.Item}, opts...)
		held := h.held
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, scriptFailuresAPIKey, "DbMcpService.PendingResponse#failed", held, opts...)
		res := f.finish(t, h, true)
		if !res.IsError || text(res) != h.held.Text {
			t.Fatalf("result = %q, want exactly the held failure text %q", text(res), h.held.Text)
		}
		if op := f.scriptOpRow(t, "jsTransform", src); op.Error == nil || *op.Error != text(res) {
			t.Fatalf("op row error = %v, want the same text", op.Error)
		}
		if got := len(srv.Requests()); got != before+1 {
			t.Fatalf("server saw %d requests, want one", got-before)
		}
		f.setReviewApp(t, false)
	})

	t.Run("six scripted calls at once all run: no slot leaked by a crash or timeout", func(t *testing.T) {
		calls := make([]chan callOutcome, 6)
		for i := range calls {
			calls[i] = callToolAsync(sess, "api_request", scriptArgs(envID, "GET", u, map[string]any{"transform": fmt.Sprintf("return %d", i)}))
		}
		results := make([]string, len(calls))
		for i, ch := range calls {
			results[i] = text(wait(t, ch))
		}
		for i, got := range results {
			if want := fmt.Sprintf(`{"output":%d}`, i); got != want {
				t.Errorf("call %d = %q, want %q", i, got, want)
			}
		}
	})
}
