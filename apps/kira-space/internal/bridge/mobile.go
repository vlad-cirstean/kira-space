package bridge

import (
	"io/fs"
	"log/slog"
	"net/netip"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/lannet"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileterm"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileweb"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/embedded"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/pairing"
)

// Stop reasons the pane shows while the server is enabled but stopped.
const (
	mobileStopNotTrusted  = "notTrusted"
	mobileStopAway        = "away"
	mobileStopOtherRouter = "otherRouter"
	mobileStopUnavailable = "unavailable"
)

// MobileNetwork is a network as the pane shows it.
type MobileNetwork struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Subnet    string `json:"subnet"`
	RouterIP  string `json:"routerIp"`
	RouterMAC string `json:"routerMac"`
}

// MobileStatus is the wire projection of the mobile server state plus its settings.
type MobileStatus struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	AppURL  string `json:"appUrl"`
	// AgentInput is the global switch for phones replying to agents and attaching to terminals.
	AgentInput bool   `json:"agentInput"`
	Error      string `json:"error"`
	// StopReason is "" while running or disabled; StopDetail is its user-facing text.
	StopReason string `json:"stopReason"`
	StopDetail string `json:"stopDetail"`
	// Current is what the default route shows now (nil when undetectable); Trusted is the stored one.
	Current *MobileNetwork `json:"current"`
	Trusted *MobileNetwork `json:"trusted"`
	// TrustedAt is epoch ms, 0 when nothing is trusted.
	TrustedAt int64 `json:"trustedAt"`
}

// MobilePairingRequest is the approval prompt's wire projection (absolute deadline in epoch ms).
type MobilePairingRequest struct {
	RequestID   string `json:"requestId"`
	ClientID    string `json:"clientId"`
	Label       string `json:"label"`
	Code        string `json:"code"`
	RemoteIP    string `json:"remoteIp"`
	UserAgent   string `json:"userAgent"`
	ExpiresAtMs int64  `json:"expiresAtMs"`
}

type MobilePairingSnapshot struct {
	Pending *MobilePairingRequest `json:"pending"`
	Queued  int                   `json:"queued"`
}

type MobilePairingActionResult struct {
	Result string `json:"result"` // "resolved" | "alreadyResolved" | "expired"
}

func toWireMobileSnapshot(snap pairing.Snapshot[mobileweb.MobileMeta]) MobilePairingSnapshot {
	out := MobilePairingSnapshot{Queued: snap.Queued}
	if p := snap.Pending; p != nil {
		out.Pending = &MobilePairingRequest{
			RequestID: p.RequestID, ClientID: p.ClientID, Label: p.Meta.Label, Code: p.Meta.Code,
			RemoteIP: p.Meta.RemoteIP, UserAgent: p.Meta.UserAgent, ExpiresAtMs: p.ExpiresAt.UnixMilli(),
		}
	}
	return out
}

func toWireMobileResult(r pairing.ActionResult) MobilePairingActionResult {
	switch r {
	case pairing.Resolved:
		return MobilePairingActionResult{Result: "resolved"}
	case pairing.Expired:
		return MobilePairingActionResult{Result: "expired"}
	default:
		return MobilePairingActionResult{Result: "alreadyResolved"}
	}
}

// MobileTerminalBroker is the phone-terminal broker (*mobileterm.Broker): the server-side attach
// plus the desktop's view of holds and its reclaim.
type MobileTerminalBroker interface {
	mobileweb.TerminalBroker
	Holds() []mobileterm.Hold
	Reclaim(terminalID string, cols, rows int)
}

// MobileAccessService is the Mobile access pane's surface: enable/disable the phone web server,
// trust a network, move its port, list and revoke phones, answer the pairing prompt. It owns the server lifecycle.
type MobileAccessService struct {
	Deps appcore.Deps
	// Reader is the read-only ADE slice the phone sees; AgentSessions the live session list.
	Reader        mobileweb.Reader
	AgentSessions func() any
	// AgentInfo is the agent provider's display facts for the phone.
	AgentInfo func() any
	// Assets is the embedded mobile build.
	Assets fs.FS
	Hub    *mobileweb.Hub
	// Broker outlives server restarts; AttachMobilePush runs its expiry loop and shuts it down.
	Broker *mobileweb.Broker
	// Writer is the phone's write path and Launches the rendezvous with the desktop window that
	// opens a phone-started launch's terminal. Terminals owns phone-attached agent terminals.
	Writer    mobileweb.Writer
	Launches  *MobileLaunches
	Terminals MobileTerminalBroker

	// Detect, Find and Poll default to lannet and ten seconds.
	Detect func() (lannet.Network, error)
	Find   func(lannet.Identity) (lannet.Network, error)
	Poll   time.Duration
	// IsLAN is a test seam passed to the server's Config.IsLAN (loopback in the flow harness); nil in production.
	IsLAN func(netip.Addr) bool

	embedded embedded.Service[*mobileweb.Server, MobileStatus]
	sup      mobileSupervisor
}

