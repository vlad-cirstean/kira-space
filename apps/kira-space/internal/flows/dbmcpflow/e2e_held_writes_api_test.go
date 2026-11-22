package dbmcpflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-held-writes.spec.ts (contract e2e-held-writes-api): a transform's
// plain write is staged while its response is held, and reaches the store only on Send. Every other exit
// (withhold, timeout, switches turned off, client gone, failed transform, refused write) keeps nothing.

const (
	heldWritesAPIKey = "e2e-held-writes-api"
	stageW           = `plain.w = [1, 2]; return 1;`
	tempNotSetUser   = "a temporary variable of that name was supplied by the user and cannot be replaced"
)

func TestE2EHeldWritesAPI(t *testing.T) {
	f := newFixture(t, flowharness.WithApprovalTimeout(2*time.Second))
	srv, sess, envID := f.reviewSession(t, "")
	f.setReviewApp(t, true)
	opts := f.e2eOpts(srv.URL)
	u := usersURL(srv.URL)
	script := func(transform string) map[string]any {
		return scriptArgs(envID, "GET", u, map[string]any{"transform": transform})
	}
	nothingStored := func(t *testing.T) {
		t.Helper()
		if names := f.keptNames(externalScope); len(names) != 0 {
			t.Fatalf("kept = %v, want nothing stored", names)
		}
	}
	fresh := func(t *testing.T) {
		t.Helper()
		f.app.W.Kept.ClearAll()
		f.setAPI(t, true, "never")
	}

	t.Run("send", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(stageW))
		if len(h.held.Keep) != 1 || h.held.Keep[0].Name != "w" || h.held.Keep[0].Preview != "[1,2]" {
			t.Fatalf("held keep = %+v, want w with its preview", h.held.Keep)
		}
		nothingStored(t)
		f.app.Contract(t, heldWritesAPIKey, "ApprovalsService.List#api", []approvals.Item{h.req.Item}, opts...)
		held := h.held
		held.Text, held.Raw = stable(held.Text), stable(held.Raw)
		f.app.Contract(t, heldWritesAPIKey, "DbMcpService.PendingResponse#api", held, opts...)
		f.app.Contract(t, heldWritesAPIKey, "args:ApprovalsService.Answer#send", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: true})
		listBefore, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, heldWritesAPIKey, "KeptVarsService.List#beforeSend", listBefore, opts...)

		res := f.finish(t, h, true)
		if res.IsError || !strings.HasPrefix(text(res), `{"output":1,"tempSet":[{"name":"w","type":"array","items":2`) {
			t.Fatalf("result = %q, want the result and the kept summary", text(res))
		}
		e, ok := f.keptValue(t, "w")
		if !ok || string(e.Value) != "[1,2]" {
			t.Fatalf("stored = %s ok=%v, want [1,2]", e.Value, ok)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, heldWritesAPIKey, "KeptVarsService.List#afterSend", list, append(opts, flowharness.Replace(e.ID, "kv_w"))...)
	})

	t.Run("withhold", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(stageW))
		f.app.Contract(t, heldWritesAPIKey, "args:ApprovalsService.Answer#withhold", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: false})
		res := f.finish(t, h, false)
		if !res.IsError || !strings.Contains(text(res), "the user withheld the response") || strings.Contains(text(res), "kept") {
			t.Fatalf("result = %q, want the withheld text", text(res))
		}
		nothingStored(t)
	})

	t.Run("review timeout", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(stageW))
		res := wait(t, h.pending)
		if !res.IsError || !strings.Contains(text(res), "got no review within") || !strings.Contains(text(res), "was withheld") {
			t.Fatalf("result = %q, want the timed-out text", text(res))
		}
		nothingStored(t)
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue = %d after the timeout, want 0", n)
		}
	})

	t.Run("tools turned off during the review", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(stageW))
		f.setAPI(t, false, "never")
		res := f.finish(t, h, true)
		if !res.IsError || !strings.Contains(text(res), "API requests are turned off in Kira Space's Studio MCP settings while its response was in review") {
			t.Fatalf("result = %q, want the tools-off withhold", text(res))
		}
		nothingStored(t)
	})

	t.Run("scripts turned off during the review", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(stageW))
		f.setDbMcp(t, heldWritesAPIKey, "scriptsOff", &model.DbMcpPatch{APIScripts: ptr(false)})
		res := f.finish(t, h, true)
		const want = "the GET 127.0.0.1 request was sent, but scripts are turned off in Kira Space's Studio MCP settings while its response was in review, so it was withheld."
		if !res.IsError || text(res) != want {
			t.Fatalf("result = %q, want %q", text(res), want)
		}
		nothingStored(t)
		f.app.Contract(t, heldWritesAPIKey, "result#scriptsOff", text(res))
		f.setDbMcp(t, heldWritesAPIKey, "scriptsOn", &model.DbMcpPatch{APIScripts: ptr(true)})
	})

	t.Run("client closes the session during the review", func(t *testing.T) {
		fresh(t)
		client := f.externalSession(t)
		abandon, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			_, _ = client.CallTool(abandon, &mcp.CallToolParams{Name: "api_request", Arguments: script(stageW)})
		}()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		cancel()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
		nothingStored(t)
	})

	t.Run("transform fails after plain write", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(`plain.w = [1, 2]; throw new Error("boom");`))
		if len(h.held.Keep) != 0 || !h.held.IsError {
			t.Fatalf("held = %+v, want an error with no staged values", h.held)
		}
		res := f.finish(t, h, true)
		if !res.IsError || !strings.Contains(text(res), "transform script failed:") {
			t.Fatalf("result = %q, want the failure text", text(res))
		}
		nothingStored(t)
	})

	t.Run("name held by a user entry", func(t *testing.T) {
		fresh(t)
		user, err := f.app.W.Kept.Put(keptvars.Put{
			Scope: externalScope, Name: "w", Value: []byte(`"mine"`), Secret: true, Origin: srv.URL,
			Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "GET " + u, Client: "claude-code"},
		})
		if err != nil {
			t.Fatal(err)
		}
		h := f.startHeld(t, sess, script(`secret.w = "other"; return 1;`))
		res := f.finish(t, h, true)
		if res.IsError || !strings.Contains(text(res), `"tempNotSet":[{"name":"w","error":"`+tempNotSetUser+`"}]`) {
			t.Fatalf("result = %q, want the result and tempNotSet naming w", text(res))
		}
		if strings.Contains(text(res), `"tempSet":[`) {
			t.Fatalf("result = %q, want no tempSet summary", text(res))
		}
		now, ok := f.keptValue(t, "w")
		if !ok || now.ID != user.ID || string(now.Value) != `"mine"` {
			t.Fatalf("w = %+v ok=%v, want the user's entry untouched", now.Meta, ok)
		}
		f.app.Contract(t, heldWritesAPIKey, "result#userEntry", text(res))
	})

	t.Run("scope full", func(t *testing.T) {
		fresh(t)
		for i := 0; i < 64; i++ {
			f.keep(t, externalScope, fmt.Sprintf("e%d", i), "1")
		}
		before := f.keptNames(externalScope)
		h := f.startHeld(t, sess, script(stageW))
		res := f.finish(t, h, true)
		if res.IsError || !strings.Contains(text(res), `"tempNotSet":[{"name":"w","error":"too many temporary variables (64 per session)`) {
			t.Fatalf("result = %q, want tempNotSet for the full scope", text(res))
		}
		if after := f.keptNames(externalScope); strings.Join(after, ",") != strings.Join(before, ",") {
			t.Fatalf("kept changed from %d to %d names: something was evicted", len(before), len(after))
		}
		f.app.Contract(t, heldWritesAPIKey, "result#scopeFull", text(res))
	})

	t.Run("value over 1 MiB", func(t *testing.T) {
		fresh(t)
		h := f.startHeld(t, sess, script(`plain.w = "x".repeat(1100000); return 1;`))
		res := f.finish(t, h, true)
		if !res.IsError || strings.Contains(text(res), `"temp"`) {
			t.Fatalf("result = %q, want a failed script", text(res))
		}
		nothingStored(t)
		f.app.Contract(t, heldWritesAPIKey, "result#tooLarge", text(res))
	})
}
