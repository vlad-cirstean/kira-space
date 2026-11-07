package adeflow_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// P302 editor fixes. Contract ade-workflow-editor: tests/ui/ade-v2-workflows.spec.ts reads these.

const (
	weScenario     = "ade-workflow-editor"
	neverRunsError = "cannot return to a manual or skipped step"
	implementerErr = "implementer must be an earlier agent step"
)

const weReviewFlow = "  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n" +
	"    review:\n      send_to: implementer\n      implementer: impl\n" +
	"    results:\n      - id: approved\n        ok: true\n      - id: rejected\n        ok: false\n        next: stop\n"

func workflowEntry(t *testing.T, app *flowharness.App, file string) (adewire.WorkflowEntry, adewire.WorkflowsResult) {
	t.Helper()
	list, err := app.W.AdeTask.Workflows(ctx)
	if err != nil {
		t.Fatalf("Workflows: %v", err)
	}
	for _, e := range list.Workflows {
		if e.FileName == file {
			if e.Workflow == nil {
				t.Fatalf("workflow %s invalid: %+v", file, e.Error)
			}
			return e, list
		}
	}
	t.Fatalf("no workflow %s in %+v", file, list)
	return adewire.WorkflowEntry{}, list
}

func badRequest(t *testing.T, err error) string {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeBadRequest) {
		t.Fatalf("err = %v, want a bad-request ipc error", err)
	}
	return ie.Message
}

func stepIndex(t *testing.T, wf adewire.Workflow, id string) int {
	t.Helper()
	for i, s := range wf.Steps {
		if s.ID == id {
			return i
		}
	}
	t.Fatalf("workflow has no step %s", id)
	return -1
}

func TestWorkflowReturnToStepThatNeverRuns(t *testing.T) {
	t.Run("yaml return to a manual step", func(t *testing.T) {
		app := flowharness.New(t)
		y := flowYAML("flow",
			"  - id: m\n    name: m\n    kind: manual\n    status: in-review\n"+
				agentStep("s", "    results:\n      - id: done\n        ok: true\n      - id: failed\n        ok: false\n        next: m\n"))
		line := 0
		for i, l := range strings.Split(y, "\n") {
			if strings.Contains(l, "next: m") {
				line = i + 1
			}
		}
		got, err := app.W.AdeTask.ValidateWorkflowYaml(ctx, adewire.ValidateWorkflowYamlArgs{Yaml: y})
		if err != nil {
			t.Fatalf("ValidateWorkflowYaml: %v", err)
		}
		if got.Workflow != nil || got.Error == nil || got.Error.Line != line || !strings.Contains(got.Error.Message, neverRunsError) {
			t.Fatalf("validation = %+v / %+v, want line %d: %s", got.Workflow, got.Error, line, neverRunsError)
		}
		app.Contract(t, weScenario, "AdeTaskService.ValidateWorkflowYaml#return-to-manual", got)
	})

	t.Run("graph save return to a skipped step", func(t *testing.T) {
		app := flowharness.New(t)
		saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("plan", "")+agentStep("impl", "    skip: true\n")+agentStep("tests", "")))
		entry, _ := workflowEntry(t, app, "flow.yaml")
		wf := *entry.Workflow
		tests := &wf.Steps[stepIndex(t, wf, "tests")]
		for i := range tests.Results {
			if tests.Results[i].ID == "failed" {
				tests.Results[i].Next, tests.Results[i].Loop = "impl", true
			}
		}
		before, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
		if err != nil {
			t.Fatal(err)
		}
		_, refused := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash})
		if msg := badRequest(t, refused); !strings.Contains(msg, neverRunsError) {
			t.Fatalf("message = %q, want %q", msg, neverRunsError)
		}
		after, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
		if err != nil || after.Hash != before.Hash {
			t.Fatalf("file changed by a refused save: %+v, %v", after, err)
		}
		app.Contract(t, weScenario, "AdeTaskService.SaveWorkflow#return-to-skipped", refused.Error())
	})
}