func NewMobileAccessService(s *MobileAccessService) *MobileAccessService {
	if s.Detect == nil {
		s.Detect = lannet.Detect
	}
	if s.Find == nil {
		s.Find = lannet.Find
	}
	if s.Poll <= 0 {
		s.Poll = 10 * time.Second
	}
	s.embedded = embedded.Service[*mobileweb.Server, MobileStatus]{
		StartFn: func(bool) (*mobileweb.Server, error) {
			cfg, err := s.Deps.Repos.Settings.GetAll()
			if err != nil {
				return nil, err
			}
			srv := mobileweb.New(mobileweb.Config{
				Reader: s.Reader, AgentSessions: s.AgentSessions, AgentInfo: s.AgentInfo, Devices: s.Deps.Repos.MobileDevices,
				Writer: s.Writer, Terminals: s.Terminals, AgentInputEnabled: s.agentInputEnabled,
				Hub: s.Hub, Broker: s.Broker, Assets: s.Assets,
				Port: cfg.Mobile.Port, Network: s.sup.network(), IsLAN: s.IsLAN,
				OnDevicesChanged: s.emitDevices,
				OnStatusChanged:  func() { s.emitStatus(s.embedded.Status()) },
			})
			if err := srv.Start(); err != nil {
				return nil, err
			}
			return srv, nil
		},
		StopFn: func(srv *mobileweb.Server) {
			if err := srv.Close(); err != nil {
				slog.Warn("mobile access: close", "scope", "mobileweb", "err", err)
			}
		},
		StatusFn: func(srv *mobileweb.Server, startErr error) MobileStatus {
			st := MobileStatus{Port: model.DefaultMobileSettings().Port}
			if cfg, err := s.Deps.Repos.Settings.GetAll(); err == nil {
				st.Enabled, st.Port, st.AgentInput = cfg.Mobile.Enabled, cfg.Mobile.Port, cfg.Mobile.AgentInput
			}
			if srv != nil {
				ws := srv.Status()
				st.Running, st.AppURL = ws.Running, ws.AppURL
			}
			if startErr != nil {
				st.Error = startErr.Error()
			}
			s.fillNetwork(&st)
			return st
		},
	}
	return s
}

// agentInputEnabled reads the global switch on each request, so turning it off takes effect at once.
func (s *MobileAccessService) agentInputEnabled() bool {
	cfg, err := s.Deps.Repos.Settings.GetAll()
	return err == nil && cfg.Mobile.AgentInput
}

type MobileLaunchOpenedArgs struct {
	TerminalID string `json:"terminalId"`
	// Error is the window's failure text; empty when the terminal opened.
	Error string `json:"error"`
}

// LaunchOpened is the desktop window's answer to a phone-started launch (ChannelMobileOpenLaunch).
func (s *MobileAccessService) LaunchOpened(args MobileLaunchOpenedArgs) error {
	if args.TerminalID == "" {
		return ipcerr.Required("terminalId")
	}
	if s.Launches != nil {
		s.Launches.Ack(args.TerminalID, args.Error)
	}
	return nil
}

// Status reports the server state. While the supervisor is off it also reads the current network,
// so the pane can offer "Trust this network" before the server is enabled.
func (s *MobileAccessService) Status() MobileStatus { return s.withCurrent(s.embedded.Status()) }

// AttachMobilePush forwards the broker and drives its expiry loop. The returned detach runs in teardown:
// it unsubscribes and aborts every parked request.
func AttachMobilePush(s *MobileAccessService) (detach func()) {
	unPairing := s.Broker.Subscribe(func(snap pairing.Snapshot[mobileweb.MobileMeta]) {
		s.Deps.Events.Emit(ChannelMobilePairing, toWireMobileSnapshot(snap))
	})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Broker.RunExpiry(stop, time.Second)
	}()
	return func() {
		unPairing()
		close(stop)
		<-done
		s.Broker.Shutdown()
	}
}

func (s *MobileAccessService) emitStatus(st MobileStatus) {
	s.Deps.Events.Emit(ChannelMobileStatus, st)
}

func (s *MobileAccessService) emitDevices() {
	devices, err := s.Deps.Repos.MobileDevices.List()
	if err != nil {
		slog.Warn("mobile access: list devices", "scope", "mobileweb", "err", err)
		return
	}
	s.Deps.Events.Emit(ChannelMobileDevices, devices)
}

type MobileSetEnabledArgs struct {
	Enabled bool `json:"enabled"`
}

