package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/memory/memfmt"
)

// TestMemoryMcpFollowsDocs has the real claude use the kira-memory server from goal-only prompts,
// so the server instructions and tool docs alone must steer it: each stored item carries a reason,
// origin and scope, a docs folder goes by path with no read of its content, and a request to
// condense a file goes to ingest_files with distill true and the path, again with no read of the
// file. A code comment stays a
// plain comment that cites the stored fact by id, with no asComment call.
func TestMemoryMcpFollowsDocs(t *testing.T) {
	app := newRealApp(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(app.Work, "billing")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "remote", "add", "origin", "https://github.com/acme/billing.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, cfg, string(mustRaw(map[string]any{"mcpServers": map[string]any{
		memoryServer: map[string]any{"command": exe, "args": []string{"memory-mcp"}, "env": map[string]string{
			"KIRA_SPACE_HOME": app.SpaceHome, "HOME": app.Home, "PATH": os.Getenv("PATH"),
		}},
	}})))
	var memTools []string
	for _, n := range []string{"store_memory", "search_memories", "memory_history", "memory_context", "ingest_files", "ingest_status"} {
		memTools = append(memTools, "mcp__"+memoryServer+"__"+n)
	}

	t.Run("store", func(t *testing.T) {
		out, _ := app.claudeP(t, repo, nil,
			"Remember for the team: we deploy billing-api with Helm, because ops standardised on Helm charts in 2025. Use the kira-memory server.",
			"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", strings.Join(memTools, ","),
			"--output-format", "stream-json", "--verbose")
		uses := toolUses(parseStream(out), "mcp__"+memoryServer+"__store_memory")
		if len(uses) == 0 {
			t.Fatalf("no store_memory tool use in the stream:\n%s", tail(out, 1500))
		}
		var in struct {
			Items []struct {
				Reason string `json:"reason"`
				Origin string `json:"origin"`
				Scope  string `json:"scope"`
			} `json:"items"`
		}
		if err := json.Unmarshal(uses[0].Input, &in); err != nil || len(in.Items) == 0 {
			t.Fatalf("store_memory input = %s (%v), want at least one item", uses[0].Input, err)
		}
		for _, it := range in.Items {
			if strings.TrimSpace(it.Reason) == "" || it.Origin != "user" ||
				(it.Scope != "company" && it.Scope != "project" && it.Scope != "personal") {
				t.Errorf("item %+v, want a reason, origin user and a known scope", it)
			}
		}
	})

	t.Run("list", func(t *testing.T) {
		out, _ := app.claudeP(t, repo, nil,
			"Remember for the team: billing amounts are stored in cents, rounded half up, and capped at 12 digits. Use the kira-memory server.",
			"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", strings.Join(memTools, ","),
			"--output-format", "stream-json", "--verbose")
		uses := toolUses(parseStream(out), "mcp__"+memoryServer+"__store_memory")
		if len(uses) == 0 {
			t.Fatalf("no store_memory tool use in the stream:\n%s", tail(out, 1500))
		}
		var in struct {
			Items []struct {
				Fact string `json:"fact"`
			} `json:"items"`
		}
		if err := json.Unmarshal(uses[0].Input, &in); err != nil || len(in.Items) != 1 {
			t.Fatalf("store_memory input = %s (%v), want exactly one item", uses[0].Input, err)
		}
		fact := strings.ToLower(in.Items[0].Fact)
		for _, want := range []string{"cents", "half up", "12"} {
			if !strings.Contains(fact, want) {
				t.Errorf("fact %q lacks %q; the list about one subject must stay one fact", in.Items[0].Fact, want)
			}
		}
	})

	t.Run("files", func(t *testing.T) {
		docs := filepath.Join(repo, "docs")
		deploy := filepath.Join(docs, "deploy.md")
		writeFile(t, deploy, "# Deploy\n\nbilling-api ships through Helm charts; ops owns the chart repository.\n")
		tools := strings.Join(append(append([]string{}, memTools...), "Read", "Glob", "Grep"), ",")
		out, _ := app.claudeP(t, repo, nil,
			"Add the docs in "+docs+" to long-term memory.",
			"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", tools,
			"--output-format", "stream-json", "--verbose")
		msgs := parseStream(out)
		for _, m := range msgs {
			if m.Type != "assistant" {
				continue
			}
			for _, b := range blocks(m) {
				if b.Type == "tool_use" {
					t.Logf("tool use %s %s", b.Name, tail(string(b.Input), 200))
				}
			}
		}
		uses := toolUses(msgs, "mcp__"+memoryServer+"__ingest_files")
		if len(uses) == 0 {
			t.Fatalf("no ingest_files tool use in the stream:\n%s", tail(out, 1500))
		}
		var in struct {
			Paths   []string `json:"paths"`
			Project string   `json:"project"`
		}
		if err := json.Unmarshal(uses[0].Input, &in); err != nil {
			t.Fatal(err)
		}
		if in.Project != "" && in.Project != "github.com/acme/billing" {
			t.Errorf("ingest_files project = %q, want it omitted or github.com/acme/billing", in.Project)
		}
		var byPath bool
		for _, p := range in.Paths {
			byPath = byPath || p == docs || p == deploy
		}
		if !byPath {
			t.Errorf("ingest_files paths = %v, want %s or %s", in.Paths, docs, deploy)
		}
		for _, tool := range []string{"Read", "Grep"} {
			for _, u := range toolUses(msgs, tool) {
				if strings.Contains(string(u.Input), docs) {
					t.Errorf("claude used %s on the docs itself: %s", tool, u.Input)
				}
			}
		}
		// Cancel the job so the test spends nothing on extraction.
		jobs, err := app.W.MemoryImport.Jobs()
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range jobs {
			if err := app.W.MemoryImport.Cancel(bridge.MemoryImportJobArgs{ID: j.ID}); err != nil {
				t.Logf("cancel job %s: %v", j.ID, err)
			}
		}
	})

	t.Run("distill", func(t *testing.T) {
		notes := filepath.Join(repo, "docs", "runbook.md")
		writeFile(t, notes, "# Runbook\n\nbilling-api restarts take about 40 seconds. Restart workers first, never the database, because open invoices would lock.\n")
		tools := strings.Join(append(append([]string{}, memTools...), "Read", "Glob", "Grep"), ",")
		out, _ := app.claudeP(t, repo, nil,
			"Condense "+notes+" into short lines; store nothing.",
			"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", tools,
			"--output-format", "stream-json", "--verbose")
		msgs := parseStream(out)
		uses := toolUses(msgs, "mcp__"+memoryServer+"__ingest_files")
		if len(uses) == 0 {
			t.Fatalf("no ingest_files tool use in the stream:\n%s", tail(out, 1500))
		}
		var in struct {
			Distill bool     `json:"distill"`
			Paths   []string `json:"paths"`
			Text    string   `json:"text"`
		}
		if err := json.Unmarshal(uses[0].Input, &in); err != nil {
			t.Fatal(err)
		}
		if !in.Distill || len(in.Paths) != 1 || in.Paths[0] != notes || in.Text != "" {
			t.Errorf("ingest_files input = %s, want distill true, path %s and no text", uses[0].Input, notes)
		}
		if got := toolUses(msgs, "mcp__"+memoryServer+"__store_memory"); len(got) != 0 {
			t.Errorf("claude stored memory in a distill request: %s", got[0].Input)
		}
		for _, tool := range []string{"Read", "Grep"} {
			for _, u := range toolUses(msgs, tool) {
				if strings.Contains(string(u.Input), notes) {
					t.Errorf("claude used %s on the file itself: %s", tool, u.Input)
				}
			}
		}
	})

	t.Run("comment", func(t *testing.T) {
		file := filepath.Join(repo, "billing.go")
		writeFile(t, file, "package billing\n\nfunc Charge(amount int) {}\n")
		tools := strings.Join(append(append([]string{}, memTools...), "Read", "Edit"), ",")
		out, _ := app.claudeP(t, repo, nil,
			"Add a comment above func Charge in billing.go saying amounts are integer cents because the ledger rejects fractions, and remember that for the team with the kira-memory server.",
			"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", tools,
			"--output-format", "stream-json", "--verbose")
		uses := toolUses(parseStream(out), "mcp__"+memoryServer+"__store_memory")
		if len(uses) == 0 {
			t.Fatalf("no store_memory tool use in the stream:\n%s", tail(out, 1500))
		}
		for _, u := range uses {
			if strings.Contains(string(u.Input), "asComment") {
				t.Errorf("store_memory input has asComment: %s", u.Input)
			}
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		ids := regexp.MustCompile(memfmt.FactID).FindAllString(string(src), -1)
		if len(ids) == 0 {
			t.Fatalf("billing.go cites no fact id:\n%s", src)
		}
		stored, err := app.W.Memory.Recent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, m := range stored {
			found = found || m.LineageID == ids[0]
		}
		if !found {
			t.Errorf("billing.go cites %s, which no stored memory carries", ids[0])
		}
		if ps := memfmt.RetiredComments(src); len(ps) != 0 {
			t.Errorf("billing.go holds a retired kira-memory block: %v\n%s", ps, src)
		}
	})
}
