package dbmcpflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-held-writes-db.spec.ts (contract e2e-held-writes-db): values a result
// script stages with kept.set reach the store only when the user sends the held result. Every other exit
// drops them.

const (
	heldKey     = "e2e-held-writes-db"
	dbStageW      = `kept.set("w", [1,2]); return 1;`
	waitingText = "got no review within"
)

func TestE2EHeldWritesDB(t *testing.T) {
	f := newFixture(t, flowharness.WithApprovalTimeout(2*time.Second))
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})
	opts := scriptChainOpts(f)

	hold := func(t *testing.T, script string) (pendingReq, chan callOutcome) {
		t.Helper()
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", script))
		req := f.pending(t)
		if req.Stage != "response" {
			t.Fatalf("pending = %+v, want a held result", req)
		}
		return req, pending
	}
	noW := func(t *testing.T) {
		t.Helper()
		if _, ok := f.keptValue(t, "w"); ok {
			t.Fatal("w stored, want nothing kept")
		}
	}
	reset := func(t *testing.T) {
		t.Helper()
		f.app.W.Kept.ClearAll()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	}

	t.Run("send", func(t *testing.T) {
		req, pending := hold(t, dbStageW)
		held := f.pendingResponse(t, req.RequestID)
		if len(held.Keep) != 1 || held.Keep[0].Name != "w" || held.Keep[0].Preview != "[1,2]" {
			t.Fatalf("keep = %+v, want w with its preview", held.Keep)
		}
		noW(t)
		f.app.Contract(t, heldKey, "ApprovalsService.List#db", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, heldKey, "DbMcpService.PendingResponse#db", held, opts...)
		f.app.Contract(t, heldKey, "args:ApprovalsService.Answer#send", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if res.IsError || !strings.Contains(text(res), `"kept":[{"name":"w","type":"array","items":2,`) {
			t.Fatalf("sent = %q, want the envelope and the kept summary", text(res))
		}
		e, ok := f.keptValue(t, "w")
		if !ok || string(e.Value) != "[1,2]" || e.Source.MCP != "db" || e.Source.Kind != keptvars.SourceScript {
			t.Fatalf("stored = %+v ok=%v, want w from the db script", e, ok)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, heldKey, "KeptVarsService.List#afterSend", list, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(e.ID, "kv_w"))
		f.app.Contract(t, heldKey, "result#send", text(res), flowharness.Mask("expiresInSeconds"))
		reset(t)
	})

	t.Run("withhold", func(t *testing.T) {
		req, pending := hold(t, dbStageW)
		f.app.Contract(t, heldKey, "args:ApprovalsService.Answer#withhold", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: false})
		f.answer(t, false, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || text(res) != withheldR {
			t.Fatalf("withheld = %q, want %q", text(res), withheldR)
		}
		noW(t)
	})

	t.Run("review timeout", func(t *testing.T) {
		_, pending := hold(t, dbStageW)
		res := wait(t, pending)
		if !res.IsError || !strings.Contains(text(res), waitingText) || !strings.Contains(text(res), "withheld") || strings.Contains(text(res), "one") {
			t.Fatalf("unreviewed = %q, want a timed-out withhold with no data", text(res))
		}
		noW(t)
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	})

	t.Run("scripts turned off during review", func(t *testing.T) {
		req, pending := hold(t, dbStageW)
		off := false
		if _, err := f.app.W.Settings.Set(settingsPatchScripts(&off)); err != nil {
			t.Fatal(err)
		}
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || !strings.Contains(text(res), "scripts were turned off while its result was in review, so it was withheld.") {
			t.Fatalf("after scripts off = %q, want the withheld text", text(res))
		}
		noW(t)
		on := true
		if _, err := f.app.W.Settings.Set(settingsPatchScripts(&on)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("client closes the session during review", func(t *testing.T) {
		abandon, cancel := context.WithCancel(ctx)
		go func() {
			_, _ = sess.CallTool(abandon, &mcp.CallToolParams{Name: "run_query", Arguments: f.queryScriptArgs(scriptStmt, "read", dbStageW)})
		}()
		req := f.pending(t)
		if req.Stage != "response" {
			t.Fatalf("pending = %+v, want a held result", req)
		}
		cancel()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
		noW(t)
	})

	t.Run("transform fails after kept.set", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"review.read": "send"})
		res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `kept.set("w", [1,2]); throw new Error("boom")`))
		if !res.IsError || !strings.Contains(text(res), "boom") {
			t.Fatalf("failing script = %q, want its failure", text(res))
		}
		noW(t)
		f.mcpSet(t, map[string]string{"review.read": "review"})
	})

	t.Run("the name is a user entry", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"review.read": "send"})
		user, err := f.app.W.Kept.Put(keptvars.Put{
			Scope: externalScope, Name: "w", Value: json.RawMessage(`"typed"`), Secret: true,
			Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "input w", Client: "claude-code"},
		})
		if err != nil {
			t.Fatal(err)
		}
		res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", dbStageW))
		if res.IsError || !strings.Contains(text(res), `"keptNotSaved":[{"name":"w","error":"`+keptvars.ErrNameTaken.Error()+`"}]`) || strings.Contains(text(res), `"kept":[`) {
			t.Fatalf("result = %q, want the output and keptNotSaved naming the user entry", text(res))
		}
		if e, ok := f.keptValue(t, "w"); !ok || e.ID != user.ID || string(e.Value) != `"typed"` {
			t.Fatalf("w = %+v ok=%v, want the user entry untouched", e, ok)
		}
		f.app.Contract(t, heldKey, "result#userEntry", text(res))
		reset(t)
	})

	t.Run("the scope holds 64 entries", func(t *testing.T) {
		for i := range keptvars.MaxScopeEntries {
			f.keep(t, externalScope, fmt.Sprintf("n%d", i), "1")
		}
		res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", dbStageW))
		if res.IsError || !strings.Contains(text(res), `"keptNotSaved":[{"name":"w","error":"`+keptvars.ErrScopeFull.Error()+`"}]`) {
			t.Fatalf("result = %q, want keptNotSaved naming the scope cap", text(res))
		}
		if n := len(f.app.W.Kept.List(externalScope)); n != keptvars.MaxScopeEntries {
			t.Fatalf("%d entries, want 64 and nothing evicted", n)
		}
		noW(t)
		f.app.Contract(t, heldKey, "result#scopeFull", text(res))
		reset(t)
	})

	t.Run("a value over 1 MiB", func(t *testing.T) {
		res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `kept.set("w", "x".repeat(2 << 20)); return 1;`))
		if !res.IsError {
			t.Fatalf("result = %q, want the script to fail", text(res))
		}
		noW(t)
		f.app.Contract(t, heldKey, "result#tooLarge", text(res))
	})

	if n := f.queueLen(t); n != 0 {
		t.Fatalf("queue %d, want none", n)
	}
}

func settingsPatchScripts(on *bool) bridge.SettingsSetArgs {
	return bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIScripts: on}}}
}
