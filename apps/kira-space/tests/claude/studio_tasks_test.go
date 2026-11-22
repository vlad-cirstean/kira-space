package claude

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// studioRun is one task through the real claude: how many Studio MCP calls and ToolSearch calls it took.
type studioRun struct {
	msgs         []streamMsg
	out          string
	mcpCalls     int
	toolSearches int
	cost         float64
}

// studioMCPConfig writes the mcp.json the app would register for the Studio server.
func studioMCPConfig(t *testing.T, url, helper string) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// runStudioTask gives the model the task only (no tool names), counts its calls and logs them.
func runStudioTask(t *testing.T, app *realApp, cfg, prompt string) studioRun {
	t.Helper()
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Use the kira-studio MCP tools, loading them with ToolSearch if they are deferred. "+prompt,
		"--strict-mcp-config", "--mcp-config", cfg, "--max-budget-usd", "0.2",
		"--allowedTools", "ToolSearch,mcp__"+dbServer,
		"--output-format", "stream-json", "--verbose")
	r := studioRun{out: out, msgs: parseStream(out)}
	for _, m := range r.msgs {
		switch m.Type {
		case "assistant":
			for _, b := range blocks(m) {
				switch {
				case b.Type == "tool_use" && b.Name == "ToolSearch":
					r.toolSearches++
				case b.Type == "tool_use" && strings.HasPrefix(b.Name, "mcp__"+dbServer+"__"):
					r.mcpCalls++
				}
			}
		case "result":
			r.cost = m.TotalCostUSD
		}
	}
	t.Logf("STUDIO-TASK %s: mcp calls %d, ToolSearch calls %d, cost %.4f USD", t.Name(), r.mcpCalls, r.toolSearches, r.cost)
	if res := resultMsg(t, r.msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v\n%s", res, tail(out, 1500))
	}
	return r
}

func (r studioRun) answer(t *testing.T) string { return resultMsg(t, r.msgs).Result }

// answerApprovals answers every pending approval of the given kinds (approve or withhold) until stop.
func answerApprovals(app *realApp, approve bool, kinds ...string) (stop func() int) {
	done := make(chan struct{})
	finished := make(chan struct{})
	var answered atomic.Int32
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			case <-time.After(200 * time.Millisecond):
			}
			items, err := app.W.ApprovalsSvc.List()
			if err != nil {
				continue
			}
			for _, it := range items {
				match := len(kinds) == 0
				for _, k := range kinds {
					match = match || it.Kind == k
				}
				if !match {
					continue
				}
				if _, err := app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{it.ID}, Approve: approve}); err == nil {
					answered.Add(1)
				}
			}
		}
	}()
	return func() int {
		close(done)
		<-finished
		return int(answered.Load())
	}
}

func seedNotes(t *testing.T, dbFile string, stmts string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(stmts); err != nil {
		t.Fatal(err)
	}
}

func exposeKubeDev(t *testing.T, app *realApp, fake *flowharness.KubeFake, review string) {
	t.Helper()
	cfgPath := filepath.Join(app.Home, "kubeconfig")
	fake.WriteKubeconfig(t, cfgPath, flowharness.KubeconfigOpts{BinDir: app.BinDir})
	t.Setenv("KUBECONFIG", cfgPath)
	res, err := app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatalf("Kubernetes.Clusters: %v", err)
	}
	for _, c := range res.Clusters {
		if c.Context != "kind-dev" {
			continue
		}
		if _, err := app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: c.ID, Mcp: kubernetes.McpSettings{
			Description: "real-claude kube fixture", Enabled: true, Approval: "allow", Review: review,
		}}); err != nil {
			t.Fatalf("Kubernetes.SetMcp: %v", err)
		}
		return
	}
	t.Fatalf("no kind-dev cluster in %+v", res.Clusters)
}

