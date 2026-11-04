package shell

import (
	"log/slog"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const boundsDebounce = 300 * time.Millisecond

// defaultWindowWidth and defaultWindowHeight are the first-launch window size on a screen large
// enough to fit it unclamped — unconditional constants before P22 D6, now defaultBounds' upper
// bound.
const (
	defaultWindowWidth  = 1280
	defaultWindowHeight = 800

	// minWindowWidth and minWindowHeight are a considered floor for this app's UI — a tree
	// sidebar + grid + two toolbars + a status bar — not Electron's old 900×600 (lowering them is
	// declined: the floor ratio is only 1.21×, first-launch-only like the rest of
	// this avenue, against a real usability cost at the bottom end).
	minWindowWidth  = 1024
	minWindowHeight = 640

	// windowSizeMargin keeps a screen-clamped default from touching the work area's edges
	// exactly — a window sized precisely to the visible area still reads as cramped even though
	// nothing overlaps the menu bar or Dock.
	windowSizeMargin = 40
)

// WindowDeps is Attach's dependencies, shared across every window the app opens — only the key
// passed to Attach varies per window (P8 C2). Windows is a WindowStore (P103 Part 3): each app's
// own *repos.WindowsRepo, wrapped in a one-line adapter at its own construction site.
type WindowDeps struct {
	Windows   WindowStore
	StartedAt time.Time
}

// Options builds one window's options from its own record (D11): the stored rectangle if it has
// one, a screen-clamped default otherwise, always at its own `/?window=<key>` URL and `Name`
// (D2 — the shell mints the key and hands it to the renderer this way, since it must be readable
// synchronously at boot, before hydrateTabs() runs). There is no exact analogue of Electron's
// show:false/ready-to-show pattern here — WindowRuntimeReady fires only after the frontend has
// already loaded — a deliberate small divergence, not hidden (§1's window.ts read).
//
// primaryWorkArea is the primary screen's work area for the no-stored-rectangle path (P22 D6(a));
// nil is a legitimate input (falls back to the unclamped 1280×800 default) rather than a caller
// error. Every window opened at boot is built and handed to Wails before app.Run() (main.go's own
// top comment: "... -> the main window -> app.Run()"), and on macOS (screen_darwin.go's own
// `run()`) the screen cache app.Screen.GetPrimary() reads isn't populated until the native run
// loop's ApplicationDidFinishLaunching fires — strictly after app.Run() is called, never before —
// so it reliably returns nil for every window opened at startup, and this clamp's fallback is
// what actually runs there. main.go resolves primaryWorkArea fresh on every openWindow call
// (round-2 review finding 4) rather than once before Run(), which does let DefaultBounds run with
// a real work area for a window opened well after Run() — "New Window", or Dock-reopen minting a
// fresh window — though CascadeFrom (below) supplies its *own* already-resolved bounds directly to
// Options' `w.Bounds != nil` branch for the common "New Window" case (an existing window to
// cascade from), bypassing DefaultBounds entirely; DefaultBounds only runs there when CascadeFrom
// has nothing to cascade from.
//
// cfg.WindowTitle replaces the literal "Kira Studio" (P103 Part 3) — each app's own main.go
// supplies its own Config at the one call site.
func Options(sec SecurityOptions, w WindowRecord, primaryWorkArea *application.Rect, cfg Config) application.WebviewWindowOptions {
	width, height := DefaultBounds(primaryWorkArea)
	opts := application.WebviewWindowOptions{
		// P1 D2: `Title` stays — AppKit still uses it for the window list and Mission Control
		// even with `MacTitleBarHidden`'s `HideTitle: true`.
		Title:     cfg.WindowTitle,
		Width:     width,
		Height:    height,
		MinWidth:  minWindowWidth,
		MinHeight: minWindowHeight,
		// P1 F3: matches --kira-bg-chrome (#161618, tokens.css) — was #18181B, invisible until
		// the title-bar strip became the app's own paint over a transparent AppKit title bar.
		BackgroundColour: application.NewRGB(22, 22, 24),
		URL:              "/?window=" + w.Key,
		Name:             w.Key,
		Permissions:      sec.Permissions,
		EnableFileDrop:   false,
		// Not `Frameless`: it hides the traffic-light controls and the app would draw its own.
		// No toolbar: the compact unified toolbar renders the traffic lights smaller than native.
		Mac: application.MacWindow{
			WebviewPreferences: sec.Webview,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
				HideTitle:          true,
				FullSizeContent:    true,
			},
		},
	}

	if w.Bounds != nil {
		b := w.Bounds
		opts.Width = int(b.Width)
		opts.Height = int(b.Height)
		opts.X = int(b.X)
		opts.Y = int(b.Y)
		opts.InitialPosition = application.WindowXY
	}
	return opts
}

