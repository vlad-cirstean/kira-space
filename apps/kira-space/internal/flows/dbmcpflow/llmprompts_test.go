package dbmcpflow

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// Edited instructions and a tool description reach the next MCP session with no restart.
func TestEditedPromptsReachNewSession(t *testing.T) {
	f := newFixture(t)
	before, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer before.Close()
	if before.InitializeResult().Instructions == "" {
		t.Fatal("default instructions are empty")
	}

	const instructions = "Use these tools only for the notes database."
	const runQuery = "Run one SQL statement."
	l, err := f.app.W.LlmPromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var changes []llmprompts.Change
	for _, p := range l.Prompts {
		switch p.ID {
		case llmprompts.DbmcpInstructions.ID():
			changes = append(changes, llmprompts.Change{ID: p.ID, Text: instructions, BaseHash: p.Hash})
		case llmprompts.DbmcpToolRunQuery.ID():
			changes = append(changes, llmprompts.Change{ID: p.ID, Text: runQuery, BaseHash: p.Hash})
		}
	}
	if len(changes) != 2 {
		t.Fatalf("found %d of the 2 dbmcp prompts", len(changes))
	}
	if _, err := f.app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: changes}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	if got := after.InitializeResult().Instructions; got != instructions {
		t.Fatalf("instructions = %q, want the edit", got)
	}
	tools, err := after.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "run_query" {
			if tool.Description != runQuery {
				t.Fatalf("run_query description = %q, want the edit", tool.Description)
			}
			return
		}
	}
	t.Fatal("run_query not listed")
}
