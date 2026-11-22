package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-kube-script.spec.ts (contract e2e-kube-script): a kube list keeps a pod
// name and a token, the agent's next kube_pod_logs reads the pod in its prepare script, an API request reads
// it with {{kept.pod}}, and a reviewed scripted read stages its keeps until Send.

const (
	kubeScriptKey = "e2e-kube-script"
	kubeListKeep  = `kira.storeValue("pod", result.rows[0].name); kira.storeSecret("token", "tok-kube-" + result.rows[0].name); return {first: result.rows[0].name, count: result.rowCount};`
	kubeLogsPrep  = `return {pod: kira.getValue("pod"), tail: 2};`
	kubeLogsTrans = `return result.lines.length;`
)

type kubeScriptCard struct {
	Tool    string
	Changed []string
	Request struct {
		Namespace, Pod, Container string
		Tail                      int
	}
	Kept []struct {
		Name, Preview string
		Secret        bool
	}
	Script struct {
		Prepare   *struct{ Source string }
		Transform struct {
			MayKeep []struct {
				Name   string
				Secret bool
			}
		}
	}
}

func decodeKubeCard(t *testing.T, it approvals.Item) kubeScriptCard {
	t.Helper()
	var c kubeScriptCard
	if err := json.Unmarshal(it.Payload, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (k *kubeFixture) scriptContract(t *testing.T, key string, got any, extra ...flowharness.ContractOption) {
	t.Helper()
	opts := append([]flowharness.ContractOption{
		flowharness.Replace(k.fake.URL, "<kube>"), flowharness.Replace(k.cfg, "<kubeconfig>"),
		flowharness.Mask("id", "clusterId", "requestId", "connectionId", "expiresAtMs", "expiresAt", "createdAt", "startedAt", "durationMs", "cwd"),
	}, extra...)
	k.app.Contract(t, kubeScriptKey, key, got, opts...)
}

func (k *kubeFixture) scriptsSwitch(t *testing.T, on bool) {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{APIScripts: &on}}}
	if _, err := k.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
}

func (k *kubeFixture) logsScript(script map[string]any) map[string]any {
	args := map[string]any{"clusterId": k.ids["kind-dev"], "namespace": "default", "container": "app"}
	args["script"] = script
	return args
}

