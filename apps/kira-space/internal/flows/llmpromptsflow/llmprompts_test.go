package llmpromptsflow_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const (
	scenario = "llm-prompts"
	editID   = llmprompts.ID("ade.sendBack.line")
	editText = "{{.From}} failed on {{.Branch}}: {{.Reason}}. Redo it."
	statusID = llmprompts.ID("headless.tool.finish_step.arg.status")
	wait     = 5 * time.Second
)

func find(t *testing.T, l llmprompts.PromptList, id llmprompts.ID) llmprompts.PromptView {
	t.Helper()
	for _, p := range l.Prompts {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("%s not listed", id)
	return llmprompts.PromptView{}
}

func list(t *testing.T, app *flowharness.App) llmprompts.PromptList {
	t.Helper()
	l, err := app.W.LlmPromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func failure(t *testing.T, err error) map[string]any {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *ipcerr.Error", err)
	}
	out := map[string]any{"code": ie.Code, "message": ie.Message}
	if len(ie.Details) > 0 {
		var d any
		if err := json.Unmarshal(ie.Details, &d); err != nil {
			t.Fatal(err)
		}
		out["details"] = d
	}
	return out
}

// List returns every entry, all at their defaults on a fresh home.
func TestList(t *testing.T) {
	app := flowharness.New(t)
	l := list(t, app)
	if len(l.Prompts) != 270 {
		t.Fatalf("listed %d prompts, want 270", len(l.Prompts))
	}
	for _, p := range l.Prompts {
		if p.Overridden || p.Text != p.DefaultText || p.Hash != llmprompts.Hash(p.Text) {
			t.Fatalf("%s not at default: %+v", p.ID, p)
		}
	}
	app.Contract(t, scenario, "LlmPromptsService.List#entry", find(t, l, editID), flowharness.Omit("defaultText", "text"))
}

// Preview renders a draft without saving it and reports a broken one without failing.
func TestPreview(t *testing.T) {
	app := flowharness.New(t)
	draft := editText
	got, err := app.W.LlmPromptsSvc.Preview(llmprompts.PreviewArgs{ID: editID, Text: &draft})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Errors) != 0 || len(got.Parts) == 0 {
		t.Fatalf("preview = %+v", got)
	}
	app.Contract(t, scenario, "LlmPromptsService.Preview#parts", got)

	bad := "{{.Nope}}"
	got, err = app.W.LlmPromptsSvc.Preview(llmprompts.PreviewArgs{ID: statusID, Text: &bad})
	if err != nil || len(got.Errors) == 0 {
		t.Fatalf("preview of a bad draft = %+v, %v", got, err)
	}
	app.Contract(t, scenario, "LlmPromptsService.Preview#errors", got)
	if list(t, app).Revision != 0 {
		t.Fatal("preview changed the store")
	}
	_, err = app.W.LlmPromptsSvc.Preview(llmprompts.PreviewArgs{ID: "nope"})
	if code := failure(t, err)["code"]; code != string(ipcerr.CodeNotFound) {
		t.Fatalf("unknown id code = %v", code)
	}
}

// Save overrides a prompt, pushes the revision, and saving the default text resets it.
func TestSaveAndReset(t *testing.T) {
	app := flowharness.New(t)
	cur := find(t, list(t, app), editID)
	mark := app.Events.Mark()
	saved, err := app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: editID, Text: editText, BaseHash: cur.Hash}}})
	if err != nil {
		t.Fatal(err)
	}
	got := find(t, saved, editID)
	if !got.Overridden || got.Text != editText || got.DefaultText != cur.DefaultText || saved.Revision != 1 {
		t.Fatalf("saved = %+v, revision %d", got, saved.Revision)
	}
	if find(t, list(t, app), editID).Text != editText {
		t.Fatal("List does not return the override")
	}
	ev := app.Events.WaitAfter(t, mark, bridge.ChannelLlmPromptsChanged, nil, wait)
	app.Contract(t, scenario, "event:"+ev.Channel, ev.Data)
	app.Contract(t, scenario, "LlmPromptsService.Save#entry", got, flowharness.Mask("updatedAt"), flowharness.Omit("defaultText"))

	reset, err := app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: editID, Text: cur.DefaultText, BaseHash: got.Hash}}})
	if err != nil {
		t.Fatal(err)
	}
	if p := find(t, reset, editID); p.Overridden || p.Text != cur.DefaultText || reset.Revision != 2 {
		t.Fatalf("reset = %+v, revision %d", p, reset.Revision)
	}
}

// An invalid change is refused with its findings and changes nothing.
func TestSaveInvalid(t *testing.T) {
	app := flowharness.New(t)
	cur := find(t, list(t, app), statusID)
	mark := app.Events.Mark()
	_, err := app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: statusID, Text: "{{range .Results}}{{.Nope}}{{end}}", BaseHash: cur.Hash}}})
	app.Contract(t, scenario, "LlmPromptsService.Save#invalid", failure(t, err))
	if l := list(t, app); l.Revision != 0 || find(t, l, statusID).Overridden {
		t.Fatalf("invalid save changed the store: revision %d", l.Revision)
	}
	if n := len(app.Events.Since(mark, bridge.ChannelLlmPromptsChanged)); n != 0 {
		t.Fatalf("%d change pushes after a refused save", n)
	}
}

// A change made from a stale text is refused.
func TestSaveConflict(t *testing.T) {
	app := flowharness.New(t)
	cur := find(t, list(t, app), editID)
	if _, err := app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: editID, Text: editText, BaseHash: cur.Hash}}}); err != nil {
		t.Fatal(err)
	}
	_, err := app.W.LlmPromptsSvc.Save(llmprompts.SaveArgs{Changes: []llmprompts.Change{{ID: editID, Text: cur.DefaultText + " ", BaseHash: cur.Hash}}})
	if code := failure(t, err)["code"]; code != string(ipcerr.CodeConflict) {
		t.Fatalf("code = %v, want %s", code, ipcerr.CodeConflict)
	}
}
