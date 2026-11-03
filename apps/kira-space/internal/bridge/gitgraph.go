package bridge

import (
	"context"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/graphplane"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// GitGraphStreamName is the named stream carrying graph chunks (graphplane's wire).
const GitGraphStreamName = "gitgraph"

// maxGitGraphFrameBytes caps one frame (32 MiB); a graph-chunk blob must fit in one.
const maxGitGraphFrameBytes = 32 << 20

// ServeGitGraph runs the graph data plane for one renderer connection. Each open frame names its
// window, so streams share the Conn GitService uses for that window. A window with no Conn holds
// no repo, so its stream is refused as not open.
func ServeGitGraph(ctx context.Context, windows *GitWindows, conn StreamSession) {
	graphplane.Serve(ctx, conn, func(ctx context.Context, windowKey string, params json.RawMessage, emit func(payload any, blob []byte) error) error {
		w, ok := windows.lookup(windowKey)
		if !ok {
			return ipcerr.T(ipcerr.CodeConflict, "backend.space.gitrpc.repoNotOpen.body", nil)
		}
		return w.handlers.GraphStream(ctx, params, emit)
	}, maxGitGraphFrameBytes)
}
