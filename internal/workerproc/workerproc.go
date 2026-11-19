// Package workerproc runs a helper process that speaks NDJSON over stdin/stdout: it owns the pipes,
// the hello handshake under a timeout, a graceful stop, a kill and the stderr tail used in error
// messages. Idle and backoff policy stay with each caller.
package workerproc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// StopGrace is how long Stop waits for a worker to exit after stdin closes.
	StopGrace  = 2 * time.Second
	stderrTail = 2048
)

// ErrHelloTimeout means the worker did not send its hello line in time.
var ErrHelloTimeout = errors.New("worker hello timed out")

// Proc is a started worker process.
type Proc struct {
	Stdin io.WriteCloser
	Dec   *json.Decoder

	cmd     *exec.Cmd
	tail    *tailBuffer
	done    chan struct{}
	waitErr error
}

// Start launches cmd with stdin/stdout pipes and a stderr tail.
func Start(cmd *exec.Cmd) (*Proc, error) {
	tail := &tailBuffer{}
	cmd.Stderr = tail
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &Proc{Stdin: stdin, Dec: json.NewDecoder(stdout), cmd: cmd, tail: tail, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// Hello decodes the worker's first line into v. On failure it kills the worker and returns
// ErrHelloTimeout when timeout or ctx ran out, else the decode error.
func (p *Proc) Hello(ctx context.Context, timeout time.Duration, v any) error {
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(hctx, p.Kill)
	defer stop()
	if err := p.Dec.Decode(v); err != nil {
		p.Kill()
		if hctx.Err() != nil {
			return ErrHelloTimeout
		}
		return err
	}
	return nil
}

// Done closes when the process has exited.
func (p *Proc) Done() <-chan struct{} { return p.done }

// Kill terminates the process.
func (p *Proc) Kill() { _ = p.cmd.Process.Kill() }

// Stop closes stdin so the worker exits on its own, killing it after StopGrace.
func (p *Proc) Stop() {
	_ = p.Stdin.Close()
	select {
	case <-p.done:
	case <-time.After(StopGrace):
		p.Kill()
		<-p.done
	}
}

// Diagnostics is " (exit error: stderr tail)" once the process has exited or after StopGrace, else
// the tail alone; empty when there is nothing to say.
func (p *Proc) Diagnostics() string {
	select {
	case <-p.done:
	case <-time.After(StopGrace):
	}
	var parts []string
	select {
	case <-p.done:
		if p.waitErr != nil {
			parts = append(parts, p.waitErr.Error())
		}
	default:
	}
	if s := strings.TrimSpace(p.tail.String()); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf(" (%s)", strings.Join(parts, ": "))
}

// tailBuffer keeps the last stderrTail bytes the worker wrote.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if over := t.buf.Len() - stderrTail; over > 0 {
		t.buf.Next(over)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buf.String()
}
