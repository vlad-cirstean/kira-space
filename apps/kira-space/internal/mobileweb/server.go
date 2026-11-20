// Package mobileweb serves a mobile web app for the ADE agents module over plain HTTP on the
// trusted LAN interface. It is a second transport over the same services the Wails bridge binds,
// so it sits at bridge level in the layering rules.
package mobileweb

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/lannet"
	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"
)

// Config wires the server to the services it reads and the narrow Writer it writes through.
type Config struct {
	Reader Reader
	// Writer, Terminals and AgentInputEnabled are optional: without them the matching routes answer
	// 503 (Writer, Terminals) or refuse agent input (AgentInputEnabled).
	Writer            Writer
	Terminals         TerminalBroker
	AgentInputEnabled func() bool
	AgentSessions     func() any
	// AgentInfo is the active agent provider's display facts; the phone names the agent from it.
	AgentInfo func() any
	// ScriptRuns lists the newest script runs; Computer reads the machine summary. Both are optional:
	// without them the routes answer an empty list and a null Docker.
	ScriptRuns func(ctx context.Context, limit int) ([]any, error)
	Computer   func(ctx context.Context) (Computer, error)
	// Approvals is the app's approvals queue; without it the approvals routes answer 503.
	Approvals Approvals
	Devices   DeviceStore
	Hub       *Hub
	// Broker outlives the server: the desktop approval dialog stays subscribed across restarts.
	Broker *Broker
	// Assets is the mobile build (index.html, assets/).
	Assets fs.FS
	Port   int
	// Network is the initial binding: the one interface address the server listens on.
	Network lannet.Network
	// IsLAN decides which addresses the server may bind and serve (default lannet.IsLAN). Tests
	// widen it to loopback; production code leaves it nil.
	IsLAN func(netip.Addr) bool
	// Now, OnDevicesChanged and OnStatusChanged are optional (defaults: time.Now, none, none).
	// OnStatusChanged fires, off the server's goroutines, after the bound address changed.
	Now              func() time.Time
	OnDevicesChanged func()
	OnStatusChanged  func()
}

// Status is what the desktop pane shows.
type Status struct {
	Running bool
	AppURL  string
}

// Server is the mobile web server: one plain-HTTP listener on the trusted interface address.
type Server struct {
	cfg Config

	failedAuth *limiterSet
	deviceRate *limiterSet
	pairRate   *limiterSet
	writeRate  *limiterSet
	attachRate *limiterSet
	idem       *idemStore
	flight     singleflight.Group

	touchMu  sync.Mutex
	touched  map[string]time.Time
	expiries map[string]int64 // authenticated devices seen, id to ExpiresAt; guarded by touchMu

	mu         sync.Mutex
	running    bool
	net        lannet.Network
	isLAN      func(netip.Addr) bool // seam: tests widen it to loopback
	srv        *boundServer
	baseCtx    context.Context
	appHandler http.Handler
	baseCancel context.CancelFunc
	stop       chan struct{}
	wg         sync.WaitGroup
}

func New(cfg Config) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	isLAN := cfg.IsLAN
	if isLAN == nil {
		isLAN = lannet.IsLAN
	}
	return &Server{
		cfg:        cfg,
		net:        cfg.Network,
		isLAN:      isLAN,
		failedAuth: newLimiterSet(rate.Every(time.Minute), 10, cfg.Now),
		deviceRate: newLimiterSet(20, 40, cfg.Now),
		pairRate:   newLimiterSet(rate.Every(10*time.Second), 3, cfg.Now),
		writeRate:  newLimiterSet(1, 10, cfg.Now),
		attachRate: newLimiterSet(rate.Every(2*time.Second), 3, cfg.Now),
		idem:       newIdemStore(idemTTL),
		touched:    map[string]time.Time{},
		expiries:   map[string]int64{},
	}
}

// Start binds the listener or fails. It refuses a wildcard, loopback or non-LAN address: the
// network layer already guarantees one, this is the second check.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	addr := s.net.Addr.Addr()
	if !addr.IsValid() || addr.IsUnspecified() || !s.isLAN(addr) {
		return fmt.Errorf("mobileweb: refusing to listen on %s: not a private LAN address", addr)
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	s.baseCtx = baseCtx
	s.appHandler = s.guard(securityHeaders(s.appMux()))
	b, err := s.listen(addr)
	if err != nil {
		cancel()
		return err
	}
	s.srv, s.baseCancel, s.stop, s.running = b, cancel, make(chan struct{}), true
	s.serve(b)
	s.wg.Add(1)
	go s.maintain(s.stop)
	return nil
}

// boundServer is one listener with the server that will serve it.
type boundServer struct {
	srv *http.Server
	ln  net.Listener
}

// serve starts serving b. Callers hold s.mu with s.running set.
func (s *Server) serve(b *boundServer) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := b.srv.Serve(b.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("mobileweb: serve", "scope", "mobileweb", "err", err)
		}
	}()
}

// listen opens the listener on addr, or fails. Serving starts in serve.
func (s *Server) listen(addr netip.Addr) (*boundServer, error) {
	ln, err := net.Listen("tcp4", net.JoinHostPort(addr.String(), strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return nil, listenError(addr, s.cfg.Port, err)
	}
	return &boundServer{ln: ln, srv: &http.Server{
		Handler: s.appHandler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return s.baseCtx },
	}}, nil
}

