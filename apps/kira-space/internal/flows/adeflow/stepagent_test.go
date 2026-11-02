package adeflow_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// The per-step agent key: agent, review and manual-with-session steps take it, the writer keeps it,
// an unknown provider and a manual step without a session are refused. UI half: agent-provider.spec.ts.
func TestStepAgentKey(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("pinned", "    agent: opencode\n")+agentStep("plain", "")+manualStep+"    agent: opencode\n"))
	entry, _ := workflowEntry(t, app, "flow.yaml")
	steps := entry.Workflow.Steps
	if got := steps[stepIndex(t, *entry.Workflow, "pinned")].Agent; got != "opencode" {
		t.Fatalf("pinned agent = %q, want opencode", got)
	}
	if got := steps[stepIndex(t, *entry.Workflow, "plain")].Agent; got != "" {
		t.Fatalf("plain agent = %q, want none", got)
	}
	if got := steps[stepIndex(t, *entry.Workflow, "signoff")].Agent; got != "opencode" {
		t.Fatalf("manual session agent = %q, want opencode", got)
	}

	// A save through the editor writes the key, and clearing it removes the key.
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	wf.Steps[stepIndex(t, wf, "plain")].Agent = "claude"
	wf.Steps[stepIndex(t, wf, "pinned")].Agent = ""
	saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash})
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
	}
	y, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "saved yaml", y.Yaml, "agent: claude", "agent: opencode")
	if n := strings.Count(y.Yaml, "agent: opencode"); n != 1 {
		t.Fatalf("yaml holds %d opencode keys, want only the manual step's:\n%s", n, y.Yaml)
	}

	bad := func(name, yaml, want string) {
		t.Helper()
		e, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: name + ".yaml", Yaml: yaml})
		if err != nil {
			t.Fatal(err)
		}
		if e.Error == nil {
			t.Fatalf("%s saved, want it refused", name)
		}
		mustContain(t, name+" error", e.Error.Message, want)
	}
	bad("unknown", flowYAML("unknown", agentStep("one", "    agent: nope\n")), "agent")
	bad("nosession", flowYAML("nosession", "  - id: m\n    name: M\n    kind: manual\n    status: In review\n    agent: opencode\n"), "agent")
}
