package appshell

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// RegisterGitStream registers the git stream — Kira Studio's own RegisterEngineStream, carrying
// gitrpc's own wire protocol (bridge.ServeGitStream). Its Conn is the window's, shared with
// bridge.GitService. The key is the window name, or "main" with no window (-tags server),
// matching the frontend's window-key fallback.
func RegisterGitStream(app *application.App, windows *bridge.GitWindows) {
	app.HandleStream(bridge.GitStreamName, func(c *application.StreamConn) {
		key := "main"
		if w := c.Window(); w != nil {
			key = w.Name()
		}
		bridge.ServeGitStream(windows, c, key)
	})
}

// RegisterGitGraphStream registers the graph data plane (bridge.ServeGitGraph).
func RegisterGitGraphStream(app *application.App, windows *bridge.GitWindows) {
	app.HandleStream(bridge.GitGraphStreamName, func(c *application.StreamConn) {
		bridge.ServeGitGraph(context.Background(), windows, c)
	})
}
