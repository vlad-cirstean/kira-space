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
		ref       bool // parse with a step library holding the agent step `lib`
	}
	const ref = head + "  - step: lib\n"
	fix := map[string]fixture{
		"stem":           {file: "other.yaml", src: head + agent},
		"steps":          {src: head + manual + "    skip: true\n"},
		"unique-step":    {src: head + agent + agent},
		"kind-keys":      {src: head + manual + "    runs_on: once\n"},
		"agent-body":     {src: head + agent + "    smart_script: s\n"},
		"outside-repo":   {src: head + agent + "    outside_repo: true\n"},
		"manual-session": {src: head + agent + manual + "    prompt: hi\n"},
		"script-results": {src: head + "  - id: s\n    name: S\n    kind: script\n    runs_on: once\n    timeout: 5m\n    command: c\n    results:\n      - id: ok\n        ok: true\n"},
		"unique-result":  {src: head + agent + "    results:\n      - id: r\n        ok: true\n      - id: r\n        ok: false\n"},
		"routes":         {src: head + agent + "    results:\n      - id: r\n        ok: true\n        next: stop\n"},
		"returns":        {src: head + manual + agent + "    results:\n      - id: r\n        ok: true\n      - id: back\n        ok: false\n        next: m\n"},
		"forward":        {src: head + agent + "    results:\n      - id: r\n        ok: true\n        next: a\n"},
		"implementer":    {src: head + "  - id: r\n    name: R\n    kind: review\n    runs_on: once\n    timeout: 5m\n    review:\n      implementer: nope\n" + agent},
		"sandbox-off":    {src: head + agent + "    sandbox:\n      access: off\n      network: true\n"},
		"sandbox-paths":  {src: head + agent + "    sandbox:\n      read:\n        - /\n"},
		"reference":      {src: head + "  - step: nope\n", ref: true},
		"overrides":      {src: ref + "    timeout: 1h\n    settings: {}\n", accepted: true, ref: true},
		"ref-routes":     {src: ref + "    routes: {done: end}\n    results:\n      - id: done\n        ok: true\n", ref: true},
		"prompt-extra":   {src: ref + "    smart_script: s\n    prompt_extra: x\n", ref: true},
		"runtime-checks": {src: head + "  - id: a\n    name: A\n    kind: agent\n    runs_on: only nowhere\n    timeout: 5m\n    prompt: p\n    status: not-a-status\n", accepted: true},
	}
	for _, r := range rules {
		f, ok := fix[r.ID]
		if !ok {
			t.Errorf("rule %q has no fixture", r.ID)
			continue
		}
		delete(fix, r.ID)
		_, perr := Parse([]byte(f.src))
		if f.ref {
			_, perr = ParseWith([]byte(f.src), libReader(t, map[string]string{"lib.yaml": "id: lib\nname: L\nkind: agent\nruns_on: once\ntimeout: 5m\nprompt: p\n"}).lookup())
		}
		if r.ID == "stem" {
			_, p := decodeWorkflow(f.file, []byte(f.src), nil)
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
	}
	if _, perr := Parse([]byte(head + manual + "    sandbox:\n      access: read\n")); perr == nil {
		t.Error("sandbox on a manual step without a session accepted")
	}
	for id := range fix {
		t.Errorf("fixture %q names no rule", id)
	}
}
