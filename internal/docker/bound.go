package docker

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// BoundService is the one Wails-bound Docker surface each app embeds. An app's own
// bridge.DockerService is `struct{ *docker.BoundService }`, so binding names stay app-local while
// every method body lives here.
type BoundService struct {
	m *Manager
}

// Option tweaks NewBoundService.
type Option func(*Manager)

// WithHubAPI points the Docker Hub repository check at base (a flow-test seam).
func WithHubAPI(base string) Option { return func(m *Manager) { m.HubAPI = base } }

// WithRestoreStore enables the restore tracker over store.
func WithRestoreStore(store RestoreStore) Option { return func(m *Manager) { m.restoreStore = store } }

// WithRestoreTiming sets the tracker's tick, stop grace and restart-policy wait (a flow-test seam).
func WithRestoreTiming(tick, grace, policyWait time.Duration) Option {
	return func(m *Manager) { m.restoreTiming = restoreTiming{tick: tick, grace: grace, policyWait: policyWait} }
}

// WithBootProbe replaces the engine boot marker read (a flow-test seam).
func WithBootProbe(p BootProbe) Option { return func(m *Manager) { m.bootProbe = p } }

// NewBoundService builds the service over the real process environment.
func NewBoundService(emit appevent.Emitter, opts ...Option) *BoundService {
	m := NewManager(emit)
	for _, o := range opts {
		o(m)
	}
	if m.restoreStore != nil {
		m.restore = newRestoreTracker(m)
	}
	return &BoundService{m: m}
}

// CloseWindowBound ends every stream windowKey started. A package function, not a method: Wails
// binds every exported method, and a bound one would let any window close another's sessions.
func CloseWindowBound(b *BoundService, windowKey string) {
	b.m.closeWindow(windowKey)
}

// StartRestoreBound starts the restore tracker, which then runs without any window until shutdown.
// A package function for the same reason as CloseWindowBound.
func StartRestoreBound(b *BoundService) { b.m.restore.start() }

// RestoreCurrentBound is the restore tracker's last published state; it never touches the engine, so
// it cannot block on a dead socket. The tracker's first tick runs at boot. A package function for the
// same reason as CloseWindowBound.
func RestoreCurrentBound(b *BoundService) RestoreState { return b.m.restore.current() }

// StopRestoreBound ends the restore tracker; app teardown calls it before the store's DB closes.
func StopRestoreBound(b *BoundService) { b.m.restore.stop() }

// ServiceShutdown ends every stream at app quit. Wails calls it on each registered service and
// leaves it out of the bindings.
func (b *BoundService) ServiceShutdown() error {
	b.m.shutdown()
	return nil
}

// StatusArgs is Status' wire shape.
type StatusArgs struct {
	Refresh bool `json:"refresh"`
}

// UseContextArgs is UseContext's wire shape.
type UseContextArgs struct {
	Name string `json:"name"`
}

// ListArgs is Containers' wire shape.
type ListArgs struct {
	All bool `json:"all"`
}

// IDArgs names one container.
type IDArgs struct {
	ID string `json:"id"`
}

// InspectArgs is Inspect's wire shape.
type InspectArgs struct {
	Kind string `json:"kind"` // image | volume | network
	ID   string `json:"id"`
}

// WindowArgs names the calling window.
type WindowArgs struct {
	WindowKey string `json:"windowKey"`
}

// StatsArgs is StatsSubscribe's wire shape.
type StatsArgs struct {
	WindowKey string   `json:"windowKey"`
	IDs       []string `json:"ids"`
}

// StreamArgs names one logs stream.
type StreamArgs struct {
	StreamID string `json:"streamId"`
}

// ExecWriteArgs is ExecWrite's wire shape.
type ExecWriteArgs struct {
	TerminalID string `json:"terminalId"`
	// Data is base64: keystrokes are not always valid UTF-8.
	Data string `json:"data"`
}

