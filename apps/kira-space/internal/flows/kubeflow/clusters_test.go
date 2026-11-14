package kubeflow

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
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

// J-K8S-012: KUBECONFIG with several files merges them kubectl-style (first file wins a duplicate name),
// a missing current-context is not an error, and one malformed file reports invalid.
func TestKubeconfigEdgeCases(t *testing.T) {
	e := boot(t, nil)
	one := func(ctxName, server string) string {
		return "apiVersion: v1\nkind: Config\nclusters:\n- name: c-" + ctxName + "\n  cluster: {server: " + server + "}\n" +
			"contexts:\n- name: " + ctxName + "\n  context: {cluster: c-" + ctxName + ", user: u}\nusers:\n- name: u\n  user: {token: t}\n"
	}
	a := filepath.Join(e.app.Home, "cfg-a")
	b := filepath.Join(e.app.Home, "cfg-b")
	writeFile(t, a, one("alpha", "https://alpha.example:6443"))
	writeFile(t, b, one("beta", "https://beta.example:6443")+"")
	contexts := func() ([]string, kubernetes.ClustersResult) {
		res, err := e.app.W.Kubernetes.Clusters()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, c := range res.Clusters {
			names = append(names, c.Context)
		}
		return names, res
	}

	t.Run("two-files-no-current-context", func(t *testing.T) {
		t.Setenv("KUBECONFIG", a+string(filepath.ListSeparator)+b)
		names, res := contexts()
		if !slices.Equal(names, []string{"alpha", "beta"}) || res.Config.State != "ok" || !slices.Equal(res.Config.Paths, []string{a, b}) {
			t.Fatalf("contexts = %v config = %+v", names, res.Config)
		}
		for _, c := range res.Clusters {
			if c.Current {
				t.Errorf("%s marked current with no current-context", c.Context)
			}
		}
		e.contract(t, "KubernetesService.Clusters:twoFiles", res, flowharness.Replace(a, "<a>"), flowharness.Replace(b, "<b>"))
	})

	t.Run("duplicate-name-first-file-wins", func(t *testing.T) {
		dup := filepath.Join(e.app.Home, "cfg-dup")
		writeFile(t, dup, one("alpha", "https://other.example:6443"))
		t.Setenv("KUBECONFIG", a+string(filepath.ListSeparator)+dup)
		names, res := contexts()
		if !slices.Equal(names, []string{"alpha"}) || res.Clusters[0].Server != "https://alpha.example:6443" {
			t.Fatalf("contexts = %v first = %+v", names, res.Clusters)
		}
	})

	t.Run("one-malformed-file", func(t *testing.T) {
		bad := filepath.Join(e.app.Home, "cfg-bad")
		writeFile(t, bad, "clusters: [unterminated\n  - : :\n")
		t.Setenv("KUBECONFIG", a+string(filepath.ListSeparator)+bad)
		names, res := contexts()
		if res.Config.State != "invalid" || res.Config.Message == "" {
			t.Fatalf("config = %+v, want invalid", res.Config)
		}
		if len(names) != 0 {
			t.Fatalf("contexts = %v from a malformed set, want none", names)
		}
	})

	t.Run("missing-file-among-several", func(t *testing.T) {
		t.Setenv("KUBECONFIG", a+string(filepath.ListSeparator)+filepath.Join(e.app.Home, "nope"))
		names, res := contexts()
		if !slices.Equal(names, []string{"alpha"}) || res.Config.State != "ok" {
			t.Fatalf("contexts = %v config = %+v", names, res.Config)
		}
	})
}
