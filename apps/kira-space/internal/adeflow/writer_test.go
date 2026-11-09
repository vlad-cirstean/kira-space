package adeflow

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

func newReader(t *testing.T) *Reader {
	t.Helper()
	return &Reader{Dir: filepath.Join(t.TempDir(), "workflows"), Store: &memStore{m: map[string]string{}}, Now: time.Now}
}

// seed copies a design sample into the reader's dir with comments added.
func seed(t *testing.T, r *Reader, name string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(samplesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	text := "# my workflow\n" + strings.Replace(string(src), "steps:\n", "steps:\n  # step list\n", 1)
	text = strings.Replace(text, "    timeout: 2h\n", "    timeout: 2h # long\n", 1)
	if _, err := r.SaveYaml(name, "", text); err != nil {
		t.Fatal(err)
	}
	return text
}

func load(t *testing.T, r *Reader, name string) adewire.Workflow {
	t.Helper()
	for _, e := range r.List(nil).Workflows {
		if e.FileName == name {
			if e.Error != nil || e.Workflow == nil {
				t.Fatalf("%s: %+v", name, e.Error)
			}
			return *e.Workflow
		}
	}
	t.Fatalf("%s not listed", name)
	return adewire.Workflow{}
}

func read(t *testing.T, r *Reader, name string) string {
	t.Helper()
	y, err := r.ReadYaml(name)
	if err != nil {
		t.Fatal(err)
	}
	return y.Yaml
}

func saveOK(t *testing.T, r *Reader, name string, wf adewire.Workflow) string {
	t.Helper()
	if _, err := r.Save(name, "", wf); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := load(t, r, name); !reflect.DeepEqual(got, wf) {
		t.Fatalf("re-parse differs:\n got %+v\nwant %+v", got, wf)
	}
	return read(t, r, name)
}

func TestSave_preservesCommentsAndStyle(t *testing.T) {
	for _, name := range []string{"standard.yaml", "bugfix.yaml", "chore.yaml"} {
		r := newReader(t)
		orig := seed(t, r, name)
		wf := load(t, r, name)
		out := saveOK(t, r, name, wf)
		for _, want := range []string{"# my workflow", "# step list", "# long"} {
			if strings.Contains(orig, want) && !strings.Contains(out, want) {
				t.Errorf("%s: lost %q", name, want)
			}
		}
		if out != orig {
			t.Errorf("%s: unchanged save altered the file:\n%s", name, out)
		}
	}
}

func stepIndex(wf adewire.Workflow, id string) int {
	return slices.IndexFunc(wf.Steps, func(s adewire.WorkflowStep) bool { return s.ID == id })
}

func agentStep(id string) adewire.WorkflowStep {
	return adewire.WorkflowStep{
		ID: id, Name: "S", Kind: adewire.KindAgent, Status: "", RunsOn: "once", Before: "auto",
		Results: adewire.DefaultResults(), MaxReturns: 3, Timeout: "10m", Prompt: "go",
		AllowedTools: []string{}, Params: map[string][]string{}, Sandbox: adewire.DefaultSandbox(),
		Settings: map[string]map[string]string{},
	}
}

func TestSave_edits(t *testing.T) {
	r := newReader(t)
	seed(t, r, "standard.yaml")
	base := load(t, r, "standard.yaml")

	t.Run("edit scalar keeps comment", func(t *testing.T) {
		wf := base
		wf.Steps = slices.Clone(base.Steps)
		wf.Steps[stepIndex(wf, "impl")].Timeout = "3h"
		out := saveOK(t, r, "standard.yaml", wf)
		if !strings.Contains(out, "timeout: 3h # long") {
			t.Fatalf("comment lost:\n%s", out)
		}
	})

	t.Run("reorder steps, delete and add step, defaults not added", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		i, j := stepIndex(wf, "review"), stepIndex(wf, "spec")
		wf.Steps[i], wf.Steps[j] = wf.Steps[j], wf.Steps[i]
		wf.Steps = slices.DeleteFunc(wf.Steps, func(s adewire.WorkflowStep) bool { return s.ID == "tests" })
		lint := agentStep("lint")
		lint.Prompt, lint.AllowedTools = "Run lint.\nFix it.", []string{"Bash(git *)"}
		wf.Steps = append(wf.Steps, lint)
		out := saveOK(t, r, "standard.yaml", wf)
		if strings.Index(out, "id: review") > strings.Index(out, "id: spec") {
			t.Fatalf("steps not reordered:\n%s", out)
		}
		_, tail, _ := strings.Cut(out, "id: lint")
		step, _, _ := strings.Cut(tail, "prompt:")
		for _, k := range []string{"before:", "results:", "max_returns:", "status:", "skip:"} {
			if strings.Contains(step, k) {
				t.Fatalf("default key %s written for a new step:\n%s", k, tail)
			}
		}
		if !strings.Contains(step, "kind: agent") || !strings.Contains(tail, "prompt: |") || !strings.Contains(tail, "allowed_tools:") {
			t.Fatalf("new step shape:\n%s", tail)
		}
		if strings.Contains(out, "id: tests") {
			t.Fatalf("deleted step still present")
		}
	})

	t.Run("manual step with no prompt writes no prompt key", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		i := stepIndex(wf, "spec")
		wf.Steps[i].Prompt = ""
		out := saveOK(t, r, "standard.yaml", wf)
		_, seg, _ := strings.Cut(out, "id: spec")
		seg, _, _ = strings.Cut(seg, "- id:")
		if !strings.Contains(seg, "session: true") || strings.Contains(seg, "prompt:") || strings.Contains(seg, "results:") {
			t.Fatalf("spec step:\n%s", seg)
		}
	})

	t.Run("smart step keeps key order and params shape", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		i := stepIndex(wf, "plan")
		smart := agentStep("plan")
		smart.Name, smart.Prompt, smart.SmartScript = "Smart", "", "Summarize"
		smart.Params = map[string][]string{"lang": {"go"}, "dirs": {"a", "b"}}
		wf.Steps[i] = smart
		out := saveOK(t, r, "standard.yaml", wf)
		_, seg, _ := strings.Cut(out, "name: Smart")
		seg, _, _ = strings.Cut(seg, "- id:")
		order := []string{"runs_on:", "timeout:", "smart_script: Summarize", "params:", "dirs:", "lang: go"}
		at := 0
		for _, k := range order {
			i := strings.Index(seg[at:], k)
			if i < 0 {
				t.Fatalf("%q missing or out of order:\n%s", k, seg)
			}
			at += i
		}
		if strings.Contains(seg, "prompt:") || strings.Contains(seg, "allowed_tools:") {
			t.Fatalf("prompt keys left on a smart step:\n%s", seg)
		}
	})

	t.Run("sandbox block round trip", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		i := stepIndex(wf, "impl")
		if out := saveOK(t, r, "standard.yaml", wf); strings.Contains(out, "sandbox:") {
			t.Fatalf("default sandbox written:\n%s", out)
		}
		want := adewire.StepSandbox{Access: "read", Read: []string{"~/specs"}, Allow: []string{"~/.cache/tool"}, Network: false}
		wf.Steps[i].Sandbox = want
		out := saveOK(t, r, "standard.yaml", wf)
		for _, k := range []string{"sandbox:", "access: read", "- ~/specs", "- ~/.cache/tool", "network: false"} {
			if !strings.Contains(out, k) {
				t.Fatalf("%q missing:\n%s", k, out)
			}
		}
		wf.Steps[i].Sandbox = adewire.WriteSandbox()
		if out := saveOK(t, r, "standard.yaml", wf); !strings.Contains(out, "access: write") || strings.Contains(out, "network:") || strings.Contains(out, "~/specs") {
			t.Fatalf("off block:\n%s", out)
		}
		wf.Steps[i].Sandbox = adewire.DefaultSandbox()
		if out := saveOK(t, r, "standard.yaml", wf); strings.Contains(out, "sandbox:") {
			t.Fatalf("sandbox left after reset:\n%s", out)
		}
	})

	t.Run("review step round trip", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		rv := agentStep("code-review")
		rv.Kind, rv.Status, rv.Prompt, rv.MaxReturns, rv.AllowedTools = adewire.KindReview, "", "", 0, []string{}
		rv.Review = &adewire.ReviewStepConfig{SendTo: "new", Implementer: "impl"}
		wf.Steps = append(wf.Steps, rv)
		out := saveOK(t, r, "standard.yaml", wf)
		_, seg, _ := strings.Cut(out, "id: code-review")
		if !strings.Contains(seg, "kind: review") || !strings.Contains(seg, "review:\n") || !strings.Contains(seg, "send_to: new") || strings.Contains(seg, "prompt:") || strings.Contains(seg, "status:") {
			t.Fatalf("review block:\n%s", seg)
		}
		wf.Steps[len(wf.Steps)-1].Review = &adewire.ReviewStepConfig{SendTo: "implementer"}
		if back := saveOK(t, r, "standard.yaml", wf); strings.Contains(back, "send_to: new") {
			t.Fatalf("send_to left after reset to defaults:\n%s", back)
		}
	})

	t.Run("kind change drops foreign keys", func(t *testing.T) {
		wf := load(t, r, "standard.yaml")
		i := stepIndex(wf, "spec")
		wf.Steps[i] = adewire.WorkflowStep{ID: "spec", Name: "Spec", Kind: adewire.KindScript, Status: "todo", Command: "make spec",
			RunsOn: "once", Before: "auto", Results: adewire.DefaultResults(), MaxReturns: 3, Timeout: "5m", AllowedTools: []string{}, Params: map[string][]string{}, Sandbox: adewire.DefaultSandbox(), Settings: map[string]map[string]string{}}
		out := saveOK(t, r, "standard.yaml", wf)
		_, seg, _ := strings.Cut(out, "id: spec")
		seg, _, _ = strings.Cut(seg, "- id:")
		if strings.Contains(seg, "session:") || strings.Contains(seg, "prompt:") || !strings.Contains(seg, "command: make spec") || !strings.Contains(seg, "kind: script") {
			t.Fatalf("spec step:\n%s", seg)
		}
	})
}

