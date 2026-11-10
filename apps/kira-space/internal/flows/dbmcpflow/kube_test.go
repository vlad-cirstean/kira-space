package dbmcpflow

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const kubeScenario = "dbmcp-kube"

type kubeFixture struct {
	*fixture
	fake *flowharness.KubeFake
	cfg  string
	ids  map[string]string
	sess *mcp.ClientSession
}

func newKubeFixture(t *testing.T) *kubeFixture {
	t.Helper()
	fake := flowharness.NewKubeFake(t)
	f := newFixture(t)
	k := &kubeFixture{fixture: f, fake: fake, cfg: filepath.Join(f.app.Home, "kubeconfig"), ids: map[string]string{}}
	fake.WriteKubeconfig(t, k.cfg, flowharness.KubeconfigOpts{BinDir: f.app.BinDir})
	t.Setenv("KUBECONFIG", k.cfg)
	res, err := f.app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Clusters {
		k.ids[c.Context] = c.ID
	}
	t.Cleanup(func() {
		for _, r := range fake.Requests("") {
			if r.Method != http.MethodGet {
				t.Errorf("non-GET request reached the API server: %s %s", r.Method, r.Path)
			}
		}
	})
	k.sess, err = f.connect(t, f.token(t))
	if err != nil {
		t.Fatalf("connect with the helper token: %v", err)
	}
	t.Cleanup(func() { _ = k.sess.Close() })
	return k
}

func (k *kubeFixture) contract(t *testing.T, key string, got any, extra ...flowharness.ContractOption) {
	t.Helper()
	opts := append([]flowharness.ContractOption{
		flowharness.Replace(k.fake.URL, "<kube>"), flowharness.Replace(k.fake.DownURL(), "<down>"),
		flowharness.Replace(k.cfg, "<kubeconfig>"),
		flowharness.Mask("id", "clusterId", "requestId", "connectionId", "expiresAtMs", "startedAt", "durationMs"),
	}, extra...)
	k.app.Contract(t, kubeScenario, key, got, opts...)
}

func (k *kubeFixture) setMcp(t *testing.T, ctxName string, m kubernetes.McpSettings) {
	t.Helper()
	if _, err := k.app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: k.ids[ctxName], Mcp: m}); err != nil {
		t.Fatal(err)
	}
}

func on(approval string) kubernetes.McpSettings {
	return kubernetes.McpSettings{Enabled: true, Approval: approval}
}