// SetEnabled persists the toggle, then starts or stops the server. A start failure shows in the
// returned Error rather than failing the call.
func (s *MobileAccessService) SetEnabled(args MobileSetEnabledArgs) (MobileStatus, error) {
	merged, err := s.Deps.Repos.Settings.Set(model.SettingsPatch{Mobile: &model.MobilePatch{Enabled: &args.Enabled}})
	if err != nil {
		return MobileStatus{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)
	var st MobileStatus
	if args.Enabled {
		s.startSupervisor()
		st = s.embedded.Status()
	} else {
		s.stopSupervisor()
		st, _ = s.embedded.SetRunning(false)
	}
	st = s.withCurrent(st)
	s.emitStatus(st)
	return st, nil
}

type MobileSetAgentInputArgs struct {
	Enabled bool `json:"enabled"`
}

// SetAgentInputEnabled flips the global switch for phones replying to agents and controlling their
// terminals. A phone also needs its own flag (SetDevicePermissions); the desktop toggles are the
// explicit confirmation for both.
func (s *MobileAccessService) SetAgentInputEnabled(args MobileSetAgentInputArgs) (MobileStatus, error) {
	merged, err := s.Deps.Repos.Settings.Set(model.SettingsPatch{Mobile: &model.MobilePatch{AgentInput: &args.Enabled}})
	if err != nil {
		return MobileStatus{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)
	if !args.Enabled && s.Terminals != nil {
		s.Terminals.ReleaseAll("agent input turned off")
	}
	st := s.embedded.Status()
	s.emitStatus(st)
	return st, nil
}

type MobileDevicePermissionsArgs struct {
	ID         string `json:"id"`
	Write      bool   `json:"write"`
	AgentInput bool   `json:"agentInput"`
}

// SetDevicePermissions stores one phone's two permission flags.
func (s *MobileAccessService) SetDevicePermissions(args MobileDevicePermissionsArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Deps.Repos.MobileDevices.SetPermissions(args.ID, args.Write, args.AgentInput); err != nil {
		return ipcerr.InternalErr(err)
	}
	s.embedded.Mu.Lock()
	srv := s.embedded.Server
	s.embedded.Mu.Unlock()
	if srv != nil {
		srv.PermissionsChanged(args.ID)
	}
	s.emitDevices()
	return nil
}

// TerminalHolds lists agent terminals a phone controls, for the desktop overlay and pane note.
func (s *MobileAccessService) TerminalHolds() []mobileterm.Hold {
	if s.Terminals == nil {
		return []mobileterm.Hold{}
	}
	return s.Terminals.Holds()
}

type MobileReclaimArgs struct {
	TerminalID string `json:"terminalId"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// ReclaimTerminal takes an agent terminal back from a phone. Cols and rows are the window's size;
// 0,0 keeps the last size the window reported.
func (s *MobileAccessService) ReclaimTerminal(args MobileReclaimArgs) error {
	if args.TerminalID == "" {
		return ipcerr.Required("terminalId")
	}
	if s.Terminals != nil {
		s.Terminals.Reclaim(args.TerminalID, args.Cols, args.Rows)
	}
	return nil
}

type MobileSetPortArgs struct {
	Port int `json:"port"`
}

// SetPort persists the port and rebinds the running server onto it.
func (s *MobileAccessService) SetPort(args MobileSetPortArgs) (MobileStatus, error) {
	merged, err := s.Deps.Repos.Settings.Set(model.SettingsPatch{Mobile: &model.MobilePatch{Port: &args.Port}})
	if err != nil {
		return MobileStatus{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)
	if s.embedded.Status().Running {
		s.embedded.Stop()
	}
	// Also retries a server a busy port stopped; a no-op while the supervisor is off.
	s.reconcile()
	st := s.withCurrent(s.embedded.Status())
	s.emitStatus(st)
	return st, nil
}

func (s *MobileAccessService) Devices() ([]model.MobileDevice, error) {
	return ipcerr.InternalResult(s.Deps.Repos.MobileDevices.List())
}

type MobileIDArgs struct {
	ID string `json:"id"`
}

// Revoke ends a phone's access and open streams.
func (s *MobileAccessService) Revoke(args MobileIDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	s.embedded.Mu.Lock()
	srv := s.embedded.Server
	s.embedded.Mu.Unlock()
	var err error
	if srv != nil {
		err = srv.Revoke(args.ID)
	} else {
		err = s.Deps.Repos.MobileDevices.Revoke(args.ID, time.Now().UnixMilli())
	}
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	s.emitDevices()
	return nil
}

func (s *MobileAccessService) PendingPairing() MobilePairingSnapshot {
	return toWireMobileSnapshot(s.Broker.Pending())
}

func (s *MobileAccessService) Approve(args MobileIDArgs) (MobilePairingActionResult, error) {
	if args.ID == "" {
		return MobilePairingActionResult{}, ipcerr.Required("id")
	}
	return toWireMobileResult(s.Broker.Approve(args.ID)), nil
}

func (s *MobileAccessService) Deny(args MobileIDArgs) (MobilePairingActionResult, error) {
	if args.ID == "" {
		return MobilePairingActionResult{}, ipcerr.Required("id")
	}
	return toWireMobileResult(s.Broker.Deny(args.ID)), nil
}

// StartMobileIfEnabled and StopMobile are main.go's boot and shutdown hooks, package-level so
// Wails does not bind them.
func StartMobileIfEnabled(s *MobileAccessService) {
	cfg, err := s.Deps.Repos.Settings.GetAll()
	if err != nil {
		slog.Warn("mobileweb: read settings at boot", "scope", "mobileweb", "err", err)
		return
	}
	if cfg.Mobile.Enabled {
		s.startSupervisor()
	}
}

func StopMobile(s *MobileAccessService) {
	s.stopSupervisor()
	s.embedded.Stop()
}