func TestSave_settings(t *testing.T) {
	r := newReader(t)
	seed(t, r, "standard.yaml")
	orig := read(t, r, "standard.yaml")
	if strings.Contains(orig, "settings:") {
		t.Fatal("sample already has settings")
	}
	base := load(t, r, "standard.yaml")
	if out := saveOK(t, r, "standard.yaml", base); out != orig {
		t.Fatalf("file without settings changed:\n%s", out)
	}
	set := func(v string) adewire.Workflow {
		wf := load(t, r, "standard.yaml")
		wf.Steps = slices.Clone(wf.Steps)
		i := stepIndex(wf, "impl")
		wf.Steps[i].Settings = map[string]map[string]string{"claude": {"auto_compact_window": v}}
		if v == "" {
			wf.Steps[i].Settings = map[string]map[string]string{}
		}
		return wf
	}
	if out := saveOK(t, r, "standard.yaml", set("200000")); !strings.Contains(out, "settings:\n      claude:\n        auto_compact_window: 200000\n") {
		t.Fatalf("setting not written:\n%s", out)
	}
	if out := saveOK(t, r, "standard.yaml", set("300000")); !strings.Contains(out, "auto_compact_window: 300000") || strings.Contains(out, "200000") {
		t.Fatalf("setting not changed:\n%s", out)
	}
	if out := saveOK(t, r, "standard.yaml", set("")); out != orig {
		t.Fatalf("cleared settings left a trace:\n%s", out)
	}
	wf := set("200000")
	saveOK(t, r, "standard.yaml", wf)
	wf = load(t, r, "standard.yaml")
	i := stepIndex(wf, "impl")
	wf.Steps[i] = adewire.WorkflowStep{ID: "impl", Name: "Impl", Kind: adewire.KindScript, RunsOn: "once", Before: "auto", Results: adewire.DefaultResults(),
		Timeout: "5m", Command: "make", AllowedTools: []string{}, Params: map[string][]string{}, Sandbox: adewire.DefaultSandbox(), Settings: map[string]map[string]string{},
		MaxReturns: wf.Steps[i].MaxReturns}
	if _, err := r.Save("standard.yaml", "", wf); err != nil {
		t.Fatalf("save: %v", err)
	}
	if out := read(t, r, "standard.yaml"); strings.Contains(out, "settings:") {
		t.Fatalf("settings kept on a script step:\n%s", out)
	}
}

