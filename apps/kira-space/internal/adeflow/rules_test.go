package adeflow

import "testing"

// TestRuleDocsEnforced binds each documented rule to the validator: its example is rejected (or, for
// a rule about what is accepted, loads).
func TestRuleDocsEnforced(t *testing.T) {
	const head = "id: w\nname: W\nsteps:\n"
	const agent = "  - id: a\n    name: A\n    kind: agent\n    runs_on: once\n    timeout: 5m\n    prompt: p\n"
	const manual = "  - id: m\n    name: M\n    kind: manual\n"
	type fixture struct {
		file, src string
		accepted  bool
	}
	fix := map[string]fixture{
		"stem":           {file: "other.yaml", src: head + agent},
		"stages":         {src: "id: w\nname: W\nstages: []\n"},
		"steps":          {src: head + manual + "    skip: true\n"},
		"unique-step":    {src: head + agent + agent},
		"kind-keys":      {src: head + manual + "    runs_on: once\n"},
		"agent-body":     {src: head + agent + "    smart_script: s\n"},
		"manual-session": {src: head + agent + manual + "    prompt: hi\n"},
		"script-results": {src: head + "  - id: s\n    name: S\n    kind: script\n    runs_on: once\n    timeout: 5m\n    command: c\n    results:\n      - id: ok\n        ok: true\n"},
		"unique-result":  {src: head + agent + "    results:\n      - id: r\n        ok: true\n      - id: r\n        ok: false\n"},
		"routes":         {src: head + agent + "    results:\n      - id: r\n        ok: true\n        next: stop\n"},
		"returns":        {src: head + manual + agent + "    results:\n      - id: r\n        ok: true\n      - id: back\n        ok: false\n        next: m\n"},
		"forward":        {src: head + agent + "    results:\n      - id: r\n        ok: true\n        next: a\n"},
		"implementer":    {src: head + "  - id: r\n    name: R\n    kind: review\n    runs_on: once\n    timeout: 5m\n    review:\n      implementer: nope\n" + agent},
		"sandbox-off":    {src: head + agent + "    sandbox:\n      access: off\n      network: true\n"},
		"sandbox-paths":  {src: head + agent + "    sandbox:\n      read:\n        - /\n"},
		"legacy-status":  {src: head + manual + "    status: In review\n", accepted: true},
		"runtime-checks": {src: head + "  - id: a\n    name: A\n    kind: agent\n    runs_on: only nowhere\n    timeout: 5m\n    prompt: p\n    status: not-a-status\n", accepted: true},
	}
	for _, r := range rules {
		f, ok := fix[r.ID]
		if !ok {
			t.Errorf("rule %q has no fixture", r.ID)
			continue
		}
		delete(fix, r.ID)
		wf, perr := Parse([]byte(f.src))
		if r.ID == "stem" {
			_, p := decodeWorkflow(f.file, []byte(f.src))
			if p == nil {
				t.Errorf("rule %q: file %s accepted", r.ID, f.file)
			}
			continue
		}
		switch {
		case f.accepted && perr != nil:
			t.Errorf("rule %q: fixture rejected: %s", r.ID, perr.Message)
		case !f.accepted && perr == nil:
			t.Errorf("rule %q: fixture accepted", r.ID)
		}
		if r.ID == "legacy-status" && perr == nil && wf.Steps[0].Status != "in-review" {
			t.Errorf("legacy status read as %q", wf.Steps[0].Status)
		}
	}
	for id := range fix {
		t.Errorf("fixture %q names no rule", id)
	}
}
