package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/memory"
)

const memoryServer = "kira-memory"

// TestMemoryMcpThroughClaude points the real claude at this test binary re-executed as the
// memory-mcp stdio server and has it call search_memories, then store_memory and memory_context.
// The second run starts in a git repository whose origin remote names the project key.
func TestMemoryMcpThroughClaude(t *testing.T) {
	app := newRealApp(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, cfg, string(mustRaw(map[string]any{"mcpServers": map[string]any{
		memoryServer: map[string]any{"command": exe, "args": []string{"memory-mcp"}, "env": map[string]string{"KIRA_MEMORY_HOME": app.MemoryHome}},
	}})))

	out, _ := app.claudeP(t, app.Work, nil,
		"Call the search_memories tool once with the query \"billing\", then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", "mcp__"+memoryServer+"__search_memories",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+memoryServer+"__search_memories")
	if len(uses) == 0 {
		t.Fatalf("no search_memories tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr {
		t.Fatalf("search_memories result missing or an error (ok=%v, text=%q)", ok, text)
	}
	if !strings.Contains(text, "memories") {
		t.Fatalf("search_memories result = %q, want the server's JSON", text)
	}
	if res := resultMsg(t, msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v", res)
	}
	storeThroughClaude(t, app, cfg)
	searchScoredThroughClaude(t, app, cfg)
	wantMemoryOps(t, app, "search_memories", "store_memory", "memory_context")
}

// wantMemoryOps waits for the app to list an ok memory-module op row per tool: the server wrote
// them into kira.db and the app ingested them.
func wantMemoryOps(t *testing.T, app *realApp, tools ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		rows, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		missing := slices.Clone(tools)
		for _, r := range rows {
			if r.Module == oplog.ModuleMemory && r.Status == oplog.StatusOK {
				missing = slices.DeleteFunc(missing, func(k string) bool { return k == r.Kind })
			}
		}
		if len(missing) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no ok memory op row for %v in %d rows", missing, len(rows))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// searchScoredThroughClaude has claude search from the billing repo: every meta line carries the
// score parts and claude's call is not rejected by the new filter arguments.
func searchScoredThroughClaude(t *testing.T, app *realApp, cfg string) {
	t.Helper()
	out, _ := app.claudeP(t, filepath.Join(app.Work, "billing"), nil,
		"Call the search_memories tool once with the query \"billing-api port\" and no other argument, then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", "mcp__"+memoryServer+"__search_memories",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+memoryServer+"__search_memories")
	if len(uses) == 0 {
		t.Fatalf("no search_memories tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr {
		t.Fatalf("search_memories result missing or an error (ok=%v, text=%q)", ok, text)
	}
	// Claude Code may hand the model the structured JSON instead of the text; check whichever it got.
	var body struct {
		Memories []struct {
			Score *struct{ Total float64 } `json:"score"`
		} `json:"memories"`
	}
	if json.Unmarshal([]byte(text), &body) == nil {
		if len(body.Memories) == 0 {
			t.Fatalf("search_memories result = %q, want the stored fact", text)
		}
		for _, m := range body.Memories {
			if m.Score == nil || m.Score.Total <= 0 {
				t.Fatalf("search_memories result = %q, want a score on every memory", text)
			}
		}
		return
	}
	metas := 0
	metaLine := regexp.MustCompile(`^  v\d+ origin=`)
	for _, l := range strings.Split(text, "\n") {
		if !metaLine.MatchString(l) {
			continue
		}
		metas++
		if !strings.Contains(l, " score=") {
			t.Fatalf("meta line %q lacks score=", l)
		}
	}
	if metas == 0 {
		t.Fatalf("search_memories result = %q, want the stored fact with a meta line", text)
	}
}

// sessionID is the --session-id of the store run; the server records it when claude exports it.
const sessionID = "5f1c0f55-7a62-4a5b-9b52-0d6a2f0c1e11"

func storeThroughClaude(t *testing.T, app *realApp, cfg string) {
	t.Helper()
	repo := filepath.Join(app.Work, "billing")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", "https://github.com/acme/billing.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	tools := "mcp__" + memoryServer + "__store_memory,mcp__" + memoryServer + "__memory_context"
	out, _ := app.claudeP(t, repo, nil,
		"Call store_memory once with exactly this argument: {\"items\":[{\"fact\":\"The billing-api service listens on port 8080 in production.\","+
			"\"reason\":\"Stated in the deployment manifest.\",\"origin\":\"agent\",\"scope\":\"project\"}]} and no project argument. "+
			"Then call memory_context with its id argument set to the fact id from the result. Reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", tools, "--session-id", sessionID,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	for _, tool := range []string{"store_memory", "memory_context"} {
		uses := toolUses(msgs, "mcp__"+memoryServer+"__"+tool)
		if len(uses) == 0 {
			t.Fatalf("no %s tool use in the stream:\n%s", tool, tail(out, 1500))
		}
		if text, isErr, ok := toolResult(msgs, uses[0].ID); !ok || isErr {
			t.Fatalf("%s result missing or an error (ok=%v, text=%q, input=%s)", tool, ok, text, uses[0].Input)
		}
	}
	recent, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent) != 1 {
		t.Fatalf("Recent = %+v, %v, want the stored fact", recent, err)
	}
	m := recent[0]
	if m.Origin != memory.OriginAgent || m.Scope != memory.ScopeProject || m.Project == nil || *m.Project != "github.com/acme/billing" {
		t.Fatalf("stored memory = %+v, want an agent fact in project github.com/acme/billing", m)
	}
	// Step 0 of P272 found CLAUDE_CODE_SESSION_ID exported to MCP servers and equal to --session-id.
	if m.SessionID == nil || *m.SessionID != sessionID {
		t.Fatalf("session = %v, want %s", m.SessionID, sessionID)
	}
	if c, err := app.W.Memory.Context(ctx, bridge.MemoryContextArgs{ID: m.LineageID}); err != nil || c.Memory.ID != m.ID {
		t.Fatalf("Context = %+v, %v, want the stored fact", c, err)
	}
}

// TestMemoryInstallRegisters runs the Connect dialog's install under the temp HOME and has the
// real claude list the server back.
func TestMemoryInstallRegisters(t *testing.T) {
	app := newRealApp(t)
	res := app.W.Memory.InstallAgent(ctx)
	if res.Outcome != "installed" {
		t.Fatalf("InstallAgent = %+v, want installed", res)
	}
	// Proves the install used the temp HOME. The real ~/.claude.json is not checked: other claude
	// processes rewrite it.
	raw, err := os.ReadFile(filepath.Join(app.Home, ".claude.json"))
	if err != nil || !strings.Contains(string(raw), memoryServer) {
		t.Fatalf("temp .claude.json lacks %s (err %v):\n%s", memoryServer, err, tail(string(raw), 600))
	}
	out, errOut, err := app.run(t, app.Work, nil, "mcp", "list")
	if err != nil {
		t.Fatalf("claude mcp list: %v\n%s", err, tail(out+errOut, 800))
	}
	if !strings.Contains(out, memoryServer) {
		t.Fatalf("claude mcp list lacks %s:\n%s", memoryServer, out)
	}
	t.Logf("claude mcp list:\n%s", out)
}

// TestMemoryDistill has the real claude call ingest_files with distill true on a file path through
// the memory-mcp server: the tool runs its own nested claude calls and returns short lines; nothing
// is stored.
func TestMemoryDistill(t *testing.T) {
	app := newRealApp(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, cfg, string(mustRaw(map[string]any{"mcpServers": map[string]any{
		memoryServer: map[string]any{"command": exe, "args": []string{"memory-mcp"}, "env": map[string]string{
			"KIRA_MEMORY_HOME": app.MemoryHome, "KIRA_SPACE_HOME": app.SpaceHome, "HOME": app.Home, "PATH": os.Getenv("PATH"),
		}},
	}})))
	doc := filepath.Join(app.Work, "deploy.md")
	writeFile(t, doc, "# Deploy\n\nThe billing-api service is deployed with Helm charts. Ops standardised on Helm in 2025, so there is no raw kubectl apply in production. Deploys never run on Friday.\n")

	out, _ := app.claudeP(t, app.Work, nil,
		"Call the ingest_files tool once with distill true and paths [\""+doc+"\"] and no other argument, then reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", "mcp__"+memoryServer+"__ingest_files",
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)
	uses := toolUses(msgs, "mcp__"+memoryServer+"__ingest_files")
	if len(uses) == 0 {
		t.Fatalf("no ingest_files tool use in the stream:\n%s", tail(out, 1500))
	}
	var in struct {
		Distill bool     `json:"distill"`
		Paths   []string `json:"paths"`
	}
	if err := json.Unmarshal(uses[0].Input, &in); err != nil || !in.Distill || len(in.Paths) != 1 || in.Paths[0] != doc {
		t.Fatalf("ingest_files input = %s (%v), want distill true and path %s", uses[0].Input, err, doc)
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr || strings.TrimSpace(text) == "" {
		t.Fatalf("distill result missing, empty or an error (ok=%v, isErr=%v, text=%q)", ok, isErr, text)
	}
	if got := toolUses(msgs, "mcp__"+memoryServer+"__store_memory"); len(got) != 0 {
		t.Errorf("claude stored memory during a distill call: %s", got[0].Input)
	}
	if recent, err := app.W.Memory.Recent(ctx); err != nil || len(recent) != 0 {
		t.Errorf("Recent = %+v, %v, want nothing stored", recent, err)
	}
	if jobs, err := app.W.MemoryImport.Jobs(); err != nil || len(jobs) != 0 {
		t.Errorf("Jobs = %+v, %v, want no import job", jobs, err)
	}
}

// TestMemoryDistillBridge calls the Memory module's bridge distill (the UI path) with the real
// claude on a text of two parts that state one claim each and one rule: the merge joins the repeated
// claim into one line, a rule comes back as a rule, and nothing is stored.
func TestMemoryDistillBridge(t *testing.T) {
	app := newRealApp(t)
	mark := app.Events.Mark()
	// Each part holds more than the chunk minimum fill and tail, so each becomes one chunk.
	filler := strings.Repeat("This paragraph holds no decision, name or number.\n\n", 90)
	text := "# Ports\n\n## Part one\n\nThe billing-api service listens on port 8080 in production.\n\n" + filler +
		"## Part two\n\nThe billing-api service listens on port 8080 in production. Never expose it directly to customer apps, because the gateway applies the rate limits.\n\n" + filler

	res, err := app.W.MemoryDistill.Distill(ctx, bridge.MemoryDistillArgs{RequestID: "claude-row", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) == 0 || res.Chunks < 2 || res.InChars != len([]rune(text)) || res.OutChars == 0 {
		t.Fatalf("result = %+v", res)
	}
	var withPort, rules int
	for _, l := range res.Lines {
		if strings.TrimSpace(l.Text) == "" {
			t.Errorf("empty line in %+v", res.Lines)
		}
		if strings.Contains(l.Text, "8080") {
			withPort++
		}
		if l.Kind == memory.KindRule {
			rules++
		}
	}
	if withPort != 1 {
		t.Errorf("%d lines mention port 8080, want the repeated claim merged into one: %+v", withPort, res.Lines)
	}
	if rules < 1 {
		t.Errorf("no rule line in %+v", res.Lines)
	}
	if res.CostUSD <= 0 || res.CostUSD > 1.00 {
		t.Errorf("cost = %v USD, want above 0 and at most 1.00", res.CostUSD)
	}
	t.Logf("distill bridge: %d lines (%d rules), %d parts, %d to %d chars, %.4f USD, lines %+v unresolved %+v", len(res.Lines), rules, res.Chunks, res.InChars, res.OutChars, res.CostUSD, res.Lines, res.Unresolved)
	if recent, err := app.W.Memory.Recent(ctx); err != nil || len(recent) != 0 {
		t.Errorf("Recent = %+v, %v, want nothing stored", recent, err)
	}
	if ev := app.Events.Since(mark, bridge.ChannelMemoryChanged); len(ev) != 0 {
		t.Errorf("distill emitted %d memory changed events, want none", len(ev))
	}
}

// TestMemoryDistillTrivia checks that distill drops run trivia from a phase result and keeps the
// decision and the incident lesson, and that an environment document keeps its limits.
func TestMemoryDistillTrivia(t *testing.T) {
	app := newRealApp(t)
	distill := func(id, text string) []string {
		res, err := app.W.MemoryDistill.Distill(ctx, bridge.MemoryDistillArgs{RequestID: id, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for _, l := range res.Lines {
			lines = append(lines, l.Text)
		}
		t.Logf("%s: %d lines, %.4f USD: %q", id, len(lines), res.CostUSD, lines)
		return lines
	}
	has := func(lines []string, sub string) bool {
		for _, l := range lines {
			if strings.Contains(l, sub) {
				return true
			}
		}
		return false
	}

	run := distill("trivia-run", "# P900 result\n\n## Outcome\n\n"+
		"The retry queue uses exponential backoff capped at 30 seconds, because the upstream rate limiter bans clients that retry faster. "+
		"The cap lives in config key retryQueue.maxDelaySeconds, not in code, so ops can change it without a release.\n\n"+
		"## Run notes\n\n"+
		"My first push of branch p900-retry was refused as non-fast-forward; I rebased and pushed to a new branch.\n"+
		"The retry-queue.spec.ts run was flaky once and passed on rerun.\n"+
		"The macOS dock check is left to the user.\n"+
		"Commit with an explicit pathspec in a shared checkout, because another session's staged files land in a bare commit.\n")
	noise := regexp.MustCompile(`(?i)non-fast-forward|flak|left (to|for) the user`)
	for _, l := range run {
		if noise.MatchString(l) {
			t.Errorf("run trivia kept: %q", l)
		}
	}
	if !has(run, "pathspec") {
		t.Errorf("incident lesson (pathspec) dropped: %q", run)
	}
	if !has(run, "retryQueue.maxDelaySeconds") {
		t.Errorf("decision (retryQueue.maxDelaySeconds) dropped: %q", run)
	}

	env := distill("trivia-env", "# Sandbox setup\n\nThe sandbox has no display, so run GUI checks through the Playwright ui project.\n"+
		"The sandbox allows outbound traffic only through the package proxy, so install dependencies with bun install.\n")
	if !has(env, "display") {
		t.Errorf("environment limit (display) dropped: %q", env)
	}
}

// TestMemoryDistillRewrite runs the short-text rewrite through the bridge: one call, text only,
// nothing stored. A changed result keeps the identifier and the number; an unchanged one is valid.
func TestMemoryDistillRewrite(t *testing.T) {
	app := newRealApp(t)
	text := "\t// Retry caps at 30 s (P900), because the upstream bans clients; see `retryQueue.maxDelaySeconds`."
	res, err := app.W.MemoryDistill.Distill(ctx, bridge.MemoryDistillArgs{RequestID: "rewrite", Text: text, Mode: "rewrite"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rewrite == nil || res.Mode != "rewrite" {
		t.Fatalf("result = %+v, want a rewrite", res)
	}
	if res.CostUSD <= 0 || res.CostUSD > 0.25 {
		t.Errorf("cost = %v USD, want above 0 and at most 0.25", res.CostUSD)
	}
	if res.Rewrite.Changed && (!strings.Contains(res.Rewrite.Text, "`retryQueue.maxDelaySeconds`") || !strings.Contains(res.Rewrite.Text, "30")) {
		t.Errorf("rewrite lost a fact: %q", res.Rewrite.Text)
	}
	t.Logf("rewrite: changed %v gate %q %.4f USD: %q dropped %+v", res.Rewrite.Changed, res.Rewrite.Gate, res.CostUSD, res.Rewrite.Text, res.Rewrite.Dropped)
	if recent, err := app.W.Memory.Recent(ctx); err != nil || len(recent) != 0 {
		t.Errorf("Recent = %+v, %v, want nothing stored", recent, err)
	}
}
