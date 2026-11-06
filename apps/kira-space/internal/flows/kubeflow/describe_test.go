package kubeflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
)

func (e *env) describe(t *testing.T, kind, ns, name string) (kubernetes.Description, error) {
	t.Helper()
	return e.app.W.Kubernetes.Describe(kubernetes.DescribeArgs{ClusterID: e.ids["kind-dev"], Kind: kind, Namespace: ns, Name: name})
}

func TestDescribePod(t *testing.T) {
	e := boot(t, nil)
	got, err := e.describe(t, "v1/pods", "default", "web-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Pod" || got.Name != "web-1" || got.Namespace != "default" || got.Labels["app"] != "web" {
		t.Fatalf("overview = %+v", got)
	}
	if len(got.Owners) != 1 || got.Owners[0] != (kubernetes.Owner{Kind: "ReplicaSet", Name: "web-1-rs"}) {
		t.Fatalf("owners = %+v", got.Owners)
	}
	if len(got.Conditions) != 2 || got.Conditions[0].Type != "Ready" {
		t.Fatalf("conditions = %+v", got.Conditions)
	}
	wantContainers := []kubernetes.ContainerRef{{Name: "init", Init: true}, {Name: "app"}, {Name: "sidecar"}}
	if len(got.Containers) != 3 || got.Containers[0] != wantContainers[0] || got.Containers[1] != wantContainers[1] || got.Containers[2] != wantContainers[2] {
		t.Fatalf("containers = %+v", got.Containers)
	}
	if len(got.Events) != 2 || got.Events[0].Reason != "Started" {
		t.Fatalf("events = %+v, want the two events of this uid, newest first", got.Events)
	}
	for _, r := range e.fake.Requests("/events") {
		if mustQuery(t, r.Query).Get("fieldSelector") != "involvedObject.uid="+got.UID {
			t.Errorf("events query = %q", r.Query)
		}
	}
	if strings.Contains(got.Manifest, "managedFields") || !strings.Contains(got.Manifest, "kind: Pod") {
		t.Fatalf("manifest = %q", got.Manifest)
	}
	e.contract(t, "KubernetesService.Describe", got, flowharness.Omit("manifest"))
}

func TestDescribeSecretHidesValues(t *testing.T) {
	e := boot(t, nil)
	got, err := e.describe(t, "v1/secrets", "default", "db-creds")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Manifest, "<14 bytes>") || !strings.Contains(got.Manifest, "<5 bytes>") {
		t.Fatalf("manifest = %q", got.Manifest)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"hunter2-secret", flowharness.B64("hunter2-secret"), flowharness.B64("admin"), "admin\n"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("result leaks %q", leak)
		}
	}
	if _, ok := got.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok || got.Annotations["owner"] != "team-a" {
		t.Fatalf("annotations = %+v", got.Annotations)
	}
	e.contract(t, "KubernetesService.Describe:secret", got)
}

func TestDescribeClusterScoped(t *testing.T) {
	e := boot(t, nil)
	got, err := e.describe(t, "v1/nodes", "", "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Node" || got.Namespace != "" || len(got.Containers) != 0 || len(got.Events) != 0 {
		t.Fatalf("describe = %+v", got)
	}
}

func TestDescribeUnknown(t *testing.T) {
	e := boot(t, nil)
	_, err := e.describe(t, "v1/pods", "default", "nope")
	wantCode(t, err, "E_NOT_FOUND")
}