// ExecResizeArgs is ExecResize's wire shape.
type ExecResizeArgs struct {
	TerminalID string `json:"terminalId"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// ExecCloseArgs is ExecClose's wire shape.
type ExecCloseArgs struct {
	TerminalID string `json:"terminalId"`
}

// Status pings the engine; Refresh rebuilds the client first so a changed environment is seen.
func (b *BoundService) Status(args StatusArgs) Status {
	return b.m.status(context.Background(), args.Refresh)
}

// Contexts lists the docker contexts, "default" first.
func (b *BoundService) Contexts() ([]ContextInfo, error) {
	got, err := listContexts(b.m.env, b.m.selectedContext())
	if err != nil {
		return nil, ipcerr.Internal(err.Error())
	}
	return got, nil
}

// UseContext switches the engine for every window; "" returns to automatic resolution.
func (b *BoundService) UseContext(args UseContextArgs) (Status, error) {
	if args.Name != "" {
		if _, _, err := resolveEndpoint(b.m.env, args.Name); err != nil {
			return Status{}, ipcerr.BadRequest(err.Error())
		}
	}
	return b.m.useContext(context.Background(), args.Name), nil
}

func (b *BoundService) Containers(args ListArgs) ([]Container, error) {
	return b.m.containers(context.Background(), args.All)
}

func (b *BoundService) Images() ([]Image, error) { return b.m.images(context.Background()) }

// ImageRegistry resolves one image's registry page without any Hub call; an implied Docker Hub name
// under the containerd store comes back Unconfirmed.
func (b *BoundService) ImageRegistry(args IDArgs) (RegistryLink, error) {
	return b.m.imageRegistry(context.Background(), args.ID, false)
}

// ConfirmImageRegistry is ImageRegistry plus the Docker Hub check (cached), for the user's click: URL
// is empty when Hub has no such repository or cannot be reached.
func (b *BoundService) ConfirmImageRegistry(args IDArgs) (RegistryLink, error) {
	return b.m.imageRegistry(context.Background(), args.ID, true)
}

func (b *BoundService) Volumes() ([]Volume, error) { return b.m.volumes(context.Background()) }

func (b *BoundService) Networks() ([]Network, error) { return b.m.networks(context.Background()) }

func (b *BoundService) InspectContainer(args IDArgs) (ContainerDetail, error) {
	return b.m.inspectContainer(context.Background(), args.ID)
}

// Inspect returns the raw inspect JSON of an image, volume or network.
func (b *BoundService) Inspect(args InspectArgs) (InspectResult, error) {
	return b.m.inspect(context.Background(), args.Kind, args.ID)
}

func (b *BoundService) Start(args IDArgs) error { return b.m.start(context.Background(), args.ID) }

func (b *BoundService) Stop(args IDArgs) error { return b.m.stop(context.Background(), args.ID) }

func (b *BoundService) Restart(args IDArgs) error { return b.m.restart(context.Background(), args.ID) }

// Watch starts pushing ChannelChanged for the window.
func (b *BoundService) Watch(args WindowArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	b.m.events.watch(args.WindowKey)
	return nil
}

func (b *BoundService) Unwatch(args WindowArgs) error {
	b.m.events.unwatch(args.WindowKey)
	return nil
}

// StatsSubscribe replaces the window's set of containers whose stats it receives.
func (b *BoundService) StatsSubscribe(args StatsArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	b.m.stats.subscribe(args.WindowKey, args.IDs)
	return nil
}

func (b *BoundService) StatsUnsubscribe(args WindowArgs) error {
	b.m.stats.unsubscribe(args.WindowKey)
	return nil
}

func (b *BoundService) LogsOpen(args LogsOpenArgs) error { return b.m.logsOpen(args) }

func (b *BoundService) LogsClose(args StreamArgs) error {
	b.m.logsClose(args.StreamID)
	return nil
}

func (b *BoundService) ExecOpen(args ExecOpenArgs) (ExecOpenResult, error) { return b.m.execOpen(args) }

func (b *BoundService) ExecWrite(args ExecWriteArgs) error {
	return b.m.execWrite(args.TerminalID, args.Data)
}

func (b *BoundService) ExecResize(args ExecResizeArgs) error {
	return b.m.execResize(context.Background(), args.TerminalID, args.Cols, args.Rows)
}

func (b *BoundService) ExecClose(args ExecCloseArgs) error { return b.m.execClose(args.TerminalID) }

// DiskUsage measures the engine's storage (/system/df). Slow on large hosts; the UI calls it on demand only.
func (b *BoundService) DiskUsage() (DiskUsage, error) { return b.m.diskUsage() }

// ContainerSize measures one container's writable layer and total size on demand.
func (b *BoundService) ContainerSize(args IDArgs) (ContainerSize, error) {
	return b.m.containerSize(args.ID)
}

// ContainerEditSpec returns the editable config of one container, split into what applies in place
// and what needs a recreate.
func (b *BoundService) ContainerEditSpec(args IDArgs) (EditSpec, error) {
	return b.m.editSpec(context.Background(), args.ID)
}

// UpdateContainer applies the in-place section: rename, limits, restart policy, network attachments.
func (b *BoundService) UpdateContainer(args UpdateArgs) (UpdateResult, error) {
	return b.m.updateContainer(args)
}

// RecreateContainer replaces a container with one built from the edited spec, restoring the
// original if any step fails.
func (b *BoundService) RecreateContainer(args RecreateArgs) (RecreateResult, error) {
	return b.m.recreateContainer(args)
}

// RestoreState checks the engine now and returns whether a restart left containers to restore.
func (b *BoundService) RestoreState() (RestoreState, error) {
	return b.m.restore.stateNow(context.Background()), nil
}

// RestoreContainers starts the chosen containers of the pending restore: standalone ones first,
// then each Compose project in depends_on order.
func (b *BoundService) RestoreContainers(args RestoreArgs) (RestoreResult, error) {
	return b.m.restore.restoreContainers(context.Background(), args)
}

// RestoreDismiss drops the pending restore without starting anything.
func (b *BoundService) RestoreDismiss(args RestoreDismissArgs) error {
	return b.m.restore.dismiss(context.Background(), args.EngineID)
}
