package dbmcpflow

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-kept-secret.spec.ts (contract e2e-kept-secret): a login script keeps a
// session token as a secret and the account as a plain value; the user previews the plain one, reveals the
// secret behind the local-auth gate, and a follow-up request reads both while the agent never sees the token.

const (
	secretKey    = "e2e-kept-secret"
	sessionToken = "tok-e2e-session-7Q2xK"
	loginJSON    = `{"token":"` + sessionToken + `","account":"acct_42","orders":[11,12]}`
	loginKeep    = `secret.session_token = response.json.token; plain.account_id = response.json.account; return {account: response.json.account, orders: response.json.orders.length};`
)

func loginURL(base string) string {
	return base + "/raw?ct=" + url.QueryEscape("application/json") + "&hex=" + hex.EncodeToString([]byte(loginJSON))
}

type secretCard struct {
	Script struct {
		Transform struct {
			MayKeep []struct {
				Name   string
				Secret bool
			}
		}
	}
	Kept []struct {
		Name, Preview string
		Secret        bool
	}
}

func TestE2EKeptSecret(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, true)
	opts := f.e2eOpts(srv.URL)
	var agent []string
	remember := func(res string) { agent = append(agent, res) }
	reveal := func(t *testing.T, requestID, name string, confirmed bool) (bridge.KeptReveal, error) {
		t.Helper()
		return f.app.W.KeptVars.RevealStaged(bridge.KeptRevealStagedArgs{RequestID: requestID, Name: name, Confirmed: confirmed})
	}

	t.Run("login keeps a secret and a plain value", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", scriptArgs(envID, "POST", loginURL(srv.URL), map[string]any{"transform": loginKeep}))
		req := f.pending(t)
		var card secretCard
		if err := json.Unmarshal(req.Item.Payload, &card); err != nil {
			t.Fatal(err)
		}
		mk := card.Script.Transform.MayKeep
		if len(mk) != 2 || mk[0].Name != "session_token" || !mk[0].Secret || mk[1].Name != "account_id" || mk[1].Secret {
			t.Fatalf("mayKeep = %+v, want session_token secret and account_id plain", mk)
		}
		f.app.Contract(t, secretKey, "ApprovalsService.List#login", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, secretKey, "args:ApprovalsService.Answer#login", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answerItems(t, true, req.Item.ID)
		h := f.startHeldFromQueue(t)
		h.pending = pending
		keep := h.held.Keep
		if len(keep) != 2 || keep[0].Name != "account_id" || keep[0].Secret || keep[0].Preview != `"acct_42"` ||
			keep[1].Name != "session_token" || !keep[1].Secret || keep[1].Preview != "" {
			t.Fatalf("Will keep = %+v, want the secret without preview and the account with one", keep)
		}
		if strings.Contains(h.held.Text, sessionToken) || !strings.Contains(h.held.Text, "acct_42") {
			t.Fatalf("held text = %q, want the account and no token", h.held.Text)
		}
		if names := f.keptNames(externalScope); len(names) != 0 {
			t.Fatalf("kept store = %v before Send, want empty", names)
		}
		// The raw response carries a timing field whose digits vary, so its size does.
		sized := slices.Concat(opts, []flowharness.ContractOption{flowharness.Mask("rawBytes", "bytes")})
		f.app.Contract(t, secretKey, "ApprovalsService.List#review", []approvals.Item{h.req.Item}, sized...)
		pinned := h.held
		pinned.Text, pinned.Raw = stable(pinned.Text), stable(pinned.Raw)
		f.app.Contract(t, secretKey, "DbMcpService.PendingResponse#review", pinned, opts...)

		if _, err := reveal(t, h.req.RequestID, "account_id", true); err == nil {
			t.Fatal("revealing a plain staged value succeeded, want a refusal")
		}
		first, err := reveal(t, h.req.RequestID, "session_token", false)
		if err != nil || first.Outcome != apivars.OutcomeConfirmationRequired || first.Value != nil {
			t.Fatalf("unconfirmed reveal = %+v (%v), want confirmation required and no value", first, err)
		}
		f.app.Contract(t, secretKey, "args:KeptVarsService.RevealStaged#review", bridge.KeptRevealStagedArgs{RequestID: "<request>", Name: "session_token"})
		f.app.Contract(t, secretKey, "KeptVarsService.RevealStaged#confirmation", first)
		shown, err := reveal(t, h.req.RequestID, "session_token", true)
		if err != nil || shown.Outcome != apivars.OutcomeRevealed || shown.Value == nil || *shown.Value != sessionToken {
			t.Fatalf("confirmed reveal = %+v (%v), want the token", shown, err)
		}
		f.app.Contract(t, secretKey, "KeptVarsService.RevealStaged#review", shown)
		f.app.Contract(t, secretKey, "args:ApprovalsService.Answer#review", approvals.AnswerArgs{IDs: []string{h.req.Item.ID}, Approve: true})

		res := f.finish(t, h, true)
		remember(text(res))
		if res.IsError || strings.Contains(text(res), sessionToken) || !strings.Contains(text(res), `"name":"session_token","type":"string","items":0,"secret":true`) ||
			!strings.Contains(text(res), `"name":"account_id","type":"string","items":0,"secret":false`) {
			t.Fatalf("result = %q, want kept summaries with kinds and no token", text(res))
		}
		f.app.Contract(t, secretKey, "result#login", expiresIn.ReplaceAllString(text(res), `"expiresInSeconds":0`), opts...)
		tok, ok := f.keptValue(t, "session_token")
		if !ok || !tok.Secret || tok.Origin != srv.URL || string(tok.Value) != `"`+sessionToken+`"` {
			t.Fatalf("session_token = %+v ok=%v, want a secret bound to %s", tok.Meta, ok, srv.URL)
		}
		if acct, ok := f.keptValue(t, "account_id"); !ok || acct.Secret || acct.Origin != "" || string(acct.Value) != `"acct_42"` {
			t.Fatalf("account_id = %+v ok=%v, want a plain unbound value", acct.Meta, ok)
		}
		if _, err := reveal(t, h.req.RequestID, "session_token", true); err == nil {
			t.Fatal("staged reveal after Send succeeded, want a refusal")
		}
		list := text(callTool(t, sess, "list_saved_variables", map[string]any{}))
		remember(list)
		if !strings.Contains(list, `"name":"session_token","type":"string","items":0,"secret":true`) || strings.Contains(list, sessionToken) {
			t.Fatalf("list_saved_variables = %s, want the secret by name and kind only", list)
		}
	})

	t.Run("the follow-up request reads both", func(t *testing.T) {
		args := apiArgs(envID, "GET", srv.URL+"/echo?account={{plain.account_id}}", "Authorization", "Bearer {{secret.session_token}}")
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		var card secretCard
		if err := json.Unmarshal(req.Item.Payload, &card); err != nil {
			t.Fatal(err)
		}
		if len(card.Kept) != 2 {
			t.Fatalf("kept = %+v, want both variables", card.Kept)
		}
		for _, k := range card.Kept {
			switch {
			case k.Name == "session_token" && (!k.Secret || k.Preview != ""):
				t.Fatalf("session_token use = %+v, want secret without preview", k)
			case k.Name == "account_id" && (k.Secret || k.Preview != `"acct_42"`):
				t.Fatalf("account_id use = %+v, want plain with preview", k)
			}
		}
		ro := slices.Concat(opts, []flowharness.ContractOption{flowharness.Replace(f.keptID(t, externalScope, "session_token"), "kv_session_token"), flowharness.Replace(f.keptID(t, externalScope, "account_id"), "kv_account_id")})
		f.app.Contract(t, secretKey, "ApprovalsService.List#followup", []approvals.Item{req.Item}, ro...)
		f.app.Contract(t, secretKey, "args:ApprovalsService.Answer#followup", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answerItems(t, true, req.Item.ID)
		h := f.startHeldFromQueue(t)
		h.pending = pending
		res := f.finish(t, h, true)
		remember(text(res))
		if res.IsError || strings.Contains(text(res), sessionToken) {
			t.Fatalf("result = %q, want the echo without the token", text(res))
		}
		seen := srv.Requests()
		last := seen[len(seen)-1]
		if last.Header.Get("Authorization") != "Bearer "+sessionToken || !strings.Contains(last.Query, "account=acct_42") {
			t.Fatalf("server saw %+v, want the token in the header and the account in the query", last)
		}
	})

	t.Run("the secret stays with the origin that made it", func(t *testing.T) {
		other := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
		sent := len(srv.Requests())
		res := callTool(t, sess, "api_request", apiArgs(envID, "GET", other+"/echo", "Authorization", "Bearer {{secret.session_token}}"))
		remember(text(res))
		if !res.IsError || !strings.Contains(text(res), `the secret saved as "session_token" belongs to `+srv.URL) || f.queueLen(t) != 0 || len(srv.Requests()) != sent {
			t.Fatalf("result = %q, queue %d, want a refusal before any card", text(res), f.queueLen(t))
		}
		f.app.Contract(t, secretKey, "result#otherOrigin", text(res), append(opts, flowharness.Replace(other, "http://localhost:<port>"))...)
	})

	t.Run("Settings lists the kinds", func(t *testing.T) {
		list, err := f.app.W.KeptVars.List()
		if err != nil {
			t.Fatal(err)
		}
		var secretRow, plainRow *bridge.KeptEntryView
		for i := range list.Entries {
			switch list.Entries[i].Name {
			case "session_token":
				secretRow = &list.Entries[i]
			case "account_id":
				plainRow = &list.Entries[i]
			}
		}
		if secretRow == nil || !secretRow.Secret || secretRow.Preview != "" || secretRow.Origin != srv.URL || plainRow == nil || plainRow.Secret || plainRow.Preview != `"acct_42"` {
			t.Fatalf("list = %+v, want the secret without preview and the plain value with one", list.Entries)
		}
		ro := slices.Concat(opts, []flowharness.ContractOption{flowharness.Replace(secretRow.ID, "kv_session_token"), flowharness.Replace(plainRow.ID, "kv_account_id")})
		f.app.Contract(t, secretKey, "KeptVarsService.List#kinds", list, ro...)
		f.app.Contract(t, secretKey, "args:KeptVarsService.Reveal#kinds", bridge.KeptRevealArgs{ID: "kv_session_token", Confirmed: true})
		got, err := f.app.W.KeptVars.Reveal(bridge.KeptRevealArgs{ID: secretRow.ID, Confirmed: true})
		if err != nil || got.Value == nil || *got.Value != sessionToken {
			t.Fatalf("Reveal = %+v (%v), want the token", got, err)
		}
		f.app.Contract(t, secretKey, "KeptVarsService.Reveal#kinds", got)
	})

	t.Run("the token is nowhere the agent or the app records", func(t *testing.T) {
		f.noLeak(t, sessionToken, agent...)
	})
}

// TestDbMcpQueryScriptKeepSecret: a run_query script keeps a value as a secret. It reaches no agent text and no
// run_query, which never reads a secret; only api_request binds it.
func TestDbMcpQueryScriptKeepSecret(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)

	res := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `secret.note_key = result.rows[0][1]; return {count: result.rows.length};`))
	if res.IsError || !strings.Contains(text(res), `"name":"note_key","type":"string","items":0,"secret":true`) || strings.Contains(text(res), "one") {
		t.Fatalf("result = %s, want a secret summary and no cell", text(res))
	}
	e, ok := f.keptValue(t, "note_key")
	if !ok || !e.Secret || e.Origin != "" || e.Source.MCP != "db" || string(e.Value) != `"one"` {
		t.Fatalf("stored = %+v ok=%v, want a secret from the db script bound to no origin", e.Meta, ok)
	}
	if e.Source.Kind != keptvars.SourceScript {
		t.Fatalf("source = %s, want script", e.Source.Kind)
	}
	read := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", `return plain.note_key;`))
	if !read.IsError || !strings.Contains(text(read), `"note_key" is secret;`) {
		t.Fatalf("read back = %s, want the secret refusal", text(read))
	}
}
