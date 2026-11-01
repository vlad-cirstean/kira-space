package configflow_test

import (
	"bytes"
	"context"
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

//go:embed testdata/probe.schema.json
var probeSchema []byte

const (
	secretValue = "PLAINTEXT-s3cret-7f3a"
	pastedToken = "ghp_pastedLiteralToken9f2"
)

type probe struct {
	Name   string `json:"name"`
	Target struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"target"`
}

// memProjection is the in-memory Projection of the test kind: nothing in P450 persists it.
type memProjection struct {
	mu sync.Mutex
	m  map[string]string
}

func (p *memProjection) Snapshot() (map[string]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]string, len(p.m))
	for k, v := range p.m {
		out[k] = v
	}
	return out, nil
}

func (p *memProjection) Record(file, js string, _ int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[file] = js
	return nil
}

func (p *memProjection) Forget(file string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, file)
	return nil
}

// allBytes concatenates every file under dir whose name starts with prefix.
func allBytes(t *testing.T, dir, prefix string) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), prefix) {
			return err
		}
		b, rerr := os.ReadFile(p)
		sb.Write(b)
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// TestSecretReferenceResolution: a file holds a reference only. Resolve yields the plaintext at use
// time; the projection, the DB and the log never hold it; a pasted literal is refused without echo.
func TestSecretReferenceResolution(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	app := flowharness.New(t)
	t.Setenv("KIRA_PROBE_HOST", "probe.example")

	decode, _, err := configfs.YAMLSchema[probe](probeSchema)
	if err != nil {
		t.Fatal(err)
	}
	proj := &memProjection{m: map[string]string{}}
	folder := &configfs.Folder[probe]{Dir: filepath.Join(app.SpaceHome, "probes"), Ext: ".yaml", Decode: decode, Projection: proj}
	resolver := &configfs.Resolver{
		Env: os.LookupEnv,
		Secrets: configfs.SecretSourceFunc(func(_ context.Context, ns, name string) (string, error) {
			if ns == "probe" && name == "api_token" {
				return secretValue, nil
			}
			return "", configfs.ErrSecretMissing
		}),
		Namespace: "probe",
	}

	src := "name: demo\ntarget:\n  url: https://${KIRA_PROBE_HOST}/v1\n  headers:\n    authorization: ${secret:api_token}\n"
	entry, err := folder.Save("probe.yaml", "", []byte(src))
	if err != nil || entry.Problem != nil {
		t.Fatalf("Save = %+v, %v", entry.Problem, err)
	}

	url, used, err := resolver.Resolve(context.Background(), entry.Value.Target.URL)
	if err != nil || url != "https://probe.example/v1" || len(used) != 0 {
		t.Fatalf("url = %q, used %v, %v", url, used, err)
	}
	header, used, err := resolver.Resolve(context.Background(), entry.Value.Target.Headers["authorization"])
	if err != nil || header != secretValue || len(used) != 1 || used[0].Placeholder != "${secret:api_token}" {
		t.Fatalf("header = %q, used %+v, %v", header, used, err)
	}

	if js := proj.m["probe.yaml"]; !strings.Contains(js, "${secret:api_token}") || strings.Contains(js, secretValue) {
		t.Errorf("projection = %s", js)
	}
	for _, surface := range []struct{ name, text string }{
		{"app log", logs.String()},
		{"kira.db", allBytes(t, app.SpaceHome, "kira.db")},
		{"file", allBytes(t, folder.Dir, "probe")},
	} {
		if strings.Contains(surface.text, secretValue) {
			t.Errorf("%s holds the plaintext", surface.name)
		}
	}

	bad := strings.Replace(src, "${secret:api_token}", pastedToken, 1)
	entry, err = folder.Save("probe.yaml", "", []byte(bad))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Problem == nil || entry.Problem.Line != 5 || strings.Contains(entry.Problem.Message, pastedToken) {
		t.Fatalf("literal secret problem = %+v", entry.Problem)
	}
	if entry.Value == nil || entry.Value.Target.Headers["authorization"] != "${secret:api_token}" {
		t.Error("the last good value was not kept")
	}
	if strings.Contains(logs.String(), pastedToken) || strings.Contains(proj.m["probe.yaml"], pastedToken) {
		t.Error("the pasted literal reached the log or the projection")
	}

	if _, _, err := resolver.Resolve(context.Background(), "${KIRA_PROBE_UNSET}"); err == nil || !strings.Contains(err.Error(), "KIRA_PROBE_UNSET") {
		t.Errorf("unset env: %v", err)
	}
	if _, _, err := resolver.Resolve(context.Background(), "${secret:gone}"); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Errorf("missing secret: %v", err)
	}
}
