package shell

import (
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowEntry is one open window's live handle plus its shell.Attach cleanup.
type windowEntry struct {
	win       *application.WebviewWindow
	detach    func()
	ephemeral bool
	order     int
	focusSeq  uint64
}

// WindowRegistry tracks every currently open window by its key. Window creation runs on
// the menu-click/reopen-handler goroutine and beforeFlush runs on the quit goroutine, so this is
// mutex-guarded — unlike the single `detachWindow`/`mainWindow` package vars it replaces, which
// only ever worked because at most one window could exist at a time.
type WindowRegistry struct {
	mu      sync.Mutex
	entries map[string]windowEntry
	focusN  uint64
	// OnChange runs outside the lock after every Add, AddEphemeral and removal.
	OnChange func()
}

func NewWindowRegistry() *WindowRegistry {
	return &WindowRegistry{entries: map[string]windowEntry{}}
}

// Add registers a newly opened window under key, replacing whatever was registered there before.
// A reopened window reusing its old key is exactly what used to detach window 1's listeners the
// moment window 2 was created — now each key gets its own slot, so one key's Add can never
// clobber another key's entry. order is the window's stored order; the lowest one is the main
// window (MainKey).
func (r *WindowRegistry) Add(key string, order int, win *application.WebviewWindow, detach func()) {
	r.mu.Lock()
	r.entries[key] = windowEntry{win: win, detach: detach, order: order}
	r.mu.Unlock()
	r.changed()
}

func (r *WindowRegistry) changed() {
	if r.OnChange != nil {
		r.OnChange()
	}
}

// AddEphemeral registers a window that is never restored on relaunch (P150 review windows): it
// does not count toward Count and RemoveAndCount, so it never keeps the last real window open and
// is never the one that hides instead of closing.
func (r *WindowRegistry) AddEphemeral(key string, win *application.WebviewWindow, detach func()) {
	r.mu.Lock()
	r.entries[key] = windowEntry{win: win, detach: detach, ephemeral: true}
	r.mu.Unlock()
	r.changed()
}

// Close asks key's window to close through its normal WindowClosing hooks. false for an unknown key.
func (r *WindowRegistry) Close(key string) bool {
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	if !ok {
		return false
	}
	e.win.Close()
	return true
}

// SetTitle retitles key's window. false for an unknown key.
func (r *WindowRegistry) SetTitle(key, title string) bool {
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	if !ok {
		return false
	}
	e.win.SetTitle(title)
	return true
}

// DetachAll runs every registered window's detach exactly once. This is beforeFlush's whole job:
// the code this replaces detached only the single most-recently-created window, so an earlier
// window's bounds listener would keep firing into a WindowsRepo whose *sql.DB teardown had
// already closed (now true per-window instead of only for the newest one).
func (r *WindowRegistry) DetachAll() {
	r.mu.Lock()
	detaches := make([]func(), 0, len(r.entries))
	for _, e := range r.entries {
		detaches = append(detaches, e.detach)
	}
	r.mu.Unlock()
	for _, detach := range detaches {
		detach()
	}
}

// Count reports how many windows are currently registered — AttachCloseFlush's own real-interaction
// fix (the webview-process leak) reads this immediately before deciding how a window's
// close finishes: 1 means the window asking is the last one, so closing it would otherwise leave
// the app running (Mac.ApplicationShouldTerminateAfterLastWindowClosed: false, main.go) with no
// window able to bring its own now-orphaned webview process back — Hide (reused on the next
// Dock-click/menu reopen) replaces Close there instead of adding to the leak. Read at the same
// registry snapshot RemoveAndCount already coordinates through the same mutex, so a window closing
// at the same instant as another can't race this decision either.
func (r *WindowRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.realLocked()
}

func (r *WindowRegistry) realLocked() int {
	n := 0
	for _, e := range r.entries {
		if !e.ephemeral {
			n++
		}
	}
	return n
}

// OthersReal reports how many non-ephemeral windows besides key are registered.
func (r *WindowRegistry) OthersReal(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.realLocked()
	if e, ok := r.entries[key]; ok && !e.ephemeral {
		n--
	}
	return n
}

// closeDecision: an ephemeral window always closes for real and drops its row. A real window hides
// instead of closing, keeping its row, only when no other real window remains (D14).
func closeDecision(isEphemeral bool, otherRealWindows int) (hide, deleteRow bool) {
	if isEphemeral {
		return false, true
	}
	return otherRealWindows == 0, otherRealWindows > 0
}

// Focus brings key's own window to the front — AdeTaskService.FocusSession's own cross-window Open.
// false for an unknown key (the window closed between the lookup and this call). Callable from any
// goroutine: Show/UnMinimise/Focus self-dispatch to the main thread (vendored webview_window.go).
func (r *WindowRegistry) Focus(key string) bool {
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	if !ok || e.win == nil {
		return false
	}
	slog.Info("window raise", "scope", "window", "key", key)
	// The App-level unhide does not self-dispatch; AppKit aborts when [NSApp unhide:] runs off the main thread.
	application.InvokeSync(unhideApp)
	e.win.Show()
	e.win.UnMinimise()
	e.win.Focus()
	return true
}

// Any returns one live window, or nil if none is registered — attachDialogs' fallback for when
// app.Window.Current() can't resolve a key window (F4's second half: a dialog used to always
// attach to whichever window was created most recently, not the one that asked).
func (r *WindowRegistry) Any() application.Window {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		return e.win
	}
	return nil
}

