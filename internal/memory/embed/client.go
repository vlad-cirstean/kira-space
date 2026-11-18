package embed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/memory/workerproc"
)

// State is the client's coarse availability.
type State string

const (
	StateNotInstalled State = "notInstalled"
	StateUnavailable  State = "unavailable"
	StateReady        State = "ready"
)

// Status is State plus a human-readable cause for StateUnavailable.
type Status struct {
	State   State
	Message string
}

const (
	helloTimeout   = 30 * time.Second
	requestTimeout = 2 * time.Minute
	failureBackoff = 60 * time.Second
)

// DefaultIdleTimeout is how long an unused worker keeps its model in memory.
const DefaultIdleTimeout = 5 * time.Minute

// ErrClosed is returned by Embed after Close.
var ErrClosed = errors.New("embed: client closed")

// ClientOptions configures NewClient. Zero values pick the production defaults.
type ClientOptions struct {
	Spec Spec
	Home string
	// Command builds the private worker process for modelDir; nil runs this executable as
	// `memory-embed`, and (with Socket unset) shares one worker per user, see SocketPath.
	Command func(modelDir string) *exec.Cmd
	// Socket is the unix socket of the shared worker. Empty with a nil Command picks
	// SocketPath(Home); empty with a Command means a private worker per client.
	Socket string
	// ServeCommand builds the shared worker process listening on socket; nil runs this executable.
	ServeCommand func(modelDir, socket string) *exec.Cmd
	IdleTimeout  time.Duration
	// OnState runs after every change of Status, outside the client's locks.
	OnState func()
}

// Client runs embeddings in a lazily spawned worker process, so the model's memory returns to the
// OS when the worker exits after IdleTimeout, and a native crash cannot take the host down.
type Client struct {
	opts ClientOptions
	dir  string
	sem  chan struct{} // one request in flight

	mu       sync.Mutex
	w        *worker
	lastErr  error
	failedAt time.Time
	closed   bool
	idle     *time.Timer
	notified Status
}

func NewClient(o ClientOptions) *Client {
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = DefaultIdleTimeout
	}
	if o.Command == nil {
		o.Command = defaultCommand
		if o.Socket == "" {
			o.Socket = SocketPath(o.Home)
		}
	}
	if o.ServeCommand == nil {
		o.ServeCommand = defaultServeCommand
	}
	c := &Client{opts: o, dir: ModelDir(o.Home, o.Spec), sem: make(chan struct{}, 1)}
	c.notified = c.Status()
	return c
}

func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return "kira-space-missing-executable"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

func defaultCommand(modelDir string) *exec.Cmd {
	return exec.Command(executable(), "memory-embed", "--model-dir", modelDir)
}

func defaultServeCommand(modelDir, socket string) *exec.Cmd {
	return exec.Command(executable(), "memory-embed", "--model-dir", modelDir, "--socket", socket)
}

func (c *Client) Spec() Spec { return c.opts.Spec }

// Dir is the directory the model installs into.
func (c *Client) Dir() string { return c.dir }