// manualOf mirrors the editor's withKind(step, 'manual'): id, name, skip and status stay, the rest resets.
func manualOf(s adewire.WorkflowStep) adewire.WorkflowStep {
	return adewire.WorkflowStep{
		ID: s.ID, Name: s.Name, Kind: adewire.KindManual, Status: s.Status, Skip: s.Skip,
		Results: []adewire.StepResult{}, AllowedTools: []string{}, Params: map[string][]string{},
		Sandbox: adewire.DefaultSandboxFor(adewire.KindManual),
	}
}

func TestWorkflowEditorClearsStaleImplementer(t *testing.T) {
	base := flowYAML("editor", agentStep("plan", "")+agentStep("impl", "")+weReviewFlow+agentStep("verify", ""))
	edits := []struct {
		name  string
		apply func(t *testing.T, wf *adewire.Workflow)
	}{
		{"remove impl", func(t *testing.T, wf *adewire.Workflow) {
			i := stepIndex(t, *wf, "impl")
			wf.Steps = append(wf.Steps[:i], wf.Steps[i+1:]...)
		}},
		{"move review above impl", func(t *testing.T, wf *adewire.Workflow) {
			i := stepIndex(t, *wf, "review")
			wf.Steps[i-1], wf.Steps[i] = wf.Steps[i], wf.Steps[i-1]
		}},
		{"impl kind manual", func(t *testing.T, wf *adewire.Workflow) {
			i := stepIndex(t, *wf, "impl")
			wf.Steps[i] = manualOf(wf.Steps[i])
		}},
		{"impl smart script", func(t *testing.T, wf *adewire.Workflow) {
			i := stepIndex(t, *wf, "impl")
			wf.Steps[i].SmartScript, wf.Steps[i].Prompt, wf.Steps[i].AllowedTools = "Review diff", "", nil
		}},
	}
	for _, e := range edits {
		t.Run(e.name, func(t *testing.T) {
			app := flowharness.New(t)
			saveWorkflow(t, app, "editor", base)
			entry, list := workflowEntry(t, app, "editor.yaml")
			if got := entry.Workflow.Steps[stepIndex(t, *entry.Workflow, "review")].Review; got == nil || got.Implementer != "impl" {
				t.Fatalf("review block = %+v, want implementer impl", got)
			}
			if e.name == "remove impl" {
				app.Contract(t, weScenario, "AdeTaskService.Workflows", list)
			}

			stale := *entry.Workflow
			stale.Steps = append([]adewire.WorkflowStep(nil), stale.Steps...)
			e.apply(t, &stale)
			_, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "editor.yaml", Workflow: stale, BaseHash: entry.Hash})
			if msg := badRequest(t, err); !strings.Contains(msg, implementerErr) {
				t.Fatalf("stale implementer message = %q, want %q", msg, implementerErr)
			}

			fixed := stale
			fixed.Steps = append([]adewire.WorkflowStep(nil), stale.Steps...)
			ri := stepIndex(t, fixed, "review")
			fixed.Steps[ri].Review = &adewire.ReviewStepConfig{SendTo: adewire.ReviewSendImplementer, Implementer: ""}
			saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "editor.yaml", Workflow: fixed, BaseHash: entry.Hash})
			if err != nil || saved.Error != nil {
				t.Fatalf("SaveWorkflow cleared = %+v, %v", saved, err)
			}
			again, _ := workflowEntry(t, app, "editor.yaml")
			if got := again.Workflow.Steps[stepIndex(t, *again.Workflow, "review")].Review; got == nil || got.Implementer != "" {
				t.Fatalf("saved review block = %+v, want implementer cleared", got)
			}
			if e.name == "remove impl" {
				app.Contract(t, weScenario, "AdeTaskService.SaveWorkflow#cleared", saved)
			}
		})
	}
}

