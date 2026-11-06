package kubeflow

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestClusters(t *testing.T) {
	e := boot(t, nil)
	res, err := e.app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.State != "ok" || !slices.Equal(res.Config.Paths, []string{e.cfg}) {
		t.Fatalf("config = %+v", res.Config)
	}
	var names []string
	for _, c := range res.Clusters {
		names = append(names, c.Context)
		if c.Source != e.cfg {
			t.Errorf("%s source = %q, want %q", c.Context, c.Source, e.cfg)
		}
	}
	if want := []string{"kind-dev", "bad-token", "down", "plugin-ctx"}; !slices.Equal(names, want) {
		t.Fatalf("contexts = %v, want %v", names, want)
	}
	if !res.Clusters[0].Current || res.Clusters[0].Namespace != "default" || res.Clusters[0].Server != e.fake.URL {
		t.Fatalf("first cluster = %+v", res.Clusters[0])
	}
	e.contract(t, "KubernetesService.Clusters", res)

	t.Run("ids-survive-restart", func(t *testing.T) {
		before := map[string]string{}
		for k, v := range e.ids {
			before[k] = v
		}
		e.app.Restart()
		again, err := e.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range again.Clusters {
			if before[c.Context] != c.ID {
				t.Errorf("%s id changed across restart: %s to %s", c.Context, before[c.Context], c.ID)
			}
		}
	})

	t.Run("missing", func(t *testing.T) {
		none := filepath.Join(e.app.Home, "none")
		t.Setenv("KUBECONFIG", none)
		got, err := e.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		if got.Config.State != "missing" || len(got.Clusters) != 0 || !slices.Equal(got.Config.Paths, []string{none}) {
			t.Fatalf("result = %+v", got)
		}
		e.contract(t, "KubernetesService.Clusters:missing", got)
	})

	t.Run("invalid", func(t *testing.T) {
		bad := filepath.Join(e.app.Home, "bad")
		writeFile(t, bad, "clusters: [unterminated\n  - : :\n")
		t.Setenv("KUBECONFIG", bad)
		got, err := e.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		if got.Config.State != "invalid" || got.Config.Message == "" || len(got.Clusters) != 0 {
			t.Fatalf("result = %+v", got)
		}
		e.contract(t, "KubernetesService.Clusters:invalid", got, flowharness.Mask("message"))
	})
}
