package embed

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var errWorkerExiting = errors.New("embedding worker is exiting")

// socketServer is the shared worker: one model, many connections, one inference at a time.
type socketServer struct {
	enc  encoder
	spec Spec
	idle time.Duration

	mu      sync.Mutex // held across an inference and across the idle check
	last    time.Time
	closing bool
	conns   map[net.Conn]struct{}
	done    chan struct{}
}

// serveSocket listens on path, prints one ready line to out, and serves until no request arrived
// for idle. It removes the socket on exit and returns the process exit code. SIGPIPE is ignored:
// the client that started this process may be gone, and so is the stderr pipe it left behind.
func serveSocket(path string, out *bufio.Writer, enc encoder, spec Spec, idle time.Duration) int {
	signal.Ignore(syscall.SIGPIPE)
	fail := func(err error) int {
		_ = json.NewEncoder(out).Encode(workerHello{Error: err.Error()})
		_ = out.Flush()
		return 2
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fail(err)
	}
	defer os.Remove(path)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return fail(err)
	}
	if err := json.NewEncoder(out).Encode(workerHello{Ready: true, Model: spec.ID, Dim: spec.Dim}); err != nil {
		_ = ln.Close()
		return 1
	}
	_ = out.Flush()

	s := &socketServer{enc: enc, spec: spec, idle: idle, last: time.Now(), conns: map[net.Conn]struct{}{}, done: make(chan struct{})}
	go s.watchIdle(ln)
	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}
		if !s.track(conn) {
			_ = conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer s.untrack(conn)
			serveConn(conn, bufio.NewWriter(conn), spec, s.embed)
		}()
	}
	<-s.done
	wg.Wait()
	return 0
}

func (s *socketServer) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return false
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *socketServer) untrack(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	_ = c.Close()
}

func (s *socketServer) embed(texts []string) ([][]float32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil, errWorkerExiting
	}
	vecs, err := s.enc.embed(texts)
	s.last = time.Now()
	return vecs, err
}

// watchIdle shuts the listener and every connection once idle has passed with no request; a client
// meets a closed connection and starts a fresh worker.
func (s *socketServer) watchIdle(ln net.Listener) {
	defer close(s.done)
	for {
		s.mu.Lock()
		wait := s.idle - time.Since(s.last)
		if wait <= 0 {
			s.closing = true
			for c := range s.conns {
				_ = c.Close()
			}
			s.mu.Unlock()
			_ = ln.Close()
			return
		}
		s.mu.Unlock()
		time.Sleep(min(wait, time.Second))
	}
}