func TestSave_refusals(t *testing.T) {
	r := newReader(t)
	if _, err := r.SaveYaml("w.yaml", "", "id: w\nname: [\n"); err != nil {
		t.Fatal(err)
	}
	wf := adewire.Workflow{ID: "w", Name: "W", Steps: []adewire.WorkflowStep{{ID: "u", Name: "U", Kind: adewire.KindManual, Status: "todo", Results: []adewire.StepResult{}}}}
	before := read(t, r, "w.yaml")
	if _, err := r.Save("w.yaml", "", wf); err == nil || !strings.Contains(err.Error(), "fix the YAML error on line") {
		t.Fatalf("syntax error: %v", err)
	}
	if read(t, r, "w.yaml") != before {
		t.Fatal("file overwritten")
	}
	bad := wf
	bad.ID = "other"
	if _, err := r.Save("w.yaml", "", bad); err == nil {
		t.Fatal("id/stem mismatch accepted")
	}
	bad = wf
	bad.Steps = []adewire.WorkflowStep{{ID: "s", Name: "S", Kind: adewire.KindScript, Status: "todo"}}
	if _, err := r.Save("fresh.yaml", "", bad); err == nil {
		t.Fatal("invalid workflow accepted")
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "fresh.yaml")); !os.IsNotExist(err) {
		t.Fatal("invalid workflow written")
	}
	if _, err := r.SaveYaml("../x.yaml", "", "x"); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestNewAndImport(t *testing.T) {
	r := newReader(t)
	if _, err := os.Stat(r.Dir); !os.IsNotExist(err) {
		t.Fatal("dir exists before first write")
	}
	e1, err := r.New("My Flow!")
	if err != nil || e1.FileName != "my-flow.yaml" || e1.Workflow == nil {
		t.Fatalf("new: %+v %v", e1, err)
	}
	e2, err := r.New("My Flow!")
	if err != nil || e2.FileName != "my-flow-2.yaml" || e2.Workflow.ID != "my-flow-2" {
		t.Fatalf("collision: %+v %v", e2, err)
	}
	src := filepath.Join(t.TempDir(), "Weird Name.yaml")
	if err := os.WriteFile(src, []byte("id: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e3, err := r.Import(src)
	if err != nil || e3.FileName != "weird-name.yaml" || e3.Error == nil {
		t.Fatalf("import unparseable: %+v %v", e3, err)
	}
	if _, err := r.Import(src); err == nil {
		t.Fatal("second import over existing accepted")
	}
	good, _ := os.ReadFile(filepath.Join(samplesDir, "chore.yaml"))
	src2 := filepath.Join(t.TempDir(), "anything.yaml")
	if err := os.WriteFile(src2, good, 0o644); err != nil {
		t.Fatal(err)
	}
	e4, err := r.Import(src2)
	if err != nil || e4.FileName != "chore.yaml" || e4.Workflow == nil {
		t.Fatalf("import valid: %+v %v", e4, err)
	}
}

func TestSave_results(t *testing.T) {
	r := newReader(t)
	orig := seed(t, r, "standard.yaml")
	wf := load(t, r, "standard.yaml")
	if out := saveOK(t, r, "standard.yaml", wf); out != orig {
		t.Fatalf("file changed on an unchanged save:\n%s", out)
	}
	step := &wf.Steps[stepIndex(wf, "ci")]
	step.Results = []adewire.StepResult{
		{ID: "pass", OK: true, Description: "All green.", Next: "next"},
		{ID: "flaky", OK: false, Next: "ci", Loop: true},
		{ID: "broken", OK: false, Next: "impl", Loop: true},
	}
	step.MaxReturns = 5
	out := saveOK(t, r, "standard.yaml", wf)
	_, seg, _ := strings.Cut(out, "id: ci")
	seg, _, _ = strings.Cut(seg, "id: pr")
	if !strings.Contains(seg, "results:") || !strings.Contains(seg, "max_returns: 5") || !strings.Contains(seg, "id: broken") {
		t.Fatalf("results not written as a list:\n%s", seg)
	}
	step.Results, step.MaxReturns = adewire.DefaultResults(), 3
	back := saveOK(t, r, "standard.yaml", wf)
	_, seg, _ = strings.Cut(back, "id: ci")
	seg, _, _ = strings.Cut(seg, "id: pr")
	if strings.Contains(seg, "max_returns") || !strings.Contains(back, "# my workflow") || !strings.Contains(back, "# long") {
		t.Fatalf("defaults kept max_returns or lost comments:\n%s", back)
	}
}
