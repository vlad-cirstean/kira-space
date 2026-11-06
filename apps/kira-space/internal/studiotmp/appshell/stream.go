package appshell

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// RegisterEngineStream registers the one named stream. The handler blocks for the life of the
// connection, which is what keeps it open (stream.go:162-166's StreamHandler contract). Takes the
// router, not the engine host: the data plane is a server now, not a byte forwarder.
func RegisterEngineStream(app *application.App, router *adapterhost.Router) {
	app.HandleStream(bridge.StreamName, func(c *application.StreamConn) {
		bridge.ServeEngineStream(router, c)
	})
}
