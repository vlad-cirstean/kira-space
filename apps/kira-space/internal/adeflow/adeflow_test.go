package adeflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const samplesDir = "testdata"

func TestParse_designSamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(samplesDir, "*.yaml"))
	if err != nil || len(files) != 3 {
		t.Fatalf("samples: %v %v", files, err)
	}
	for _, f := range files {
		src, _ := os.ReadFile(f)
		wf, werr := Parse(src)
		if werr != nil {
			t.Fatalf("%s: %+v", f, werr)
		}
		if wf.ID != strings.TrimSuffix(filepath.Base(f), ".yaml") {
			t.Fatalf("%s: id %q", f, wf.ID)
		}
	}
}

const head = "id: w\nname: W\nsteps:\n"

func step(id, extra string) string {
	return "  - id: " + id + "\n    name: S\n    kind: agent\n    runs_on: each repo\n    timeout: 1h\n    prompt: go\n" + extra
}

func reviewStep(id, extra string) string {
	return "  - id: " + id + "\n    name: R\n    kind: review\n    runs_on: each repo\n    timeout: 1h\n" + extra
}

func smartStep(id, extra string) string {
	return "  - id: " + id + "\n    name: S\n    kind: agent\n    runs_on: each repo\n    timeout: 1h\n    smart_script: Summarize\n" + extra
}

func scriptStep(id, extra string) string {
	return "  - id: " + id + "\n    name: R\n    kind: script\n    status: in-review\n    runs_on: once\n    timeout: 5m\n    command: make\n" + extra
}

func manual(id, extra string) string {
	return "  - id: " + id + "\n    name: U\n    kind: manual\n    status: todo\n" + extra
}

// resultsBlock is one result item; the first call of a step opens the results list.
func resultsBlock(id, ok, next string) string {
	out := "      - id: " + id + "\n        ok: " + ok + "\n"
	if next != "" {
		out += "        next: " + next + "\n"
	}
	if id == "a" || id == "needs_input" {
		out = "    results:\n" + out
	}
	return out
}

