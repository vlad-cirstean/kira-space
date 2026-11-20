package adeflow_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const finishOverride = "Call finish_step when you are done."

func listPrompt(t *testing.T, f *runFixture, id llmprompts.ID) llmprompts.PromptView {
	t.Helper()
	l, err := f.app.W.LlmPromptsSvc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range l.Prompts {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("%s not listed", id)
	return llmprompts.PromptView{}
}

// An edited finish instruction reaches the next run's prompt with no restart.
func TestRunUsesEditedFinishInstruction(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", "")+manualStep)
	const id = llmprompts.ID("headless.finishStep.suffix")
	cur := listPrompt(t, f, id)
	if _, err := f.app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: id, Text: finishOverride, BaseHash: cur.Hash}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.start(t, "feat/api-work")
	if r := waitRun(t, f.app, f.taskID, "one", "done"); r.State != "done" {
		t.Fatalf("run = %+v", r)
	}
	got := strings.TrimRight(fakeFile(t, f.app, "api-1.prompt"), "\n")
	if !strings.Contains(got, "\n\n"+finishOverride) {
		t.Fatalf("prompt = %q, want it to carry the edited instruction", got)
	}
	if strings.Contains(got, "call finish_step with status") {
		t.Fatalf("prompt still carries the default instruction: %q", got)
	}
}

// A Space step prompt names the Details folder and ends with the proof ask; the README repeats it.
func TestRunPromptAsksForProofAndNamesDetails(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", "")+manualStep)
	f.start(t, "feat/api-work")
	if r := waitRun(t, f.app, f.taskID, "one", "done"); r.State != "done" {
		t.Fatalf("run = %+v", r)
	}
	const proof = "Write a proof for any checkable result before finishing."
	folder := dfFolder(t, f.app, f.taskID)
	got := strings.TrimRight(fakeFile(t, f.app, "api-1.prompt"), "\n")
	if want := "This task's Details files live in " + folder + "; see README.md there."; !strings.Contains(got, want) {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, proof) {
		t.Fatalf("prompt = %q, want it to end with %q", got, proof)
	}
	if readme := dfRead(t, folder, "README.md"); !strings.Contains("\n"+readme, "\n"+proof+"\n") {
		t.Fatalf("README = %q, want the proof line", readme)
	}
}

// The Run dialog renders its message, Kira Space note and finish suffix through Preview.
func TestRunDialogPreview(t *testing.T) {
	f := newRunFixture(t, nil, agentStep("one", "")+manualStep)
	finish := map[string]any{
		"implicit": true,
		"results":  []map[string]any{{"id": "done", "description": "", "ok": true}, {"id": "failed", "description": "", "ok": false}},
	}
	step := map[string]any{
		"title": "Usage alerts by email", "jiraKey": "PAY-130", "jiraURL": "https://acme.atlassian.net/browse/PAY-130",
		"repo": "{repo}", "branch": "{branch}", "worktree": "{worktree}",
		"stepNumber": 1, "stepCount": 6, "stepName": "Plan from spec",
		"stepPrompt": "Read the task spec and the Jira ticket PAY-130. Write an implementation plan per repo. Do not change code yet.",
		"message":    "", "extra": "", "context": []string{}, "finish": finish,
	}
	for _, c := range []struct {
		key  string
		id   llmprompts.ID
		vars map[string]any
	}{
		{"step", "ade.step.prompt", step},
		{"space", "headless.space.suffix", map[string]any{}},
		{"finish", "headless.finishStep.suffix", finish},
	} {
		raw, err := json.Marshal(c.vars)
		if err != nil {
			t.Fatal(err)
		}
		args := llmprompts.PreviewArgs{ID: c.id, Vars: raw}
		got, err := f.app.W.LlmPromptsSvc.Preview(args)
		if err != nil {
			t.Fatalf("Preview %s: %v", c.id, err)
		}
		if len(got.Errors) != 0 || len(got.Parts) == 0 {
			t.Fatalf("Preview %s = %+v", c.id, got)
		}
		f.app.Contract(t, "ade-run", "args:LlmPromptsService.Preview#"+c.key, args)
		f.app.Contract(t, "ade-run", "LlmPromptsService.Preview#"+c.key, got)
	}
}
