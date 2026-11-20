package claude

import "github.com/kirathecat/kira-studio/internal/harness"

var errNotFound = &harness.Error{Kind: harness.ErrNotFound, Msg: "Claude Code CLI not found. Install it, then retry."}
