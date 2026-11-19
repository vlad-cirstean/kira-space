package dbmcpflow

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-kept-expiry.spec.ts (contract e2e-kept-expiry): a kept variable that
// expires or is cleared while the chain runs or while cards wait on it ends the waiting call with a text that
// names it, across both MCPs. The UI half shows the expiry tag and sends the same Clear all.

const (
	expiryKey  = "e2e-kept-expiry"
	e6IDsStmt  = "select body from notes where id in (select value from json_each(?)) order by id"
	e6WriteSQL = "insert into notes (body) select 'x' from json_each(?)"
)

// e6Put stores a list as a script of the API MCP would, with its own lifetime.
func (f *fixture) e6Put(t *testing.T, name, value string, ttl time.Duration) keptvars.Meta {
	t.Helper()
	m, err := f.app.W.Kept.Put(keptvars.Put{Scope: externalScope, Name: name, Value: json.RawMessage(value), TTL: ttl, Source: keptSource})
	if err != nil {
		t.Fatalf("keep %s: %v", name, err)
	}
	return m
}

// e6Clear clears one variable as the Settings pane does.
func (f *fixture) e6Clear(t *testing.T, ids ...string) {
	t.Helper()
	if n, err := f.app.W.KeptVars.Clear(bridge.KeptClearArgs{IDs: ids}); err != nil || n != len(ids) {
		t.Fatalf("Clear = %d, %v, want %d", n, err, len(ids))
	}
}