func TestParse_rules(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int    // 0 = not asserted unless wantLine
		msg  string // substring; "" = valid
	}{
		{"valid minimal manual", head + manual("u", ""), 0, ""},
		{"kind required", head + "  - id: u\n    name: U\n", 0, "kind is required"},
		{"bad kind", head + "  - id: u\n    name: U\n    kind: robot\n", 6, "step 1: kind must be agent, script, review or manual"},
		{"old kind user", head + "  - id: u\n    name: U\n    kind: user\n", 6, "kind must be agent, script, review or manual"},
		{"bad status", head + "  - id: u\n    name: U\n    kind: manual\n    status: Bad Id\n", 0, "step 1: status must be"},
		{"status optional", head + "  - id: u\n    name: U\n    kind: manual\n", 0, ""},
		{"skip", head + manual("u", "    skip: true\n") + manual("v", ""), 0, ""},
		{"all skipped", head + manual("u", "    skip: true\n"), 0, "at least one step must not be skipped"},
		{"stages refused", "id: w\nname: W\nstages:\n  - id: u\n", 3, "\"stages\" is no longer supported: list every step under \"steps\" (see the Agents docs)"},
		{"unknown top key", "id: w\nname: W\nbogus: 1\nsteps: []\n", 3, "unknown key"},
		{"empty steps", "id: w\nname: W\nsteps: []\n", 3, "steps must be a non-empty list"},
		{"missing name", "id: w\nsteps:\n  - id: u\n", 0, "name is required"},
		{"bad id", "id: W!\nname: W\nsteps: []\n", 1, "id must be lowercase"},
		{"dup step id", head + manual("u", "") + manual("u", ""), 8, "step 2: id \"u\" is already used"},
		{"on_failure gone", head + step("s", "    on_failure: stop\n"), 0, "unknown key \"on_failure\""},
		{"manual session prompt optional", head + manual("u", "    session: true\n"), 0, ""},
		{"manual prompt without session", head + manual("u", "    prompt: x\n"), 0, "prompt is not allowed on manual steps without a session"},
		{"manual refuses timeout", head + manual("u", "    timeout: 1h\n"), 0, "timeout is not allowed on manual steps"},
		{"manual refuses results", head + manual("u", "    results: []\n"), 0, "results is not allowed on manual steps"},
		{"agent refuses command", head + step("s", "    command: x\n"), 0, "command is not allowed on agent steps"},
		{"agent refuses review block", head + step("s", "    review: {}\n"), 0, "review is not allowed on agent steps"},
		{"agent refuses session", head + step("s", "    session: true\n"), 0, "session is not allowed on agent steps"},
		{"runs_on only", head + strings.Replace(step("s", ""), "each repo", "only api", 1), 0, ""},
		{"runs_on only empty", head + strings.Replace(step("s", ""), "each repo", "only ", 1), 0, "runs_on must be"},
		{"runs_on bad", head + strings.Replace(step("s", ""), "each repo", "twice", 1), 0, "runs_on must be"},
		{"before bad", head + step("s", "    before: maybe\n"), 0, "before must be auto or approval"},
		{"results ok", head + step("s", resultsBlock("a", "true", "")+resultsBlock("b", "false", "s")), 0, ""},
		{"results unknown target", head + step("s", resultsBlock("a", "true", "nope")), 0, "next must be next, end, stop or a step id of this workflow"},
		{"results stop on ok", head + step("s", resultsBlock("a", "true", "stop")), 0, "stop is only allowed on a result that is not ok"},
		{"result max gone", head + step("s", resultsBlock("a", "true", "")+resultsBlock("b", "false", "s")+"        max: 2\n"), 0, "unknown key \"max\""},
		{"results no forward", head + step("s", resultsBlock("a", "false", "s")), 0, "at least one result must lead forward"},
		{"max_returns ok", head + step("s", "    max_returns: 10\n"), 0, ""},
		{"max_returns zero on agent", head + step("s", "    max_returns: 0\n"), 0, "max_returns must be a whole number from 1 to 10"},
		{"max_returns over 10", head + step("s", "    max_returns: 11\n"), 0, "max_returns must be a whole number from 1 to 10"},
		{"max_returns text", head + step("s", "    max_returns: lots\n"), 0, "max_returns must be a whole number"},
		{"max_returns zero on review", head + step("s", "") + reviewStep("r", "    max_returns: 0\n    review: {}\n"), 0, ""},
		{"max_returns over 10 on review", head + step("s", "") + reviewStep("r", "    max_returns: 11\n    review: {}\n"), 0, "from 0 to 10 on a review step"},
		{"results duplicate id", head + step("s", resultsBlock("a", "true", "")+resultsBlock("b", "false", "")+resultsBlock("b", "false", "")), 0, "already used by another result"},
		{"results reserved id", head + step("s", resultsBlock("needs_input", "true", "")), 0, "needs_input is reserved"},
		{"results ok required", head + step("s", "    results:\n      - id: a\n"), 0, "ok is required"},
		{"results later step", head + step("s", resultsBlock("a", "true", "t")) + step("t", ""), 0, ""},
		{"results back to manual step", head + manual("m", "") + step("s", resultsBlock("a", "true", "")+resultsBlock("b", "false", "m")), 0, "cannot return to a manual or skipped step"},
		{"timeout zero", head + strings.Replace(step("s", ""), "1h", "0s", 1), 0, "timeout must be"},
		{"timeout over 24h", head + strings.Replace(step("s", ""), "1h", "25h", 1), 0, "timeout must be"},
		{"timeout garbage", head + strings.Replace(step("s", ""), "1h", "soon", 1), 0, "timeout must be"},
		{"timeout required", head + "  - id: s\n    name: S\n    kind: agent\n    runs_on: once\n    prompt: go\n", 0, "timeout is required"},
		{"allowed tools ok", head + step("s", "    allowed_tools:\n      - Bash(git *)\n      - Read\n      - mcp__srv__tool\n"), 0, ""},
		{"allowed tools dup", head + step("s", "    allowed_tools:\n      - Read\n      - Read\n"), 0, "listed twice"},
		{"allowed tools bad", head + step("s", "    allowed_tools:\n      - \"bad tool\"\n"), 0, "not a tool pattern"},
		{"smart step", head + smartStep("s", "    params:\n      lang: go\n      dirs: [a, b]\n"), 0, ""},
		{"smart step no params", head + smartStep("s", ""), 0, ""},
		{"smart prompt refused", head + smartStep("s", "    prompt: go\n"), 0, "prompt is not allowed with smart_script"},
		{"smart tools refused", head + smartStep("s", "    allowed_tools: [Read]\n"), 0, "allowed_tools is not allowed with smart_script"},
		{"params alone", head + step("s", "    params:\n      a: b\n"), 0, "params is only allowed with smart_script"},
		{"smart param name", head + smartStep("s", "    params:\n      Bad-Name: x\n"), 0, "param name"},
		{"smart param nested", head + smartStep("s", "    params:\n      a:\n        b: c\n"), 0, "must be text or a list of text"},
		{"review step", head + step("s", "") + reviewStep("r", "    review:\n      send_to: new\n"), 0, ""},
		{"review block optional", head + step("s", "") + reviewStep("r", ""), 0, ""},
		{"review defaults", head + step("s", "") + reviewStep("r", "    review: {}\n"), 0, ""},
		{"review implementer", head + step("s", "") + reviewStep("r", "    review:\n      implementer: s\n"), 0, ""},
		{"review refuses prompt", head + step("s", "") + reviewStep("r", "    prompt: go\n"), 0, "prompt is not allowed on review steps"},
		{"review refuses smart", head + step("s", "") + reviewStep("r", "    smart_script: X\n"), 0, "smart_script is not allowed on review steps"},
		{"review bad send_to", head + step("s", "") + reviewStep("r", "    review:\n      send_to: both\n"), 0, "send_to must be implementer or new"},
		{"review unknown key", head + step("s", "") + reviewStep("r", "    review:\n      model: x\n"), 0, "unknown key"},
		{"review not mapping", head + step("s", "") + reviewStep("r", "    review: yes\n"), 0, "review must be a mapping"},
		{"review implementer unknown", head + step("s", "") + reviewStep("r", "    review:\n      implementer: nope\n"), 0, "implementer must be an earlier agent step"},
		{"review implementer later", head + reviewStep("r", "    review:\n      implementer: s\n") + step("s", ""), 0, "implementer must be an earlier agent step"},
		{"review implementer smart", head + smartStep("x", "") + reviewStep("r", "    review:\n      implementer: x\n"), 0, "implementer must be an earlier agent step"},
		{"review implementer review", head + step("s", "") + reviewStep("q", "") + reviewStep("r", "    review:\n      implementer: q\n"), 0, "implementer must be an earlier agent step"},
		{"sandbox full", head + step("s", "    sandbox:\n      access: read\n      read: [~/specs, /srv/docs]\n      allow: [~/.cache/tool]\n      network: false\n"), 0, ""},
		{"sandbox empty", head + step("s", "    sandbox: {}\n"), 0, ""},
		{"sandbox on review", head + step("s", "") + reviewStep("r", "    sandbox:\n      access: read\n"), 0, ""},
		{"sandbox on script refused", head + scriptStep("r", "    sandbox: {}\n"), 0, "sandbox is not allowed on script steps"},
		{"sandbox bad access", head + step("s", "    sandbox:\n      access: full\n"), 0, "access must be write, read or off"},
		{"sandbox unknown key", head + step("s", "    sandbox:\n      net: no\n"), 0, "unknown key"},
		{"sandbox not mapping", head + step("s", "    sandbox: yes\n"), 0, "sandbox must be a mapping"},
		{"sandbox relative path", head + step("s", "    sandbox:\n      read: [specs]\n"), 0, "must be absolute or start with ~/"},
		{"sandbox root path", head + step("s", "    sandbox:\n      allow: [/]\n"), 0, "path \"/\" is not allowed"},
		{"sandbox home path", head + step("s", "    sandbox:\n      allow: [\"~\"]\n"), 0, "path \"~\" is not allowed"},
		{"sandbox keychain path", head + step("s", "    sandbox:\n      read: [~/Library/Keychains/login.keychain-db]\n"), 0, "is not allowed"},
		{"sandbox claude dir", head + step("s", "    sandbox:\n      allow: [~/.claude/]\n"), 0, "is not allowed"},
		{"sandbox absolute home", head + step("s", "    sandbox:\n      allow: [/Users/me/Library]\n"), 0, "is not allowed"},
		{"sandbox library dir", head + step("s", "    sandbox:\n      allow: [\"~/x/../Library\"]\n"), 0, "is not allowed"},
		{"sandbox network not bool", head + step("s", "    sandbox:\n      network: maybe\n"), 0, "network must be true or false"},
		{"sandbox off alone", head + step("s", "    sandbox:\n      access: off\n"), 0, ""},
		{"sandbox off with paths", head + step("s", "    sandbox:\n      access: off\n      read: [~/x]\n"), 0, "sandbox off takes no other keys"},
		{"settings ok", head + step("s", "    settings:\n      claude:\n        auto_compact_window: 200000\n"), 0, ""},
		{"settings on review", head + step("s", "") + reviewStep("r", "    settings:\n      claude:\n        auto_compact_window: 100000\n"), 0, ""},
		{"settings on manual session", head + manual("u", "    session: true\n    settings:\n      claude:\n        auto_compact_window: 1000000\n"), 0, ""},
		{"settings empty blocks", head + step("s", "    settings: {}\n") + step("t", "    settings:\n      claude: {}\n"), 0, ""},
		{"settings not mapping", head + step("s", "    settings: yes\n"), 0, "settings must be a mapping"},
		{"settings unknown agent", head + step("s", "    settings:\n      gpt:\n        x: 1\n"), 0, "settings: unknown agent \"gpt\" (known: claude)"},
		{"settings agent not mapping", head + step("s", "    settings:\n      claude: 5\n"), 0, "settings.claude must be a mapping"},
		{"settings unknown setting", head + step("s", "    settings:\n      claude:\n        x: 1\n"), 0, "settings.claude: unknown setting \"x\""},
		{"settings below min", head + step("s", "    settings:\n      claude:\n        auto_compact_window: 99999\n"), 0, "settings.claude.auto_compact_window must be a whole number from 100000 to 1000000"},
		{"settings above max", head + step("s", "    settings:\n      claude:\n        auto_compact_window: 1000001\n"), 0, "must be a whole number from 100000 to 1000000"},
		{"settings not a number", head + step("s", "    settings:\n      claude:\n        auto_compact_window: lots\n"), 0, "must be a whole number from 100000 to 1000000"},
		{"settings on script refused", head + scriptStep("r", "    settings: {}\n"), 0, "settings is not allowed on script steps"},
		{"settings on manual without session", head + manual("u", "    settings: {}\n"), 0, "settings is not allowed on manual steps without a session"},
		{"script ok", head + scriptStep("r", ""), 0, ""},
		{"script needs command", head + "  - id: r\n    name: R\n    kind: script\n    runs_on: once\n    timeout: 5m\n", 0, "command is required"},
		{"script refuses prompt", head + scriptStep("r", "    prompt: y\n"), 0, "prompt is not allowed on script steps"},
		{"script routes editable", head + scriptStep("r", "    results:\n      - id: done\n        ok: true\n        next: end\n      - id: failed\n        ok: false\n"), 0, ""},
		{"script results fixed ids", head + scriptStep("r", "    results:\n      - id: done\n        ok: true\n      - id: broke\n        ok: false\n"), 0, "results of a script step must be done"},
		{"syntax error", "id: w\n  name: [\n", 0, "unexpected content here (check the indentation)"},
		{"empty file", "", 0, "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, werr := Parse([]byte(c.src))
			if c.msg == "" {
				if werr != nil {
					t.Fatalf("want valid, got %+v", werr)
				}
				return
			}
			if werr == nil {
				t.Fatalf("want error %q, got valid", c.msg)
			}
			if !strings.Contains(werr.Message, c.msg) {
				t.Fatalf("message %q lacks %q", werr.Message, c.msg)
			}
			if c.line != 0 && werr.Line != c.line {
				t.Fatalf("line %d, want %d (%q)", werr.Line, c.line, werr.Message)
			}
		})
	}
}

