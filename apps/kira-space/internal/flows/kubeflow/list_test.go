package kubeflow

import (
	"net/url"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
)

func (e *env) list(t *testing.T, kind, ns, selector string) (kubernetes.ListResult, error) {
	t.Helper()
	return e.app.W.Kubernetes.List(kubernetes.ListArgs{ClusterID: e.ids["kind-dev"], Kind: kind, Namespace: ns, LabelSelector: selector})
}

func columnNames(r kubernetes.ListResult) []string {
	var out []string
	for _, c := range r.Columns {
		out = append(out, c.Name)
	}
	return out
}

func rowNames(r kubernetes.ListResult) []string {
	var out []string
	for _, row := range r.Rows {
		out = append(out, row.Name)
	}
	return out
}

func TestList(t *testing.T) {
	e := boot(t, nil)

	t.Run("namespace", func(t *testing.T) {
		got, err := e.list(t, "v1/pods", "default", "")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"Name", "Ready", "Status", "Restarts", "Age"}; !slices.Equal(columnNames(got), want) {
			t.Fatalf("columns = %v, want %v (priority 1 hidden)", columnNames(got), want)
		}
		if want := []string{"web-1", "web-2", "db-0"}; !slices.Equal(rowNames(got), want) || got.Truncated {
			t.Fatalf("rows = %v truncated=%v", rowNames(got), got.Truncated)
		}
		e.contract(t, "KubernetesService.List", got)
	})

	t.Run("all-namespaces", func(t *testing.T) {
		got, err := e.list(t, "v1/pods", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if columnNames(got)[0] != "Namespace" || len(got.Rows) != 4 || got.Rows[3].Cells[0] != "kube-system" {
			t.Fatalf("result = %+v", got)
		}
		e.contract(t, "KubernetesService.List:allNamespaces", got)
	})

	t.Run("label-selector", func(t *testing.T) {
		got, err := e.list(t, "v1/pods", "default", "app=web")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"web-1", "web-2"}; !slices.Equal(rowNames(got), want) {
			t.Fatalf("rows = %v", rowNames(got))
		}
		var sent bool
		for _, r := range e.fake.Requests("/namespaces/default/pods") {
			sent = sent || mustQuery(t, r.Query).Get("labelSelector") == "app=web"
		}
		if !sent {
			t.Fatalf("no request carried labelSelector=app=web: %+v", e.fake.Requests("/pods"))
		}
	})

	t.Run("bad-selector", func(t *testing.T) {
		before := len(e.fake.Requests(""))
		_, err := e.list(t, "v1/pods", "default", "app in (")
		wantCode(t, err, "E_BAD_REQUEST")
		if len(e.fake.Requests("")) != before {
			t.Error("a bad selector reached the API server")
		}
	})

	t.Run("cluster-scoped-ignores-namespace", func(t *testing.T) {
		got, err := e.list(t, "v1/nodes", "default", "")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(rowNames(got), []string{"node-1"}) || slices.Contains(columnNames(got), "Namespace") {
			t.Fatalf("result = %+v", got)
		}
		for _, r := range e.fake.Requests("/nodes") {
			if r.Path != "/api/v1/nodes" {
				t.Errorf("node list path = %s", r.Path)
			}
		}
	})

	t.Run("custom-resource", func(t *testing.T) {
		got, err := e.list(t, "example.com/v1/widgets", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(rowNames(got), []string{"w1"}) {
			t.Fatalf("rows = %v", rowNames(got))
		}
	})

	t.Run("unknown-kind", func(t *testing.T) {
		_, err := e.list(t, "v1/nothing", "", "")
		wantCode(t, err, "E_NOT_FOUND")
	})
}

func TestListTruncates(t *testing.T) {
	e := boot(t, nil, flowharness.WithKubernetesOptions(kubernetes.WithListLimits(2, 3)))
	got, err := e.list(t, "v1/pods", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 3 || !got.Truncated {
		t.Fatalf("rows = %d truncated = %v, want 3 and true", len(got.Rows), got.Truncated)
	}
	var paged bool
	for _, r := range e.fake.Requests("/pods") {
		q := mustQuery(t, r.Query)
		paged = paged || (q.Get("limit") == "2" && q.Get("continue") != "")
	}
	if !paged {
		t.Fatal("no continue request with limit=2")
	}
	e.contract(t, "KubernetesService.List:truncated", got)
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return q
}
