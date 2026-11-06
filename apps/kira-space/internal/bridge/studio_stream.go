package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
)

// StreamName is the one named stream. The renderer's replacement for
// apps/kira-studio/frontend/src/bridge/port.ts opens it once per page load; Wails supersedes an
// older generation's session automatically (stream.go:747-800), which is what retires
// index.ts's own `generation` counter.
const StreamName = "engine"

// ServeEngineStream runs for the life of one connection and returns when the renderer's side
// closes (page reload, window close, app shutdown). This is a data-plane server, not a byte
// forwarder: router.AttachStream gives this session its own single writer, and every inbound
// frame is handed to router.HandleDataFrameAsync, which decides — per connection kind — whether
// to answer it in-process or forward it to the Node engine child unchanged.
// HandleDataFrameAsync runs the frame on its own goroutine so a slow read never serialises behind
// it, bounded to a fixed number of concurrent ops per session (a burst of frames used to
// spawn goroutines without limit); responses are correlated by id, so out-of-order completion is
// already the renderer's own contract (port.ts's pending map).
func ServeEngineStream(router *adapterhost.Router, conn StreamSession) {
	session, detach := router.AttachStream(conn)
	defer detach()
	for {
		frame, err := conn.Receive()
		if err != nil {
			return
		}
		router.HandleDataFrameAsync(session, frame)
	}
}
