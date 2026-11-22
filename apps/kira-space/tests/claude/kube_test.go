package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// TestKubeMcpThroughClaude has the real claude list the exposed clusters, then the pods of one
// namespace, through the embedded DB MCP server.
func TestKubeMcpThroughClaude(t *testing.T) {
	app := newRealApp(t)
	fake := flowharness.NewKubeFake(t)
	cfgPath := filepath.Join(app.Home, "kubeconfig")
	fake.WriteKubeconfig(t, cfgPath, flowharness.KubeconfigOpts{BinDir: app.BinDir})
	t.Setenv("KUBECONFIG", cfgPath)
	res, err := app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatalf("Kubernetes.Clusters: %v", err)
	}
	var dev string
	for _, c := range res.Clusters {
		if c.Context == "kind-dev" {
			dev = c.ID
		}
	}
	if dev == "" {
		t.Fatalf("no kind-dev cluster in %+v", res.Clusters)
	}
	if _, err := app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: dev, Mcp: kubernetes.McpSettings{
		Description: "real-claude kube fixture", Enabled: true, Approval: "allow",
	}}); err != nil {
		t.Fatalf("Kubernetes.SetMcp: %v", err)
	}

	_, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	listClusters, kubeRead := "mcp__"+dbServer+"__list", "mcp__"+dbServer+"__kube_read"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call list with what clusters once. Then call kube_read with op list and the kind v1/pods and the namespace default for the cluster whose context is kind-dev. Reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", listClusters+","+kubeRead,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)

	uses := toolUses(msgs, listClusters)
	if len(uses) == 0 {
		t.Fatalf("no list tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr || !strings.Contains(text, dev) || !strings.Contains(text, "real-claude kube fixture") {
		t.Fatalf("list result = %q (ok=%v, error=%v), want the cluster with its description", text, ok, isErr)
	}

	uses = kubeReadUses(msgs, kubeRead, "list")
	if len(uses) == 0 {
		t.Fatalf("no kube_read list use in the stream:\n%s", tail(out, 1500))
	}
	// The CLI defers MCP schemas, so a first call can be rejected for wrong arguments; the retry counts.
	var good bool
	for _, u := range uses {
		text, isErr, ok = toolResult(msgs, u.ID)
		if ok && !isErr && strings.Contains(text, "web-1") {
			good = true
			break
		}
	}
	if !good {
		t.Fatalf("kube_read list result = %q (ok=%v, error=%v), want the fake pods", text, ok, isErr)
	}
	if res := resultMsg(t, msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v", res)
	}
}

// TestKubeMcpThroughClaudeReviewWithheld has the real claude read pods from a cluster that reviews its
// results while the user withholds the result. The agent must report the result withheld, with no pod
// data, and must not read again to get it another way.
func TestKubeMcpThroughClaudeReviewWithheld(t *testing.T) {
	app := newRealApp(t)
	fake := flowharness.NewKubeFake(t)
	cfgPath := filepath.Join(app.Home, "kubeconfig")
	fake.WriteKubeconfig(t, cfgPath, flowharness.KubeconfigOpts{BinDir: app.BinDir})
	t.Setenv("KUBECONFIG", cfgPath)
	res, err := app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatalf("Kubernetes.Clusters: %v", err)
	}
	var dev string
	for _, c := range res.Clusters {
		if c.Context == "kind-dev" {
			dev = c.ID
		}
	}
	if dev == "" {
		t.Fatalf("no kind-dev cluster in %+v", res.Clusters)
	}
	if _, err := app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: dev, Mcp: kubernetes.McpSettings{
		Description: "real-claude kube fixture", Enabled: true, Approval: "allow", Review: "review",
	}}); err != nil {
		t.Fatalf("Kubernetes.SetMcp: %v", err)
	}

	_, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	// The user withholds every held result.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var withheld int
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			items, err := app.W.ApprovalsSvc.List()
			if err != nil {
				continue
			}
			for _, it := range items {
				if it.Kind != approvals.KindKubeResponse {
					continue
				}
				if _, err := app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{it.ID}, Approve: false}); err == nil {
					withheld++
				}
			}
		}
	}()

	listClusters, kubeRead := "mcp__"+dbServer+"__list", "mcp__"+dbServer+"__kube_read"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call list with what clusters once. Then call kube_read with op list and the kind v1/pods and the namespace default for the cluster whose context is kind-dev. Tell me the name of the first pod, or say the result was withheld if you did not get it.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", listClusters+","+kubeRead,
		"--output-format", "stream-json", "--verbose")
	close(stop)
	wg.Wait()
	msgs := parseStream(out)

	var held bool
	for _, u := range kubeReadUses(msgs, kubeRead, "list") {
		text, isErr, ok := toolResult(msgs, u.ID)
		if ok && isErr && strings.Contains(text, "the user withheld the result") {
			held = true
		}
		if ok && strings.Contains(text, "web-1") {
			t.Fatalf("a withheld result carried pod data: %q", text)
		}
	}
	if !held || withheld == 0 {
		t.Fatalf("held=%v withheld=%d, want one withheld kube_read list result:\n%s", held, withheld, tail(out, 1500))
	}
	// The CLI defers MCP schemas, so a first call can be rejected for wrong arguments before it runs; only
	// a call that reached the cluster counts. The withheld one ran once and the agent did not repeat it.
	if n := len(fake.Requests("/pods")); n != 1 {
		t.Fatalf("pods were read %d times, want once", n)
	}
	r := resultMsg(t, msgs)
	if r.IsError {
		t.Fatalf("claude run ended in error: %+v", r)
	}
	if low := strings.ToLower(r.Result); !strings.Contains(low, "withh") || strings.Contains(low, "web-1") {
		t.Fatalf("final answer = %q, want it to report the result withheld without pod data", r.Result)
	}
}

