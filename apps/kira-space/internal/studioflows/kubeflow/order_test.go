package kubeflow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func (e *env) order(t *testing.T) []string {
	t.Helper()
	res, err := e.app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range res.Clusters {
		names = append(names, c.Context)
	}
	return names
}

func TestClusterOrder(t *testing.T) {
	e := boot(t, nil)
	def := []string{"kind-dev", "bad-token", "down", "plugin-ctx"}
	moved := []string{"down", "kind-dev", "bad-token", "plugin-ctx"}
	if got := e.order(t); !slices.Equal(got, def) {
		t.Fatalf("default order = %v, want %v", got, def)
	}
	first, err := e.app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatal(err)
	}
	// Stored first so <id:n> numbering matches the Clusters contract the UI specs read.
	e.contract(t, "KubernetesService.Clusters", first)

	mark := e.app.Events.Mark()
	args := kubernetes.ReorderArgs{IDs: []string{e.ids["down"], e.ids["kind-dev"], e.ids["bad-token"], e.ids["plugin-ctx"]}}
	res, err := e.app.W.Kubernetes.Reorder(args)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range res.Clusters {
		got = append(got, c.Context)
	}
	if !slices.Equal(got, moved) || !slices.Equal(e.order(t), moved) {
		t.Fatalf("reordered = %v, then %v, want %v", got, e.order(t), moved)
	}
	if n := len(e.app.Events.Since(mark, kubernetes.ChannelClusters)); n != 1 {
		t.Fatalf("%d clusters events, want 1", n)
	}
	for _, op := range e.ops(t) {
		if op.Kind == "kubernetes" {
			t.Fatalf("Reorder recorded an op: %+v", op)
		}
	}
	e.contract(t, "args:KubernetesService.Reorder", args)
	e.contract(t, "KubernetesService.Reorder", res)

	for name, ids := range map[string][]string{
		"empty":     nil,
		"duplicate": {e.ids["down"], e.ids["down"]},
	} {
		_, err := e.app.W.Kubernetes.Reorder(kubernetes.ReorderArgs{IDs: ids})
		wantCode(t, err, "E_CONTRACT")
		if o := e.order(t); !slices.Equal(o, moved) {
			t.Fatalf("%s changed the order to %v", name, o)
		}
	}
	_, err = e.app.W.Kubernetes.Reorder(kubernetes.ReorderArgs{IDs: []string{e.ids["down"], "nope"}})
	wantCode(t, err, "E_NOT_FOUND")
	if o := e.order(t); !slices.Equal(o, moved) {
		t.Fatalf("unknown id changed the order to %v", o)
	}

	extra := filepath.Join(e.app.Home, "kubeconfig-extra")
	writeFile(t, extra, fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: aa-new
  cluster: {server: %s, insecure-skip-tls-verify: true}
contexts:
- name: aa-new
  context: {cluster: aa-new, user: aa-user}
users:
- name: aa-user
  user: {token: %s}
`, e.fake.URL, studioflowharness.KubeGoodToken))
	t.Setenv("KUBECONFIG", e.cfg+string(os.PathListSeparator)+extra)
	withNew := append(slices.Clone(moved), "aa-new")
	if o := e.order(t); !slices.Equal(o, withNew) {
		t.Fatalf("with a new context = %v, want %v", o, withNew)
	}

	e.app.Restart(t)
	if o := e.order(t); !slices.Equal(o, withNew) {
		t.Fatalf("after restart = %v, want %v", o, withNew)
	}

	mark = e.app.Events.Mark()
	if _, err := e.app.W.Kubernetes.SetMcp(kubernetes.SetMcpArgs{ClusterID: e.ids["kind-dev"], Mcp: kubernetes.McpSettings{Discover: "allow", List: "allow", Describe: "allow", Logs: "allow"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(e.app.Events.Since(mark, kubernetes.ChannelClusters)); n != 1 {
		t.Fatalf("SetMcp: %d clusters events, want 1", n)
	}
}
