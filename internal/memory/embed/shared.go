package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/internal/filelock"
	"github.com/kirathecat/kira-studio/internal/workerproc"
)

const (
	socketName = "embed.sock"
	lockName   = "embed.lock"
	// maxSocketPath keeps below the 104-byte sun_path of macOS; a longer home shares nothing.
	maxSocketPath = 100
	dialTimeout   = 2 * time.Second
	lockPoll      = 50 * time.Millisecond
)

// SocketPath is the shared worker's unix socket under home, "" when the path is too long for one.
// Every client of one home, the app and each `memory-mcp`, reaches the same worker through it, so
// one copy of the model serves all sessions.
func SocketPath(home string) string {
	p := filepath.Join(home, socketName)
	if home == "" || len(p) > maxSocketPath {
		return ""
	}
	return p
}

// connectShared dials the shared worker, starting it first when none listens. Clients racing the
// first dial serialise on a lock file: the holder spawns, the rest wait for the socket.
func (c *Client) connectShared(ctx context.Context) (*worker, error) {
	if w, err := c.dialShared(ctx); err == nil {
		return w, nil
	}
	lockPath := filepath.Join(filepath.Dir(c.opts.Socket), lockName)
	deadline := time.Now().Add(helloTimeout)
	for {
		lock, ok, err := filelock.TryLock(lockPath)
		if err != nil {
			return nil, err
		}
		if ok {
			defer func() { _ = lock.Close() }()
			return c.startShared(ctx)
		}
		select {
		case <-time.After(lockPoll):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if w, err := c.dialShared(ctx); err == nil {
			return w, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("embedding worker did not start in time")
		}
	}
}

// startShared runs under the lock: another holder may have started the worker already, else a
// stale socket file is cleared and a detached worker is spawned.
func (c *Client) startShared(ctx context.Context) (*worker, error) {
	if w, err := c.dialShared(ctx); err == nil {
		return w, nil
	}
	_ = os.Remove(c.opts.Socket)
	cmd := c.opts.ServeCommand(c.dir, c.opts.Socket)
	detach(cmd)
	proc, err := workerproc.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start embedding worker: %w", err)
	}
	// The worker prints one line once it listens; it outlives this process, so nothing more is read.
	var ready workerHello
	if err := proc.Hello(ctx, helloTimeout, &ready); err != nil {
		if errors.Is(err, workerproc.ErrHelloTimeout) {
			return nil, fmt.Errorf("embedding worker did not start in time%s", proc.Diagnostics())
		}
		return nil, fmt.Errorf("embedding worker exited before ready: %w%s", err, proc.Diagnostics())
	}
	if ready.Error != "" {
		return nil, errors.New(ready.Error)
	}
	return c.dialShared(ctx)
}

func (c *Client) dialShared(ctx context.Context) (*worker, error) {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "unix", c.opts.Socket)
	if err != nil {
		return nil, err
	}
	w := &worker{
		out: conn, dec: json.NewDecoder(conn), shared: true,
		kill: func() { _ = conn.Close() },
		stop: func() { _ = conn.Close() },
		diag: func() string { return "" },
	}
	var hello workerHello
	_ = conn.SetReadDeadline(time.Now().Add(helloTimeout))
	err = w.dec.Decode(&hello)
	_ = conn.SetReadDeadline(time.Time{})
	if err == nil {
		err = c.checkHello(hello)
	}
	if err != nil {
		_ = conn.Close()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("embedding worker closed the connection")
		}
		return nil, err
	}
	return w, nil
}
