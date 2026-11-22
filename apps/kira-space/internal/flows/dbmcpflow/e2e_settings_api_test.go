package dbmcpflow

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-settings-api.spec.ts (contract e2e-settings-api): a setting changed
// while an approval card or a held response waits. The review settings are read after the send and the
// tools and scripts switches after the card, so the answer meets the settings as they are then.

const settingsAPIKey = "e2e-settings-api"

type reviewOffer struct{ ReviewOffered, ReviewResult bool }

func offerOf(t *testing.T, it approvals.Item) reviewOffer {
	t.Helper()
	var o reviewOffer
	if err := json.Unmarshal(it.Payload, &o); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestE2ESettingsAPI(t *testing.T) {
	f := newFixture(t)
	srv, sess, envID := f.reviewSession(t, "")
	f.setAPI(t, true, "always")
	f.setReviewApp(t, false)
	opts := f.e2eOpts(srv.URL)
	u := usersURL(srv.URL)

	card := func(key string, args map[string]any) (chan callOutcome, pendingReq) {
		t.Helper()
		pending := callToolAsync(sess, "api_request", args)
		req := f.pending(t)
		if req.Item.Kind != approvals.KindAPIRequest {
			t.Fatalf("%s: item kind = %s, want api.request", key, req.Item.Kind)
		}
		f.app.Contract(t, settingsAPIKey, "ApprovalsService.List#"+key, []approvals.Item{req.Item}, opts...)
		return pending, req
	}
	// response waits until the held response of the oldest item shows, and returns it.
	response := func(reason string) pendingReq {
		t.Helper()
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		r := f.pending(t)
		if p := f.responseItem(t, r.Item); p.Reason != reason {
			t.Fatalf("held reason = %q, want %q", p.Reason, reason)
		}
		return r
	}

	t.Run("review off at the card, on while it waits", func(t *testing.T) {
		pending, req := card("step1", apiArgs(envID, "GET", u))
		if o := offerOf(t, req.Item); !o.ReviewOffered || o.ReviewResult {
			t.Fatalf("offer = %+v, want review offered and off", o)
		}
		f.setDbMcp(t, settingsAPIKey, "reviewOn", &model.DbMcpPatch{APIReviewResponses: ptr(true)})
		f.answerItems(t, true, req.Item.ID)
		held := response("setting")
		f.answer(t, true, held.RequestID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), "a@x.test") {
			t.Fatalf("result = %q, want the response after the send", text(res))
		}
	})

	t.Run("review on at the card, off while it waits", func(t *testing.T) {
		pending, req := card("step2", apiArgs(envID, "GET", u))
		if o := offerOf(t, req.Item); !o.ReviewOffered || !o.ReviewResult {
			t.Fatalf("offer = %+v, want review offered and on", o)
		}
		f.setDbMcp(t, settingsAPIKey, "reviewOff", &model.DbMcpPatch{APIReviewResponses: ptr(false)})
		f.answerItems(t, true, req.Item.ID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), "a@x.test") {
			t.Fatalf("result = %q, want the response with no review", text(res))
		}
		if n := f.queueLen(t); n != 0 {
			t.Fatalf("queue = %d, want no response item", n)
		}
	})

	t.Run("environment review written while the card waits", func(t *testing.T) {
		post := apiArgs(envID, "POST", u)
		pending, req := card("step3", post)
		f.reviewEnv(t, "agentReview:\n  write: review\n")
		f.setAPI(t, true, "always")
		testx.WaitUntil(t, 10*time.Second, func() bool {
			var out struct {
				Environments []struct{ Review []string }
			}
			raw := text(callTool(t, sess, "list", map[string]any{"what": []string{"environments"}}))
			_ = json.Unmarshal([]byte(raw), &out)
			return len(out.Environments) == 1 && slices.Contains(out.Environments[0].Review, "write")
		})
		f.answerItems(t, true, req.Item.ID)
		held := response("setting")
		f.app.Contract(t, settingsAPIKey, "ApprovalsService.List#step3held", []approvals.Item{held.Item}, opts...)
		// Back to inherit while the review waits: the response is already held and stays held.
		f.reviewEnv(t, "")
		f.setAPI(t, true, "always")
		if n := f.queueLen(t); n != 1 || f.stageNow(t) != "response" {
			t.Fatalf("queue = %d stage %q after inherit, want the response still held", n, f.stageNow(t))
		}
		f.answer(t, true, held.RequestID)
		if res := wait(t, pending); res.IsError {
			t.Fatalf("result = %q", text(res))
		}
	})

	scripted := func() map[string]any {
		return scriptArgs(envID, "POST", srv.URL+"/echo", map[string]any{"transform": "return 1"})
	}
	t.Run("scripts turned off while the card waits", func(t *testing.T) {
		sent := len(srv.Requests())
		pending, req := card("step4", scripted())
		f.setDbMcp(t, settingsAPIKey, "scriptsOff", &model.DbMcpPatch{APIScripts: ptr(false)})
		f.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		const want = "scripts are turned off in Kira Space's Studio MCP settings. Nothing was sent."
		if !res.IsError || text(res) != want {
			t.Fatalf("result = %q, want %q", text(res), want)
		}
		if got := len(srv.Requests()); got != sent {
			t.Fatalf("server saw %d requests, want %d: the request was sent", got, sent)
		}
		f.app.Contract(t, settingsAPIKey, "result#step4", text(res))

		f.setDbMcp(t, settingsAPIKey, "scriptsOn", &model.DbMcpPatch{APIScripts: ptr(true)})
		again := callToolAsync(sess, "api_request", scripted())
		f.answerItems(t, true, f.pending(t).Item.ID)
		if res := wait(t, again); text(res) != `{"output":1}` {
			t.Fatalf("scripts back on = %q, want the transform result without a restart", text(res))
		}
	})

	t.Run("tools turned off while the card waits", func(t *testing.T) {
		sent := len(srv.Requests())
		pending, req := card("step5", apiArgs(envID, "POST", srv.URL+"/echo"))
		f.setDbMcp(t, settingsAPIKey, "toolsOff", &model.DbMcpPatch{APIRequests: ptr(false)})
		f.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		const want = "API requests are turned off in Kira Space's Studio MCP settings. Nothing was sent."
		if !res.IsError || text(res) != want {
			t.Fatalf("result = %q, want %q", text(res), want)
		}
		if got := len(srv.Requests()); got != sent {
			t.Fatalf("server saw %d requests, want %d: the request was sent", got, sent)
		}
		f.app.Contract(t, settingsAPIKey, "result#step5", text(res))
	})
}
