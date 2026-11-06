package kubeflow

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	wait     = 10 * time.Second
	scenario = "kubernetes"
)

type env struct {
	app  *flowharness.App
	fake *flowharness.KubeFake
	cfg  string
	ids  map[string]string
}

// boot starts the fake API server and the app over a four-context kubeconfig.
func boot(t *testing.T, fopts []flowharness.KubeFakeOption, opts ...flowharness.Opt) *env {
	t.Helper()
	fake := flowharness.NewKubeFake(t, fopts...)
	app := flowharness.New(t, opts...)
	e := &env{app: app, fake: fake, cfg: filepath.Join(app.Home, "kubeconfig"), ids: map[string]string{}}
	fake.WriteKubeconfig(t, e.cfg, flowharness.KubeconfigOpts{BinDir: app.BinDir})
	t.Setenv("KUBECONFIG", e.cfg)
	e.loadIDs(t)
	t.Cleanup(func() {
		for _, r := range fake.Requests("") {
			if r.Method != http.MethodGet {
				t.Errorf("non-GET request reached the API server: %s %s", r.Method, r.Path)
			}
		}
	})
	return e
}

func (e *env) loadIDs(t *testing.T) {
	t.Helper()
	res, err := e.app.W.Kubernetes.Clusters()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Clusters {
		e.ids[c.Context] = c.ID
	}
}

func (e *env) contract(t *testing.T, key string, got any, extra ...flowharness.ContractOption) {
	t.Helper()
	opts := append([]flowharness.ContractOption{
		flowharness.Replace(e.fake.URL, "<kube>"), flowharness.Replace(e.fake.DownURL(), "<down>"),
		flowharness.Replace(e.cfg, "<kubeconfig>"),
	}, extra...)
	e.app.StudioContract(t, scenario, key, got, opts...)
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
}

// logsOf returns the lines and end state of one stream recorded since mark.
func logsOf(app *flowharness.App, mark int, window, streamID string) (lines []kubernetes.LogLine, ended bool, errMsg string) {
	for _, ev := range app.Events.Since(mark, kubernetes.ChannelLogs) {
		le, ok := ev.Data.(kubernetes.LogsEvent)
		if !ok || le.StreamID != streamID || ev.Window != window {
			continue
		}
		lines = append(lines, le.Lines...)
		if le.Ended {
			ended, errMsg = true, le.Error
		}
	}
	return lines, ended, errMsg
}

func waitLines(t *testing.T, app *flowharness.App, mark int, window, streamID string, n int) []kubernetes.LogLine {
	t.Helper()
	var lines []kubernetes.LogLine
	testx.WaitUntil(t, wait, func() bool {
		var ended bool
		lines, ended, _ = logsOf(app, mark, window, streamID)
		return len(lines) >= n || ended
	})
	return lines
}

func waitEnded(t *testing.T, app *flowharness.App, mark int, window, streamID string) ([]kubernetes.LogLine, string) {
	t.Helper()
	var lines []kubernetes.LogLine
	var errMsg string
	testx.WaitUntil(t, wait, func() bool {
		var ended bool
		lines, ended, errMsg = logsOf(app, mark, window, streamID)
		return ended
	})
	return lines, errMsg
}

func texts(lines []kubernetes.LogLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Text)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
