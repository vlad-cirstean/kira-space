package dbmcpflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// P742: the agent's confirm arg raises an approval for an op the policy allows. UI half:
// tests/ui/studio/approvals-confirm.spec.ts, contract approvals-confirm.

const confirmNote = "This reads the customer notes; please confirm."

func confirmArgs(f *fixture, sqlText string) map[string]any {
	return map[string]any{"connectionId": f.connID, "sql": sqlText, "path": "", "mode": modeOf(sqlText), "confirm": "  " + confirmNote + " "}
}

func TestDbMcpConfirmAllowedQuery(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "allow"})
	const stmt = "select id, body from notes order by id"

	done := callToolAsync(sess, "run_query", confirmArgs(f, stmt))
	items := f.items(t, 1)
	if it := items[0]; it.Kind != approvals.KindDbQuery || it.Reason != "confirm" || it.AgentNote != confirmNote {
		t.Fatalf("item = %+v, want a db.query confirm item with the trimmed note", it)
	}
	f.app.Contract(t, "approvals-confirm", "ApprovalsService.List", items, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "approvals-confirm", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: true})
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, done); res.IsError || !strings.Contains(text(res), "two") {
		t.Fatalf("approved confirm call = %q, want the rows", text(res))
	}

	f.mcpSet(t, map[string]string{"write": "allow"})
	done = callToolAsync(sess, "run_query", confirmArgs(f, "insert into notes (body) values ('x')"))
	items = f.items(t, 1)
	f.answerItems(t, false, items[0].ID)
	if res := wait(t, done); !res.IsError || !strings.Contains(text(res), "was denied by the user") {
		t.Fatalf("denied confirm call = %q", text(res))
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("notes = %d, want the denied insert not run", n)
	}
}

func TestDbMcpConfirmNeverRelaxes(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "allow", "write": "deny"})

	res := call(t, sess, confirmArgs(f, "insert into notes (body) values ('x')"))
	if !res.IsError || !strings.Contains(text(res), "write") {
		t.Fatalf("confirm on a denied mode = %q, want the usual refusal", text(res))
	}
	assertNoItem(t, f)

	long := confirmArgs(f, "select 1")
	long["confirm"] = strings.Repeat("é", 301)
	if res := call(t, sess, long); !res.IsError || !strings.Contains(text(res), "300") || !strings.Contains(text(res), "Nothing ran.") {
		t.Fatalf("301-rune confirm = %q, want a refusal naming 300", text(res))
	}
	assertNoItem(t, f)

	blank := confirmArgs(f, "select 1")
	blank["confirm"] = "  \n "
	if res := call(t, sess, blank); res.IsError {
		t.Fatalf("whitespace confirm = %q, want the query to run", text(res))
	}
	assertNoItem(t, f)
}

func TestDbMcpConfirmOnPrompt(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "prompt"})

	done := callToolAsync(sess, "run_query", confirmArgs(f, "select id from notes"))
	items := f.items(t, 1)
	if it := items[0]; it.Reason != "permission" || it.AgentNote != confirmNote {
		t.Fatalf("item = %+v, want reason permission and the note", it)
	}
	f.app.Contract(t, "approvals-confirm", "item#permission", items[0], flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("approved call = %q", text(res))
	}
}

func TestDbMcpConfirmBrowseAndExplain(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"masked": "allow", "read": "allow"})
	f.maskPeople(t, "email")

	done := browseAsync(sess, f.connID, map[string]any{"limit": 2, "confirm": confirmNote})
	items := f.items(t, 1)
	if it := items[0]; it.Kind != approvals.KindDbQuery || it.Reason != "confirm" || it.AgentNote != confirmNote || payloadOf(t, it).Mode != "masked" {
		t.Fatalf("browse item = %+v, want a masked db.query confirm item", it)
	}
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("approved browse = %q", text(res))
	}

	args := confirmArgs(f, "select body from notes where id = 1")
	args["explain"] = true
	done = callToolAsync(sess, "run_query", args)
	items = f.items(t, 1)
	if it := items[0]; it.Reason != "confirm" || it.AgentNote != confirmNote || !strings.Contains(string(it.Payload), `"explain":true`) {
		t.Fatalf("explain item = %+v, want an explain confirm item", it)
	}
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("approved explain = %q", text(res))
	}
}

