package shell

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// PopupSpec sizes a quick-capture style popup.
type PopupSpec struct {
	Width, Height int
}

// PopupOptions builds a small centered always-on-top frameless window. On macOS it is a
// non-activating floating panel: it shows over the frontmost app, takes keys, and does not bring
// the other app windows forward. screen nil centers on the primary screen.
func PopupOptions(sec SecurityOptions, key string, cfg Config, spec PopupSpec, screen *application.Screen) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            cfg.WindowTitle,
		Width:            spec.Width,
		Height:           spec.Height,
		BackgroundColour: application.NewRGB(22, 22, 24),
		URL:              "/?window=" + key,
		Name:             key,
		Permissions:      sec.Permissions,
		EnableFileDrop:   false,
		DisableResize:    true,
		AlwaysOnTop:      true,
		Frameless:        true,
		InitialPosition:  application.WindowCentered,
		Screen:           screen,
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		Mac: application.MacWindow{
			WebviewPreferences: sec.Webview,
			WindowClass:        application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				NonActivating: true,
				FloatingPanel: true,
			},
		},
	}
}

// OpenPopup opens key as an ephemeral popup. No bounds persistence: it is always centered, never
// restored. The screen is the focused window's, else the primary (Wails exposes no cursor screen).
func OpenPopup(d WindowOpenerDeps, key string, spec PopupSpec) {
	var screen *application.Screen
	if cur := d.App.Window.Current(); cur != nil {
		if sc, err := cur.GetScreen(); err == nil {
			screen = sc
		}
	}
	if screen == nil {
		screen = d.App.Screen.GetPrimary()
	}
	win := d.App.Window.NewWithOptions(PopupOptions(Harden(), key, d.Cfg, spec, screen))
	d.Windows.AddEphemeral(key, win, func() {})
	wireLifecycle(d, win, key, true)
}