// Status reports availability without spawning anything.
func (c *Client) Status() Status {
	if !Installed(c.dir, c.opts.Spec) {
		return Status{State: StateNotInstalled}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastErr != nil {
		return Status{State: StateUnavailable, Message: c.lastErr.Error()}
	}
	return Status{State: StateReady}
}

func (c *Client) notify() {
	st := c.Status()
	c.mu.Lock()
	changed := st != c.notified
	c.notified = st
	c.mu.Unlock()
	if changed && c.opts.OnState != nil {
		c.opts.OnState()
	}
}

// Reset clears a recorded failure and its spawn backoff, e.g. after an install.
func (c *Client) Reset() {
	c.mu.Lock()
	c.lastErr, c.failedAt = nil, time.Time{}
	c.mu.Unlock()
	c.notify()
}

// Embed returns one L2-normalised vector per text, adding the spec's query prefix when query is
// set. A cancelled ctx kills the worker: ONNX Runtime cannot be interrupted mid-run.
func (c *Client) Embed(ctx context.Context, texts []string, query bool) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer c.notify()

	in := make([]string, len(texts))
	for i, t := range texts {
		if query {
			t = c.opts.Spec.QueryPrefix + t
		}
		in[i] = t
	}
	out := make([][]float32, 0, len(in))
	for start := 0; start < len(in); start += MaxBatch {
		batch := in[start:min(start+MaxBatch, len(in))]
		vecs, err := c.roundTrip(ctx, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (c *Client) roundTrip(ctx context.Context, texts []string) ([][]float32, error) {
	return c.roundTripOnce(ctx, texts, false)
}

func (c *Client) roundTripOnce(ctx context.Context, texts []string, retried bool) ([][]float32, error) {
	w, err := c.ensureWorker(ctx)
	if err != nil {
		return nil, err
	}
	c.stopIdle()
	defer c.armIdle()

	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	stop := context.AfterFunc(rctx, w.Kill)
	defer stop()

	w.nextID++
	req := workerRequest{ID: w.nextID, Texts: texts}
	var rep workerReply
	err = json.NewEncoder(w.out).Encode(req)
	if err == nil {
		err = w.dec.Decode(&rep)
	}
	if err != nil {
		w.Kill()
		c.dropWorker(w)
		if ctxErr := rctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// A shared worker may have idled out since the last request: one fresh connection.
		if w.shared && !retried {
			return c.roundTripOnce(ctx, texts, true)
		}
		err = fmt.Errorf("embedding worker failed: %w%s", err, w.Diagnostics())
		c.setErr(err)
		return nil, err
	}
	if rep.Error != "" {
		return nil, fmt.Errorf("embedding worker: %s", rep.Error)
	}
	if rep.ID != req.ID || len(rep.Vectors) != len(texts) {
		w.Kill()
		c.dropWorker(w)
		err := errors.New("embedding worker: malformed reply")
		c.setErr(err)
		return nil, err
	}
	vecs := make([][]float32, len(rep.Vectors))
	for i, s := range rep.Vectors {
		raw, err := base64.StdEncoding.DecodeString(s)
		if err == nil {
			vecs[i], err = Decode(raw, c.opts.Spec.Dim)
		}
		if err != nil {
			return nil, fmt.Errorf("embedding worker: bad vector: %w", err)
		}
	}
	c.mu.Lock()
	c.lastErr = nil
	c.mu.Unlock()
	return vecs, nil
}

func (c *Client) setErr(err error) {
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
}

func (c *Client) dropWorker(w *worker) {
	c.mu.Lock()
	if c.w == w {
		c.w = nil
	}
	c.mu.Unlock()
}

// ensureWorker returns the running worker, spawning one when needed. A failed spawn is refused
// again for failureBackoff so per-keystroke searches cannot spawn-storm.
func (c *Client) ensureWorker(ctx context.Context) (*worker, error) {
	c.mu.Lock()
	switch {
	case c.closed:
		c.mu.Unlock()
		return nil, ErrClosed
	case c.w != nil:
		w := c.w
		c.mu.Unlock()
		return w, nil
	case c.lastErr != nil && !c.failedAt.IsZero() && time.Since(c.failedAt) < failureBackoff:
		err := c.lastErr
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()

	if !Installed(c.dir, c.opts.Spec) {
		return nil, errors.New("embedding model is not installed")
	}
	w, err := c.spawn(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		c.lastErr, c.failedAt = err, time.Now()
		return nil, err
	}
	c.lastErr, c.failedAt = nil, time.Time{}
	c.w = w
	return w, nil
}

func (c *Client) spawn(ctx context.Context) (*worker, error) {
	if c.opts.Socket != "" {
		w, err := c.connectShared(ctx)
		if err == nil || ctx.Err() != nil {
			return w, err
		}
		// No shared worker on this platform or path: a private one, as before.
	}
	return c.spawnPrivate(ctx)
}

func (c *Client) spawnPrivate(ctx context.Context) (*worker, error) {
	proc, err := workerproc.Start(c.opts.Command(c.dir))
	if err != nil {
		return nil, fmt.Errorf("start embedding worker: %w", err)
	}
	w := &worker{out: proc.Stdin, dec: proc.Dec, kill: proc.Kill, stop: proc.Stop, diag: proc.Diagnostics}
	var hello workerHello
	if err := proc.Hello(ctx, helloTimeout, &hello); err != nil {
		if errors.Is(err, workerproc.ErrHelloTimeout) {
			return nil, fmt.Errorf("embedding worker did not start in time%s", proc.Diagnostics())
		}
		return nil, fmt.Errorf("embedding worker exited before ready: %w%s", err, proc.Diagnostics())
	}
	if err := c.checkHello(hello); err != nil {
		w.Kill()
		return nil, err
	}
	return w, nil
}

func (c *Client) checkHello(hello workerHello) error {
	if hello.Error != "" || !hello.Ready || hello.Dim != c.opts.Spec.Dim {
		msg := hello.Error
		if msg == "" {
			msg = "unexpected hello"
		}
		return errors.New(msg)
	}
	return nil
}

func (c *Client) stopIdle() {
	c.mu.Lock()
	if c.idle != nil {
		c.idle.Stop()
	}
	c.mu.Unlock()
}

func (c *Client) armIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.w == nil {
		return
	}
	if c.idle != nil {
		c.idle.Stop()
	}
	c.idle = time.AfterFunc(c.opts.IdleTimeout, c.onIdle)
}

func (c *Client) onIdle() {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	default:
		return // a request is in flight; it re-arms the timer
	}
	c.mu.Lock()
	w := c.w
	c.w = nil
	c.mu.Unlock()
	if w != nil {
		w.Stop()
	}
}

// Close stops the worker and refuses further requests.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	if c.idle != nil {
		c.idle.Stop()
	}
	w := c.w
	c.w = nil
	c.mu.Unlock()
	if w != nil {
		w.Stop()
	}
	return nil
}

// worker is a private worker process or a connection to the shared one, plus its request counter.
type worker struct {
	out    io.Writer
	dec    *json.Decoder
	kill   func()
	stop   func()
	diag   func() string
	shared bool
	nextID int
}

func (w *worker) Kill()               { w.kill() }
func (w *worker) Stop()               { w.stop() }
func (w *worker) Diagnostics() string { return w.diag() }