// DefaultBounds is Options' first-launch size decision (used only when a window has no stored
// rectangle — the `if w.Bounds != nil` branch above overrides it unconditionally otherwise), split
// out and exported so the clamp arithmetic is unit-testable the same way CascadeFrom's own
// cascadeRect is split out for testability (though cascadeRect itself has no dedicated test in
// this repo today, so this is the pattern's structure, not a test it literally extends). It can
// only ever *shrink* the 1280×800 default to fit a
// smaller primary screen — never grow it — and falls back to exactly 1280×800 when no usable work
// area is given.
//
// The motivation is UX, not memory: an unconditional 1280×800 first-launch window is edge-to-edge
// on a same-size laptop panel, overlapping the menu bar and Dock, which is a real bug on its own
// regardless of anything else. Its effect on the WKWebView scroll-memory plateau is real
// but small and unmeasured in magnitude — this is not the
// fix for that symptom, and it is first-launch-only besides (a window that has ever been resized
// never sees this default again).
func DefaultBounds(work *application.Rect) (width, height int) {
	if work == nil || work.Width <= 0 || work.Height <= 0 {
		return defaultWindowWidth, defaultWindowHeight
	}
	width = min(max(min(defaultWindowWidth, work.Width-windowSizeMargin), minWindowWidth), defaultWindowWidth)
	height = min(max(min(defaultWindowHeight, work.Height-windowSizeMargin), minWindowHeight), defaultWindowHeight)
	return width, height
}

// boundsFromRect converts Wails' int-fielded Rect (webview_window.go's Bounds()) to the
// float64-fielded WindowBounds a WindowStore persists — split out so the conversion is testable
// without a live *application.WebviewWindow.
func boundsFromRect(r application.Rect) WindowBounds {
	return WindowBounds{
		X:      float64(r.X),
		Y:      float64(r.Y),
		Width:  float64(r.Width),
		Height: float64(r.Height),
	}
}

