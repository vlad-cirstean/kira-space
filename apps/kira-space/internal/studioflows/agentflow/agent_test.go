package agentflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestInfoNamesTheProvider(t *testing.T) {
	app := studioflowharness.New(t)
	info, err := app.W.AgentSvc.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "claude" || info.ProductLabel != "Claude Code" || info.HeadlessLabel != "claude -p" || info.DefaultModel != "sonnet" {
		t.Fatalf("Info = %+v, want the claude provider", info)
	}
	if len(info.Models) != 3 || info.Models[0].Label != "Haiku" {
		t.Fatalf("models = %+v, want haiku, sonnet, opus", info.Models)
	}
	app.Contract(t, "agent", "AgentService.Info", info)
	for _, id := range []string{"", "claude"} {
		got, err := app.W.AgentSvc.InfoFor(id)
		if err != nil || got.ID != "claude" {
			t.Fatalf("InfoFor(%q) = %+v, %v, want claude", id, got, err)
		}
	}
	if _, err := app.W.AgentSvc.InfoFor("gone"); err == nil {
		t.Fatal("InfoFor(gone) succeeded, want provider gone")
	}
}
