package dbmcpflow

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-kept-chain.spec.ts (contract e2e-kept-chain): a value kept by one call
// is used by the next across two external clients, stays out of a Studio session's scope, and is withdrawn
// from a waiting card when the session ends or a commit replaces it.

const (
	keptChainKey = "e2e-kept-chain"
	chainKeep    = `plain.ids = response.json.users.map(u => u.id); return response.json.total;`
	chainPrep    = `return {body: {items: plain.ids.map(id => ({id}))}};`
	chainTrans   = `plain.first = String(response.json.body ? 1 : 0); return 1;`
	bigID        = "9007199254740993"
)

func TestE2EKeptChain(t *testing.T) {
	f := newFixture(t)
	srv, a, envID := f.apiSetup(t, "127.0.0.1", false)
	b := f.externalSession(t)
	opts := f.e2eOpts(srv.URL)
	u := usersURL(srv.URL)
	echo := srv.URL + "/echo"
	studio, releaseStudio := f.studioGrant(t, "e2e-s1")
	// A second grant keeps the session server up when S ends, so the answer of S's waiting call is delivered.
	f.studioGrant(t, "e2e-s2")

	t.Run("A keeps ids from a response", func(t *testing.T) {
		pending := callToolAsync(a, "api_request", scriptArgs(envID, "GET", u, map[string]any{"transform": chainKeep}))
		f.answerItems(t, true, f.pending(t).Item.ID)
		res := wait(t, pending)
		if res.IsError || !strings.HasPrefix(text(res), `{"output":2,"tempSet":[{"name":"ids","type":"array","items":2,"secret":false,`) || strings.Contains(text(res), bigID) {
			t.Fatalf("result = %q, want the total and a summary without the ids", text(res))
		}
		f.app.Contract(t, keptChainKey, "result#step1", expiresIn.ReplaceAllString(text(res), `"expiresInSeconds":0`))
		e, ok := f.keptValue(t, "ids")
		if !ok || string(e.Value) != keptIDs || e.Source.MCP != "api" || e.Source.Kind != keptvars.SourceScript {
			t.Fatalf("stored = %+v ok=%v, want ids kept by an API script in the external scope", e.Meta, ok)
		}
	})

	t.Run("B reads it into a request body", func(t *testing.T) {
		list := text(callTool(t, b, "list", map[string]any{"what": []string{"temp"}}))
		if !strings.Contains(list, `"ids"`) {
			t.Fatalf("B's listing = %s, want the shared ids", list)
		}
		pending := callToolAsync(b, "api_request", jsonPost(envID, echo, `{"ids": {{plain.ids}}}`))
		req := f.pending(t)
		var pl keptScriptItem
		if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
			t.Fatal(err)
		}
		if len(pl.Kept) != 1 || pl.Kept[0].ID != f.keptID(t, externalScope, "ids") || pl.Kept[0].Preview != keptIDs ||
			len(pl.Kept[0].Sites) != 1 || pl.Kept[0].Sites[0].Field != "body" {
			t.Fatalf("kept = %+v, want A's entry read in the body", pl.Kept)
		}
		ids := f.keptID(t, externalScope, "ids")
		ro := append(slices.Clip(opts), flowharness.Replace(ids, "kv_ids"))
		f.app.Contract(t, keptChainKey, "ApprovalsService.List#step2", []approvals.Item{req.Item}, ro...)
		f.app.Contract(t, keptChainKey, "args:ApprovalsService.Answer#step2", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError {
			t.Fatalf("result = %q", text(res))
		}
		seen := srv.Requests()
		if len(seen) != 2 || string(seen[1].Body) != `{"ids": `+keptIDs+`}` {
			t.Fatalf("server saw %+v, want the exact ids in the body", seen)
		}
	})

	t.Run("B reads and keeps through scripts", func(t *testing.T) {
		args := scriptArgs(envID, "POST", echo, map[string]any{"prepare": chainPrep, "transform": chainTrans})
		pending := callToolAsync(b, "api_request", args)
		req := f.pending(t)
		var pl keptScriptItem
		if err := json.Unmarshal(req.Item.Payload, &pl); err != nil {
			t.Fatal(err)
		}
		if len(pl.Kept) != 1 || pl.Kept[0].Name != "ids" || len(pl.Kept[0].Sites) != 1 || pl.Kept[0].Sites[0].Field != "script" || pl.Kept[0].Sites[0].Part != "prepare" ||
			chainPrep[pl.Kept[0].Sites[0].From:pl.Kept[0].Sites[0].To] != `plain.ids` {
			t.Fatalf("kept = %+v, want the prepare read site", pl.Kept)
		}
		mk := pl.Script.Transform.MayKeep
		if len(mk) != 1 || mk[0].Name != "first" {
			t.Fatalf("mayKeep = %+v, want first", mk)
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"))
		f.app.Contract(t, keptChainKey, "ApprovalsService.List#step3", []approvals.Item{req.Item}, ro...)
		f.answer(t, true, req.RequestID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), `"name":"first"`) {
			t.Fatalf("result = %q, want first kept", text(res))
		}
		if e, ok := f.keptValue(t, "first"); !ok || string(e.Value) != `"1"` {
			t.Fatalf("first = %s ok=%v, want \"1\"", e.Value, ok)
		}
	})

	t.Run("a Studio session has its own scope", func(t *testing.T) {
		s := studio
		if list := text(callTool(t, s, "list", map[string]any{"what": []string{"temp"}})); strings.Contains(list, `"ids"`) || strings.Contains(list, `"first"`) {
			t.Fatalf("S's listing = %s, want neither external variable", list)
		}
		res := callTool(t, s, "api_request", jsonPost(envID, echo, `{"ids": {{plain.ids}}}`))
		if !res.IsError || !strings.Contains(text(res), `no temporary variable "ids"`) || f.queueLen(t) != 0 {
			t.Fatalf("S read = %q, queue %d, want a refusal before any card", text(res), f.queueLen(t))
		}
		pending := callToolAsync(s, "api_request", scriptArgs(envID, "GET", u, map[string]any{"transform": `plain.ids = [5]; return 1;`}))
		f.answerItems(t, true, f.pending(t).Item.ID)
		if r := wait(t, pending); r.IsError {
			t.Fatalf("S keep = %q", text(r))
		}
		if e, ok := f.keptValue(t, "ids"); !ok || string(e.Value) != keptIDs {
			t.Fatalf("external ids = %s ok=%v, want it unchanged", e.Value, ok)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		var labels []string
		for _, e := range list.Entries {
			if e.Name == "ids" {
				labels = append(labels, e.ScopeLabel)
			}
		}
		if len(labels) != 2 || labels[0] == labels[1] {
			t.Fatalf("ids rows carry scope labels %v, want two different ones", labels)
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"),
			flowharness.Replace(f.keptID(t, "session:e2e-s1", "ids"), "kv_ids_session"), flowharness.Replace(f.keptID(t, externalScope, "first"), "kv_first"))
		f.app.Contract(t, keptChainKey, "KeptVarsService.List#scopes", list, ro...)
	})

	t.Run("the session ends while its card waits", func(t *testing.T) {
		s := studio
		sent := len(srv.Requests())
		pending := callToolAsync(s, "api_request", jsonPost(envID, echo, `{"ids": {{plain.ids}}}`))
		f.items(t, 1)
		releaseStudio()
		// The session's token is revoked with its grant, so the transport may end the call before the handler's
		// text reaches the client; when the text does arrive it is the ended-with-the-session refusal.
		out := <-pending
		const want = `temporary variable "ids" ended with the session while waiting for approval; nothing was sent. Produce it again, then repeat the request.`
		switch {
		case out.err != nil:
			if !strings.Contains(out.err.Error(), "terminated without response") && !strings.Contains(out.err.Error(), "Unauthorized") {
				t.Fatalf("call error = %v, want the transport ending the call", out.err)
			}
		case !out.res.IsError || text(out.res) != want:
			t.Fatalf("result = %q, want %q", text(out.res), want)
		}
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
		if got := len(srv.Requests()); got != sent {
			t.Fatalf("server saw %d requests, want %d", got, sent)
		}
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range list.Entries {
			if strings.HasPrefix(e.ScopeLabel, "Session") {
				t.Fatalf("entry %+v of the ended session is still listed", e)
			}
		}
		ro := append(slices.Clip(opts), flowharness.Replace(f.keptID(t, externalScope, "ids"), "kv_ids"), flowharness.Replace(f.keptID(t, externalScope, "first"), "kv_first"))
		f.app.Contract(t, keptChainKey, "KeptVarsService.List#ended", list, ro...)
	})

	t.Run("a commit replaces the variable a card waits on", func(t *testing.T) {
		sent := len(srv.Requests())
		waiting := callToolAsync(b, "api_request", jsonPost(envID, echo, `{"ids": {{plain.ids}}}`))
		first := f.items(t, 1)[0]
		replacing := callToolAsync(a, "api_request", scriptArgs(envID, "GET", u, map[string]any{"transform": `plain.ids = [7, 8]; return 1;`}))
		items := f.items(t, 2)
		f.answerItems(t, true, items[1].ID)
		if res := wait(t, replacing); res.IsError {
			t.Fatalf("replacing call = %q", text(res))
		}
		res := wait(t, waiting)
		if !res.IsError || !strings.Contains(text(res), `temporary variable "ids"`) || !strings.Contains(text(res), "was replaced") {
			t.Fatalf("waiting call = %q, want a replaced-while-waiting error", text(res))
		}
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
		if got := len(srv.Requests()); got != sent+1 {
			t.Fatalf("server saw %d requests, want only the replacing call's (%d)", got, sent+1)
		}
		if first.ID == "" {
			t.Fatal("no first item")
		}
	})

	t.Run("the op log never holds an id", func(t *testing.T) {
		var carried bool
		for _, r := range f.opsRecent(t) {
			if r.Command == nil {
				continue
			}
			if strings.Contains(*r.Command, bigID) {
				t.Errorf("op command %q holds a kept value", *r.Command)
			}
			carried = carried || (r.Kind == "jsPrepare" && strings.Contains(*r.Command, `plain.ids`))
		}
		if !carried {
			t.Error("no jsPrepare row carries the plain read")
		}
	})
}
