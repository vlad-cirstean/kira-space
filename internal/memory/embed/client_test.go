package embed

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/memory/modelstore"
)

var testSpec = Spec{
	ID: "fake", Dim: 8, QueryPrefix: "q: ",
	Files: []File{{Name: ModelFile, Size: 1}, {Name: TokenizerFile, Size: 1}},
}

// fakeEncoder derives a unit vector from each text's hash.
type fakeEncoder struct {
	crashAfter int // exit the process after this many embed calls; 0 never
	slow       bool
	calls      int
}

func (f *fakeEncoder) embed(texts []string) ([][]float32, error) {
	f.calls++
	if f.slow {
		time.Sleep(30 * time.Second)
	}
	if f.crashAfter > 0 && f.calls > f.crashAfter {
		fmt.Fprintln(os.Stderr, "fake native abort")
		os.Exit(3)
	}
	return fakeVectors(texts), nil
}

func (f *fakeEncoder) close() {}

func fakeVectors(texts []string) [][]float32 {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		sum := sha256.Sum256([]byte(t))
		v := make([]float32, testSpec.Dim)
		for j := range v {
			v[j] = float32(sum[j]) + 1
		}
		normalize(v)
		out[i] = v
	}
	return out
}

func TestMain(m *testing.M) {
	if mode := os.Getenv("KIRA_EMBED_FAKE_WORKER"); mode != "" {
		if mode == "hello-error" {
			fmt.Println(`{"error":"no runtime here"}`)
			os.Exit(2)
		}
		if mode == "shared" {
			idle, _ := strconv.Atoi(os.Getenv("KIRA_EMBED_FAKE_IDLE_MS"))
			os.Exit(serveSocket(os.Getenv("KIRA_EMBED_FAKE_SOCKET"), bufio.NewWriter(os.Stdout), &fakeEncoder{}, testSpec, time.Duration(idle)*time.Millisecond))
		}
		enc := &fakeEncoder{slow: mode == "slow"}
		if n, ok := strings.CutPrefix(mode, "crash-after="); ok {
			enc.crashAfter, _ = strconv.Atoi(n)
		}
		os.Exit(serve(os.Stdin, bufio.NewWriter(os.Stdout), enc, testSpec))
	}
	os.Exit(m.Run())
}

func newTestClient(t *testing.T, mode string, idle time.Duration, spawns *atomic.Int32) *Client {
	t.Helper()
	home := t.TempDir()
	dir := ModelDir(home, testSpec)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range testSpec.Files {
		if err := os.WriteFile(dir+"/"+f.Name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := modelstore.WriteManifest(dir, testSpec.ID, testSpec.Files); err != nil {
		t.Fatal(err)
	}
	c := NewClient(ClientOptions{
		Spec: testSpec, Home: home, IdleTimeout: idle,
		Command: func(string) *exec.Cmd {
			if spawns != nil {
				spawns.Add(1)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "KIRA_EMBED_FAKE_WORKER="+mode)
			return cmd
		},
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestParallelEmbedsSerialiseOnOneWorker(t *testing.T) {
	var spawns atomic.Int32
	c := newTestClient(t, "ok", time.Minute, &spawns)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text := fmt.Sprintf("fact %d", i)
			got, err := c.Embed(context.Background(), []string{text}, false)
			if err != nil {
				t.Error(err)
				return
			}
			if Dot(got[0], fakeVectors([]string{text})[0]) < 0.999 {
				t.Errorf("reply %d crossed with another request", i)
			}
		}()
	}
	wg.Wait()
	if n := spawns.Load(); n != 1 {
		t.Errorf("spawns = %d, want 1", n)
	}
}

func TestQueryPrefixAndBatching(t *testing.T) {
	c := newTestClient(t, "ok", time.Minute, nil)
	texts := make([]string, MaxBatch+5)
	for i := range texts {
		texts[i] = fmt.Sprint("t", i)
	}
	got, err := c.Embed(context.Background(), texts, true)
	if err != nil || len(got) != len(texts) {
		t.Fatalf("got %d vectors, err %v", len(got), err)
	}
	want := fakeVectors([]string{testSpec.QueryPrefix + texts[MaxBatch+4]})[0]
	if Dot(got[len(got)-1], want) < 0.999 {
		t.Error("query prefix not applied across batches")
	}
}

func TestIdleTimeoutClosesWorkerAndNextCallRespawns(t *testing.T) {
	var spawns atomic.Int32
	c := newTestClient(t, "ok", 100*time.Millisecond, &spawns)
	if _, err := c.Embed(context.Background(), []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.w == nil
	})
	if _, err := c.Embed(context.Background(), []string{"b"}, false); err != nil {
		t.Fatal(err)
	}
	if n := spawns.Load(); n != 2 {
		t.Errorf("spawns = %d, want 2", n)
	}
}

func TestCrashMidRequestThenRespawn(t *testing.T) {
	var spawns atomic.Int32
	c := newTestClient(t, "crash-after=1", time.Minute, &spawns)
	if _, err := c.Embed(context.Background(), []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	_, err := c.Embed(context.Background(), []string{"b"}, false)
	if err == nil || !strings.Contains(err.Error(), "fake native abort") {
		t.Fatalf("err = %v, want the worker's stderr in it", err)
	}
	if st := c.Status(); st.State != StateUnavailable {
		t.Errorf("state after crash = %v", st.State)
	}
	if _, err := c.Embed(context.Background(), []string{"c"}, false); err != nil {
		t.Fatalf("respawn failed: %v", err)
	}
	if st := c.Status(); st.State != StateReady {
		t.Errorf("state after respawn = %v", st.State)
	}
	if n := spawns.Load(); n != 2 {
		t.Errorf("spawns = %d, want 2", n)
	}
}

func TestHelloErrorBacksOffUntilReset(t *testing.T) {
	var spawns atomic.Int32
	c := newTestClient(t, "hello-error", time.Minute, &spawns)
	_, err := c.Embed(context.Background(), []string{"a"}, false)
	if err == nil || !strings.Contains(err.Error(), "no runtime here") {
		t.Fatalf("err = %v", err)
	}
	if st := c.Status(); st.State != StateUnavailable || !strings.Contains(st.Message, "no runtime here") {
		t.Errorf("status = %+v", st)
	}
	if _, err := c.Embed(context.Background(), []string{"a"}, false); err == nil {
		t.Fatal("second call should fail")
	}
	if n := spawns.Load(); n != 1 {
		t.Errorf("spawns during backoff = %d, want 1", n)
	}
	c.Reset()
	_, _ = c.Embed(context.Background(), []string{"a"}, false)
	if n := spawns.Load(); n != 2 {
		t.Errorf("spawns after Reset = %d, want 2", n)
	}
}

func TestContextCancelKillsSlowRequest(t *testing.T) {
	c := newTestClient(t, "slow", time.Minute, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Embed(ctx, []string{"a"}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("cancel did not interrupt the worker")
	}
	if st := c.Status(); st.State != StateReady {
		t.Errorf("a cancel is not a failure; state = %v", st.State)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}
