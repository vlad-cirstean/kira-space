package apiflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Contract api-agent-access, read by tests/ui/api-agent-access.spec.ts: an environment file shares
// variables with agents through `agent`/`agentHosts`, the overview lists names and hosts only, and a
// misplaced or missing host list is a file problem with a line. The edits recorded are the ones the
// Agent access tab sends.
func TestApiAgentAccess(t *testing.T) {
	app := studioflowharness.New(t)
	const sc = "api-agent-access"
	const envFile = "staging.environment.yaml"
	cfg := app.W.ApiConfig
	mask := studioflowharness.Mask("hash", "baseHash")
	paths := studioflowharness.Replace(app.KiraHome, "<home>")

	seed := "name: Staging\nvariables:\n  - name: base_url\n    value: https://staging.example.com\n  - name: token\n    secret: ${secret:staging.token}\n"
	app.WriteAPIFile(t, envFile, []byte(seed))
	if got, err := cfg.AgentAccess(); err != nil || len(got) != 0 {
		t.Fatalf("AgentAccess before sharing = %+v, %v, want none", got, err)
	}
	view, err := cfg.File(bridge.ApiFileArgs{File: envFile})
	if err != nil || view.Problem != nil {
		t.Fatalf("File = %+v, %v", view, err)
	}
	app.Contract(t, sc, "file#read", view, mask, paths)

	call := func(name, text string, edits []configfs.Edit) configfs.FormResult {
		t.Helper()
		args := bridge.ApiFormArgs{File: envFile, Text: text, Edits: edits}
		app.Contract(t, sc, "args:ApiConfigService.Form#"+name, args, mask, paths)
		res, err := cfg.Form(args)
		if err != nil {
			t.Fatalf("Form %s: %v", name, err)
		}
		app.Contract(t, sc, "form#"+name, res, mask, paths)
		return res
	}
	variables := func(token map[string]any) []configfs.Edit {
		return []configfs.Edit{{Path: []string{"variables"}, Value: []any{
			map[string]any{"name": "base_url", "value": "https://staging.example.com"}, token,
		}}}
	}

	call("read", seed, []configfs.Edit{})
	shared := call("share", seed, variables(map[string]any{
		"name": "token", "secret": "${secret:staging.token}", "agent": true, "agentHosts": []any{"staging.example.com"},
	}))
	if shared.Problem != nil || !strings.Contains(shared.Text, "agentHosts") {
		t.Fatalf("share = %+v, want agent keys in the text", shared)
	}
	noHosts := call("no-hosts", shared.Text, variables(map[string]any{
		"name": "token", "secret": "${secret:staging.token}", "agent": true,
	}))
	if noHosts.Problem == nil || noHosts.Problem.Line == 0 || !strings.Contains(strings.ToLower(noHosts.Problem.Message), "host") {
		t.Fatalf("no-hosts problem = %+v, want one about hosts with a line", noHosts.Problem)
	}
	reads := call("reads", seed, []configfs.Edit{{Path: []string{"agentAutoApproveReads"}, Value: true}})
	if reads.Problem != nil || !strings.Contains(reads.Text, "agentAutoApproveReads: true") {
		t.Fatalf("reads = %+v", reads)
	}
	yamlSide := strings.Replace(seed, "    value: https://staging.example.com\n", "    value: https://staging.example.com\n    agent: true\n", 1)
	if res := call("yaml", yamlSide, []configfs.Edit{}); res.Problem != nil || res.Text != yamlSide {
		t.Fatalf("yaml = %+v", res)
	}
	plain := call("hosts-on-plain", strings.Replace(seed, "    value: https://staging.example.com\n", "    value: https://staging.example.com\n    agentHosts: [a.com]\n", 1), []configfs.Edit{})
	if plain.Problem == nil || !strings.Contains(strings.ToLower(plain.Problem.Message), "host") {
		t.Fatalf("hosts-on-plain problem = %+v", plain.Problem)
	}

	saveArgs := bridge.ApiSaveFileArgs{File: envFile, Text: shared.Text, BaseHash: view.Hash}
	app.Contract(t, sc, "args:ApiConfigService.SaveFile", saveArgs, mask, paths)
	saved, err := cfg.SaveFile(saveArgs)
	if err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v, %v", saved, err)
	}
	app.Contract(t, sc, "file#saved", saved, mask, paths)

	access, err := cfg.AgentAccess()
	if err != nil || len(access) != 1 {
		t.Fatalf("AgentAccess = %+v, %v, want Staging", access, err)
	}
	e := access[0]
	if e.Name != "Staging" || e.AutoApproveReads || len(e.Variables) != 1 || e.Variables[0].Name != "token" || !e.Variables[0].Secret || len(e.Variables[0].Hosts) != 1 {
		t.Fatalf("AgentAccess[0] = %+v, want token with one host", e)
	}
	app.Contract(t, sc, "AgentAccess", access, mask, paths)

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
