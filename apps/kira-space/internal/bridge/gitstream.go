package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/rpcstream"
)

// GitStreamName is the second named stream (docs/HISTORY.md (v1.5 C10)) —
// StreamName's peer, carrying gitrpc's own wire protocol instead of adapterhost's.
const GitStreamName = "git"

// maxGitStreamFrameBytes caps one frame (32 MiB) — a graph-chunk blob must fit in one.
const maxGitStreamFrameBytes = 32 << 20

// ServeGitStream runs for the life of one renderer connection, mirroring ServeEngineStream
// (stream.go). There is no handshake: the peer is this process's own webview, not an external
// client the trust store exists to gate. The window's Conn is shared with GitService; the last
// stream of a window to close releases it. Git events leave as app events (GitWindows), not frames.
func ServeGitStream(windows *GitWindows, conn StreamSession, windowKey string) {
	w, detach := windows.attachStream(windowKey)
	defer detach()
	sess := rpcstream.NewSession(conn, rpcstream.Handlers{
		ContractVersion: gitrpc.ContractVersion,
		Request:         guardRepoSettingsSet(w.handlers.Request),
		Stream:          w.handlers.Stream,
		MaxFrameBytes:   maxGitStreamFrameBytes,
	})
	sess.Serve()
}
