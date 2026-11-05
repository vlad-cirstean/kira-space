package apiflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Contract api-agent-access, read by tests/ui/api-agent-access.spec.ts: an environment file shares
// variables with agents through `agent`/`agentHosts`, the overview lists names and hosts only, and a
// misplaced or missing host list is a file problem with a line.
func TestApiAgentAccess(t *testing.T) {
	app := flowharness.New(t)
	const sc = "api-agent-access"
	const envFile = "staging.environment.yaml"
	cfg := app.W.ApiConfig
	mask := flowharness.Mask("hash", "baseHash", "id")
	paths := flowharness.Replace(app.KiraHome, "<home>")

	seed := "name: Staging\nvariables:\n  - name: base_url\n    value: https://staging.example.com\n  - name: token\n    secret: ${secret:staging.token}\n"
	app.WriteAPIFile(t, envFile, []byte(seed))
	if got, err := cfg.AgentAccess(); err != nil || len(got) != 0 {
		t.Fatalf("AgentAccess before sharing = %+v, %v, want none", got, err)
	}

	edits := []configfs.Edit{
		{Path: []string{"agentAutoApproveReads"}, Value: true},
		{Path: []string{"variables"}, Value: []any{
			map[string]any{"name": "base_url", "value": "https://staging.example.com", "agent": true},
			map[string]any{"name": "token", "secret": "${secret:staging.token}", "agent": true, "agentHosts": []any{"staging.example.com", "*.example.org"}},
		}},
	}
	args := bridge.ApiFormArgs{File: envFile, Text: seed, Edits: edits}
	app.Contract(t, sc, "args:ApiConfigService.Form", args, mask, paths)
	form, err := cfg.Form(args)
	if err != nil || form.Problem != nil {
		t.Fatalf("Form = %+v, %v", form, err)
	}
	view, err := cfg.File(bridge.ApiFileArgs{File: envFile})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cfg.SaveFile(bridge.ApiSaveFileArgs{File: envFile, Text: form.Text, BaseHash: view.Hash})
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", saved, err)
	}

	access, err := cfg.AgentAccess()
	if err != nil || len(access) != 1 {
		t.Fatalf("AgentAccess = %+v, %v, want Staging", access, err)
	}
	e := access[0]
	if e.Name != "Staging" || !e.AutoApproveReads || len(e.Variables) != 2 || !e.Variables[1].Secret || len(e.Variables[1].Hosts) != 2 {
		t.Fatalf("AgentAccess[0] = %+v", e)
	}
	app.Contract(t, sc, "AgentAccess", access, mask, paths)

	problem := func(name, text, want string) {
		t.Helper()
		res, err := cfg.Form(bridge.ApiFormArgs{File: envFile, Text: text, Edits: []configfs.Edit{}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Problem == nil || res.Problem.Line == 0 || !strings.Contains(strings.ToLower(res.Problem.Message), want) {
			t.Errorf("%s problem = %+v, want one mentioning %q", name, res.Problem, want)
		}
		app.Contract(t, sc, "problem#"+name, res.Problem, mask, paths)
	}
	problem("no-hosts", strings.Replace(seed, "    secret: ${secret:staging.token}\n", "    secret: ${secret:staging.token}\n    agent: true\n", 1), "host")
	problem("hosts-on-plain", strings.Replace(seed, "    value: https://staging.example.com\n", "    value: https://staging.example.com\n    agentHosts: [a.com]\n", 1), "host")

	col := newCollection(t, app, "Orders API")
	colView, err := cfg.File(bridge.ApiFileArgs{File: col.File})
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(colView.Text, "variables:", "variables:\n  - name: x\n    value: y\n    agent: true", 1)
	if bad == colView.Text {
		bad = colView.Text + "variables:\n  - name: x\n    value: y\n    agent: true\n"
	}
	res, err := cfg.Form(bridge.ApiFormArgs{File: col.File, Text: bad, Edits: []configfs.Edit{}})
	if err != nil || res.Problem == nil {
		t.Fatalf("collection with agent = %+v, %v, want a schema problem", res, err)
	}
}