// TestStudioMcpTasksThroughClaude (P733) runs five representative tasks through the real claude and logs how
// many Studio MCP calls and ToolSearch calls each took. The prompts name no tool: the descriptions and the
// instructions must carry the model. The numbers are the before/after of the P733 tool list.
func TestStudioMcpTasksThroughClaude(t *testing.T) {
	t.Run("sql explore then query", func(t *testing.T) {
		app := newRealApp(t)
		connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
			seedNotes(t, dbFile, `CREATE TABLE notes (id INTEGER PRIMARY KEY, author TEXT, body TEXT);
				INSERT INTO notes (author, body) VALUES ('alice','a1'),('alice','a2'),('bob','b1')`)
		})
		r := runStudioTask(t, app, studioMCPConfig(t, url, helper),
			"The database connection named "+connName+" has a notes table. With one grouped aggregate query, tell me how many notes each author wrote.")
		if a := strings.ToLower(r.answer(t)); !strings.Contains(a, "alice") || !strings.Contains(a, "2") || !strings.Contains(a, "bob") {
			t.Fatalf("answer = %q, want alice 2 and bob 1", a)
		}
	})

	t.Run("api request with an environment variable", func(t *testing.T) {
		app := newRealApp(t)
		srv := flowharness.HTTP(t)
		app.WriteAPIFile(t, "dev.environment.yaml", []byte("name: Dev\ncolor: green\nagentAutoApproveReads: true\nvariables:\n"+
			"  - name: base_url\n    value: "+srv.URL+"\n    agent: true\n"+
			"  - name: token\n    value: tok-real-claude-1\n    agent: true\n"))
		appr := "always"
		on := true
		if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIRequests: &on, APIApproval: &appr}}}); err != nil {
			t.Fatal(err)
		}
		_, url, helper := enableDbMcp(t, app)
		stop := answerApprovals(app, true)
		r := runStudioTask(t, app, studioMCPConfig(t, url, helper),
			"Send a GET to the /bearer path of the Dev HTTP environment's base URL, with the environment's token as a bearer Authorization header. Reply with the HTTP status code.")
		stop()
		var seen bool
		for _, q := range srv.Requests() {
			if q.Path == "/bearer" && strings.Contains(q.Header.Get("Authorization"), "tok-real-claude-1") {
				seen = true
			}
		}
		if !seen || !strings.Contains(r.answer(t), "200") {
			t.Fatalf("bearer request seen = %v, answer = %q, want the token sent and status 200", seen, r.answer(t))
		}
	})

	t.Run("kube list then logs", func(t *testing.T) {
		const crashPod, crashLine = "checkout-7d9f6", "panic: cannot reach payments-db:5432"
		app := newRealApp(t)
		fake := flowharness.NewKubeFake(t, flowharness.WithCrashLoopPod("shop", crashPod, []string{"starting checkout", crashLine}))
		exposeKubeDev(t, app, fake, "inherit")
		_, url, helper := enableDbMcp(t, app)
		r := runStudioTask(t, app, studioMCPConfig(t, url, helper),
			"In the Kubernetes cluster whose context is kind-dev, read the logs of the crash-looping pod in namespace shop. Reply with the last log line.")
		if !strings.Contains(r.answer(t), "payments-db") {
			t.Fatalf("answer = %q, want the last log line", r.answer(t))
		}
	})

	t.Run("hold for review", func(t *testing.T) {
		app := newRealApp(t)
		connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
			seedNotes(t, dbFile, `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT); INSERT INTO notes (body) VALUES ('secret-body-1'),('secret-body-2')`)
			f.McpReview = model.McpReview{Read: "review", Masked: "review"}
		})
		stop := answerApprovals(app, false, approvals.KindDbResponse)
		r := runStudioTask(t, app, studioMCPConfig(t, url, helper),
			"Tell me the body of the first note in the notes table of the database connection named "+connName+", or say the result was withheld if you did not get it.")
		if n := stop(); n == 0 {
			t.Fatalf("no result was held and withheld:\n%s", tail(r.out, 1500))
		}
		if a := strings.ToLower(r.answer(t)); !strings.Contains(a, "withh") || strings.Contains(a, "secret-body") {
			t.Fatalf("answer = %q, want the result reported withheld without data", a)
		}
	})

	t.Run("saved variables chain", func(t *testing.T) {
		app := newRealApp(t)
		connName, url, helper := enableDbMcp(t, app, func(f *model.ConnectionFields, dbFile string) {
			seedNotes(t, dbFile, `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT); INSERT INTO notes (body) VALUES ('one'),('two')`)
		})
		r := runStudioTask(t, app, studioMCPConfig(t, url, helper),
			"The connection named "+connName+" has a notes table (id, body). Read every note's id in one read query, then in a second read query fetch the bodies of those ids, ordered by id."+
				" Do not put the ids in your own text or your own sql: they must never pass through you. Reply with the bodies.")
		if a := r.answer(t); !strings.Contains(a, "one") || !strings.Contains(a, "two") {
			t.Fatalf("answer = %q, want both bodies", a)
		}
	})
}
