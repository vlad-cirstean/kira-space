package appshell

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// RegisterGitGraphStream registers the graph data plane (bridge.ServeGitGraph). Each open frame
// names its window key, so one connection carries any window's streams.
func RegisterGitGraphStream(app *application.App, windows *bridge.GitWindows) {
	app.HandleStream(bridge.GitGraphStreamName, func(c *application.StreamConn) {
		bridge.ServeGitGraph(context.Background(), windows, c)
	})
}

// RegisterEngineStream registers the Studio engine data-plane stream; the handler blocks for the
// life of the connection.
func RegisterEngineStream(app *application.App, router *adapterhost.Router) {
	app.HandleStream(bridge.StreamName, func(c *application.StreamConn) {
		bridge.ServeEngineStream(router, c)
	})
}
