package shell_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type noopEmitter struct{}

func (noopEmitter) Emit(string, any)           {}
func (noopEmitter) EmitTo(string, string, any) {}
func (noopEmitter) EmitFocused(string, any)    {}

func allMenuItems(m *application.Menu) []*application.MenuItem {
	var out []*application.MenuItem
	for i := 0; ; i++ {
		item := m.ItemAt(i)
		if item == nil {
			return out
		}
		out = append(out, item)
	}
}

// acceleratorTemplate exercises every ItemKind BuildMenu understands, including the
// "Control+Tab"-shaped trap a verbatim accelerator string would hit — Kira Studio's own
// BuildTemplate is app-specific (apps/kira-space/internal/appshell) and out of reach for the
// shared package, so this is a synthetic template covering the same accelerator
// vocabulary rather than a real app's menu.
func acceleratorTemplate() []shell.Section {
	return []shell.Section{
		{
			Label: "Test",
			Items: []shell.Item{
				{Kind: shell.ItemRole, Role: application.About},
				{Kind: shell.ItemSeparator},
				{Kind: shell.ItemEmit, Label: "Emit Item", Accelerator: "CmdOrCtrl+E", Channel: "test:emit"},
				{Kind: shell.ItemQuit, Label: "Quit Test", Accelerator: "CmdOrCtrl+Q"},
				{Kind: shell.ItemNewWindow, Label: "New Window", Accelerator: "CmdOrCtrl+Shift+N"},
			},
		},
		{
			Label: "Window",
			Items: []shell.Item{
				{Kind: shell.ItemRole, Role: application.Minimise},
				// Wails' parseAccelerator accepts "Ctrl", never "Control" — the exact trap this
				// test exists to catch (menuitem.go:275-287 drops an unparseable accelerator
				// silently rather than erroring).
				{Kind: shell.ItemRole, Role: application.CloseWindow, Accelerator: "Ctrl+Tab"},
			},
		},
	}
}

// TestBuildMenuAcceleratorsAllParse walks the real built *application.Menu alongside the
// Wails-free template that produced it (same item count and order — BuildMenu appends exactly one
// built item per template item, separators included) and asserts that every template item
// carrying a non-empty Accelerator parsed: this is the test that catches
// SetAccelerator's silent failure mode (menuitem.go:275-287); without it a mistranslation is
// invisible.
func TestBuildMenuAcceleratorsAllParse(t *testing.T) {
	const appName = "Shell Test"
	events := appevent.NewEvents(noopEmitter{})
	template := acceleratorTemplate()
	menu := shell.BuildMenu(shell.MenuDeps{AppName: appName, IsDev: true, Template: template, OnEmit: events.Signal, Quit: func() {}, NewWindow: func() {}})

	builtSections := allMenuItems(menu)
	if len(builtSections) != len(template) {
		t.Fatalf("built menu has %d top-level sections, want %d", len(builtSections), len(template))
	}

	checked := 0
	for i, section := range template {
		sub := builtSections[i].GetSubmenu()
		if sub == nil {
			t.Fatalf("section %q has no submenu", section.Label)
		}
		builtItems := allMenuItems(sub)
		if len(builtItems) != len(section.Items) {
			t.Fatalf("section %q built %d items, want %d", section.Label, len(builtItems), len(section.Items))
		}
		for j, wantItem := range section.Items {
			if wantItem.Accelerator == "" {
				continue
			}
			checked++
			if got := builtItems[j].GetAccelerator(); got == "" {
				t.Errorf("section %q item %d (%q): GetAccelerator() = \"\", want the parsed form of %q",
					section.Label, j, wantItem.Label, wantItem.Accelerator)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no accelerator-bearing items were checked — the test asserts nothing")
	}
}
