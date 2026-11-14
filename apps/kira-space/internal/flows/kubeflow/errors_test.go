package kubeflow

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// J-K8S-022: each credential or network failure class maps to its own code and message, and one list
// call sends a bounded number of requests (no retry storm).
func TestListFailureClasses(t *testing.T) {
	e := boot(t, []flowharness.KubeFakeOption{flowharness.ForbidResource("secrets")})
	listOn := func(ctx, kind string) error {
		_, err := e.app.W.Kubernetes.List(kubernetes.ListArgs{ClusterID: e.ids[ctx], Kind: kind, Namespace: "default"})
		return err
	}
	cases := []struct {
		name, ctx, kind, code string
		maxReqs               int
		listPath              string // the list endpoint; must be hit at most once
	}{
		{"expired-or-wrong-token", "bad-token", "v1/pods", "E_UNAUTHORIZED", 4, ""},
		{"rbac-forbidden", "kind-dev", "v1/secrets", "E_FORBIDDEN", 10, "/secrets"},
		{"unreachable", "down", "v1/pods", "E_UNAVAILABLE", 0, ""},
	}
	msgs := map[string]string{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := len(e.fake.Requests(""))
			start := time.Now()
			err := listOn(c.ctx, c.kind)
			var ie *ipcerr.Error
			if !errors.As(err, &ie) || ie.Code != c.code {
				t.Fatalf("error = %v, want code %s", err, c.code)
			}
			if d := time.Since(start); d > 8*time.Second {
				t.Fatalf("failure took %s", d)
			}
			if n := len(e.fake.Requests("")) - before; n > c.maxReqs {
				t.Fatalf("%d requests for one failed list, want at most %d", n, c.maxReqs)
			}
			if c.listPath != "" {
				if n := len(e.fake.Requests(c.listPath)); n != 1 {
					t.Fatalf("%s hit %d times, want 1 (no retry)", c.listPath, n)
				}
			}
			if strings.Contains(ie.Message, flowharness.KubeGoodToken) || strings.Contains(ie.Message, "wrong-token") {
				t.Fatalf("message leaks a token: %q", ie.Message)
			}
			msgs[c.name] = ie.Message
		})
	}
	seen := map[string]string{}
	for name, m := range msgs {
		if other, dup := seen[m]; dup {
			t.Errorf("%s and %s share the message %q", name, other, m)
		}
		seen[m] = name
	}
}
