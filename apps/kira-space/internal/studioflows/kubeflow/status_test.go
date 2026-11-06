package kubeflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func (e *env) status(t *testing.T, ctx string, refresh bool) kubernetes.Status {
	t.Helper()
	st, err := e.app.W.Kubernetes.Status(kubernetes.StatusArgs{ClusterID: e.ids[ctx], Refresh: refresh})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStatus(t *testing.T) {
	e := boot(t, nil)

	t.Run("ok", func(t *testing.T) {
		st := e.status(t, "kind-dev", false)
		if st.State != "ok" || st.ServerVersion != "v1.31.0-fake" {
			t.Fatalf("status = %+v", st)
		}
		e.contract(t, "KubernetesService.Status", st)
	})

	t.Run("unauthorized", func(t *testing.T) {
		st := e.status(t, "bad-token", false)
		if st.State != "unavailable" || st.Reason != "unauthorized" {
			t.Fatalf("status = %+v", st)
		}
		e.contract(t, "KubernetesService.Status:unauthorized", st, flowharness.Mask("message"))
	})

	t.Run("unreachable", func(t *testing.T) {
		st := e.status(t, "down", false)
		if st.State != "unavailable" || st.Reason != "unreachable" {
			t.Fatalf("status = %+v", st)
		}
	})

	t.Run("unknown-id", func(t *testing.T) {
		_, err := e.app.W.Kubernetes.Status(kubernetes.StatusArgs{ClusterID: "nope"})
		wantCode(t, err, "E_NOT_FOUND")
	})

	t.Run("refresh-rereads-kubeconfig", func(t *testing.T) {
		raw, err := os.ReadFile(e.cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer writeFile(t, e.cfg, string(raw))
		if st := e.status(t, "bad-token", true); st.Reason != "unauthorized" {
			t.Fatalf("before rewrite: %+v", st)
		}
		writeFile(t, e.cfg, strings.Replace(string(raw), "wrong-token", flowharness.KubeGoodToken, 1))
		if st := e.status(t, "bad-token", true); st.State != "ok" {
			t.Fatalf("after rewrite: %+v", st)
		}
	})
}

func TestStatusExecPlugin(t *testing.T) {
	var bin string
	e := boot(t, nil, flowharness.WithKubernetesOptions(kubernetes.WithEnv(func() kubernetes.Env {
		return kubernetes.Env{Path: bin + ":/usr/bin:/bin", Kubeconfig: os.Getenv("KUBECONFIG")}
	})))
	bin = e.app.BinDir
	// client-go caches a plugin token process-wide, so the failing run goes first.
	// The process PATH lacks the plugin dir: only the resolved environment can find it.
	t.Setenv("PATH", "/usr/bin:/bin")

	t.Run("exit-1", func(t *testing.T) {
		fail := filepath.Join(bin, "kira-fake-cred.fail")
		writeFile(t, fail, "")
		defer os.Remove(fail)
		st := e.status(t, "plugin-ctx", true)
		if st.State != "unavailable" || st.Reason != "plugin" {
			t.Fatalf("status = %+v", st)
		}
		e.contract(t, "KubernetesService.Status:plugin", st, flowharness.Mask("message"))
	})

	t.Run("found-on-resolved-path", func(t *testing.T) {
		if st := e.status(t, "plugin-ctx", true); st.State != "ok" {
			t.Fatalf("status = %+v", st)
		}
	})
}
