package bridge

import "github.com/kirathecat/kira-studio/internal/scriptruns"

// ScriptRunsService is this app's binding-name shim over scriptruns.Bound.
type ScriptRunsService struct {
	*scriptruns.Bound
}

// ChannelScriptRunsChanged pushes one changed script run to every window.
const ChannelScriptRunsChanged = "kira:scriptRuns:changed"

// ChannelScriptRunLog pushes the log lines a smart run just stored: {runId, chunks}.
const ChannelScriptRunLog = "kira:scriptRunLog:appended"
