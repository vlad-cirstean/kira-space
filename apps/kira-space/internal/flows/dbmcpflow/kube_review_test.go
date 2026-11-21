package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Contract kube-review: tests/ui/studio/approvals-kube-review.spec.ts renders these items and held results.
const kubeReviewKey = "kube-review"

func review(mode string) kubernetes.McpSettings {
	return kubernetes.McpSettings{Enabled: true, Approval: "allow", Review: mode}
}

// setKubeApp sets the app leaf; a non-empty key records the call for the Settings switch spec.
func (k *kubeFixture) setKubeApp(t *testing.T, on bool, key string) {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: &model.DbMcpPatch{KubeReviewResponses: &on}}}
	if _, err := k.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
	if key != "" {
		k.app.Contract(t, kubeReviewKey, "args:SettingsService.Set#"+key, args)
	}
}

func (k *kubeFixture) heldText(t *testing.T, requestID string) bridge.DbMcpPendingResponse {
	t.Helper()
	held, err := k.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: requestID})
	if err != nil {
		t.Fatal(err)
	}
	return held
}

type kubeResponseItem struct {
	Context   string `json:"context"`
	Operation string `json:"operation"`
	Statement string `json:"statement"`
	IsError   bool   `json:"isError"`
	Bytes     int    `json:"bytes"`
	Requested bool   `json:"requested"`
}