func TestDbMcpConfirmHeldResultClean(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "allow", "review.read": "review"})

	done := callToolAsync(sess, "run_query", confirmArgs(f, readStmt))
	pre := f.items(t, 1)[0]
	if pre.Kind != approvals.KindDbQuery || pre.Reason != "confirm" {
		t.Fatalf("pre-run item = %+v", pre)
	}
	f.answerItems(t, true, pre.ID)
	held := f.pending(t)
	if held.Stage != "response" {
		t.Fatalf("stage = %q, want the held result", held.Stage)
	}
	if held.Item.Reason == "confirm" || held.Item.AgentNote != "" {
		t.Fatalf("held item reason %q note %q, want neither the confirm reason nor the note", held.Item.Reason, held.Item.AgentNote)
	}
	f.answerItems(t, true, held.Item.ID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("sent result = %q", text(res))
	}
}

func TestDbMcpConfirmKubeScriptAPI(t *testing.T) {
	t.Run("kube", func(t *testing.T) {
		k := newKubeFixture(t)
		k.setMcp(t, "kind-dev", on("allow"))
		args := k.listPods("default", map[string]any{"confirm": confirmNote})
		pending := k.callAsync("kube.list", args)
		req := k.pending(t)
		if req.Item.Kind != approvals.KindKubeRead || req.Item.Reason != "confirm" || req.Item.AgentNote != confirmNote {
			t.Fatalf("item = %+v, want a kube.read confirm item", req.Item)
		}
		if k.podRequests() != 0 {
			t.Fatal("the list ran before approval")
		}
		k.answerItems(t, true, req.Item.ID)
		if out := <-pending; out.err != nil || out.res.IsError {
			t.Fatalf("approved list = %+v %v", out.res, out.err)
		}
		if k.podRequests() == 0 {
			t.Fatal("the approved list never reached the cluster")
		}
	})

	t.Run("script", func(t *testing.T) {
		f := newFixture(t)
		sess := f.session(t)
		f.createScript(t, scripts.CustomScriptFields{Name: "hello", Command: `echo confirmed`, MCP: allowMCP("Says hello.")}, nil)
		pending := callToolAsync(sess, "run_script", map[string]any{"scriptId": "hello", "waitSeconds": 10, "confirm": confirmNote})
		req := f.pending(t)
		if req.Item.Kind != approvals.KindScriptRun || req.Item.Reason != "confirm" || req.Item.AgentNote != confirmNote {
			t.Fatalf("item = %+v, want a script.run confirm item", req.Item)
		}
		f.answerItems(t, true, req.Item.ID)
		if res := wait(t, pending); res.IsError || !strings.Contains(text(res), "confirmed") {
			t.Fatalf("approved run = %q", text(res))
		}
		if res := callTool(t, sess, "run_script", map[string]any{"runId": "x", "confirm": confirmNote}); !res.IsError || !strings.Contains(text(res), "confirm applies") {
			t.Fatalf("confirm on a run read = %q, want a refusal", text(res))
		}
	})

	for _, name := range []string{"api-auto-read", "api-never"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			srv, sess, envID := f.apiSetup(t, "127.0.0.1", name == "api-auto-read")
			if name == "api-never" {
				f.patchAPI(t, "approval", &model.DbMcpPatch{APIApproval: ptr("never")})
			}
			args := apiArgs(envID, "GET", "{{base_url}}/echo")
			args["confirm"] = confirmNote
			pending := callToolAsync(sess, "api_request", args)
			req := f.pending(t)
			if req.Item.Kind != approvals.KindAPIRequest || req.Item.Reason != "confirm" || req.Item.AgentNote != confirmNote {
				t.Fatalf("item = %+v, want an api.request confirm item", req.Item)
			}
			if n := len(srv.Requests()); n != 0 {
				t.Fatalf("server saw %d requests before approval", n)
			}
			f.answerItems(t, true, req.Item.ID)
			if out := decodeResult[apiResult](t, wait(t, pending)); out.Status != 200 {
				t.Fatalf("approved request = %+v", out)
			}
		})
	}
}
