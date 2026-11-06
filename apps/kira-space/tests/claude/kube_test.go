package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
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
		Description: "real-claude kube fixture", Discover: "allow", List: "allow", Describe: "deny", Logs: "deny",
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
	text, isErr, ok = toolResult(msgs, uses[0].ID)
	if !ok || isErr || !strings.Contains(text, "web-1") {
		t.Fatalf("kube_list_resources result = %q (ok=%v, error=%v), want the fake pods", text, ok, isErr)
	}
	if res := resultMsg(t, msgs); res.IsError {
		t.Fatalf("claude run ended in error: %+v", res)
	}
}
