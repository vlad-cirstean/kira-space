package shell_test

import (
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// fakeWindowStore is a plain in-memory shell.WindowStore — WindowDeps.Windows is an interface
// (exactly the methods window.go's Attach calls: SetBounds), so a test double replaces the real
// *repos.WindowsRepo/SQLite round trip this file used before the hoist: Attach's own logic never
// touches SQL, only calls through the interface, so a fake proves the same debounce/detach
// behaviour without a per-app storage package the shared package cannot import.
type fakeWindowStore struct {
	mu     sync.Mutex
	bounds map[string]shell.WindowBounds
}

func newFakeWindowStore() *fakeWindowStore {
	return &fakeWindowStore{bounds: map[string]shell.WindowBounds{}}
}

func (f *fakeWindowStore) SetBounds(key string, b shell.WindowBounds) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bounds[key] = b
	return nil
}

func (f *fakeWindowStore) boundsOf(key string) (shell.WindowBounds, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.bounds[key]
	return b, ok
}

// sentinelBounds lets a test tell "persist() ran and overwrote this" apart from "persist() never
// ran": win.Bounds() always reports the zero Rect{} in these tests (no native window backs it), so
// two persisted writes are otherwise indistinguishable by value.
var sentinelBounds = shell.WindowBounds{X: 111, Y: 222, Width: 333, Height: 444}

// newAttachedWindow builds a fake WindowStore and a real, display-less *application.WebviewWindow
// (testApp never calls Run(), so its impl stays nil). HandleWindowEvent still dispatches to
// registered listeners the same way a real resize/move would — WindowClosing is the one event
// this can't safely exercise here: Wails registers its own internal WindowClosing listener on
// every window (webview_window.go's NewWindow) that calls InvokeSync against the main thread
// dispatcher app.Run() would normally be pumping, which panics once nothing is pumping it,
// regardless of anything this package does.
func newAttachedWindow(t *testing.T) (*application.WebviewWindow, *fakeWindowStore) {
	t.Helper()
	win := testApp.Window.NewWithOptions(application.WebviewWindowOptions{})
	return win, newFakeWindowStore()
}

// TestAttach_PersistsResizeAfterDebounce is the positive control for the regression test below: it
// proves a resize dispatched to an attached window really does reach persist() after the debounce
// window, so TestAttach_DetachStopsPersisting's negative assertion means what it claims.
func TestAttach_PersistsResizeAfterDebounce(t *testing.T) {
	win, windows := newAttachedWindow(t)
	shell.Attach(win, shell.WindowDeps{Windows: windows, StartedAt: time.Now()}, "main")

	win.HandleWindowEvent(uint(events.Common.WindowDidResize))
	time.Sleep(500 * time.Millisecond)

	if _, ok := windows.boundsOf("main"); !ok {
		t.Fatal("resize on an attached window was never persisted")
	}
}

// TestAttach_DetachStopsPersisting covers the half of the fix that a unit test can:
// main.go used to discard shell.Attach's returned detach entirely, instead of wiring it into
// beforeFlush the way Attach's own doc comment says it must be — a resize or move landing during
// the shutdown flush-wait (after beforeFlush runs but before teardown closes the DB) could still
// fire persist() against a store whose backing DB was mid-close. main.go's own wiring isn't
// something a test can exercise (there is no seam short of standing up the whole app), but the
// invariant it now relies on is: once detach runs, nothing it unsubscribed fires persist() again.
// This proves that half directly — a WindowDidResize dispatched after detach() no longer calls
// SetBounds.
func TestAttach_DetachStopsPersisting(t *testing.T) {
	win, windows := newAttachedWindow(t)
	detach := shell.Attach(win, shell.WindowDeps{Windows: windows, StartedAt: time.Now()}, "main")

	detach()

	// Seed a known, distinctive value: if a stray persist() still fires, it overwrites this with
	// the zero Rect{} win.Bounds() reports in this test, revealing it even though the two values
	// are otherwise indistinguishable in this environment.
	if err := windows.SetBounds("main", sentinelBounds); err != nil {
		t.Fatalf("windows.SetBounds (seed): %v", err)
	}

	win.HandleWindowEvent(uint(events.Common.WindowDidResize))
	win.HandleWindowEvent(uint(events.Common.WindowDidMove))
	time.Sleep(500 * time.Millisecond)

	got, ok := windows.boundsOf("main")
	if !ok || got != sentinelBounds {
		t.Fatalf("persist ran after detach: bounds = %+v (ok=%v), want unchanged sentinel %+v", got, ok, sentinelBounds)
	}
}

