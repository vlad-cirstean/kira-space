// Package jsengine runs untrusted JavaScript (goja) for the MCP servers. Each Exec spawns a fresh
// `kira-space js-worker` child, so memory, CPU and native-recursion ceilings hold by killing the
// child, never the app. The package imports no domain package: callers pass data in through
// Bindings and get one JSON value or a structured Error out.
package jsengine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/kirathecat/kira-studio/internal/memory/workerproc"
)

// Run is one script execution.
type Run struct {
	Source   string
	Bindings []Binding
	Limits   Limits    // zero fields take defaults
	Now      time.Time // Date.now() and new Date() for the whole run; zero is time.Now() at Exec start
	Seed     uint64    // Math.random seed; 0 derives it from SourceHash
}

// Result is a successful run.
type Result struct {
	JSON    json.RawMessage // compact, key order as JSON.stringify gives it, big integers exact; "null" for undefined
	Type    string          // undefined | null | boolean | number | bigint | string | array | object
	Elapsed time.Duration   // script time inside the worker, spawn excluded
}

// Engine runs scripts. Safe for concurrent use; each Exec runs in its own worker process.
type Engine struct {
	sem     chan struct{}
	command func() *exec.Cmd
}

var errExecPanic = errors.New("jsengine exec panicked")

// New returns an Engine with MaxConcurrent slots. The worker is this binary with "js-worker".
func New() *Engine {
	exe, err := os.Executable()
	if err == nil {
		if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = resolved
		}
	}
	return newEngine(func() *exec.Cmd { return exec.Command(exe, "js-worker") })
}

func newEngine(command func() *exec.Cmd) *Engine {
	return &Engine{sem: make(chan struct{}, MaxConcurrent), command: command}
}

