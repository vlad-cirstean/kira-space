// Package docker is the Docker engine module's Go side, shared by both apps: endpoint resolution,
// resource reads, container actions, and per-window event, stats, log and exec streams. Streams
// are owned per window and torn down on explicit close, window close and app quit.
package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/moby/moby/client"
	"golang.org/x/sync/singleflight"
)

const (
	pingTimeout = 3 * time.Second
	callTimeout = 15 * time.Second
)

// Status reasons.
const (
	reasonNotInstalled = "not-installed"
	reasonDaemonDown   = "daemon-down"
	reasonPermission   = "permission-denied"
	reasonUnreachable  = "unreachable"
	reasonTLS          = "tls"
	reasonError        = "error"
)

// EngineInfo is the engine facts the overview shows.
type EngineInfo struct {
	Version         string `json:"version"`
	APIVersion      string `json:"apiVersion"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	OperatingSystem string `json:"operatingSystem"`
	KernelVersion   string `json:"kernelVersion"`
	CPUs            int    `json:"cpus"`
	MemTotal        int64  `json:"memTotal"`
	Containers      int    `json:"containers"`
	Running         int    `json:"running"`
	Paused          int    `json:"paused"`
	Stopped         int    `json:"stopped"`
	Images          int    `json:"images"`
}

// Status is the connection state plus the endpoint it describes.
type Status struct {
	State    string      `json:"state"` // "ok" | "unavailable"
	Reason   string      `json:"reason,omitempty"`
	Message  string      `json:"message,omitempty"`
	Endpoint Endpoint    `json:"endpoint"`
	Engine   *EngineInfo `json:"engine,omitempty"`
}

// Manager owns the lazily built engine client, the context chosen in the UI and every stream.
type Manager struct {
	Emit appevent.Emitter
	env  resolveEnv

	mu       sync.Mutex
	cli      *client.Client
	endpoint Endpoint
	selected string

	events *eventsWatcher
	// restore is nil without a RestoreStore.
	restore       *restoreTracker
	restoreStore  RestoreStore
	restoreTiming restoreTiming
	bootProbe     BootProbe
	stats         *statsHub
	logs          *logRegistry
	execs         *execRegistry

	// containerd caches whether the engine uses the containerd image store; reset with the client.
	containerd *bool
	// HubAPI is the Docker Hub web API base; a flow-test seam, production keeps DefaultHubAPI.
	HubAPI string
	hub    hubCache

	flight singleflight.Group
	// editing holds the container IDs with an edit in flight.
	editing sync.Map
	// restarting holds the container IDs with a restart in flight.
	restarting sync.Map
}

// NewManager builds a Manager reading the real process environment.
func NewManager(emit appevent.Emitter) *Manager {
	return newManager(emit, osResolveEnv())
}

func newManager(emit appevent.Emitter, env resolveEnv) *Manager {
	m := &Manager{Emit: emit, env: env, HubAPI: DefaultHubAPI, restoreTiming: defaultRestoreTiming}
	m.events = newEventsWatcher(m)
	m.logs = newLogRegistry()
	m.execs = newExecRegistry()
	m.stats = newStatsHub(engineStats{m}, func(windowKey string, ev StatsEvent) { emit.EmitTo(windowKey, ChannelStats, ev) }, statsEmitInterval)
	return m
}

// useContext switches the engine for every window ("" = automatic resolution): all streams end,
// the client is rebuilt, and windows are told to refetch everything.
func (m *Manager) useContext(ctx context.Context, name string) Status {
	m.mu.Lock()
	m.selected = name
	m.resetLocked()
	m.mu.Unlock()
	m.stopStreams()
	m.events.restart()
	m.restore.contextChanged()
	st := m.status(ctx, false)
	m.Emit.Emit(ChannelStatus, st)
	m.Emit.Emit(ChannelChanged, ChangedEvent{Kinds: allKinds})
	return st
}

// stopStreams ends every stream that belongs to the current engine.
func (m *Manager) stopStreams() {
	m.stats.reset()
	m.logs.closeAll()
	m.execs.closeAll()
}

// closeWindow ends everything windowKey started.
func (m *Manager) closeWindow(windowKey string) {
	m.events.unwatch(windowKey)
	m.stats.unsubscribe(windowKey)
	m.logs.closeWindow(windowKey)
	m.execs.closeWindow(windowKey)
}

// shutdown ends every stream and drops the client.
func (m *Manager) shutdown() {
	m.restore.stop()
	m.events.shutdown()
	m.stopStreams()
	m.mu.Lock()
	m.resetLocked()
	m.mu.Unlock()
}

// client returns the cached client, building it from the resolved endpoint on first use.
func (m *Manager) client() (*client.Client, Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cli != nil {
		return m.cli, m.endpoint, nil
	}
	ep, opts, err := resolveEndpoint(m.env, m.selected)
	if err != nil {
		return nil, ep, err
	}
	cli, err := client.New(append(opts, client.WithAPIVersionNegotiation())...)
	if err != nil {
		return nil, ep, err
	}
	m.cli, m.endpoint = cli, ep
	return cli, ep, nil
}

// resetLocked drops the cached client; the next client() call rebuilds it.
func (m *Manager) resetLocked() {
	if m.cli != nil {
		_ = m.cli.Close()
		m.cli = nil
		m.containerd = nil
	}
	if m.env.Current != nil {
		m.env.Current.forget()
	}
}

// dropFallbackClient drops the cached client when it came from the default or probe fallback, so the
// next call re-resolves once an engine socket appears. It reports whether it dropped.
func (m *Manager) dropFallbackClient() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cli == nil || (m.endpoint.Source != sourceDefault && m.endpoint.Source != sourceProbe) {
		return false
	}
	m.resetLocked()
	return true
}

// endpointRemote reports whether the endpoint is tcp:// or ssh://, without building a client.
func (m *Manager) endpointRemote() bool {
	m.mu.Lock()
	cli, ep, selected := m.cli, m.endpoint, m.selected
	m.mu.Unlock()
	if cli == nil {
		var err error
		if ep, _, err = resolveEndpoint(m.env, selected); err != nil {
			return false
		}
	}
	return ep.Remote
}

func (m *Manager) selectedContext() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.selected
}

// status pings the engine; refresh drops the cached client first so a changed environment is seen.
func (m *Manager) status(ctx context.Context, refresh bool) Status {
	if refresh {
		m.mu.Lock()
		m.resetLocked()
		m.mu.Unlock()
	}
	cli, ep, err := m.client()
	if err != nil {
		return m.unavailable(ep, err)
	}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	ping, err := cli.Ping(pctx, client.PingOptions{NegotiateAPIVersion: true})
	if err != nil {
		return m.unavailable(ep, err)
	}
	info, err := cli.Info(pctx, client.InfoOptions{})
	if err != nil {
		return m.unavailable(ep, err)
	}
	i := info.Info
	return Status{State: "ok", Endpoint: ep, Engine: &EngineInfo{
		Version: i.ServerVersion, APIVersion: ping.APIVersion, OS: i.OSType, Arch: i.Architecture,
		OperatingSystem: i.OperatingSystem, KernelVersion: i.KernelVersion, CPUs: i.NCPU, MemTotal: i.MemTotal,
		Containers: i.Containers, Running: i.ContainersRunning, Paused: i.ContainersPaused,
		Stopped: i.ContainersStopped, Images: i.Images,
	}}
}

func (m *Manager) unavailable(ep Endpoint, err error) Status {
	if ep.Host == "" {
		ep.Host = m.env.Getenv("DOCKER_HOST")
	}
	return Status{State: "unavailable", Reason: m.classify(ep, err), Message: err.Error(), Endpoint: ep}
}

// classify maps a connection error to one of the status reasons.
func (m *Manager) classify(ep Endpoint, err error) string {
	var (
		certErr  *tls.CertificateVerificationError
		recErr   tls.RecordHeaderError
		unknown  x509.UnknownAuthorityError
		hostname x509.HostnameError
		netErr   net.Error
	)
	switch {
	case errors.As(err, &certErr), errors.As(err, &recErr), errors.As(err, &unknown), errors.As(err, &hostname):
		return reasonTLS
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM), errors.Is(err, fs.ErrPermission):
		return reasonPermission
	case ep.Remote:
		return reasonUnreachable
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED):
		if errors.Is(err, fs.ErrNotExist) && m.notInstalled(ep) {
			return reasonNotInstalled
		}
		return reasonDaemonDown
	case errors.As(err, &netErr) && netErr.Timeout():
		return reasonUnreachable
	default:
		return reasonError
	}
}

// notInstalled reports a missing unix socket with no ~/.docker directory: Docker was never set up.
func (m *Manager) notInstalled(ep Endpoint) bool {
	return ep.Source != sourceSelected && ep.Source != sourceEnv && !m.env.exists(m.env.configDir())
}
