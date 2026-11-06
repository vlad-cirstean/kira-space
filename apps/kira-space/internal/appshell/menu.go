// Package appshell is Kira Space's own residue of the app-shell composition: the
// generic half (window/menu/quit/close-flush machinery, the deferred Wails adapters) is repo-root
// internal/shell, shared with Kira Studio; this package is everything left over that is genuinely
// this app's own — its own menu content, its own dialog request-struct adapter and its own stream
// registration. A distinct package name (not "shell") so one main.go can import both repo-root
// internal/shell and this package with no alias.
package appshell

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// BuildTemplate is Kira Space's own menu — Kira Studio's own BuildTemplate
// (apps/kira-space/internal/studiotmp/appshell/menu.go) trimmed to this app's own commands: no connections/
// requests/import items, no run/format (this app has none of those).
func BuildTemplate(appName string, isDev bool) []shell.Section {
	appItems := shell.AppMenuHead()
	appItems = append(appItems,
		shell.Item{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.settings.label", nil), Accelerator: shell.Shortcuts["app.settings"].Accelerator(), Channel: bridge.ChannelOpenSettings},
		shell.Item{Kind: shell.ItemSeparator},
	)
	appItems = append(appItems, shell.AppMenuTail(appName)...)
	appSection := shell.Section{Label: appName, Items: appItems}

	editSection := shell.Section{Label: i18n.T("backend.menu.edit.label", nil), Items: shell.EditMenu()}

	viewItems := []shell.Item{
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.toggleProjectPanel.label", nil), Accelerator: shell.Shortcuts["view.toggleProjectPanel"].Accelerator(), Channel: bridge.ChannelToggleProjectPanel},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.toggleOperationsPanel.label", nil), Accelerator: shell.Shortcuts["view.toggleOperationsPanel"].Accelerator(), Channel: bridge.ChannelToggleOperationsPanel},
		{Kind: shell.ItemSeparator},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.commandPalette.label", nil), Accelerator: shell.Shortcuts["view.commandPalette"].Accelerator(), Channel: bridge.ChannelCommandPalette},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.find.label", nil), Accelerator: shell.Shortcuts["view.find"].Accelerator(), Channel: bridge.ChannelViewFind},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.space.menu.reviewCode.label", nil), Accelerator: shell.Shortcuts["ade.reviewCode"].Accelerator(), Channel: bridge.ChannelReviewCode},
	}
	if isDev {
		viewItems = append(viewItems,
			shell.Item{Kind: shell.ItemSeparator},
			shell.Item{Kind: shell.ItemRole, Role: application.Reload},
			shell.Item{Kind: shell.ItemRole, Role: application.OpenDevTools},
		)
	}
	viewSection := shell.Section{Label: i18n.T("backend.menu.view.label", nil), Items: viewItems}

	// role: 'close' defaults to CmdOrCtrl+W, which "Close Tab" below now claims — re-accelerated to
	// Shift+W, Kira Studio's own remap (its menu.go's identical comment).
	windowItems := append([]shell.Item{
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.nextTab.label", nil), Accelerator: shell.Shortcuts["tab.next"].Accelerator(), Channel: bridge.ChannelTabNext},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.previousTab.label", nil), Accelerator: shell.Shortcuts["tab.prev"].Accelerator(), Channel: bridge.ChannelTabPrev},
		{Kind: shell.ItemEmit, Label: i18n.T("backend.menu.closeTab.label", nil), Accelerator: shell.Shortcuts["tab.close"].Accelerator(), Channel: bridge.ChannelTabClose},
		{Kind: shell.ItemSeparator},
		{Kind: shell.ItemNewWindow, Label: i18n.T("backend.menu.newWindow.label", nil), Accelerator: shell.Shortcuts["window.new"].Accelerator()},
		{Kind: shell.ItemSeparator},
	}, shell.WindowMenuTail(shell.Shortcuts["window.close"].Accelerator())...)
	windowSection := shell.Section{Label: i18n.T("backend.menu.window.label", nil), Items: windowItems}

	return []shell.Section{appSection, editSection, viewSection, windowSection}
}
