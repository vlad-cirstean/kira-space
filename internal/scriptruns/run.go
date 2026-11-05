// Package scriptruns records every Automations execution: one row per run, live state, then the
// shared outcome. Both apps embed the bound half and own the table (script_runs).
package scriptruns

import "github.com/kirathecat/kira-studio/internal/runoutcome"

// Trigger says what started a run.
type Trigger string

const (
	TriggerTerminal  Trigger = "terminal"
	TriggerManual    Trigger = "manual"
	TriggerADE       Trigger = "ade"
	TriggerScheduled Trigger = "scheduled"
	// TriggerMCP is a run an agent started through the Database MCP server.
	TriggerMCP Trigger = "mcp"
)

// Run kinds.
const (
	KindScript = "script"
	KindSmart  = "smart"
)

// States are the outcome statuses plus Running and Waiting. Waiting is a scheduled run that asks
// the user before it starts; both are live.
const (
	StateRunning = "running"
	StateWaiting = "waiting"
)

// RunParam is a param as a run shows it; a secret value is stored as the mask.
type RunParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RunTools is what a smart run was launched with.
type RunTools struct {
	Tools        []string `json:"tools"`
	AllowedTools []string `json:"allowedTools"`
	McpServers   []string `json:"mcpServers"`
}

// Run mirrors packages/shared/domain/scriptRuns.ts's scriptRunSchema.
type Run struct {
	ID         string              `json:"id"`
	ScriptID   string              `json:"scriptId"`
	ScriptName string              `json:"scriptName"`
	Color      string              `json:"color"`
	Kind       string              `json:"kind"`
	Trigger    Trigger             `json:"trigger"`
	State      string              `json:"state"`
	TerminalID string              `json:"terminalId"`
	Cwd        string              `json:"cwd"`
	Command    string              `json:"command"`
	Outcome    *runoutcome.Outcome `json:"outcome"`
	CreatedAt  int64               `json:"createdAt"`
	StartedAt  *int64              `json:"startedAt"`
	FinishedAt *int64              `json:"finishedAt"`
	// Smart runs only; empty for a script run.
	Model     string `json:"model"`
	SessionID string `json:"sessionId"`
	// Provider is the harness provider id that ran the conversation.
	Provider string     `json:"provider"`
	Prompt   string     `json:"prompt"`
	Params   []RunParam `json:"params"`
	Tools    RunTools   `json:"tools"`
	// ResumedFrom is the run this one continues; Resumable says a restart interrupted this run and
	// its Claude session can continue.
	ResumedFrom string `json:"resumedFrom"`
	Resumable   bool   `json:"resumable"`
	// Set when the run was started for an ADE task (Kira Space); empty otherwise.
	TaskID      string `json:"taskId"`
	TaskTitle   string `json:"taskTitle"`
	BranchID    string `json:"branchId"`
	BranchLabel string `json:"branchLabel"`
}