// Exec runs r in a worker process. The error is *Error for script failures, a plain error for
// programming errors (limits above a ceiling) and spawn faults.
func (e *Engine) Exec(ctx context.Context, r Run) (res Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("exec panicked", "scope", "jsengine", "err", errExecPanic)
			res, err = Result{}, newError(KindInternal, "internal error")
		}
	}()
	lim, lerr := r.Limits.normalize()
	if lerr != nil {
		return Result{}, lerr
	}
	if _, cerr := compileSource(r.Source); cerr != nil {
		return Result{}, cerr
	}
	if berr := validateBindings(r.Bindings, lim.InputBytes); berr != nil {
		return Result{}, berr
	}
	total := 0
	byName := make(map[string]Binding, len(r.Bindings))
	wire := make([]wireBinding, len(r.Bindings))
	for i, b := range r.Bindings {
		total += len(b.Value)
		byName[b.Name] = b
		wire[i] = wireBinding{Name: b.Name, Value: b.Value}
		for fn := range b.Funcs {
			wire[i].Funcs = append(wire[i].Funcs, fn)
		}
		if b.Funcs != nil && len(wire[i].Funcs) == 0 {
			wire[i].Funcs = []string{}
		}
	}
	if total*memoryPerInput > lim.MemoryBytes {
		return Result{}, newError(KindInput, "input too large for the memory limit; raise MemoryBytes")
	}
	marker, merr := newMarker(r.Bindings)
	if merr != nil {
		return Result{}, merr
	}
	now := r.Now
	if now.IsZero() {
		now = time.Now()
	}
	seed := r.Seed
	if seed == 0 {
		sum, _ := hex.DecodeString(SourceHash(r.Source))
		seed = binary.BigEndian.Uint64(sum[:8])
	}

	select {
	case e.sem <- struct{}{}:
	case <-ctx.Done():
		return Result{}, newError(KindCanceled, "canceled while waiting for a free script slot")
	}
	// The deadline starts at the slot, not at Exec: queue time is not script time. Spawn counts.
	deadline := time.Now().Add(lim.Timeout)

	cmd := e.command()
	cmd.Env = []string{"GOTRACEBACK=none", "GOMAXPROCS=2", "TZ=UTC"}
	cmd.Dir = os.TempDir()
	p, serr := workerproc.Start(cmd)
	if serr != nil {
		<-e.sem
		return Result{}, fmt.Errorf("jsengine: start worker: %w", serr)
	}
	stop := make(chan struct{})
	defer func() {
		close(stop)
		p.Kill()
		<-p.Done()
		<-e.sem
	}()

	var hello workerMsg
	if herr := p.Hello(ctx, helloTimeout, &hello); herr != nil {
		<-p.Done()
		switch {
		case ctx.Err() != nil:
			return Result{}, newError(KindCanceled, "script canceled")
		case errors.Is(herr, workerproc.ErrHelloTimeout):
			return Result{}, newError(KindInternal, "script worker did not start")
		}
		return Result{}, exitError(p.Diagnostics())
	}
	var baseRSS uint64
	var proc *process.Process
	if pr, perr := process.NewProcess(int32(cmd.Process.Pid)); perr == nil {
		proc = pr
		if mi, merr := pr.MemoryInfo(); merr == nil {
			baseRSS = mi.RSS
		}
	}
	rssLimit := baseRSS + uint64(lim.hardCeiling()) + 64<<20

	line, jerr := json.Marshal(workerRequest{
		Source: r.Source, Bindings: wire, Limits: lim,
		TimeoutNs: int64(time.Until(deadline)), NowMs: now.UnixMilli(), Seed: seed, Marker: marker,
	})
	if jerr != nil {
		return Result{}, newError(KindInternal, "internal error")
	}
	if _, werr := p.Stdin.Write(append(line, '\n')); werr != nil {
		<-p.Done()
		return Result{}, exitError(p.Diagnostics())
	}

	lines := make(chan workerMsg)
	go func() {
		defer close(lines)
		for {
			var m workerMsg
			if err := p.Dec.Decode(&m); err != nil {
				return
			}
			select {
			case lines <- m:
			case <-stop:
				return
			}
		}
	}()

	hctx, hcancel := context.WithDeadline(ctx, deadline)
	defer hcancel()
	hostRes := make(chan replyMsg, 1)
	killTimer := time.NewTimer(time.Until(deadline) + killGrace)
	defer killTimer.Stop()
	rssTick := time.NewTicker(50 * time.Millisecond)
	defer rssTick.Stop()

	for {
		select {
		case m, ok := <-lines:
			if !ok {
				<-p.Done()
				return Result{}, exitError(p.Diagnostics())
			}
			switch {
			case m.Call != nil:
				go func(c *callMsg) { hostRes <- callHost(hctx, byName, c, lim.InputBytes) }(m.Call)
			case m.Result != nil:
				p.Stop()
				if len(m.Result.JSON) > lim.OutputBytes {
					return Result{}, newError(KindOutput, "result exceeds %d bytes", lim.OutputBytes)
				}
				return Result{JSON: m.Result.JSON, Type: m.Result.Type, Elapsed: time.Duration(m.Result.ElapsedNs)}, nil
			case m.Error != nil:
				p.Stop()
				return Result{}, m.Error
			}
		case rep := <-hostRes:
			b, _ := json.Marshal(parentMsg{Reply: &rep})
			_, _ = p.Stdin.Write(append(b, '\n')) // a dead worker shows up as closed lines
		case <-ctx.Done():
			return Result{}, newError(KindCanceled, "script canceled")
		case <-killTimer.C:
			return Result{}, newError(KindTimeout, "script exceeded its time limit")
		case <-rssTick.C:
			if proc == nil {
				continue
			}
			if mi, merr := proc.MemoryInfo(); merr == nil && mi.RSS > rssLimit {
				return Result{}, newError(KindMemory, "script exceeded its memory limit")
			}
		}
	}
}

// newMarker draws the per-run BigInt marker, redrawing if any input contains it.
func newMarker(bs []Binding) (string, *Error) {
	for range 8 {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", newError(KindInternal, "internal error")
		}
		m := hex.EncodeToString(raw[:])
		clash := false
		for _, b := range bs {
			if bytes.Contains(b.Value, []byte(m)) {
				clash = true
			}
		}
		if !clash {
			return m, nil
		}
	}
	return "", newError(KindInternal, "internal error")
}