// TestKubeMcpKeptChainThroughClaude gives the real claude only the task: read the logs of the crash-looping
// pod in a namespace. The pod name must not pass through the model, so the agent keeps it with a kube
// transform and reads it in the prepare script of kube_read logs on its own.
func TestKubeMcpKeptChainThroughClaude(t *testing.T) {
	const crashPod, crashLine = "checkout-7d9f6", "panic: cannot reach payments-db:5432"
	app := newRealApp(t)
	fake := flowharness.NewKubeFake(t, flowharness.WithCrashLoopPod("shop", crashPod, []string{"starting checkout", crashLine}))
	cfgPath := filepath.Join(app.Home, "kubeconfig")
	fake.WriteKubeconfig(t, cfgPath, flowharness.KubeconfigOpts{BinDir: app.BinDir})
	t.Setenv("KUBECONFIG", cfgPath)
	res, err := app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatalf("Kubernetes.Clusters: %v", err)
	}
	var dev string
	for _, c := range res.Clusters {
		if c.Context == "kind-dev" {
			dev = c.ID
		}
	}
	if dev == "" {
		t.Fatalf("no kind-dev cluster in %+v", res.Clusters)
	}
	if _, err := app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: dev, Mcp: kubernetes.McpSettings{
		Description: "real-claude kube fixture", Enabled: true, Approval: "allow", Review: "inherit",
	}}); err != nil {
		t.Fatalf("Kubernetes.SetMcp: %v", err)
	}

	_, url, helper := enableDbMcp(t, app)
	cfg := filepath.Join(t.TempDir(), "mcp.json")
	raw, err := json.Marshal(map[string]any{"mcpServers": map[string]any{
		dbServer: map[string]any{"type": "http", "url": url, "headersHelper": loginshell.Quote(helper)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	pre := "mcp__" + dbServer + "__"
	kubeRead := pre + "kube_read"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Use the kira-studio MCP tools, loading them with ToolSearch if they are deferred. In the cluster whose context is kind-dev, read the logs of the crash-looping pod in namespace shop."+
			" Never write the pod's name yourself: it must not pass through you. Reply with the last log line.",
		"--strict-mcp-config", "--mcp-config", cfg,
		"--allowedTools", "ToolSearch,"+pre+"list,"+kubeRead,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)

	var kept, read bool
	var keptName string
	for _, u := range kubeReadUses(msgs, kubeRead, "list") {
		var in struct {
			Script *struct{ Transform string } `json:"script"`
		}
		if err := json.Unmarshal(u.Input, &in); err != nil {
			t.Fatalf("kube_read list input %s: %v", u.Input, err)
		}
		if in.Script == nil || !storedName.MatchString(in.Script.Transform) {
			continue
		}
		name := storedName.FindStringSubmatch(in.Script.Transform)[1]
		text, isErr, ok := toolResult(msgs, u.ID)
		if strings.Contains(text, crashPod) {
			t.Fatalf("scripted kube_read list result = %q, want no pod name", text)
		}
		// A script the model got wrong (error, nothing saved) is its own retry, not a product fault.
		if !ok || isErr || !strings.Contains(text, `"saved":[{"name":"`+name+`"`) {
			continue
		}
		keptName, kept = name, true
	}
	for _, u := range kubeReadUses(msgs, kubeRead, "logs") {
		var in struct {
			Name   string                    `json:"name"`
			Script *struct{ Prepare string } `json:"script"`
		}
		if err := json.Unmarshal(u.Input, &in); err != nil {
			t.Fatalf("kube_read logs input %s: %v", u.Input, err)
		}
		if strings.Contains(string(u.Input), crashPod) {
			t.Fatalf("kube_read logs input %s carries the pod name, want it kept and read by name", u.Input)
		}
		text, isErr, ok := toolResult(msgs, u.ID)
		if in.Script != nil && keptName != "" && strings.Contains(in.Script.Prepare, keptName) && ok && !isErr && strings.Contains(text, crashLine) {
			read = true
		}
	}
	if !kept || !read {
		t.Fatalf("kept = %v, read through prepare = %v, want both:\n%s", kept, read, tail(out, 2500))
	}
	if r := resultMsg(t, msgs); r.IsError || strings.Contains(r.Result, crashPod) || !strings.Contains(r.Result, "payments-db") {
		t.Fatalf("final answer = %q (error %v), want the last log line and no pod name", r.Result, r.IsError)
	}
}

// kubeReadUses is the kube_read calls of one op.
func kubeReadUses(msgs []streamMsg, tool, op string) []contentBlock {
	var out []contentBlock
	for _, u := range toolUses(msgs, tool) {
		var in struct {
			Op string `json:"op"`
		}
		if json.Unmarshal(u.Input, &in) == nil && in.Op == op {
			out = append(out, u)
		}
	}
	return out
}
