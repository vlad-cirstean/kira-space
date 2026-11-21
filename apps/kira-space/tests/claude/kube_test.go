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

	listClusters, listResources := "mcp__"+dbServer+"__kube_list_clusters", "mcp__"+dbServer+"__kube_list_resources"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call kube_list_clusters once. Then call kube_list_resources with the kind v1/pods and the namespace default for the cluster whose context is kind-dev. Reply with the word done.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", listClusters+","+listResources,
		"--output-format", "stream-json", "--verbose")
	msgs := parseStream(out)

	uses := toolUses(msgs, listClusters)
	if len(uses) == 0 {
		t.Fatalf("no kube_list_clusters tool use in the stream:\n%s", tail(out, 1500))
	}
	text, isErr, ok := toolResult(msgs, uses[0].ID)
	if !ok || isErr || !strings.Contains(text, dev) || !strings.Contains(text, "real-claude kube fixture") {
		t.Fatalf("kube_list_clusters result = %q (ok=%v, error=%v), want the cluster with its description", text, ok, isErr)
	}

	uses = toolUses(msgs, listResources)
	if len(uses) == 0 {
		t.Fatalf("no kube_list_resources tool use in the stream:\n%s", tail(out, 1500))
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
		t.Fatalf("kube_list_resources result = %q (ok=%v, error=%v), want the fake pods", text, ok, isErr)
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

	listClusters, listResources := "mcp__"+dbServer+"__kube_list_clusters", "mcp__"+dbServer+"__kube_list_resources"
	out, _ := app.claudeP(t, t.TempDir(), nil,
		"Call kube_list_clusters once. Then call kube_list_resources with the kind v1/pods and the namespace default for the cluster whose context is kind-dev. Tell me the name of the first pod, or say the result was withheld if you did not get it.",
		"--strict-mcp-config", "--mcp-config", cfg, "--allowedTools", listClusters+","+listResources,
		"--output-format", "stream-json", "--verbose")
	close(stop)
	wg.Wait()
	msgs := parseStream(out)

	var held bool
	for _, u := range toolUses(msgs, listResources) {
		text, isErr, ok := toolResult(msgs, u.ID)
		if ok && isErr && strings.Contains(text, "the user withheld the result") {
			held = true
		}
		if ok && strings.Contains(text, "web-1") {
			t.Fatalf("a withheld result carried pod data: %q", text)
		}
	}
	if !held || withheld == 0 {
		t.Fatalf("held=%v withheld=%d, want one withheld kube_list_resources result:\n%s", held, withheld, tail(out, 1500))
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