func (k *kubeFixture) call(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := k.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

type kubeOutcome struct {
	res *mcp.CallToolResult
	err error
}

func (k *kubeFixture) callAsync(name string, args map[string]any) chan kubeOutcome {
	ch := make(chan kubeOutcome, 1)
	go func() {
		res, err := k.sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		ch <- kubeOutcome{res, err}
	}()
	return ch
}

func (k *kubeFixture) listPods(namespace string, extra map[string]any) map[string]any {
	args := map[string]any{"clusterId": k.ids["kind-dev"], "kind": "v1/pods", "namespace": namespace}
	for key, v := range extra {
		args[key] = v
	}
	return args
}

func (k *kubeFixture) podRequests() int { return len(k.fake.Requests("/pods")) }

func wantToolError(t *testing.T, res *mcp.CallToolResult, sub string) {
	t.Helper()
	if !res.IsError || !strings.Contains(text(res), sub) {
		t.Fatalf("result = %q (error %v), want a tool error containing %q", text(res), res.IsError, sub)
	}
}

func TestKubeMcp(t *testing.T) {
	k := newKubeFixture(t)
	dev := k.ids["kind-dev"]

	t.Run("defaults-and-settings", func(t *testing.T) {
		res, err := k.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range res.Clusters {
			if c.Mcp != (kubernetes.McpSettings{Approval: "prompt"}) {
				t.Fatalf("%s mcp = %+v, want off, ask and no description", c.Context, c.Mcp)
			}
		}
		k.contract(t, "KubernetesService.Clusters", res)

		for _, bad := range []kubernetes.McpSettings{
			{Enabled: true, Approval: "bogus"},
			{Enabled: true, Approval: "deny"},
			{Enabled: true, Approval: ""},
			{Enabled: true, Approval: "allow", Description: strings.Repeat("é", 1001)},
		} {
			_, err := k.app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: dev, Mcp: bad})
			if ie, ok := err.(*ipcerr.Error); !ok || ie.Code != string(ipcerr.CodeBadRequest) {
				t.Fatalf("SetMcp(%+v) error = %v, want E_BAD_REQUEST", bad, err)
			}
		}
		if _, err := k.app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: "nope", Mcp: on("allow")}); err == nil {
			t.Fatal("SetMcp on an unknown cluster succeeded")
		} else if ie, ok := err.(*ipcerr.Error); !ok || ie.Code != string(ipcerr.CodeNotFound) {
			t.Fatalf("SetMcp unknown id error = %v, want E_NOT_FOUND", err)
		}

		// The Settings switch: turn the cluster on, keep the default approval.
		enable := kubernetes.SetMcpArgs{ClusterID: dev, Mcp: on("prompt")}
		if got, err := k.app.W.Kubernetes.SetMcp(enable); err != nil || got != enable.Mcp {
			t.Fatalf("SetMcp enable = %+v (%v), want the stored settings", got, err)
		}
		k.contract(t, "args:KubernetesService.SetMcp:enable", enable)

		// The cluster dialog: description and approval.
		args := kubernetes.SetMcpArgs{ClusterID: dev, Mcp: kubernetes.McpSettings{
			Description: "dev cluster, safe to read", Enabled: true, Approval: "allow",
		}}
		got, err := k.app.W.Kubernetes.SetMcp(args)
		if err != nil || got != args.Mcp {
			t.Fatalf("SetMcp = %+v (%v), want the stored settings", got, err)
		}
		k.contract(t, "args:KubernetesService.SetMcp", args)
		k.contract(t, "KubernetesService.SetMcp", got)
		enabled, err := k.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		k.contract(t, "KubernetesService.Clusters:enabled", enabled)

		k.app.Restart()
		again, err := k.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range again.Clusters {
			want := kubernetes.McpSettings{Approval: "prompt"}
			if c.ID == dev {
				want = args.Mcp
			}
			if c.Mcp != want {
				t.Fatalf("%s mcp after restart = %+v, want %+v", c.Context, c.Mcp, want)
			}
		}
		k.sess.Close()
		if k.sess, err = k.connect(t, k.token(t)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("list-clusters", func(t *testing.T) {
		res := k.call(t, "kube_list_clusters", nil)
		out := text(res)
		if res.IsError || !strings.Contains(out, dev) || !strings.Contains(out, "dev cluster, safe to read") ||
			!strings.Contains(out, `"approval":"allow"`) {
			t.Fatalf("kube_list_clusters = %q", out)
		}
		for _, hidden := range []string{`"permissions"`, `"user"`, `"source"`, k.cfg, k.ids["down"]} {
			if strings.Contains(out, hidden) {
				t.Errorf("kube_list_clusters leaks %q: %s", hidden, out)
			}
		}
	})

	t.Run("off-is-unknown", func(t *testing.T) {
		// Same text for an off cluster and an unknown id, on every tool.
		toolArgs := func(name, id string) map[string]any {
			switch name {
			case "kube_discover":
				return map[string]any{"clusterId": id}
			case "kube_list_resources":
				return map[string]any{"clusterId": id, "kind": "v1/pods"}
			case "kube_describe":
				return map[string]any{"clusterId": id, "kind": "v1/pods", "name": "web-1"}
			default:
				return map[string]any{"clusterId": id, "namespace": "default", "pod": "web-1"}
			}
		}
		tools := []string{"kube_discover", "kube_list_resources", "kube_describe", "kube_pod_logs"}
		wantNoCluster := func(id string, args func(name string) map[string]any) {
			for _, name := range tools {
				res := k.call(t, name, args(name))
				want := fmt.Sprintf("no cluster %q exposed to MCP", id)
				if !res.IsError || text(res) != want {
					t.Fatalf("%s on %q = %q (error %v), want %q", name, id, text(res), res.IsError, want)
				}
			}
		}
		for _, id := range []string{k.ids["down"], "missing"} {
			wantNoCluster(id, func(name string) map[string]any { return toolArgs(name, id) })
		}

		requests := len(k.fake.Requests(""))
		k.setMcp(t, "kind-dev", kubernetes.McpSettings{Enabled: false, Approval: "allow"})
		if out := text(k.call(t, "kube_list_clusters", nil)); out != "[]" {
			t.Fatalf("kube_list_clusters with every cluster off = %q, want []", out)
		}
		wantNoCluster(dev, func(name string) map[string]any { return toolArgs(name, dev) })
		if n := len(k.fake.Requests("")); n != requests {
			t.Fatalf("an off cluster reached the API server: %d requests, had %d", n, requests)
		}
		if n := len(k.list(t)); n != 0 {
			t.Fatalf("an off cluster parked %d approvals", n)
		}
		k.setMcp(t, "kind-dev", on("prompt"))
	})

	t.Run("ask", func(t *testing.T) {
		k.setMcp(t, "kind-dev", on("prompt"))
		before := k.podRequests()
		const stmt = "kubectl get pods -n default --context kind-dev"
		stmtRows := func() (n int) {
			recent, err := k.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range recent {
				if r.Kind == "kubernetes" && r.Command != nil && *r.Command == stmt {
					n++
				}
			}
			return n
		}
		rowsBefore := stmtRows()
		pending := k.callAsync("kube_list_resources", k.listPods("default", nil))
		req := k.pending(t)
		if req.Item.Kind != approvals.KindKubeRead || req.Statement != stmt || req.Cluster == nil ||
			*req.Cluster != (kubeCluster{Context: "kind-dev", Server: k.fake.URL, Namespace: "default", Operation: "list"}) {
			t.Fatalf("pending approval = %+v (cluster %+v)", req, req.Cluster)
		}
		if k.podRequests() != before {
			t.Fatal("the list ran before approval")
		}
		// Contract dbmcp-kube: tests/ui/kubernetes-mcp.spec.ts renders this request in the approval dialog.
		// The item id stays a stable placeholder so the UI spec answers the id it rendered.
		k.app.Contract(t, kubeScenario, "ApprovalsService.List", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(k.fake.URL, "<kube>"))
		k.app.Contract(t, kubeScenario, "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		k.answerItems(t, true, req.Item.ID)
		out := <-pending
		if out.err != nil || out.res.IsError || !strings.Contains(text(out.res), "web-1") {
			t.Fatalf("approved list = %+v (%s)", out, text(out.res))
		}

		var row oplog.Record
		testx.WaitUntil(t, 10*time.Second, func() bool {
			recent, err := k.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range recent {
				if r.Kind == "kubernetes" && r.Status != "running" {
					row = r
					return true
				}
			}
			return false
		})
		if row.Source != "mcp" || row.Command == nil || *row.Command != stmt || row.Rows == nil || *row.Rows != 3 || row.ConnectionID != nil {
			t.Fatalf("op row = %+v, want an mcp row with the statement and 3 rows", row)
		}
		if n := stmtRows(); n != rowsBefore+1 {
			t.Fatalf("one MCP read wrote %d rows, want 1", n-rowsBefore)
		}
		// Contract dbmcp-kube: tests/ui/kubernetes-mcp.spec.ts renders this row in the Operations panel.
		k.contract(t, "OpsService.Recent", []oplog.Record{row})

		before = k.podRequests()
		denied := k.callAsync("kube_list_resources", k.listPods("default", nil))
		req = k.pending(t)
		k.answerItems(t, false, req.Item.ID)
		out = <-denied
		if out.err != nil {
			t.Fatal(out.err)
		}
		wantToolError(t, out.res, "denied by the user")
		if k.podRequests() != before {
			t.Fatal("a denied list reached the API server")
		}

		turnedOff := k.callAsync("kube_list_resources", k.listPods("default", nil))
		req = k.pending(t)
		k.setMcp(t, "kind-dev", kubernetes.McpSettings{Enabled: false, Approval: "prompt"})
		k.answerItems(t, true, req.Item.ID)
		out = <-turnedOff
		if out.err != nil {
			t.Fatal(out.err)
		}
		wantToolError(t, out.res, "no cluster")
		if k.podRequests() != before {
			t.Fatal("a cluster turned off during the wait still ran the list")
		}
	})

	t.Run("allow", func(t *testing.T) {
		k.setMcp(t, "kind-dev", on("allow"))
		defer func() {
			if n := len(k.list(t)); n != 0 {
				t.Errorf("allow parked %d approvals", n)
			}
		}()
		if res := k.call(t, "kube_discover", map[string]any{"clusterId": dev}); res.IsError || !strings.Contains(text(res), "kube-system") || !strings.Contains(text(res), `"key":"v1/pods"`) {
			t.Fatalf("kube_discover = %q, want namespaces and kinds", text(res))
		}
		res := k.call(t, "kube_list_resources", k.listPods("", map[string]any{"maxRows": 1}))
		if res.IsError || !strings.Contains(text(res), `"truncated":true`) || !strings.Contains(text(res), `"rowCount":1`) {
			t.Fatalf("capped kube_list_resources = %q", text(res))
		}
		sec := k.call(t, "kube_describe", map[string]any{"clusterId": dev, "kind": "v1/secrets", "name": "db-creds", "namespace": "default"})
		if sec.IsError || !strings.Contains(text(sec), `14 bytes`) || strings.Contains(text(sec), "hunter2-secret") ||
			strings.Contains(text(sec), flowharness.B64("hunter2-secret")) {
			t.Fatalf("secret describe leaks or lacks the byte count: %q", text(sec))
		}
		logs := k.call(t, "kube_pod_logs", map[string]any{"clusterId": dev, "namespace": "default", "pod": "web-1", "container": "app", "tail": 2})
		if logs.IsError || !strings.Contains(text(logs), "line two") || !strings.Contains(text(logs), "line three") || strings.Contains(text(logs), "line one") {
			t.Fatalf("kube_pod_logs tail 2 = %q", text(logs))
		}
		wantToolError(t, k.call(t, "kube_pod_logs", map[string]any{"clusterId": dev, "namespace": "default", "pod": "web-1", "container": "nope"}), "")
	})
}
