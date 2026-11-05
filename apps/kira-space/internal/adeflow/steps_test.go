package adeflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

const implementLib = `id: implement
name: Implement
kind: agent
runs_on: each repo
timeout: 30m
prompt: |
  Implement the task.
allowed_tools:
  - Bash(git *)
settings:
  claude:
    auto_compact_window: 200000
results:
  - id: done
    ok: true
  - id: failed
    ok: false
`

func libReader(t *testing.T, files map[string]string) *Reader {
	t.Helper()
	root := t.TempDir()
	lib := &StepLibrary{Dir: filepath.Join(root, "steps"), Now: time.Now}
	if err := os.MkdirAll(lib.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(lib.Dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Reader{Dir: filepath.Join(root, "workflows"), Store: &memStore{m: map[string]string{}}, Now: time.Now, Steps: lib}
}

func parseRef(t *testing.T, r *Reader, items string) (adewire.Workflow, *adewire.WorkflowError) {
	t.Helper()
	v := r.Validate(head + items)
	if v.Workflow != nil {
		return *v.Workflow, nil
	}
	return adewire.Workflow{}, v.Error
}

func TestReference_resolve(t *testing.T) {
	r := libReader(t, map[string]string{"implement.yaml": implementLib})
	wf, werr := parseRef(t, r, `  - step: implement
    prompt_extra: |
      Also update CHANGELOG.md.
    timeout: 2h
    settings:
      claude:
        auto_compact_window: 300000
    routes: { failed: fix }
  - id: fix
    name: Fix
    kind: manual
`)
	if werr != nil {
		t.Fatalf("%+v", werr)
	}
	s := wf.Steps[0]
	if s.ID != "implement" || s.Use != "implement" || s.Timeout != "2h" || s.Prompt != "Implement the task." || s.PromptExtra != "Also update CHANGELOG.md." {
		t.Fatalf("%+v", s)
	}
	if s.Settings["claude"]["auto_compact_window"] != "300000" || len(s.AllowedTools) != 1 {
		t.Fatalf("override replaced a list or merged wrong: %+v", s)
	}
	if got := s.Results[1]; got.Next != "fix" || got.Loop {
		t.Fatalf("route: %+v", got)
	}
}

func TestReference_errors(t *testing.T) {
	r := libReader(t, map[string]string{"implement.yaml": implementLib, "bad.yaml": "id: bad\nname: B\nkind: agent\n"})
	cases := []struct{ name, items, msg string }{
		{"missing library step", "  - step: nope\n", `library step "nope" does not exist`},
		{"invalid library step", "  - step: bad\n", `library step "bad" is invalid`},
		{"kind override", "  - step: implement\n    kind: script\n", "kind comes from library step"},
		{"routes and results", "  - step: implement\n    routes: {failed: end}\n    results:\n      - id: done\n        ok: true\n", "routes and results cannot both be set"},
		{"unknown route result", "  - step: implement\n    routes: {nope: end}\n", `the library step has no result "nope"`},
		{"prompt_extra on manual without session", "  - step: implement\n    prompt_extra: x\n    smart_script: s\n", "prompt_extra needs"},
		{"inline refuses prompt_extra", "  - id: a\n    name: A\n    kind: manual\n    prompt_extra: x\n", `unknown key "prompt_extra"`},
		{"duplicate id", "  - step: implement\n  - step: implement\n", "already used by another step"},
	}
	for _, c := range cases {
		_, werr := parseRef(t, r, c.items)
		if werr == nil || !strings.Contains(werr.Message, c.msg) {
			t.Errorf("%s: %+v, want %q", c.name, werr, c.msg)
		}
	}
	if _, werr := parseRef(t, r, "  - step: implement\n    id: a\n  - step: implement\n    id: b\n"); werr != nil {
		t.Errorf("distinct ids: %+v", werr)
	}
	if (&Reader{}).Validate(head+"  - step: implement\n").Error == nil {
		t.Error("reference without a library accepted")
	}
}

func TestLibraryStep_refusals(t *testing.T) {
	cases := map[string]string{
		"skip":        implementLib + "skip: true\n",
		"implementer": "id: r\nname: R\nkind: review\nruns_on: once\ntimeout: 5m\nreview:\n  implementer: a\n",
		"step route":  strings.Replace(implementLib, "    ok: false\n", "    ok: false\n    next: other\n", 1),
	}
	for name, src := range cases {
		if _, _, werr := ParseStep([]byte(src)); werr == nil {
			t.Errorf("%s accepted in a library step", name)
		}
	}
	if _, _, werr := ParseStep([]byte(implementLib)); werr != nil {
		t.Errorf("valid library step: %+v", werr)
	}
}

// TestReference_roundTrip saves a resolved workflow and checks the file holds only the differences
// from the library step and loads back to the same workflow.
func TestReference_roundTrip(t *testing.T) {
	r := libReader(t, map[string]string{"implement.yaml": implementLib})
	if _, err := r.SaveYaml("w.yaml", "", head+"  - step: implement\n    # keep me\n    timeout: 2h\n  - id: fix\n    name: Fix\n    kind: manual\n"); err != nil {
		t.Fatal(err)
	}
	wf := load(t, r, "w.yaml")
	wf.Steps[0].PromptExtra = "More."
	wf.Steps[0].Settings["claude"]["auto_compact_window"] = "250000"
	wf.Steps[0].Results[1].Next = "fix"
	wf.Steps[0].Skip = true
	wf.Steps[1].Skip = false
	saveOK(t, r, "w.yaml", wf)
	got := read(t, r, "w.yaml")
	for _, want := range []string{"step: implement", "# keep me", "timeout: 2h", "prompt_extra: More.", "routes:", "failed: fix", "skip: true", "auto_compact_window: 250000"} {
		if !strings.Contains(got, want) {
			t.Errorf("file lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"prompt: ", "runs_on", "allowed_tools", "name: Implement", "id: implement", "results:"} {
		if strings.Contains(got, not) {
			t.Errorf("file repeats library value %q:\n%s", not, got)
		}
	}
	if back := load(t, r, "w.yaml"); !equalJSON(back, wf) {
		t.Errorf("round trip differs:\n%+v\n%+v", back.Steps[0], wf.Steps[0])
	}
	// override replaced by the library value is dropped again
	wf.Steps[0].Timeout = "30m"
	wf.Steps[0].Skip = false
	wf.Steps[0].PromptExtra = ""
	wf.Steps[0].Results[1].Next = "stop"
	wf.Steps[0].Settings["claude"]["auto_compact_window"] = "200000"
	saveOK(t, r, "w.yaml", wf)
	if got := read(t, r, "w.yaml"); strings.Contains(got, "timeout") || strings.Contains(got, "routes") || strings.Contains(got, "prompt_extra") || strings.Contains(got, "skip") {
		t.Errorf("overrides left behind:\n%s", got)
	}
}

func TestReference_routesVersusResults(t *testing.T) {
	r := libReader(t, map[string]string{"implement.yaml": implementLib})
	if _, err := r.SaveYaml("w.yaml", "", head+"  - step: implement\n"); err != nil {
		t.Fatal(err)
	}
	wf := load(t, r, "w.yaml")
	wf.Steps[0].Results = append(wf.Steps[0].Results, adewire.StepResult{ID: "more", OK: true, Next: "end"})
	saveOK(t, r, "w.yaml", wf)
	if got := read(t, r, "w.yaml"); !strings.Contains(got, "results:") || strings.Contains(got, "routes:") {
		t.Errorf("a changed result set must write results:\n%s", got)
	}
}

func TestReference_libraryEditPropagatesAndBreaks(t *testing.T) {
	r := libReader(t, map[string]string{"implement.yaml": implementLib})
	if _, err := r.SaveYaml("w.yaml", "", head+"  - step: implement\n"); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get("w"); got.Steps[0].Timeout != "30m" {
		t.Fatalf("%+v", got.Steps[0])
	}
	if _, err := r.Steps.SaveYaml("implement.yaml", "", strings.Replace(implementLib, "30m", "45m", 1)); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get("w"); got.Steps[0].Timeout != "45m" {
		t.Fatalf("library edit did not reach the workflow: %+v", got.Steps[0])
	}
	if _, err := r.Steps.SaveYaml("implement.yaml", "", "id: implement\nname: x\n"); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range r.List(nil).Workflows {
		if e.FileName == "w.yaml" {
			found = true
			if e.Error == nil || e.Workflow == nil || e.Workflow.Steps[0].Timeout != "45m" {
				t.Fatalf("broken library step must keep the last valid workflow: %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("w.yaml not listed")
	}
}

func TestStepLibrary_NewAndDocs(t *testing.T) {
	r := libReader(t, nil)
	e, err := r.Steps.New("Run Tests", adewire.KindScript, nil)
	if err != nil || e.Error != nil || e.FileName != "run-tests.yaml" {
		t.Fatalf("%+v %v", e, err)
	}
	if e2, err := r.Steps.New("Run Tests", adewire.KindAgent, nil); err != nil || e2.FileName != "run-tests-2.yaml" || e2.Error != nil {
		t.Fatalf("%+v %v", e2, err)
	}
	inline := adewire.WorkflowStep{ID: "x", Name: "X", Kind: adewire.KindManual, Skip: true, Results: []adewire.StepResult{}}
	if e3, err := r.Steps.New("From inline", adewire.KindManual, &inline); err != nil || e3.Step == nil || e3.Step.Skip {
		t.Fatalf("%+v %v", e3, err)
	}
	if err := r.Steps.WriteDocs(); err != nil {
		t.Fatal(err)
	}
	doc, _ := os.ReadFile(filepath.Join(r.Steps.Dir, "AGENTS.md"))
	if !strings.Contains(string(doc), "prompt") || strings.Contains(string(doc), "`skip`**") {
		t.Errorf("steps AGENTS.md:\n%s", doc)
	}
	for _, f := range yamlFences(string(doc)) {
		if v := ValidateStepYaml(f); v.Error != nil {
			t.Errorf("example invalid: %+v", v.Error)
		}
	}
}

func yamlFences(doc string) []string {
	var out []string
	for _, part := range strings.Split(doc, "```yaml\n")[1:] {
		if i := strings.Index(part, "```"); i >= 0 {
			out = append(out, part[:i])
		}
	}
	return out
}

func equalJSON(a, b adewire.Workflow) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
