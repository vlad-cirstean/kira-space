package kubeflow

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestNamespaces(t *testing.T) {
	e := boot(t, nil)
	got, err := e.app.W.Kubernetes.Namespaces(kubernetes.ClusterArgs{ClusterID: e.ids["kind-dev"]})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"apps", "default", "kube-system"}; !slices.Equal(got.Namespaces, want) || got.Restricted || got.Fallback != "default" {
		t.Fatalf("namespaces = %+v", got)
	}
	e.contract(t, "KubernetesService.Namespaces", got)
}

func TestNamespacesRestricted(t *testing.T) {
	e := boot(t, []studioflowharness.KubeFakeOption{studioflowharness.ForbidNamespaces()})
	got, err := e.app.W.Kubernetes.Namespaces(kubernetes.ClusterArgs{ClusterID: e.ids["kind-dev"]})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Restricted || got.Fallback != "default" || len(got.Namespaces) != 0 {
		t.Fatalf("namespaces = %+v", got)
	}
	e.contract(t, "KubernetesService.Namespaces:restricted", got)
}

func TestKinds(t *testing.T) {
	e := boot(t, nil)
	got, err := e.app.W.Kubernetes.Kinds(kubernetes.ClusterArgs{ClusterID: e.ids["kind-dev"]})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial {
		t.Error("partial = false, want true: broken.example.com answers 503")
	}
	byKey := map[string]kubernetes.Kind{}
	for _, k := range got.Kinds {
		byKey[k.Key] = k
		if k.Key == "v1/pods/log" || k.Key == "v1/pods/status" {
			t.Errorf("subresource listed: %s", k.Key)
		}
	}
	for key, category := range map[string]string{
		"v1/pods": "workloads", "apps/v1/deployments": "workloads", "v1/services": "network",
		"v1/secrets": "config", "v1/nodes": "cluster", "example.com/v1/widgets": "custom",
	} {
		if byKey[key].Category != category {
			t.Errorf("%s category = %q, want %q", key, byKey[key].Category, category)
		}
	}
	for key, k := range byKey {
		if k.Logs != (key == "v1/pods") {
			t.Errorf("%s logs = %v", key, k.Logs)
		}
	}
	if !byKey["v1/pods"].Namespaced || byKey["v1/nodes"].Namespaced {
		t.Error("namespaced flags wrong")
	}
	e.contract(t, "KubernetesService.Kinds", got)
}