// MainKey returns the key of the main window: the non-ephemeral window with the lowest stored
// order, smallest key as tiebreak. false when none is open.
func (r *WindowRegistry) MainKey() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	best, bestOrder, found := "", 0, false
	for k, e := range r.entries {
		if e.ephemeral {
			continue
		}
		if !found || e.order < bestOrder || (e.order == bestOrder && k < best) {
			best, bestOrder, found = k, e.order, true
		}
	}
	return best, found
}

// Touch records that key's window just took focus: later Geometry calls rank it above older ones.
func (r *WindowRegistry) Touch(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[key]; ok {
		r.focusN++
		e.focusSeq = r.focusN
		r.entries[key] = e
	}
}

// Geometry snapshots every live window for the tab-drop hit-test. Bounds, IsVisible and
// IsMinimised hop to the main thread, so they run outside r.mu.
func (r *WindowRegistry) Geometry() []WindowGeom {
	type live struct {
		key string
		e   windowEntry
	}
	r.mu.Lock()
	entries := make([]live, 0, len(r.entries))
	for k, e := range r.entries {
		if e.win != nil {
			entries = append(entries, live{k, e})
		}
	}
	r.mu.Unlock()
	geoms := make([]WindowGeom, 0, len(entries))
	for _, l := range entries {
		b := l.e.win.Bounds()
		geoms = append(geoms, WindowGeom{
			Key: l.key, Order: l.e.order, Ephemeral: l.e.ephemeral, FocusSeq: l.e.focusSeq,
			Bounds: WindowBounds{X: float64(b.X), Y: float64(b.Y), Width: float64(b.Width), Height: float64(b.Height)},
			Hidden: !l.e.win.IsVisible() || l.e.win.IsMinimised(),
		})
	}
	return geoms
}

// Has reports whether key is registered, ephemeral or not.
func (r *WindowRegistry) Has(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.entries[key]
	return ok
}

// Focused reports whether key's window has focus. false for an unknown key or a nil window.
func (r *WindowRegistry) Focused(key string) bool {
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	return ok && e.win != nil && e.win.IsFocused()
}

// AnyFocused reports whether any registered window is the key window. IsFocused hops to the main
// thread, so never call this on it, and handles are probed outside r.mu.
func (r *WindowRegistry) AnyFocused() bool {
	r.mu.Lock()
	wins := make([]*application.WebviewWindow, 0, len(r.entries))
	for _, e := range r.entries {
		if e.win != nil {
			wins = append(wins, e.win)
		}
	}
	r.mu.Unlock()
	for _, w := range wins {
		if w.IsFocused() {
			return true
		}
	}
	return false
}

// Keys returns every currently registered window's key — Quitter's LiveWindowKeys seam (P8 C8):
// which windows the quit handshake must wait for is decided at the moment quitting actually
// starts, not fixed at construction time.
func (r *WindowRegistry) Keys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.entries))
	for k := range r.entries {
		keys = append(keys, k)
	}
	return keys
}

// RemoveAndCount unregisters key (a no-op if it was never registered, e.g. a duplicate close
// event) and runs its detach, then reports how many windows remain registered — one atomic
// operation so a window closing at the same instant as another can't race the decision made
// from the result: only the close that empties the registry keeps that window's `windows` row
// (so a later Dock click restores the same workbench); every other close deletes its row.
func (r *WindowRegistry) RemoveAndCount(key string) (remaining int) {
	_, remaining, _ = r.removeAndCount(key)
	return remaining
}

func (r *WindowRegistry) removeAndCount(key string) (wasEphemeral bool, remaining int, found bool) {
	r.mu.Lock()
	e, ok := r.entries[key]
	if ok {
		delete(r.entries, key)
	}
	remaining = r.realLocked()
	r.mu.Unlock()
	if ok {
		e.detach()
		r.changed()
	}
	return e.ephemeral, remaining, ok
}

// RowDecision unregisters key and reports whether its windows row should be deleted
// (closeDecision's deleteRow). An unknown key reports false, like RemoveAndCount's no-op.
func (r *WindowRegistry) RowDecision(key string) bool {
	eph, remaining, found := r.removeAndCount(key)
	if !found {
		return false
	}
	_, del := closeDecision(eph, remaining)
	return del
}
