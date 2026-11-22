package dbmcpflow

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-prepared-visible.spec.ts (contract e2e-prepared-visible): a prepare script
// sets arguments of a kube list, a kube logs read and an API request. The card payload lists exactly the keys
// the script set, and every final request field holds the value the script returned. The UI half shows each one.

const preparedKey = "e2e-prepared-visible"

type preparedKube struct {
	Changed []string
	Request struct {
		Namespace, LabelSelector, Pod, Container string
		Previous, Timestamps                     bool
		Tail, MaxRows                            int
	}
}

type preparedAPI struct {
	Method, URL string
	Query       []struct{ Name, Value string }
	Headers     []struct{ Name, Value string }
	Body        *struct{ ContentType, Text string }
	Script      struct{ Prepare struct{ Changed []string } }
}

func TestE2EPreparedVisible(t *testing.T) {
	captureLog(t)
	k := newKubeFixture(t)
	srv, apiSess, envID := k.apiSetup(t, "127.0.0.1", false)
	k.setAPI(t, true, "always")
	k.setMcp(t, "kind-dev", on("prompt"))

	contract := func(t *testing.T, key string, got any, extra ...flowharness.ContractOption) {
		t.Helper()
		opts := append([]flowharness.ContractOption{
			flowharness.Replace(k.fake.URL, "<kube>"), flowharness.Replace(k.cfg, "<kubeconfig>"),
			flowharness.Mask("id", "clusterId", "requestId", "connectionId", "expiresAtMs", "expiresAt", "createdAt", "startedAt", "durationMs", "cwd", "hash"),
		}, extra...)
		k.app.Contract(t, preparedKey, key, got, opts...)
	}
	deny := func(t *testing.T, id string) {
		t.Helper()
		k.answerItems(t, false, id)
	}

	t.Run("a list prepare sets the namespace, selector and row cap", func(t *testing.T) {
		args := k.listPods("default", map[string]any{"script": map[string]any{"prepare": `return {namespace: "kube-system", labelSelector: "app=web", maxRows: 7};`}})
		pending := k.callAsync("kube.list", args)
		req := k.pending(t)
		var c preparedKube
		if err := json.Unmarshal(req.Item.Payload, &c); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(c.Changed, []string{"namespace", "labelSelector", "maxRows"}) || c.Request.Namespace != "kube-system" || c.Request.LabelSelector != "app=web" || c.Request.MaxRows != 7 {
			t.Fatalf("card %+v, want the three set keys and their values", c)
		}
		contract(t, "ApprovalsService.List#list", []approvals.Item{req.Item})
		deny(t, req.Item.ID)
		<-pending
	})

	t.Run("a logs prepare sets the pod, container and flags", func(t *testing.T) {
		args := k.logsScript(map[string]any{"prepare": `return {name: "web-1", container: "sidecar", tail: 3, previous: false, timestamps: true};`})
		pending := k.callAsync("kube.logs", args)
		req := k.pending(t)
		var c preparedKube
		if err := json.Unmarshal(req.Item.Payload, &c); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(c.Changed, []string{"name", "container", "previous", "timestamps", "tail"}) || c.Request.Pod != "web-1" || c.Request.Container != "sidecar" || c.Request.Previous || !c.Request.Timestamps || c.Request.Tail != 3 {
			t.Fatalf("card %+v, want the five set keys and their values", c)
		}
		contract(t, "ApprovalsService.List#logs", []approvals.Item{req.Item})
		deny(t, req.Item.ID)
		<-pending
	})

	t.Run("an API prepare sets the request and a JSON body names its content type", func(t *testing.T) {
		script := `return {method: "POST", url: "` + srv.URL + `/echo?a=1", headers: [{name: "X-Prepared", value: "yes"}], query: [{name: "q", value: "2"}], body: {items: [1, 2]}};`
		pending := callToolAsync(apiSess, "api_request", scriptArgs(envID, "GET", srv.URL+"/echo", map[string]any{"prepare": script}))
		req := k.pending(t)
		var c preparedAPI
		if err := json.Unmarshal(req.Item.Payload, &c); err != nil {
			t.Fatal(err)
		}
		want := []string{"method", "url", "headers", "query", "body", "contentType"}
		if !slices.Equal(c.Script.Prepare.Changed, want) || c.Method != "POST" || c.Body == nil || c.Body.ContentType != "application/json" ||
			!reflect.DeepEqual(c.Headers[0], struct{ Name, Value string }{"X-Prepared", "yes"}) || c.Query[len(c.Query)-1].Name != "q" {
			t.Fatalf("card %+v, want the set keys, POST, the header, the query entry and a JSON content type", c)
		}
		contract(t, "ApprovalsService.List#api", []approvals.Item{req.Item}, flowharness.Replace(srv.URL, "<api>"))
		deny(t, req.Item.ID)
		wait(t, pending)
	})
}
