package appshell

import (
	"context"

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
