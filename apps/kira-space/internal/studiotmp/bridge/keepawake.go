package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/internal/keepawake"
)

// KeepAwakeService is the titlebar toggle's keep-awake reason on one internal/keepawake.Controller.
// DbMcpService's own shape: the renderer-facing methods are exported, the app-internal triggers
// (system wake, teardown) are package-level functions, since Wails binds every exported method of a
// registered service and none of those may be renderer-callable. The manual half (Toggle) is
// shared with Kira Space's own KeepAwakeService. The agent-aware setting lives in Kira Space only.
type KeepAwakeService struct {
	Deps appcore.StudioDeps
	// Ctl and Toggle are exported so main.go can inject the platform driver and the shared manual
	// toggle — TerminalService.Registry's own shape. Wails binds a registered service's exported
	// *methods*, never its fields, so this widens nothing on the wire.
	Ctl    *keepawake.Controller
	Toggle *keepawake.Toggle
}

// KeepAwakeStatus is the wire projection every method below returns, and ChannelKeepAwake's
// payload.
type KeepAwakeStatus struct {
	// Manual is the titlebar button's own state — app-wide, process-scoped (§3.2: never persisted
	// across relaunch).
	Manual bool `json:"manual"`
	// Supported is false on every non-darwin build — the titlebar hides the button outright rather
	// than offering a control that does nothing.
	Supported bool `json:"supported"`
	// Error names why an acquire failed (caffeinate missing, spawn refused). "" otherwise.
	Error string `json:"error"`
}

func (s *KeepAwakeService) status() KeepAwakeStatus {
	st := s.Toggle.State()
	return KeepAwakeStatus{Manual: st.Manual, Supported: st.Supported, Error: st.Error}
}

// Status reads the service's current state — never starts or stops anything. The boot-time
// hydrate.
func (s *KeepAwakeService) Status() KeepAwakeStatus {
	return s.status()
}

// KeepAwakeSetManualArgs is SetManual's own argument shape.
type KeepAwakeSetManualArgs struct {
	Enabled bool `json:"enabled"`
}

// SetManual flips the titlebar toggle's own reason and broadcasts the result to every window —
// Emit, not EmitTo, since the assertion is app-wide by definition (§3.2).
func (s *KeepAwakeService) SetManual(args KeepAwakeSetManualArgs) KeepAwakeStatus {
	toggled := s.Toggle.SetManual(args.Enabled)
	st := KeepAwakeStatus{Manual: toggled.Manual, Supported: toggled.Supported, Error: toggled.Error}
	s.Deps.Events.Emit(ChannelKeepAwake, st)
	return st
}

// KeepAwakeSystemDidWake is shell.AttachSystemWake's own trigger (§5) — release-then-reacquire
// while held, a no-op while idle (Controller.Rearm's own guard).
func KeepAwakeSystemDidWake(s *KeepAwakeService) {
	s.Ctl.Rearm()
}

// StopKeepAwake is main.go's own shutdown call — releases the assertion unconditionally,
// regardless of which reasons are still set, so the window between "app is quitting" and
// "caffeinate is dead" stays as short as possible.
func StopKeepAwake(s *KeepAwakeService) {
	s.Ctl.Close()
}
