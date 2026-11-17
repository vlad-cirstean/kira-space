package embed

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/memory/modelstore"
)

// sharedClient is a client on the shared socket in dir; spawns counts the workers it started.
func sharedClient(t *testing.T, dir string, idle time.Duration, spawns *atomic.Int32) *Client {
	t.Helper()
	model := ModelDir(dir, testSpec)
	if err := os.MkdirAll(model, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range testSpec.Files {
		if err := os.WriteFile(filepath.Join(model, f.Name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := modelstore.WriteManifest(model, testSpec.ID, testSpec.Files); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, socketName)
	c := NewClient(ClientOptions{
		Spec: testSpec, Home: dir, Socket: sock, IdleTimeout: time.Hour,
		Command: func(string) *exec.Cmd { return exec.Command("false") },
		ServeCommand: func(string, string) *exec.Cmd {
			spawns.Add(1)
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "KIRA_EMBED_FAKE_WORKER=shared",
				"KIRA_EMBED_FAKE_SOCKET="+sock, "KIRA_EMBED_FAKE_IDLE_MS="+strconv.Itoa(int(idle.Milliseconds())))
			return cmd
		},
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func embedOK(t *testing.T, c *Client, text string) {
	t.Helper()
	got, err := c.Embed(context.Background(), []string{text}, false)
	if err != nil {
		t.Errorf("embed %q: %v", text, err)
		return
	}
	if Dot(got[0], fakeVectors([]string{text})[0]) < 0.999 {
		t.Errorf("wrong vector for %q", text)
	}
}

// Clients racing the first dial start exactly one worker; once it idles out the socket is gone and
// the next request, from an old connection or a new client, starts exactly one more.
func TestSharedWorkerOneSpawnPerGeneration(t *testing.T) {
	dir, err := os.MkdirTemp("", "ke")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	var spawns atomic.Int32
	a := sharedClient(t, dir, 700*time.Millisecond, &spawns)
	b := sharedClient(t, dir, 700*time.Millisecond, &spawns)

	var wg sync.WaitGroup
	for i, c := range []*Client{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			embedOK(t, c, "first "+strconv.Itoa(i))
		}()
	}
	wg.Wait()
	if n := spawns.Load(); n != 1 {
		t.Fatalf("spawns after racing first dial = %d, want 1", n)
	}

	waitFor(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, socketName))
		return os.IsNotExist(err)
	})
	embedOK(t, a, "after idle, old connection")
	embedOK(t, sharedClient(t, dir, 700*time.Millisecond, &spawns), "after idle, new client")
	if n := spawns.Load(); n != 2 {
		t.Fatalf("spawns after idle exit = %d, want 2", n)
	}
}
