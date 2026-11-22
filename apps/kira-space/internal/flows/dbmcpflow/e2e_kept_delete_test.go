package dbmcpflow

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-kept-delete.spec.ts (contract e2e-kept-delete): the user deletes a kept
// variable a card waits on, and drops one value a held result would keep before sending it.

const (
	deleteKey  = "e2e-kept-delete"
	deleteKeep = `kira.storeValue("token", response.json.token); kira.storeValue("account_id", response.json.account); return {account: response.json.account};`
)

func TestE2EKeptDelete(t *testing.T) {
	captureLog(t)
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, true)
	opts := f.e2eOpts(srv.URL)

	t.Run("a held result keeps only what the user leaves", func(t *testing.T) {
		pending := callToolAsync(sess, "api_request", scriptArgs(envID, "POST", loginURL(srv.URL), map[string]any{"transform": deleteKeep}))
		req := f.pending(t)
		f.answerItems(t, true, req.Item.ID)
		h := f.startHeldFromQueue(t)
		h.pending = pending
		if got := keepNames(h.held.Keep); !slices.Equal(got, []string{"token", "account_id"}) {
			t.Fatalf("Will keep = %v, want token and account_id", got)
		}
		sized := append(slices.Clip(opts), flowharness.Mask("rawBytes", "bytes"))
		f.app.Contract(t, deleteKey, "ApprovalsService.List#review", []approvals.Item{h.req.Item}, sized...)
		f.app.Contract(t, deleteKey, "DbMcpService.PendingResponse#both", stableHeld(h.held), opts...)
		drop := func(name string) error {
			return f.app.W.KeptVars.DropStaged(bridge.KeptDropStagedArgs{RequestID: h.req.RequestID, Name: name})
		}
		f.app.Contract(t, deleteKey, "args:KeptVarsService.DropStaged#review", bridge.KeptDropStagedArgs{RequestID: "<request>", Name: "account_id"})
		if err := drop("account_id"); err != nil {
			t.Fatalf("DropStaged = %v, want ok", err)
		}
		if err := drop("account_id"); err == nil {
			t.Fatal("second DropStaged succeeded, want not found")
		}
		if err := drop("nope"); err == nil {
			t.Fatal("DropStaged of an unknown name succeeded, want not found")
		}
		after := f.pendingResponse(t, h.req.RequestID)
		if got := keepNames(after.Keep); !slices.Equal(got, []string{"token"}) {
			t.Fatalf("Will keep after drop = %v, want token", got)
		}
		f.app.Contract(t, deleteKey, "DbMcpService.PendingResponse#dropped", stableHeld(after), opts...)

		res := f.finish(t, h, true)
		var out struct {
			Kept     []struct{ Name string }        `json:"kept"`
			NotSaved []struct{ Name, Error string } `json:"keptNotSaved"`
		}
		if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
			t.Fatalf("result %q: %v", text(res), err)
		}
		if res.IsError || len(out.Kept) != 1 || out.Kept[0].Name != "token" ||
			len(out.NotSaved) != 1 || out.NotSaved[0].Name != "account_id" || out.NotSaved[0].Error != "the user chose not to keep it" {
			t.Fatalf("result = %q, want token kept and account_id reported as not kept by the user", text(res))
		}
		f.app.Contract(t, deleteKey, "result#send", expiresIn.ReplaceAllString(text(res), `"expiresInSeconds":0`), opts...)
		if names := f.keptNames(externalScope); !slices.Equal(names, []string{"token"}) {
			t.Fatalf("kept store = %v, want only token", names)
		}
	})

	t.Run("deleting a variable a card waits on ends the call", func(t *testing.T) {
		sent := len(srv.Requests())
		pending := callToolAsync(sess, "api_request", apiArgs(envID, "GET", srv.URL+"/echo", "Authorization", "Bearer {{kept.token}}"))
		req := f.pending(t)
		id := f.keptID(t, externalScope, "token")
		ro := append(slices.Clip(opts), flowharness.Replace(id, "kv_token"))
		f.app.Contract(t, deleteKey, "ApprovalsService.List#waiting", []approvals.Item{req.Item}, ro...)
		f.app.Contract(t, deleteKey, "args:KeptVarsService.Clear#row", bridge.KeptClearArgs{IDs: []string{"kv_token"}})
		f.e6Clear(t, id)
		e6WantGone(t, text(wait(t, pending)), "token", "was cleared by the user", "nothing was sent.")
		if f.queueLen(t) != 0 || len(srv.Requests()) != sent {
			t.Fatalf("queue %d, server saw %d new requests, want the card gone and none sent", f.queueLen(t), len(srv.Requests())-sent)
		}
	})
}

func keepNames(keep []bridge.DbMcpPendingKeep) []string {
	var names []string
	for _, k := range keep {
		names = append(names, k.Name)
	}
	return names
}

// stableHeld masks the held text and raw response, whose timing digits vary.
func stableHeld(h bridge.DbMcpPendingResponse) bridge.DbMcpPendingResponse {
	h.Text, h.Raw = stable(h.Text), stable(h.Raw)
	return h
}
