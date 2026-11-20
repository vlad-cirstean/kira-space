package agentmcp

import "github.com/kirathecat/kira-studio/internal/harness"

// ServerName is the MCP server name in the run's --mcp-config.
const ServerName = "kira-space"

// Tool ids of the Kira tool server; harness.ToolNames renders them in a provider's names.
var (
	FinishStep = harness.MCPTool{Server: ServerName, Tool: "finish_step"}
	Git        = harness.MCPTool{Server: ServerName, Tool: "git"}
	State      = harness.MCPTool{Server: ServerName, Tool: "state"}
	CreateTask = harness.MCPTool{Server: ServerName, Tool: "create_task"}

	// SpaceToolIDs are the Kira Space tools; a run and a TUI launch with a Space grant pass them
	// instead of finish_step.
	SpaceToolIDs = []harness.MCPTool{State}

	// OrchestratorToolIDs are the tools only an orchestrator task carries, next to SpaceToolIDs.
	OrchestratorToolIDs = []harness.MCPTool{CreateTask}

	// StateToolIDs is the state tool alone: a review round or a sandboxed run without Space.
	StateToolIDs = []harness.MCPTool{State}

	// ReviewReply and SandboxToolIDs are state actions now: a review round or a sandboxed run lists state.
	ReviewReply    = State
	SandboxToolIDs = StateToolIDs
)