// P510 script sandbox and outside-repo steps survive a save and reload. Contract ade-workflow-editor.
func TestWorkflowEditorOutsideRepo(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("plan", "")))
	entry, _ := workflowEntry(t, app, "flow.yaml")
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	script := adewire.WorkflowStep{
		ID: "build", Name: "build", Kind: adewire.KindScript, Command: "make", OutsideRepo: true, Before: "auto", Timeout: "10m",
		Results: adewire.DefaultResults(), MaxReturns: adewire.DefaultMaxReturns, AllowedTools: []string{}, Params: map[string][]string{},
		Sandbox: adewire.StepSandbox{Access: adewire.SandboxRead, Read: []string{}, Allow: []string{}, Network: false},
	}
	session := adewire.WorkflowStep{
		ID: "work", Name: "work", Kind: adewire.KindManual, Session: true, OutsideRepo: true, Results: []adewire.StepResult{},
		AllowedTools: []string{}, Params: map[string][]string{}, Sandbox: adewire.DefaultSandboxFor(adewire.KindManual),
	}
	wf.Steps = append(wf.Steps, script, session)
	saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash})
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
	}
	text, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range strings.Split(text.Yaml, "  - id: ")[2:] {
		mustContain(t, "step yaml", step, "outside_repo: true")
		mustLack(t, "step yaml", step, "runs_on")
	}
	mustContain(t, "script yaml", text.Yaml, "sandbox:", "access: read", "network: false")

	again, list := workflowEntry(t, app, "flow.yaml")
	got := again.Workflow.Steps[stepIndex(t, *again.Workflow, "build")]
	if !got.OutsideRepo || got.RunsOn != "" || got.Sandbox.Access != adewire.SandboxRead || got.Sandbox.Network {
		t.Fatalf("reloaded script step = %+v, want outside repo, no runs_on, read sandbox without network", got)
	}
	if w := again.Workflow.Steps[stepIndex(t, *again.Workflow, "work")]; !w.OutsideRepo || !w.Session {
		t.Fatalf("reloaded manual step = %+v, want a session outside the repos", w)
	}
	app.Contract(t, weScenario, "AdeTaskService.Workflows#outside", list)

	bad := *again.Workflow
	bad.Steps = append([]adewire.WorkflowStep(nil), bad.Steps...)
	bad.Steps[stepIndex(t, bad, "build")].Command = "git switch {branch}"
	_, refused := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: bad, BaseHash: again.Hash})
	msg := badRequest(t, refused)
	mustContain(t, "message", msg, "{branch}", "outside_repo")
	app.Contract(t, weScenario, "AdeTaskService.SaveWorkflow#outside-vars", refused.Error())
}

// P511 a manual session step keeps its sandbox through a save and reload; off is the default and
// writes no block; a step without a session refuses one. Contract ade-workflow-editor.
func TestWorkflowEditorManualSandbox(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("plan", "")))
	entry, _ := workflowEntry(t, app, "flow.yaml")
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	step := func(id string, sb adewire.StepSandbox) adewire.WorkflowStep {
		s := manualOf(adewire.WorkflowStep{ID: id, Name: id})
		s.Session, s.Sandbox = true, sb
		return s
	}
	read := adewire.StepSandbox{Access: adewire.SandboxRead, Read: []string{}, Allow: []string{}, Network: false}
	wf.Steps = append(wf.Steps, step("look", read), step("free", adewire.DefaultSandboxFor(adewire.KindManual)))
	saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: entry.Hash})
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
	}
	text, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(text.Yaml, "  - id: ")
	mustContain(t, "look yaml", parts[2], "sandbox:", "access: read", "network: false")
	mustLack(t, "free yaml", parts[3], "sandbox")

	again, list := workflowEntry(t, app, "flow.yaml")
	look := again.Workflow.Steps[stepIndex(t, *again.Workflow, "look")]
	if look.Sandbox.Access != adewire.SandboxRead || look.Sandbox.Network || !look.Session {
		t.Fatalf("reloaded look step = %+v, want a read session sandbox without network", look)
	}
	free := again.Workflow.Steps[stepIndex(t, *again.Workflow, "free")]
	if free.Sandbox.Access != adewire.SandboxOff {
		t.Fatalf("reloaded free step sandbox = %+v, want off", free.Sandbox)
	}
	app.Contract(t, weScenario, "AdeTaskService.Workflows#manual-sandbox", list)

	const noSession = "  - id: sign\n    name: sign\n    kind: manual\n    sandbox:\n      access: read\n"
	refused, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "bad.yaml", Yaml: flowYAML("bad", noSession)})
	if err != nil || refused.Error == nil {
		t.Fatalf("SaveWorkflowYaml = %+v, %v, want a validation error", refused, err)
	}
	mustContain(t, "message", refused.Error.Message, "sandbox is not allowed on manual steps without a session")
}