// Attach wires resize/move persistence (debounced 300ms, matching window.ts's
// BOUNDS_DEBOUNCE_MS) for this one window's own `windows` row and the startup log line. The
// returned detach unsubscribes all four window-event listeners and cancels any pending debounced
// write; call it once per window, from beforeFlush (main.go used to discard it, so a
// resize/move landing during the shutdown flush-wait — after beforeFlush but before teardown
// closes the DB — could still fire persist() against a WindowsRepo whose *sql.DB teardown had
// already closed, racing the shutdown sequence this listener has no other way to know is
// underway) — and, per window, when that window closes (the WindowClosing listener).
func Attach(win *application.WebviewWindow, d WindowDeps, key string) (detach func()) {
	db := newDebouncer(boundsDebounce)

	persist := func() {
		bounds := boundsFromRect(win.Bounds())
		if err := d.Windows.SetBounds(key, bounds); err != nil {
			slog.Warn("persist window bounds failed", "scope", "window", "key", key, "err", err)
		}
	}

	offResize := win.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		db.trigger(persist)
	})
	offMove := win.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) {
		db.trigger(persist)
	})
	// window.ts's closed handler only clears its pending timeout, never flushes — parity keeps
	// that here too (D8): a bounds write in flight when the window closes is simply dropped.
	offClosing := win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		db.cancel()
	})
	offReady := win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		ms := time.Since(d.StartedAt).Milliseconds()
		slog.Info("did-finish-load", "scope", "startup", "uptimeMs", ms)
	})

	// P396 trace. WindowDidChangeSpace may never fire (AppKit has no such window notification).
	traced := []struct {
		t    events.WindowEventType
		name string
	}{
		{events.Mac.WindowDidBecomeKey, "WindowDidBecomeKey"},
		{events.Mac.WindowDidResignKey, "WindowDidResignKey"},
		{events.Mac.WindowDidChangeOcclusionState, "WindowDidChangeOcclusionState"},
		{events.Mac.WindowDidChangeScreen, "WindowDidChangeScreen"},
		{events.Mac.WindowDidChangeSpace, "WindowDidChangeSpace"},
	}
	traceOffs := make([]func(), 0, len(traced))
	for _, e := range traced {
		traceOffs = append(traceOffs, win.OnWindowEvent(e.t, func(*application.WindowEvent) {
			slog.Debug("window event", "scope", "window", "key", key, "event", e.name)
		}))
	}

	return func() {
		offResize()
		offMove()
		offClosing()
		offReady()
		for _, off := range traceOffs {
			off()
		}
		db.cancel()
	}
}

// cascadeStep is the classic one-title-bar-step offset a freshly opened window gets from the
// window it cascades from (D10) — large enough that the new window is unmistakably a second
// window, not a resize glitch on the first.
const cascadeStep = 24

// CascadeFrom computes a fresh window's rectangle from an existing one (a new workbench
// with no stored rectangle of its own inherits the focused window's size, offset by one
// title-bar step, wrapped back to the screen's work-area origin rather than left to drift off
// it). Returns nil when there is no window to cascade from (the very first window, or a Linux
// build where Current() never resolves one — Options' own defaults take over in that case).
func CascadeFrom(from application.Window) *WindowBounds {
	if from == nil {
		return nil
	}
	b := from.Bounds()
	var work *application.Rect
	if screen, err := from.GetScreen(); err == nil && screen != nil {
		work = &screen.WorkArea
	}
	return cascadeRect(b, work)
}

// cascadeRect is CascadeFrom's pure arithmetic, split out so the offset/wrap rule doesn't need a
// live *application.WebviewWindow to reason about.
func cascadeRect(from application.Rect, work *application.Rect) *WindowBounds {
	x, y := from.X+cascadeStep, from.Y+cascadeStep
	if work != nil {
		if x+from.Width > work.X+work.Width || y+from.Height > work.Y+work.Height {
			x, y = work.X, work.Y
		}
		if x < work.X {
			x = work.X
		}
		if y < work.Y {
			y = work.Y
		}
	}
	return &WindowBounds{X: float64(x), Y: float64(y), Width: float64(from.Width), Height: float64(from.Height)}
}

// onAnyScreen reports whether b overlaps at least one work area.
func onAnyScreen(b application.Rect, work []application.Rect) bool {
	for _, w := range work {
		if b.X < w.X+w.Width && b.X+b.Width > w.X && b.Y < w.Y+w.Height && b.Y+b.Height > w.Y {
			return true
		}
	}
	return false
}

// EnsureOnScreen moves a window restored with stored bounds to the primary work-area origin when
// those bounds overlap no current screen (an external display unplugged since the last run).
func EnsureOnScreen(win *application.WebviewWindow, app *application.App) {
	screens := app.Screen.GetAll()
	work := make([]application.Rect, 0, len(screens))
	for _, sc := range screens {
		work = append(work, sc.WorkArea)
	}
	primary := app.Screen.GetPrimary()
	if len(work) == 0 || primary == nil || onAnyScreen(win.Bounds(), work) {
		return
	}
	win.SetPosition(primary.WorkArea.X, primary.WorkArea.Y)
}