func listenError(addr netip.Addr, port int, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("mobileweb: port %d is in use on %s; choose another port in Settings", port, addr)
	}
	return fmt.Errorf("mobileweb: listen on %s:%d: %w", addr, port, err)
}

// maintain sweeps idle limiter entries. The bridge supervisor follows network changes.
func (s *Server) maintain(stop <-chan struct{}) {
	defer s.wg.Done()
	sweep := time.NewTicker(time.Minute)
	defer sweep.Stop()
	for {
		select {
		case <-sweep.C:
			s.failedAuth.sweep()
			s.deviceRate.sweep()
			s.pairRate.sweep()
			s.writeRate.sweep()
			s.attachRate.sweep()
			s.sweepTouched()
			s.sweepExpired()
		case <-stop:
			return
		}
	}
}

func (s *Server) sweepTouched() {
	cutoff := s.cfg.Now().Add(-touchEvery)
	s.touchMu.Lock()
	defer s.touchMu.Unlock()
	for id, at := range s.touched {
		if at.Before(cutoff) {
			delete(s.touched, id)
		}
	}
}

// sweepExpired ends the streams and terminals of devices past ExpiresAt: authenticate checks
// expiry only at request start, so a stream opened earlier would otherwise outlive it.
func (s *Server) sweepExpired() {
	now := s.cfg.Now().UnixMilli()
	var expired []string
	s.touchMu.Lock()
	for id, at := range s.expiries {
		if at <= now {
			expired = append(expired, id)
			delete(s.expiries, id)
		}
	}
	s.touchMu.Unlock()
	for _, id := range expired {
		s.cfg.Hub.DisconnectDevice(id)
		if s.cfg.Terminals != nil {
			s.cfg.Terminals.ReleaseDevice(id)
		}
	}
}

// SetNetwork rebinds when the interface address changed (a new DHCP lease): the new listener opens
// first, then the old one closes. A failure keeps the current binding; the supervisor retries on
// its next poll. A no-op while stopped.
func (s *Server) SetNetwork(n lannet.Network) {
	s.mu.Lock()
	if !s.running || n.Addr == s.net.Addr {
		s.mu.Unlock()
		return
	}
	if n.Addr.Addr() == s.net.Addr.Addr() {
		s.net = n
		s.mu.Unlock()
		return
	}
	addr := n.Addr.Addr()
	if !s.isLAN(addr) {
		s.mu.Unlock()
		slog.Warn("mobileweb: rebind refused", "scope", "mobileweb", "addr", addr.String())
		return
	}
	b, err := s.listen(addr)
	if err != nil {
		s.mu.Unlock()
		slog.Warn("mobileweb: bind new address", "scope", "mobileweb", "err", err)
		return
	}
	old := s.srv
	s.srv, s.net = b, n
	s.serve(b)
	s.mu.Unlock()

	_ = old.srv.Close()
	if s.cfg.OnStatusChanged != nil {
		go s.cfg.OnStatusChanged()
	}
}

// guard refuses a peer outside the bound subnet or not private/link-local IPv4, and a Host header
// that is not the bound address (DNS-rebinding guard).
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		n, isLAN := s.net, s.isLAN
		s.mu.Unlock()
		if !peerAllowed(r.RemoteAddr, n.Addr, isLAN) || !hostAllowed(r.Host, n.Addr.Addr()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Close stops accepting, releases parked pairing requests and open streams, then drains with a 5s
// grace before forcing connections shut. Safe to call when stopped.
func (s *Server) Close() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	srv, cancel, stop := s.srv.srv, s.baseCancel, s.stop
	s.srv = nil
	s.mu.Unlock()

	// Cancelling the base context fires each parked pair request's AfterFunc, so Shutdown does not
	// wait out the 120s approval window.
	cancel()
	close(stop)
	s.cfg.Hub.CloseAll()
	if s.cfg.Terminals != nil {
		s.cfg.Terminals.ReleaseAll("server stopped")
	}
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	err := srv.Shutdown(ctx)
	if err != nil {
		_ = srv.Close()
	}
	s.wg.Wait()
	return err
}

// Status reports the running state and the URL a phone opens.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return Status{}
	}
	return Status{Running: true, AppURL: "http://" + net.JoinHostPort(s.net.Addr.Addr().String(), strconv.Itoa(s.cfg.Port)) + "/"}
}

// Revoke marks the device revoked, then ends its streams. The store write comes first so a
// reconnect racing the disconnect is already refused.
func (s *Server) Revoke(id string) error {
	if err := s.cfg.Devices.Revoke(id, s.cfg.Now().UnixMilli()); err != nil {
		return err
	}
	s.cfg.Hub.DisconnectDevice(id)
	if s.cfg.Terminals != nil {
		s.cfg.Terminals.ReleaseDevice(id)
	}
	return nil
}

// PermissionsChanged ends a device's terminals when its agent input flag is off. The desktop
// calls it after storing new flags.
func (s *Server) PermissionsChanged(id string) {
	if s.cfg.Terminals == nil {
		return
	}
	row, found, err := s.cfg.Devices.ByID(id)
	if err != nil {
		slog.Warn("mobileweb: device lookup", "scope", "mobileweb", "device", id, "err", err)
		return
	}
	if !found || row.RevokedAt != nil || !row.CanAgentInput {
		s.cfg.Terminals.ReleaseDevice(id)
	}
}