func decodeKubeResponse(t *testing.T, it approvals.Item) kubeResponseItem {
	t.Helper()
	var p kubeResponseItem
	if err := json.Unmarshal(it.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// recordKube records the held item, its pending result and the answer under key, for the UI half.
func (k *kubeFixture) recordKube(t *testing.T, key string, req pendingReq, held bridge.DbMcpPendingResponse) {
	t.Helper()
	opts := []flowharness.ContractOption{flowharness.Mask("createdAt", "expiresAt", "cwd"), flowharness.Replace(k.fake.URL, "<kube>")}
	k.app.Contract(t, kubeReviewKey, "ApprovalsService.List#"+key, []approvals.Item{req.Item}, opts...)
	k.app.Contract(t, kubeReviewKey, "DbMcpService.PendingResponse#"+key, held, opts...)
	k.app.Contract(t, kubeReviewKey, "args:ApprovalsService.Answer#"+key, approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
}

// held runs a kube tool on a cluster that reviews and returns the held item and its pending result.
func (k *kubeFixture) held(t *testing.T, name string, args map[string]any) (chan kubeOutcome, pendingReq, bridge.DbMcpPendingResponse) {
	t.Helper()
	pending := k.callAsync(name, args)
	req := k.pending(t)
	if req.Item.Kind != approvals.KindKubeResponse || req.Stage != "response" {
		t.Fatalf("%s raised %s (stage %s), want kube.response", name, req.Item.Kind, req.Stage)
	}
	return pending, req, k.heldText(t, req.RequestID)
}

func (k *kubeFixture) describeArgs(kind, name, namespace string) map[string]any {
	return map[string]any{"clusterId": k.ids["kind-dev"], "kind": kind, "name": name, "namespace": namespace}
}

func (k *kubeFixture) logsArgs(extra map[string]any) map[string]any {
	args := map[string]any{"clusterId": k.ids["kind-dev"], "namespace": "default", "pod": "web-1", "container": "app", "tail": 2}
	for key, v := range extra {
		args[key] = v
	}
	return args
}

// TestKubeResponseReview: a held list, describe or logs result raises one kube.response item without its
// body; Send returns the exact bytes the user saw, and each read ran once.
func TestKubeResponseReview(t *testing.T) {
	k := newKubeFixture(t)
	k.setMcp(t, "kind-dev", review("review"))

	cases := []struct {
		key, tool, op, want string
		args                map[string]any
	}{
		{"list", "kube_list_resources", "list", "web-1", k.listPods("default", nil)},
		{"describeSecret", "kube_describe", "describe", "db-creds", k.describeArgs("v1/secrets", "db-creds", "default")},
		{"describePod", "kube_describe", "describe", "web-1", k.describeArgs("v1/pods", "web-1", "default")},
		{"logs", "kube_pod_logs", "logs", "line three", k.logsArgs(nil)},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			before := len(k.fake.Requests(""))
			pending, req, held := k.held(t, c.tool, c.args)
			p := decodeKubeResponse(t, req.Item)
			if p.Operation != c.op || p.Context != "kind-dev" || p.IsError || p.Requested || p.Bytes != len(held.Text) || held.Raw != "" || held.Script != nil {
				t.Fatalf("payload %+v pending %+v, want a plain held %s", p, held, c.op)
			}
			if !strings.Contains(held.Text, c.want) {
				t.Fatalf("held text = %s, want it to carry %q", held.Text, c.want)
			}
			listJSON, _ := json.Marshal(k.list(t))
			if strings.Contains(string(listJSON), c.want) && c.want != "web-1" && c.want != "db-creds" {
				t.Errorf("the approvals list carries the result %q", c.want)
			}
			for _, leak := range []string{"hunter2-secret", flowharness.B64("hunter2-secret")} {
				if strings.Contains(held.Text, leak) || strings.Contains(string(listJSON), leak) {
					t.Errorf("secret value %q reached the held text or the queue", leak)
				}
			}
			k.recordKube(t, c.key, req, held)

			k.answerItems(t, true, req.Item.ID)
			out := <-pending
			if out.err != nil || out.res.IsError || text(out.res) != held.Text {
				t.Fatalf("sent result = %q (%v), want exactly the held text", text(out.res), out.err)
			}
			if n := len(k.fake.Requests("")) - before; n == 0 {
				t.Fatal("the read never reached the API server")
			}
		})
	}
}

// TestKubeResponseWithhold: every way the result does not reach the agent gives app text only, and the
// read ran exactly once.
func TestKubeResponseWithhold(t *testing.T) {
	k := newKubeFixture(t, flowharness.WithApprovalHeldCaps(300, 1<<20))
	k.setMcp(t, "kind-dev", review("review"))

	pending, req, _ := k.held(t, "kube_pod_logs", k.logsArgs(nil))
	k.answerItems(t, false, req.Item.ID)
	out := <-pending
	const want = `the user withheld the result of this Kubernetes logs on "kind-dev"; it ran, but you get no data. Do not rerun it to get the result another way.`
	if out.err != nil || !out.res.IsError || text(out.res) != want {
		t.Fatalf("withheld = %q (error %v), want %q", text(out.res), out.res.IsError, want)
	}
	if n := len(k.fake.Requests("/log")); n != 1 {
		t.Fatalf("the logs read ran %d times, want once", n)
	}
	for _, leak := range []string{"line one", "line three"} {
		if strings.Contains(want, leak) {
			t.Errorf("withhold text carries result data %q", leak)
		}
	}

	// An over-cap result is withheld unseen and raises no item.
	res := k.call(t, "kube_list_resources", k.listPods("default", nil))
	if !res.IsError || !strings.Contains(text(res), "MB, over the 8 MB review limit") || !strings.Contains(text(res), "withheld unseen") {
		t.Fatalf("over the cap = %q, want the too-large refusal", text(res))
	}
	if n := k.queueLen(t); n != 0 {
		t.Fatalf("a too-large result raised %d items", n)
	}
}

// TestKubeResponseSettings: where the hold comes from, and what is never held.
func TestKubeResponseSettings(t *testing.T) {
	k := newKubeFixture(t)
	dev := k.ids["kind-dev"]

	listed := func() bool {
		var views []struct {
			ID            string `json:"id"`
			ReviewResults bool   `json:"reviewResults"`
		}
		if err := json.Unmarshal([]byte(text(k.call(t, "kube_list_clusters", nil))), &views); err != nil {
			t.Fatal(err)
		}
		for _, v := range views {
			if v.ID == dev {
				return v.ReviewResults
			}
		}
		t.Fatal("cluster not listed")
		return false
	}
	sendOrHold := func(name string) (held bool) {
		t.Helper()
		pending := k.callAsync(name, k.listPods("default", nil))
		for range 100 {
			select {
			case out := <-pending:
				if out.err != nil || out.res.IsError {
					t.Fatalf("%s = %+v", name, out)
				}
				return false
			default:
			}
			if all := k.list(t); len(all) > 0 {
				if all[0].Kind != approvals.KindKubeResponse {
					t.Fatalf("queued %s, want kube.response", all[0].Kind)
				}
				k.answerItems(t, true, all[0].ID)
				<-pending
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("neither a result nor an item")
		return false
	}

	// Inherit follows the app leaf.
	k.setMcp(t, "kind-dev", review("inherit"))
	if listed() || sendOrHold("kube_list_resources") {
		t.Fatal("inherit with the app setting off held a result")
	}
	k.setKubeApp(t, true, "on")
	if !listed() || !sendOrHold("kube_list_resources") {
		t.Fatal("inherit with the app setting on did not hold")
	}
	// The cluster's own value wins both ways.
	k.setMcp(t, "kind-dev", review("send"))
	if listed() || sendOrHold("kube_list_resources") {
		t.Fatal("send with the app setting on held a result")
	}
	k.setKubeApp(t, false, "")
	k.setMcp(t, "kind-dev", review("review"))
	if !listed() || !sendOrHold("kube_list_resources") {
		t.Fatal("review with the app setting off did not hold")
	}

	// Discover returns namespaces and kinds only: never held.
	if res := k.call(t, "kube_discover", map[string]any{"clusterId": dev}); res.IsError || k.queueLen(t) != 0 {
		t.Fatalf("kube_discover = %q, queue %d, want a plain result", text(res), k.queueLen(t))
	}
}

// TestKubeReadReviewOption: on an ask cluster the approval card offers the review; a tick holds this one
// result, and a cluster that already reviews shows it on.
func TestKubeReadReviewOption(t *testing.T) {
	k := newKubeFixture(t)
	k.setMcp(t, "kind-dev", kubernetes.McpSettings{Enabled: true, Approval: "prompt", Review: "send"})

	readPayload := func(it approvals.Item) (offered, on bool) {
		var p struct {
			ReviewOffered bool `json:"reviewOffered"`
			ReviewResult  bool `json:"reviewResult"`
		}
		if err := json.Unmarshal(it.Payload, &p); err != nil {
			t.Fatal(err)
		}
		return p.ReviewOffered, p.ReviewResult
	}

	// No tick: the result goes straight out.
	pending := k.callAsync("kube_list_resources", k.listPods("default", nil))
	req := k.pending(t)
	if offered, on := readPayload(req.Item); !offered || on {
		t.Fatalf("send cluster payload offered=%v on=%v, want the option offered and off", offered, on)
	}
	k.app.Contract(t, kubeReviewKey, "ApprovalsService.List#readOption", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(k.fake.URL, "<kube>"))
	k.answerItems(t, true, req.Item.ID)
	if out := <-pending; out.err != nil || out.res.IsError || k.queueLen(t) != 0 {
		t.Fatalf("unticked read = %+v, queue %d, want the result sent", out, k.queueLen(t))
	}

	// Tick: this result is held.
	pending = k.callAsync("kube_list_resources", k.listPods("default", nil))
	req = k.pending(t)
	k.app.Contract(t, kubeReviewKey, "args:ApprovalsService.Answer#tick", approvals.AnswerArgs{
		IDs: []string{req.Item.ID}, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true}`)},
	})
	k.answerWith(t, true, req.Item.ID, map[string]string{req.Item.ID: `{"reviewResult":true}`})
	req = k.pending(t)
	if req.Item.Kind != approvals.KindKubeResponse {
		t.Fatalf("ticked read queued %s, want kube.response", req.Item.Kind)
	}
	if p := decodeKubeResponse(t, req.Item); !p.Requested {
		t.Fatalf("payload %+v, want requested", p)
	}
	k.answerItems(t, true, req.Item.ID)
	if out := <-pending; out.err != nil || out.res.IsError {
		t.Fatalf("ticked then sent = %+v", out)
	}

	// A cluster that reviews shows the option on.
	k.setMcp(t, "kind-dev", kubernetes.McpSettings{Enabled: true, Approval: "prompt", Review: "review"})
	pending = k.callAsync("kube_list_resources", k.listPods("default", nil))
	req = k.pending(t)
	if offered, on := readPayload(req.Item); !offered || !on {
		t.Fatalf("review cluster payload offered=%v on=%v, want both", offered, on)
	}
	k.app.Contract(t, kubeReviewKey, "ApprovalsService.List#readOptionOn", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(k.fake.URL, "<kube>"))
	k.answerItems(t, true, req.Item.ID)
	req = k.pending(t)
	k.answerItems(t, true, req.Item.ID)
	<-pending

	// Discover has no result to review: no option.
	pending = k.callAsync("kube_discover", map[string]any{"clusterId": k.ids["kind-dev"]})
	req = k.pending(t)
	if offered, _ := readPayload(req.Item); offered {
		t.Fatal("discover offered a result review")
	}
	k.answerItems(t, true, req.Item.ID)
	<-pending
}

// TestKubeResponseClusterOff: a cluster turned off while its result is in review withholds it.
func TestKubeResponseClusterOff(t *testing.T) {
	k := newKubeFixture(t)
	k.setMcp(t, "kind-dev", review("review"))
	pending, req, _ := k.held(t, "kube_list_resources", k.listPods("default", nil))
	k.setMcp(t, "kind-dev", kubernetes.McpSettings{Enabled: false, Approval: "allow", Review: "review"})
	k.answerItems(t, true, req.Item.ID)
	out := <-pending
	if out.err != nil || !out.res.IsError || !strings.Contains(text(out.res), "turned off while its result was in review") || strings.Contains(text(out.res), "web-1") {
		t.Fatalf("result = %q, want the turned-off withhold", text(out.res))
	}
}
