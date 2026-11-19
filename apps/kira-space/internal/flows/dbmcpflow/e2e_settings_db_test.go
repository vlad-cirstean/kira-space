package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/e2e-settings-db.spec.ts (contract e2e-settings-db): a setting changed
// between the approval card and the send takes effect for the call that already waits.

const settingsKey = "e2e-settings-db"

// setScripts flips the agent scripts switch as the settings pane does and records the call.
func (f *fixture) setScripts(t *testing.T, key string, on bool) {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIScripts: &on}}}
	if _, err := f.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, settingsKey, "args:SettingsService.Set#"+key, args)
}

// recordCard stores the waiting item and the answer that approves it under key.
func (f *fixture) recordCard(t *testing.T, scenario, key string, req pendingReq, approve bool) {
	t.Helper()
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(f.dbPath, "/data/mcp.db")}
	f.app.Contract(t, scenario, "ApprovalsService.List#"+key, []approvals.Item{req.Item}, opts...)
	f.app.Contract(t, scenario, "args:ApprovalsService.Answer#"+key, approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: approve})
}

func reviewOnOf(t *testing.T, it approvals.Item) bool {
	t.Helper()
	var pl struct {
		ReviewOffered bool `json:"reviewOffered"`
		ReviewResult  bool `json:"reviewResult"`
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil || !pl.ReviewOffered {
		t.Fatalf("payload %s (%v), want a card that offers review", it.Payload, err)
	}
	return pl.ReviewResult
}

func TestE2ESettingsDB(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	const insert = "insert into notes (body) values ('three')"

	t.Run("review turned on while the read card waits", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"read": "prompt", "review.read": "send"})
		pending := callToolAsync(sess, "run_query", withMode(f.connID, scriptStmt, "read"))
		req := f.pending(t)
		if reviewOnOf(t, req.Item) {
			t.Fatal("card shows review on, want off at card time")
		}
		f.recordCard(t, settingsKey, "step6", req, true)
		f.mcpSet(t, map[string]string{"review.read": "review"})
		f.answer(t, true, req.RequestID)
		testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
		held := f.pending(t)
		if held.Stage != "response" {
			t.Fatalf("pending = %+v, want the held result", held)
		}
		f.recordCard(t, settingsKey, "step6-held", held, true)
		f.answer(t, true, held.RequestID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), `"kind":"tabular"`) {
			t.Fatalf("sent = %q, want the rows", text(res))
		}
		f.mcpSet(t, map[string]string{"read": "allow", "review.read": "send"})
	})

	t.Run("scripts turned off while a scripted write card waits", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"write": "prompt"})
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(insert, "write", "return 1"))
		req := f.pending(t)
		if pl := decodeScriptPayload(t, req.Item); pl.Script == nil {
			t.Fatalf("payload = %s, want the script on the card", req.Item.Payload)
		}
		f.recordCard(t, settingsKey, "step7", req, true)
		f.setScripts(t, "scriptsOff", false)
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || !strings.Contains(text(res), "scripts are turned off") || !strings.Contains(text(res), "Nothing ran.") {
			t.Fatalf("approved after scripts off = %q, want a scripts-off refusal ending in Nothing ran.", text(res))
		}
		if got := f.notesBodies(t); len(got) != 2 {
			t.Fatalf("notes = %q, want the statement not to have run", got)
		}
		f.app.Contract(t, settingsKey, "result#step7", text(res))

		f.setScripts(t, "scriptsOn", true)
		again := callTool(t, sess, "run_query", f.queryScriptArgs(scriptStmt, "read", pickBodies))
		if again.IsError || text(again) != bodiesEnvelope {
			t.Fatalf("scripted read after switching back on = %q, want %s", text(again), bodiesEnvelope)
		}
	})

	t.Run("scripts turned off while a scripted review waits", func(t *testing.T) {
		f.mcpSet(t, map[string]string{"write": "deny", "review.read": "review"})
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", keepIDsScript))
		req := f.pending(t)
		if req.Stage != "response" {
			t.Fatalf("pending = %+v, want a held scripted result", req)
		}
		f.recordCard(t, settingsKey, "step8", req, true)
		f.setScripts(t, "scriptsOffReview", false)
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || !strings.Contains(text(res), "scripts were turned off while its result was in review, so it was withheld.") ||
			strings.Contains(text(res), "one") {
			t.Fatalf("approved after scripts off = %q, want the withheld text with no data", text(res))
		}
		if _, ok := f.keptValue(t, "ids"); ok {
			t.Fatal("a withheld scripted result kept a value")
		}
		f.app.Contract(t, settingsKey, "result#step8", text(res))
	})

	if n := f.queueLen(t); n != 0 {
		t.Fatalf("queue %d, want none", n)
	}
}
