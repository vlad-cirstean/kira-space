package kubeflow

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func (e *env) ops(t *testing.T) []model.OpRecord {
	t.Helper()
	recs, err := e.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// newestOp waits for the n-th row and returns it once it is not running (or is, when wantRunning).
func (e *env) newestOp(t *testing.T, n int, wantRunning bool) model.OpRecord {
	t.Helper()
	var row model.OpRecord
	testx.WaitUntil(t, wait, func() bool {
		recs := e.ops(t)
		if len(recs) < n || (recs[0].Status == "running") != wantRunning {
			return false
		}
		row = recs[0]
		return true
	})
	return row
}

func wantOp(t *testing.T, row model.OpRecord, status, command string, rows *int) {
	t.Helper()
	if row.Kind != "kubernetes" || row.Status != status || row.Source != "user" || row.ConnectionID != nil || row.TabID != nil ||
		row.Command == nil || *row.Command != command || (rows == nil) != (row.Rows == nil) || (rows != nil && *rows != *row.Rows) {
		var cmd string
		if row.Command != nil {
			cmd = *row.Command
		}
		t.Fatalf("op = %+v (command %q), want %s %q rows %v", row, cmd, status, command, rows)
	}
}

func ptr(n int) *int { return &n }

func TestOpsRecordEveryAction(t *testing.T) {
	e := boot(t, nil)
	dev := e.ids["kind-dev"]
	const ctxFlag = " --context kind-dev"

	e.status(t, "kind-dev", false)
	wantOp(t, e.newestOp(t, 1, false), "ok", "kubectl version"+ctxFlag, nil)

	kinds, err := e.app.W.Kubernetes.Kinds(kubernetes.ClusterArgs{ClusterID: dev})
	if err != nil {
		t.Fatal(err)
	}
	wantOp(t, e.newestOp(t, 2, false), "ok", "kubectl api-resources"+ctxFlag, ptr(len(kinds.Kinds)))

	ns, err := e.app.W.Kubernetes.Namespaces(kubernetes.ClusterArgs{ClusterID: dev})
	if err != nil {
		t.Fatal(err)
	}
	wantOp(t, e.newestOp(t, 3, false), "ok", "kubectl get namespaces"+ctxFlag, ptr(len(ns.Namespaces)))

	list := kubernetes.ListArgs{ClusterID: dev, Kind: "v1/pods", Namespace: "default"}
	if _, err := e.app.W.Kubernetes.List(list); err != nil {
		t.Fatal(err)
	}
	wantOp(t, e.newestOp(t, 4, false), "ok", "kubectl get pods -n default"+ctxFlag, ptr(3))

	before := len(e.fake.Requests("/pods"))
	list.Background = true
	if _, err := e.app.W.Kubernetes.List(list); err != nil {
		t.Fatal(err)
	}
	if len(e.fake.Requests("/pods")) == before {
		t.Fatal("background list did not reach the API server")
	}
	if n := len(e.ops(t)); n != 4 {
		t.Fatalf("background list wrote a row: %d rows, want 4", n)
	}

	if _, err := e.app.W.Kubernetes.Describe(kubernetes.DescribeArgs{ClusterID: dev, Kind: "v1/pods", Namespace: "default", Name: "web-1"}); err != nil {
		t.Fatal(err)
	}
	wantOp(t, e.newestOp(t, 5, false), "ok", "kubectl describe pods web-1 -n default"+ctxFlag, ptr(1))

	mark := e.app.Events.Mark()
	follow := func(a *kubernetes.LogsOpenArgs) { a.Follow, a.Tail = true, 1000 }
	if err := e.open("w1", "f1", follow); err != nil {
		t.Fatal(err)
	}
	waitLines(t, e.app, mark, "w1", "f1", 3)
	const logsCmd = "kubectl logs web-1 -n default -c app --tail 1000 -f" + ctxFlag
	running := e.newestOp(t, 6, true)
	// op:start carries no command; it arrives with op:end.
	if running.Kind != "kubernetes" || running.Source != "user" || running.ConnectionID != nil || running.Command != nil {
		t.Fatalf("running op = %+v", running)
	}
	e.contract(t, "OpsService.Recent:running", []model.OpRecord{running}, flowharness.Mask("id", "startedAt", "durationMs"))
	if err := e.app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: running.ID}); err != nil {
		t.Fatal(err)
	}
	if _, errMsg := waitEnded(t, e.app, mark, "w1", "f1"); errMsg != "" {
		t.Fatalf("cancelled stream ended with error %q", errMsg)
	}
	testx.WaitUntil(t, wait, func() bool { return e.fake.FollowEnded() == 1 })
	wantOp(t, e.newestOp(t, 6, false), "cancelled", logsCmd, ptr(3))

	if err := e.open("w1", "f2", follow); err != nil {
		t.Fatal(err)
	}
	waitLines(t, e.app, mark, "w1", "f2", 3)
	if err := e.app.W.Kubernetes.LogsClose(kubernetes.StreamArgs{StreamID: "f2"}); err != nil {
		t.Fatal(err)
	}
	wantOp(t, e.newestOp(t, 7, false), "ok", logsCmd, ptr(3))

	wantCode(t, e.open("w1", "bad", func(a *kubernetes.LogsOpenArgs) { a.Container = "nope" }), "E_BAD_REQUEST")
	bad := e.newestOp(t, 8, false)
	wantOp(t, bad, "error", "kubectl logs web-1 -n default -c nope --tail 100"+ctxFlag, nil)
	if bad.Error == nil || *bad.Error == "" {
		t.Fatalf("failed open row has no error: %+v", bad)
	}

	st, err := e.app.W.Kubernetes.Status(kubernetes.StatusArgs{ClusterID: e.ids["down"]})
	if err != nil || st.State != "unavailable" {
		t.Fatalf("down status = %+v (%v)", st, err)
	}
	down := e.newestOp(t, 9, false)
	if down.Status != "error" || down.Error == nil || *down.Error != st.Message {
		t.Fatalf("down op = %+v, want an error row carrying the status message", down)
	}

	_, err = e.app.W.Kubernetes.Status(kubernetes.StatusArgs{ClusterID: "nope"})
	wantCode(t, err, "E_NOT_FOUND")
	if _, err := e.app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: dev, Mcp: kubernetes.McpSettings{Discover: "allow", List: "allow", Describe: "allow", Logs: "allow"}}); err != nil {
		t.Fatal(err)
	}
	recs := e.ops(t)
	if len(recs) != 9 {
		t.Fatalf("%d rows, want 9: pre-op failures and SetMcp record nothing", len(recs))
	}
	e.contract(t, "OpsService.Recent", recs, flowharness.Mask("id", "startedAt", "durationMs", "error"))
}
