package shell

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// WindowRepo is what OpenWindow/OpenNewWindow/ReopenWindows need from an app's own
// *repos.WindowsRepo — WindowStore's own SetBounds (Attach's write) plus List/Create/Delete. Each
// app's own windowStore adapter (already there for WindowStore) grows these
// three methods, converting its own storage/model.WindowRecord at the same boundary SetBounds
// already crosses.
type WindowRepo interface {
	WindowStore
	List() ([]WindowRecord, error)
	Create(rec WindowRecord) error
	Delete(key string) error
}

// ToWindowRecord builds a WindowRecord from an app's own already-resolved fields — this package
// stays independent of any per-app storage package (WindowBounds' own doc comment), so a caller
// converts its own storage/model.WindowRecord into these three primitives first.
func ToWindowRecord(key string, order int, bounds *WindowBounds) WindowRecord {
	return WindowRecord{Key: key, Order: order, Bounds: bounds}
}

// WindowOpenerDeps is OpenWindow/OpenNewWindow/ReopenWindows' own shared dependency set — every
// piece main.go's own windowOpener struct used to carry, narrowed to what these three functions
// touch: Terminal is the terminal.Registry itself (not the whole per-app bound
// TerminalService, which stays a concrete Wails-bound type), and Repo is the one
// adapter each app already builds for WindowDeps.Windows.
type WindowOpenerDeps struct {
	App        *application.App
	WindowDeps WindowDeps
	Windows    *WindowRegistry
	CloseFlush *CloseFlushCoordinator
	Quitter    *Quitter
	Terminal   *terminal.Registry
	Repo       WindowRepo
	Cfg        Config
	// Ephemeral reports windows that are never restored on relaunch and never count as the last
	// window (P150 review windows). nil = none.
	Ephemeral func(key string) bool
	// OnWindowClosing runs once per closing window, after its terminals close — for modules that own
	// window-scoped streams (Docker). nil = none.
	OnWindowClosing []func(key string)
}

// OpenWindow opens one workbench from an already-persisted record and registers it — the one path
// every window (startup, reopen, "New Window") ultimately goes through. Its own WindowClosing
// listener: delete the row only if another window remains open, so closing the last
// window leaves it behind for the next Dock click or relaunch to restore.
//
// primaryWorkArea (Options' first-launch size clamp) is resolved fresh here, on every
// call, rather than captured once before app.Run() — GetPrimary() is
// backed by a cache macOS only starts populating once its native run loop's
// ApplicationDidFinishLaunching fires (application_darwin.go's own `run()`), which happens only
// after C.run() — i.e. strictly after app.Run() is called, never before. A value captured before
// Run() is therefore permanently nil for every window opened this way, including "New Window" and
// Dock-reopen, even though those happen well after Run() and the cache is long since populated by
// the time they run. Resolving it per call fixes that for them. It can NOT fix the very first
// window(s) opened at startup (each app's own main.go calls this before app.Run()): those are
// still created before app.Run() ever runs, so no ordering of this lookup changes their
// primaryWorkArea, which stays nil — first real launch keeps the unclamped 1280×800 default until
// the window is resized once (DefaultBounds' own doc comment). Deferring startup window creation
// until after ApplicationDidFinishLaunching would close that gap but is a materially larger
// structural change, out of scope here.
func OpenWindow(d WindowOpenerDeps, rec WindowRecord) {
	var primaryWorkArea *application.Rect
	if screen := d.App.Screen.GetPrimary(); screen != nil {
		primaryWorkArea = &screen.WorkArea
	}
	win := d.App.Window.NewWithOptions(Options(Harden(), rec, primaryWorkArea, d.Cfg))
	detach := Attach(win, d.WindowDeps, rec.Key)
	if rec.Bounds != nil {
		// Screens are only known once the run loop starts; startup windows open before app.Run.
		win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			EnsureOnScreen(win, d.App)
		})
	}
	ephemeral := d.Ephemeral != nil && d.Ephemeral(rec.Key)
	if ephemeral {
		d.Windows.AddEphemeral(rec.Key, win, detach)
	} else {
		d.Windows.Add(rec.Key, rec.Order, win, detach)
	}
	wireLifecycle(d, win, rec.Key, ephemeral)
}

// wireLifecycle attaches the close-flush and WindowClosing teardown every managed window shares.
func wireLifecycle(d WindowOpenerDeps, win *application.WebviewWindow, key string, ephemeral bool) {
	// Real-interaction fix (item 8): isLastWindow reads the registry fresh at the moment this
	// window's own close-flush wait completes (closeflush.go's own doc comment) — this window is
	// still counted (RemoveAndCount, below, is what removes it, and only once a real Close()
	// actually goes through), so `== 1` means "I am the only one left".
	AttachCloseFlush(win, key, d.CloseFlush, func() bool {
		hide, _ := closeDecision(ephemeral, d.Windows.OthersReal(key))
		return hide
	})
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		// A window that closes mid-quit-handshake without ever acking through the flush channel is
		// removed from the pending set here rather than being waited out for the full timeout
		// — a no-op when no quit is in flight, since Quitter.Flushed ignores a key it isn't
		// currently waiting on.
		d.Quitter.Flushed(key)
		// P83 §4's teardown table: a terminal never outlives the window that opened it, even when
		// the renderer never gets to ack.
		d.Terminal.CloseWindow(key)
		for _, fn := range d.OnWindowClosing {
			fn(key)
		}
		if d.Windows.RowDecision(key) {
			if err := d.Repo.Delete(key); err != nil {
				slog.Warn("delete window row", "scope", "window", "key", key, "err", err)
			}
		}
	})
}

// OpenNewWindow is the *New Window* (⇧⌘N) menu command (D8): a fresh workbench, ordered after
// every existing one, cascaded from whichever window is currently focused (D10).
func OpenNewWindow(d WindowOpenerDeps) { OpenNewWindowKeyed(d, ids.NewID()) }

// OpenNewWindowKeyed is OpenNewWindow under a caller-chosen key, so state registered for that key
// beforehand (files an external open brought) is there before the page boots.
func OpenNewWindowKeyed(d WindowOpenerDeps, key string) {
	records, err := d.Repo.List()
	if err != nil {
		slog.Error("list windows", "scope", "window", "err", err)
		return
	}
	order := 0
	for _, r := range records {
		if r.Order >= order {
			order = r.Order + 1
		}
	}
	bounds := CascadeFrom(d.App.Window.Current())
	rec := WindowRecord{Key: key, Order: order, Bounds: bounds}
	if err := d.Repo.Create(rec); err != nil {
		slog.Error("create window", "scope", "window", "err", err)
		return
	}
	OpenWindow(d, rec)
}

// ReopenWindows is the Dock-reopen path (AttachReopen only calls this when zero windows are live):
// bring back the highest-order stored workbench, or mint a fresh "main" one if every window row
// was somehow deleted (D5).
func ReopenWindows(d WindowOpenerDeps) {
	records, err := d.Repo.List()
	if err != nil {
		slog.Error("list windows for reopen", "scope", "window", "err", err)
		return
	}
	if len(records) == 0 {
		rec := WindowRecord{Key: ids.NewID(), Order: 0}
		if err := d.Repo.Create(rec); err != nil {
			slog.Error("create window for reopen", "scope", "window", "err", err)
			return
		}
		OpenWindow(d, rec)
		return
	}
	best := records[0]
	for _, r := range records[1:] {
		if r.Order > best.Order {
			best = r
		}
	}
	OpenWindow(d, best)
}
