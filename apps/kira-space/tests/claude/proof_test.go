package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// proofMcpApp starts the real app with the kira-db MCP enabled and writes the claude MCP config.
func proofMcpApp(t *testing.T) (*realApp, string, string) {
	t.Helper()
	app := newRealApp(t)
	_, url, helper := enableDbMcp(t, app)
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
	return app, cfg, "mcp__" + dbServer + "__"
}

// TestProofMcpThroughClaude has the real claude write a validation proof through the kira-db proof
// tools and checks the stored session.
func TestProofMcpThroughClaude(t *testing.T) {
	app, cfg, pre := proofMcpApp(t)
	app.claudeP(t, t.TempDir(), nil,
		`Use the kira-db proof tools. Call proof_start_session with name "Claude proof". Then call proof_add_step twice with the returned draftId: `+
			`first a table step (description "Two rows", view "table", data {"columns":[{"name":"id"},{"name":"label"}],"rows":[[1,"a"],[2,"b"]]}), `+
			`then a document step (description "One doc", view "document", data {"documents":[{"body":{"ok":true}}]}). `+
			`Then call proof_close_session with the draftId. Reply with the word done.`,
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", pre+"proof_start_session,"+pre+"proof_add_step,"+pre+"proof_close_session",
		"--output-format", "stream-json", "--verbose")

	list, err := app.W.Proof.Sessions()
	if err != nil || len(list) != 1 || list[0].Name != "Claude proof" {
		t.Fatalf("Sessions = %+v, %v; want one named %q", list, err, "Claude proof")
	}
	det, err := app.W.Proof.Session(bridge.ProofSessionArgs{SessionID: list[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	steps := det.Current.Steps
	if len(steps) != 2 || steps[0].View != "table" || steps[0].Description != "Two rows" || steps[1].View != "document" || steps[1].Description != "One doc" {
		t.Fatalf("steps = %+v, want table then document in order", steps)
	}
}

// TestProofChartThroughClaude has the real claude add a bar chart step from the tool text alone
// and checks the stored payload.
func TestProofChartThroughClaude(t *testing.T) {
	app, cfg, pre := proofMcpApp(t)
	app.claudeP(t, t.TempDir(), nil,
		`Use the kira-db proof tools. Call proof_start_session with name "Chart proof". Then call proof_add_step once with the returned draftId: `+
			`a chart step with description "Errors by service" showing errors per service as a bar chart: api 3, web 5, worker 0. `+
			`Then call proof_close_session with the draftId. Reply with the word done.`,
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", pre+"proof_start_session,"+pre+"proof_add_step,"+pre+"proof_close_session",
		"--output-format", "stream-json", "--verbose")

	list, err := app.W.Proof.Sessions()
	if err != nil || len(list) != 1 || list[0].Name != "Chart proof" {
		t.Fatalf("Sessions = %+v, %v; want one named %q", list, err, "Chart proof")
	}
	det, err := app.W.Proof.Session(bridge.ProofSessionArgs{SessionID: list[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	steps := det.Current.Steps
	if len(steps) != 1 || steps[0].View != "chart" || steps[0].Description != "Errors by service" {
		t.Fatalf("steps = %+v, want one chart step %q", steps, "Errors by service")
	}
	step, err := app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ID, Rev: det.Current.Rev, Ordinal: steps[0].Ordinal})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type string `json:"type"`
		X    struct {
			Type string `json:"type"`
		} `json:"x"`
		Series []struct {
			Points [][]any `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(step.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "bar" || data.X.Type != "category" || len(data.Series) != 1 {
		t.Fatalf("data = %s, want one bar series on a category axis", step.Data)
	}
	want := [][]any{{"api", 3.0}, {"web", 5.0}, {"worker", 0.0}}
	got := data.Series[0].Points
	if len(got) != len(want) {
		t.Fatalf("points = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != 2 || got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Fatalf("points = %v, want %v", got, want)
		}
	}
}