func TestE2EKubeScript(t *testing.T) {
	captureLog(t)
	k := newKubeFixture(t)
	srv, apiSess, envID := k.apiSetup(t, "127.0.0.1", false)
	k.setAPI(t, true, "always")
	k.setMcp(t, "kind-dev", on("prompt"))
	var agent []string
	remember := func(res string) { agent = append(agent, res) }

	t.Run("a list keeps a plain pod and a secret token", func(t *testing.T) {
		args := k.listPods("default", map[string]any{"script": map[string]any{"transform": kubeListKeep}})
		pending := k.callAsync("kube_list_resources", args)
		req := k.pending(t)
		if req.Item.Kind != approvals.KindKubeRead {
			t.Fatalf("raised %s, want kube.read", req.Item.Kind)
		}
		card := decodeKubeCard(t, req.Item)
		mk := card.Script.Transform.MayKeep
		if card.Tool != "kube_list_resources" || len(mk) != 2 || mk[0].Name != "pod" || mk[0].Secret || mk[1].Name != "token" || !mk[1].Secret {
			t.Fatalf("card %+v, want pod plain and token secret in mayKeep", card)
		}
		k.scriptContract(t, "ApprovalsService.List#list", []approvals.Item{req.Item})
		k.scriptContract(t, "args:ApprovalsService.Answer#list", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		k.answerItems(t, true, req.Item.ID)
		out := <-pending
		if out.err != nil || out.res.IsError {
			t.Fatalf("list = %q (%v)", text(out.res), out.err)
		}
		remember(text(out.res))
		got := text(out.res)
		if !strings.Contains(got, `"kind":"script"`) || !strings.Contains(got, `"first":"web-1"`) || !strings.Contains(got, `"name":"token","type":"string","items":0,"secret":true`) || strings.Contains(got, "tok-kube-web-1") {
			t.Fatalf("envelope = %s, want the script output and kept summaries without the token", got)
		}
		k.scriptContract(t, "result#list", got)
		pod, ok := k.keptValue(t, "pod")
		if !ok || pod.Secret || pod.Source.MCP != "kube" || string(pod.Value) != `"web-1"` {
			t.Fatalf("pod = %+v ok=%v, want a plain kube-sourced value", pod.Meta, ok)
		}
		tok, ok := k.keptValue(t, "token")
		if !ok || !tok.Secret || tok.Origin != "" || string(tok.Value) != `"tok-kube-web-1"` {
			t.Fatalf("token = %+v ok=%v, want a secret bound to no origin", tok.Meta, ok)
		}
		k.scriptOpRow(t, "jsTransform", kubeListKeep)
	})

	t.Run("logs read the kept pod in prepare", func(t *testing.T) {
		before := len(k.fake.Requests("/log"))
		pending := k.callAsync("kube_pod_logs", k.logsScript(map[string]any{"prepare": kubeLogsPrep, "transform": kubeLogsTrans}))
		req := k.pending(t)
		card := decodeKubeCard(t, req.Item)
		if len(card.Changed) != 2 || card.Changed[0] != "pod" || card.Changed[1] != "tail" || card.Request.Pod != "web-1" || card.Request.Tail != 2 || len(card.Kept) != 1 || card.Kept[0].Name != "pod" || card.Kept[0].Secret || card.Kept[0].Preview != `"web-1"` {
			t.Fatalf("card %+v, want the prepared pod, tail and one plain kept read", card)
		}
		k.scriptContract(t, "ApprovalsService.List#logs", []approvals.Item{req.Item}, flowharness.Replace(k.keptID(t, externalScope, "pod"), "kv_pod"))
		k.scriptContract(t, "args:ApprovalsService.Answer#logs", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		k.answerItems(t, true, req.Item.ID)
		out := <-pending
		if out.err != nil || out.res.IsError || !strings.Contains(text(out.res), `"output":2`) {
			t.Fatalf("logs = %q (%v), want the transform's line count", text(out.res), out.err)
		}
		remember(text(out.res))
		reqs := k.fake.Requests("/log")
		if len(reqs) != before+1 || !strings.Contains(reqs[len(reqs)-1].Path, "/pods/web-1/log") {
			t.Fatalf("log requests %+v, want one GET on web-1", reqs)
		}
		k.scriptContract(t, "result#logs", text(out.res))
	})

	t.Run("an API request reads the kube value", func(t *testing.T) {
		pending := callToolAsync(apiSess, "api_request", apiArgs(envID, "GET", srv.URL+"/echo?pod={{kept.pod}}"))
		req := k.pending(t)
		k.scriptContract(t, "ApprovalsService.List#api", []approvals.Item{req.Item}, flowharness.Replace(k.keptID(t, externalScope, "pod"), "kv_pod"), flowharness.Replace(srv.URL, "<api>"))
		k.scriptContract(t, "args:ApprovalsService.Answer#api", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		k.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		remember(text(res))
		seen := srv.Requests()
		if res.IsError || !strings.Contains(seen[len(seen)-1].Query, "pod=web-1") {
			t.Fatalf("api = %q, server saw %+v, want pod=web-1", text(res), seen[len(seen)-1])
		}
	})

	t.Run("the kube secret reaches an API header and no SQL", func(t *testing.T) {
		pending := callToolAsync(apiSess, "api_request", apiArgs(envID, "GET", srv.URL+"/echo", "Authorization", "Bearer {{kept.token}}"))
		req := k.pending(t)
		k.answerItems(t, true, req.Item.ID)
		res := wait(t, pending)
		remember(text(res))
		seen := srv.Requests()
		if res.IsError || seen[len(seen)-1].Header.Get("Authorization") != "Bearer tok-kube-web-1" {
			t.Fatalf("api = %q, want the token sent in the header", text(res))
		}
		k.noLeak(t, "tok-kube-web-1", agent...)
	})

	t.Run("an API-kept value is read back in kube prepare", func(t *testing.T) {
		k.keep(t, externalScope, "ns", `"default"`)
		pending := k.callAsync("kube_pod_logs", map[string]any{
			"clusterId": k.ids["kind-dev"], "namespace": "other", "pod": "web-1", "container": "app",
			"script": map[string]any{"prepare": `return {namespace: kira.getValue("ns")};`},
		})
		req := k.pending(t)
		if c := decodeKubeCard(t, req.Item); len(c.Changed) != 1 || c.Changed[0] != "namespace" || c.Request.Namespace != "default" {
			t.Fatalf("card %+v, want the namespace set by prepare", c)
		}
		k.answerItems(t, true, req.Item.ID)
		if out := <-pending; out.err != nil || out.res.IsError {
			t.Fatalf("logs = %q (%v)", text(out.res), out.err)
		}
	})

	t.Run("prepare clamps tail", func(t *testing.T) {
		k.setMcp(t, "kind-dev", on("allow"))
		res := k.call(t, "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: "web-1", tail: 9000};`, "transform": kubeLogsTrans}))
		if res.IsError || k.queueLen(t) != 0 {
			t.Fatalf("logs = %q, queue %d, want a read under allow with no card", text(res), k.queueLen(t))
		}
		k.setMcp(t, "kind-dev", on("prompt"))
	})

	t.Run("refusals read nothing", func(t *testing.T) {
		k.keep(t, externalScope, "typed", `"typed-by-user"`)
		if _, err := k.app.W.Kept.Put(keptPutUser("usertok", "u-secret-77")); err != nil {
			t.Fatal(err)
		}
		cases := []struct {
			name, tool string
			args       map[string]any
			want       string
		}{
			{"empty", "kube_pod_logs", k.logsScript(map[string]any{}), "script"},
			{"keepInPrepare", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `kira.storeValue("x", 1); return {pod: "web-1"};`}), "transform script only"},
			{"secretRead", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: kira.getValue("token")};`}), `kept variable "token" is secret or was typed by the user`},
			{"userTypedRead", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: kira.getValue("usertok")};`}), `kept variable "usertok" is secret or was typed by the user`},
			{"unknownKept", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: kira.getValue("nope")};`}), `"nope"`},
			{"clusterKey", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {clusterId: "x", pod: "web-1"};`}), "unknown key"},
			{"unknownKey", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {bogus: 1, pod: "web-1"};`}), "unknown key"},
			{"wrongType", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: "web-1", tail: "ten"};`}), "tail"},
			{"emptied", "kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: ""};`}), "cannot be empty"},
			{"syntax", "kube_list_resources", k.listPods("default", map[string]any{"script": map[string]any{"transform": "return ("}}), "syntax error"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				before := len(k.fake.Requests(""))
				res := k.call(t, c.tool, c.args)
				wantToolError(t, res, c.want)
				remember(text(res))
				if n := len(k.fake.Requests("")) - before; n != 0 || k.queueLen(t) != 0 {
					t.Fatalf("%d requests, queue %d, want nothing read and no card", n, k.queueLen(t))
				}
			})
		}
		k.noLeak(t, "tok-kube-web-1", agent...)
		k.noLeak(t, "u-secret-77", agent...)
	})

	t.Run("scripts off", func(t *testing.T) {
		k.scriptsSwitch(t, false)
		defer k.scriptsSwitch(t, true)
		res := k.call(t, "kube_pod_logs", k.logsScript(map[string]any{"prepare": kubeLogsPrep}))
		wantToolError(t, res, "script")
	})

	t.Run("scripts turned off during the card", func(t *testing.T) {
		pending := k.callAsync("kube_pod_logs", k.logsScript(map[string]any{"prepare": kubeLogsPrep, "transform": kubeLogsTrans}))
		req := k.pending(t)
		before := len(k.fake.Requests("/log"))
		k.scriptsSwitch(t, false)
		k.answerItems(t, true, req.Item.ID)
		out := <-pending
		if out.err != nil {
			t.Fatal(out.err)
		}
		wantToolError(t, out.res, "Nothing was read.")
		k.scriptsSwitch(t, true)
		if n := len(k.fake.Requests("/log")) - before; n != 0 {
			t.Fatalf("%d log reads, want none", n)
		}
	})

	t.Run("a kept value that vanishes cancels the card", func(t *testing.T) {
		k.keep(t, externalScope, "gone", `"web-1"`)
		pending := k.callAsync("kube_pod_logs", k.logsScript(map[string]any{"prepare": `return {pod: kira.getValue("gone")};`}))
		k.pending(t)
		k.dropKept("gone")
		out := <-pending
		if out.err != nil {
			t.Fatal(out.err)
		}
		wantToolError(t, out.res, "nothing was read")
		if n := k.queueLen(t); n != 0 {
			t.Fatalf("queue %d, want the card withdrawn", n)
		}
	})

	t.Run("a reviewed scripted read stages its keeps until Send", func(t *testing.T) {
		k.setMcp(t, "kind-dev", review("review"))
		for _, n := range []string{"pod", "token", "ns", "typed", "usertok"} {
			k.dropKept(n)
		}
		args := k.listPods("default", map[string]any{"script": map[string]any{"transform": kubeListKeep}})
		pending, req, held := k.held(t, "kube_list_resources", args)
		p := decodeKubeResponse(t, req.Item)
		if p.Operation != "list" || held.Raw == "" || held.Script == nil || len(held.Keep) != 2 || held.Keep[0].Name != "pod" || held.Keep[0].Secret || held.Keep[0].Preview != `"web-1"` || !held.Keep[1].Secret || held.Keep[1].Preview != "" {
			t.Fatalf("payload %+v held %+v, want the envelope, raw, script and two staged keeps", p, held)
		}
		if strings.Contains(held.Text, "tok-kube-web-1") || strings.Contains(held.Raw, "tok-kube-web-1") || !strings.Contains(held.Text, `"first":"web-1"`) {
			t.Fatalf("held text %q, want the output without the token", held.Text)
		}
		if names := k.keptNames(externalScope); len(names) != 0 {
			t.Fatalf("kept store = %v before Send, want empty", names)
		}
		sized := []flowharness.ContractOption{flowharness.Mask("rawBytes", "bytes")}
		k.scriptContract(t, "ApprovalsService.List#review", []approvals.Item{req.Item}, sized...)
		k.scriptContract(t, "DbMcpService.PendingResponse#review", held)
		reveal := func(name string, confirmed bool) (bridge.KeptReveal, error) {
			return k.app.W.KeptVars.RevealStaged(bridge.KeptRevealStagedArgs{RequestID: req.RequestID, Name: name, Confirmed: confirmed})
		}
		if _, err := reveal("pod", true); err == nil {
			t.Fatal("revealing a plain staged value succeeded, want a refusal")
		}
		first, err := reveal("token", false)
		if err != nil || first.Outcome != apivars.OutcomeConfirmationRequired || first.Value != nil {
			t.Fatalf("unconfirmed reveal = %+v (%v), want confirmation required", first, err)
		}
		shown, err := reveal("token", true)
		if err != nil || shown.Outcome != apivars.OutcomeRevealed || shown.Value == nil || *shown.Value != "tok-kube-web-1" {
			t.Fatalf("confirmed reveal = %+v (%v), want the token", shown, err)
		}
		k.scriptContract(t, "args:KeptVarsService.RevealStaged#review", bridge.KeptRevealStagedArgs{RequestID: "<request>", Name: "token"})
		k.scriptContract(t, "KeptVarsService.RevealStaged#confirmation", first)
		k.scriptContract(t, "KeptVarsService.RevealStaged#review", shown)
		k.scriptContract(t, "args:ApprovalsService.Answer#review", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})

		k.answerItems(t, true, req.Item.ID)
		out := <-pending
		if out.err != nil || out.res.IsError || !strings.Contains(text(out.res), `"output":{"first":"web-1","count":3}`) || !strings.Contains(text(out.res), `"name":"token","type":"string","items":0,"secret":true`) || strings.Contains(text(out.res), "tok-kube-web-1") {
			t.Fatalf("sent = %q (%v), want the held output, kept summaries and no token", text(out.res), out.err)
		}
		if names := k.keptNames(externalScope); len(names) != 2 {
			t.Fatalf("kept store = %v after Send, want pod and token", names)
		}
		if _, err := reveal("token", true); err == nil {
			t.Fatal("staged reveal after Send succeeded, want a refusal")
		}
	})

	t.Run("Withhold and timeout store nothing", func(t *testing.T) {
		for _, n := range []string{"pod", "token"} {
			k.dropKept(n)
		}
		args := k.listPods("default", map[string]any{"script": map[string]any{"transform": kubeListKeep}})
		pending, req, _ := k.held(t, "kube_list_resources", args)
		k.answerItems(t, false, req.Item.ID)
		out := <-pending
		if out.err != nil || out.res == nil {
			t.Fatalf("withheld call = %v", out.err)
		}
		if names := k.keptNames(externalScope); len(names) != 0 {
			t.Fatalf("kept store = %v after Withhold, want empty", names)
		}
		k.noLeak(t, "tok-kube-web-1", text(out.res))
	})
}

func keptPutUser(name, value string) keptvars.Put {
	return keptvars.Put{Scope: externalScope, Name: name, Value: json.RawMessage(`"` + value + `"`), Secret: true,
		Source: keptvars.Source{Kind: keptvars.SourceUser, MCP: "api", Tool: "api_request", Request: "input " + name, Client: "claude-code"}}
}

func (k *kubeFixture) dropKept(name string) {
	for _, m := range k.app.W.Kept.List(externalScope) {
		if m.Name == name {
			k.app.W.Kept.Clear(m.ID)
		}
	}
}