type memStore struct{ m map[string]string }

func (s *memStore) LastValid() (map[string]string, error) { return s.m, nil }
func (s *memStore) RecordLastValid(f, js string, _ int64) error {
	s.m[f] = js
	return nil
}

func (s *memStore) ForgetLastValid(f string) error {
	delete(s.m, f)
	return nil
}

func TestReader_lastValidSurvivesBreakage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "workflows")
	store := &memStore{m: map[string]string{}}
	r := &Reader{Dir: dir, Store: store}
	if got := r.List(nil); len(got.Workflows) != 0 || got.Dir != dir {
		t.Fatalf("missing dir: %+v", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("reader created the dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	good := head + manual("u", "")
	path := filepath.Join(dir, "w.yaml")
	os.WriteFile(path, []byte(good), 0o644)
	os.WriteFile(filepath.Join(dir, "ignored.yml"), []byte(good), 0o644)
	got := r.List(func(id string) int { return 2 })
	if len(got.Workflows) != 1 || got.Workflows[0].Workflow == nil || got.Workflows[0].UsedBy != 2 || got.Workflows[0].Error != nil {
		t.Fatalf("%+v", got)
	}
	os.WriteFile(path, []byte(good+"    bogus: 1\n"), 0o644)
	got = r.List(nil)
	e := got.Workflows[0]
	if e.Error == nil || e.Workflow == nil || e.Workflow.ID != "w" {
		t.Fatalf("broken file lost last valid: %+v", e)
	}
	os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(good), 0o644) // id w != stem x
	for _, e := range r.List(nil).Workflows {
		if e.FileName == "x.yaml" && e.Error == nil {
			t.Fatal("id/stem mismatch accepted")
		}
	}
	if _, ok := r.Get("w"); !ok {
		t.Fatal("Get w")
	}
}

func TestParse_returnLimits(t *testing.T) {
	loopTo := resultsBlock("a", "true", "") + resultsBlock("b", "false", "s")
	cases := []struct {
		name, src string
		want      int
	}{
		{"default", head + step("s", ""), 3},
		{"declared", head + step("s", "    max_returns: 5\n"), 5},
		{"review default is no limit", head + step("s", "") + reviewStep("r", ""), 0},
		{"script default", head + scriptStep("s", ""), 3},
		{"results without max", head + step("s", loopTo), 3},
	}
	for _, c := range cases {
		wf, werr := Parse([]byte(c.src))
		if werr != nil {
			t.Fatalf("%s: %+v", c.name, werr)
		}
		if got := wf.Steps[len(wf.Steps)-1].MaxReturns; got != c.want {
			t.Fatalf("%s: MaxReturns %d, want %d", c.name, got, c.want)
		}
	}
	two := head + step("s", "") + step("t", resultsBlock("a", "true", "")+resultsBlock("b", "false", "t")+resultsBlock("c", "false", "s"))
	wf, werr := Parse([]byte(two))
	if werr != nil {
		t.Fatalf("%+v", werr)
	}
	if r := wf.Steps[1].Results; !r[1].Loop || !r[2].Loop || r[0].Loop {
		t.Fatalf("loop flags: %+v", r)
	}
}