// TestDefaultBounds covers the clamp arithmetic — a large screen leaves the 1280×800
// default untouched, a screen exactly that size still shrinks it (the margin, not just an
// inequality, is what's under test), a small screen floors at the app's own minimum rather than
// the screen's, and a nil or empty work area — the case this app's own startup order hits today,
// per Options' doc comment — falls back to exactly today's unclamped default.
func TestDefaultBounds(t *testing.T) {
	tests := []struct {
		name       string
		work       *application.Rect
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "nil work area falls back to the unclamped default",
			work:       nil,
			wantWidth:  1280,
			wantHeight: 800,
		},
		{
			name:       "a screen much larger than the default leaves it unchanged",
			work:       &application.Rect{Width: 3840, Height: 2160},
			wantWidth:  1280,
			wantHeight: 800,
		},
		{
			name:       "a screen exactly the default's size still shrinks — the margin bites",
			work:       &application.Rect{Width: 1280, Height: 800},
			wantWidth:  1280 - 40,
			wantHeight: 800 - 40,
		},
		{
			name:       "a screen smaller than the app's own minimum floors at the minimum, not the screen",
			work:       &application.Rect{Width: 900, Height: 600},
			wantWidth:  1024,
			wantHeight: 640,
		},
		{
			name:       "a zero-value work area (unresolved screen) falls back to the unclamped default",
			work:       &application.Rect{},
			wantWidth:  1280,
			wantHeight: 800,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight := shell.DefaultBounds(tt.work)
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Errorf("DefaultBounds(%+v) = (%d, %d), want (%d, %d)", tt.work, gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
			if gotWidth > 1280 || gotHeight > 800 {
				t.Errorf("DefaultBounds(%+v) = (%d, %d) grew past the 1280×800 default — the clamp must only ever shrink it", tt.work, gotWidth, gotHeight)
			}
		})
	}
}

// TestOptions_CustomTitleBarPosture pins P1 D2/F1-F3: a posture value asserted so a revert to
// Frameless (which would silently hide the traffic lights, F2) or to the stale #18181B background
// (F3) is never silent, the same shape TestHarden_DenyByDefaultPosture (security_test.go) uses.
func TestOptions_CustomTitleBarPosture(t *testing.T) {
	opts := shell.Options(shell.Harden(), shell.WindowRecord{Key: "main"}, nil, shell.Config{WindowTitle: "Shell Test"})

	wantTitleBar := application.MacTitleBar{
		AppearsTransparent: true,
		HideTitle:          true,
		FullSizeContent:    true,
	}
	if opts.Mac.TitleBar != wantTitleBar {
		t.Errorf("Mac.TitleBar = %+v, want %+v", opts.Mac.TitleBar, wantTitleBar)
	}
	if opts.Frameless {
		t.Error("Frameless = true, want false — Frameless hides the macOS traffic lights (F2)")
	}
	wantBackground := application.NewRGB(22, 22, 24)
	if opts.BackgroundColour != wantBackground {
		t.Errorf("BackgroundColour = %+v, want %+v (matches --kira-bg-chrome, #161618)", opts.BackgroundColour, wantBackground)
	}
}

// TestOptions_StoredBoundsOverrideTheClamp proves the clamp applies only to the no-stored-
// rectangle path: a window with a persisted rectangle restores it exactly, regardless of what the
// primary screen's work area would otherwise clamp the default to.
func TestOptions_StoredBoundsOverrideTheClamp(t *testing.T) {
	rec := shell.WindowRecord{
		Key:    "main",
		Bounds: &shell.WindowBounds{X: 10, Y: 20, Width: 2000, Height: 1500},
	}
	small := &application.Rect{Width: 320, Height: 240}

	opts := shell.Options(shell.Harden(), rec, small, shell.Config{WindowTitle: "Shell Test"})

	if opts.Width != 2000 || opts.Height != 1500 || opts.X != 10 || opts.Y != 20 {
		t.Fatalf("Options with stored bounds = %+v, want the stored rectangle unclamped", opts)
	}
	if opts.InitialPosition != application.WindowXY {
		t.Errorf("InitialPosition = %v, want WindowXY for a restored rectangle", opts.InitialPosition)
	}
}