// e6ExpiryOf is when the item's first kept variable expires, from the card payload.
func e6ExpiryOf(t *testing.T, it approvals.Item) time.Time {
	t.Helper()
	var pl struct {
		Kept []struct {
			ExpiresAt time.Time `json:"expiresAt"`
		} `json:"kept"`
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil || len(pl.Kept) == 0 {
		t.Fatalf("payload %s (%v), want kept rows", it.Payload, err)
	}
	return pl.Kept[0].ExpiresAt
}

func e6WantGone(t *testing.T, got, name, why, nothing string) {
	t.Helper()
	want := `kept variable "` + name + `" ` + why + ` while waiting for approval; ` + nothing
	if !strings.HasPrefix(got, want) {
		t.Fatalf("result = %q, want it to start with %q", got, want)
	}
}

func TestE2EKeptExpiry(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.apiSetup(t, "127.0.0.1", false)
	echo := srv.URL + "/echo"
	opts := f.e2eOpts(srv.URL)
	idle := func(t *testing.T) {
		t.Helper()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	}

	t.Run("an expiring list binds as a param", func(t *testing.T) {
		f.e6Put(t, "ids", "[1,2]", 3*time.Second)
		res := callTool(t, sess, "run_query", f.paramsArgs(e6IDsStmt, "read", "ids"))
		if res.IsError || !strings.Contains(text(res), "one") || !strings.Contains(text(res), "two") {
			t.Fatalf("read = %q, want both rows", text(res))
		}
	})

	t.Run("an API card waits past the expiry", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", jsonPost(envID, echo, `{"ids": {{kept.ids}}}`))
		req := f.pending(t)
		if exp := e6ExpiryOf(t, req.Item); !exp.Before(time.UnixMilli(req.Item.ExpiresAt)) {
			t.Fatalf("kept expires %v, item %v, want the variable first", exp, time.UnixMilli(req.Item.ExpiresAt))
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, expiryKey, "ApprovalsService.List#expiring", []approvals.Item{req.Item}, ro...)
		res := wait(t, pending)
		if !res.IsError {
			t.Fatalf("result = %q, want a refusal", text(res))
		}
		e6WantGone(t, text(res), "ids", "expired", "nothing was sent.")
		idle(t)
		if n := len(srv.Requests()); n != 0 {
			t.Fatalf("server saw %d requests, want none", n)
		}
	})

	t.Run("the next chained call finds it gone", func(t *testing.T) {
		res := callTool(t, sess, "run_query", f.paramsArgs(e6IDsStmt, "read", "ids"))
		if !res.IsError || !strings.Contains(text(res), `no kept variable "ids" (it may have expired or been cleared);`) || !strings.Contains(text(res), "list_kept_variables") {
			t.Fatalf("read = %q, want the not-found text naming list_kept_variables", text(res))
		}
		if list := text(callTool(t, sess, "list_kept_variables", map[string]any{})); strings.Contains(list, `"ids"`) {
			t.Fatalf("listing = %s, want ids gone", list)
		}
	})

	t.Run("a DB write card waits past the expiry", func(t *testing.T) {
		before := f.notesBodies(t)
		short := f.e6Put(t, "short", "[1]", 3*time.Second)
		pending := callToolAsync(sess, "run_query", f.paramsArgs(e6WriteSQL, "write", "short"))
		req := f.pending(t)
		if exp := e6ExpiryOf(t, req.Item); !exp.Before(time.UnixMilli(req.Item.ExpiresAt)) {
			t.Fatalf("kept expires %v, item %v, want the variable first", exp, time.UnixMilli(req.Item.ExpiresAt))
		}
		ro := append(slices.Clip(opts), flowharness.Replace(short.ID, "kv_short"))
		f.app.Contract(t, expiryKey, "ApprovalsService.List#expiringDb", []approvals.Item{req.Item}, ro...)
		res := wait(t, pending)
		if !res.IsError {
			t.Fatalf("result = %q, want a refusal", text(res))
		}
		e6WantGone(t, text(res), "short", "expired", "nothing ran.")
		idle(t)
		if got := f.notesBodies(t); !slices.Equal(got, before) {
			t.Fatalf("notes = %q, want %q", got, before)
		}
	})

	t.Run("a clear mid-chain refuses the next call and ends a waiting write", func(t *testing.T) {
		before := f.notesBodies(t)
		keep := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
		if keep.IsError || !strings.Contains(text(keep), `"kept":[{"name":"ids"`) {
			t.Fatalf("keep = %q, want ids kept", text(keep))
		}
		f.e6Clear(t, f.keptID(t, externalScope, "ids"))
		res := callTool(t, sess, "run_query", f.paramsArgs(e6IDsStmt, "read", "ids"))
		if !res.IsError || !strings.Contains(text(res), `no kept variable "ids"`) {
			t.Fatalf("read after clear = %q, want not found", text(res))
		}

		callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
		pending := callToolAsync(sess, "run_query", f.paramsArgs(e6WriteSQL, "write", "ids"))
		f.items(t, 1)
		f.e6Clear(t, f.keptID(t, externalScope, "ids"))
		out := wait(t, pending)
		if !out.IsError {
			t.Fatalf("result = %q, want a refusal", text(out))
		}
		e6WantGone(t, text(out), "ids", "was cleared by the user", "nothing ran.")
		idle(t)
		if got := f.notesBodies(t); !slices.Equal(got, before) {
			t.Fatalf("notes = %q, want %q", got, before)
		}
	})

	t.Run("Clear all ends three waiting cards", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"read": "prompt"})
		f.keep(t, externalScope, "ids", "[1,2]")
		before := f.notesBodies(t)
		sent := len(srv.Requests())
		api := callToolAsync(sess, "api_request", jsonPost(envID, echo, `{"ids": {{kept.ids}}}`))
		f.items(t, 1)
		write := callToolAsync(sess, "run_query", f.paramsArgs(e6WriteSQL, "write", "ids"))
		f.items(t, 2)
		script := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return kept.get("ids").length;`))
		items := f.items(t, 3)
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, expiryKey, "ApprovalsService.List#three", items, ro...)
		waiting, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, expiryKey, "KeptVarsService.List#waiting", waiting, ro...)
		if n, err := f.app.W.KeptVars.ClearAll(); err != nil || n != 1 {
			t.Fatalf("ClearAll = %d, %v, want 1", n, err)
		}
		e6WantGone(t, text(wait(t, api)), "ids", "was cleared by the user", "nothing was sent.")
		e6WantGone(t, text(wait(t, write)), "ids", "was cleared by the user", "nothing ran.")
		e6WantGone(t, text(wait(t, script)), "ids", "was cleared by the user", "nothing ran.")
		idle(t)
		if got := len(srv.Requests()); got != sent {
			t.Fatalf("server saw %d requests, want %d", got, sent)
		}
		if got := f.notesBodies(t); !slices.Equal(got, before) {
			t.Fatalf("notes = %q, want %q", got, before)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, expiryKey, "KeptVarsService.List#afterClear", list, opts...)
		f.mcpSet(t, map[string]string{"read": "allow"})
	})

	t.Run("a clear while a review waits leaves the review", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"review.read": "review"})
		f.keep(t, externalScope, "ids", "[1,2]")
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return kept.get("ids");`))
		req := f.pending(t)
		if req.Stage != "response" {
			t.Fatalf("pending = %+v, want a held result", req)
		}
		f.e6Clear(t, f.keptID(t, externalScope, "ids"))
		if n := f.queueLen(t); n != 1 {
			t.Fatalf("queue %d after the clear, want the review to stay (watchKept covers card waits only)", n)
		}
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if res.IsError || !strings.Contains(text(res), `"output":[1,2]`) {
			t.Fatalf("sent = %q, want the output the script computed before the clear", text(res))
		}
		f.mcpSet(t, map[string]string{"review.read": "send"})
	})

	t.Run("retention applies to later values only", func(t *testing.T) {
		old := f.keep(t, externalScope, "older", "1")
		f.setDbMcp(t, expiryKey, "retention", &model.DbMcpPatch{KeptRetentionMinutes: ptr(5)})
		res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `kept.set("fresh", 1); return 1;`))
		if res.IsError {
			t.Fatalf("keep = %q", text(res))
		}
		fresh, ok := f.keptValue(t, "fresh")
		if !ok {
			t.Fatal("fresh not kept")
		}
		if got := fresh.ExpiresAt.Sub(fresh.CreatedAt); got != 5*time.Minute {
			t.Fatalf("fresh lives %v, want 5m", got)
		}
		again, _ := f.keptValue(t, "older")
		if !again.ExpiresAt.Equal(old.ExpiresAt) || old.ExpiresAt.Sub(old.CreatedAt) != time.Hour {
			t.Fatalf("older expires %v (was %v), want its 60 minute expiry untouched", again.ExpiresAt, old.ExpiresAt)
		}
	})
}
