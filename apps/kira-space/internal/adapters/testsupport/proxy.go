package testsupport

import (
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// PausableProxy is a TCP forwarder whose server-to-client direction can be frozen, so a test can
// hold a query mid-flight (request sent, response withheld) deterministically.
type PausableProxy struct {
	ln     net.Listener
	target string
	cfg    model.ResolvedConnectionConfig

	mu     sync.Mutex
	cond   *sync.Cond
	paused bool
	conns  []net.Conn
}

// StartPausableProxy forwards a fresh local port to cfg's host and port until the test ends; use
// Config for the connection config that dials through it.
func StartPausableProxy(t *testing.T, cfg model.ResolvedConnectionConfig) *PausableProxy {
	t.Helper()
	return StartPausableProxyTo(t, net.JoinHostPort(*cfg.Host, strconv.Itoa(*cfg.Port)), cfg)
}

// StartPausableProxyTo forwards a fresh local port to target ("host:port"); cfg is the config
// Config re-points. Engines configured by endpoint option (sqs, s3) use Addr instead of Config.
func StartPausableProxyTo(t *testing.T, target string, cfg model.ResolvedConnectionConfig) *PausableProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	p := &PausableProxy{ln: ln, target: target, cfg: cfg}
	p.cond = sync.NewCond(&p.mu)
	go p.accept()
	t.Cleanup(p.close)
	return p
}

func (p *PausableProxy) accept() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		upstream, err := net.Dial("tcp", p.target)
		if err != nil {
			_ = client.Close()
			continue
		}
		p.mu.Lock()
		p.conns = append(p.conns, client, upstream)
		p.mu.Unlock()
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
		go p.pump(client, upstream)
	}
}

func (p *PausableProxy) pump(client, upstream net.Conn) {
	defer client.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := upstream.Read(buf)
		if n > 0 {
			p.mu.Lock()
			for p.paused {
				p.cond.Wait()
			}
			p.mu.Unlock()
			if _, werr := client.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Pause withholds every server-to-client byte, on existing and new connections, until Resume.
func (p *PausableProxy) Pause() {
	p.mu.Lock()
	p.paused = true
	p.mu.Unlock()
}

// Resume releases the withheld bytes.
func (p *PausableProxy) Resume() {
	p.mu.Lock()
	p.paused = false
	p.mu.Unlock()
	p.cond.Broadcast()
}

func (p *PausableProxy) close() {
	p.Resume()
	_ = p.ln.Close()
	p.mu.Lock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.mu.Unlock()
}

// Config is the fixture's config re-pointed at the proxy.
func (p *PausableProxy) Config() model.ResolvedConnectionConfig {
	cfg := p.cfg
	host, portText, _ := net.SplitHostPort(p.ln.Addr().String())
	port, _ := strconv.Atoi(portText)
	cfg.Host = &host
	cfg.Port = &port
	return cfg
}

// Addr is the proxy's listen address, "127.0.0.1:port".
func (p *PausableProxy) Addr() string { return p.ln.Addr().String() }
