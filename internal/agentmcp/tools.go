package agentmcp

import "github.com/kirathecat/kira-studio/internal/harness"

// ServerName is the MCP server name in the run's --mcp-config.
const ServerName = "kira-space"

// Tool ids of the Kira tool server; harness.ToolNames renders them in a provider's names.
var (
	FinishStep  = harness.MCPTool{Server: ServerName, Tool: "finish_step"}
	RunOutcome  = harness.MCPTool{Server: ServerName, Tool: "run_outcome"}
	Git         = harness.MCPTool{Server: ServerName, Tool: "git"}
	ReviewReply = harness.MCPTool{Server: ServerName, Tool: "reply_review_comment"}
	Handover    = harness.MCPTool{Server: ServerName, Tool: "handover"}

	// SpaceToolIDs are the Kira Space tools; a headless run and a TUI launch pass them next to
	// finish_step.
	SpaceToolIDs = []harness.MCPTool{
		{Server: ServerName, Tool: "task_info"},
		{Server: ServerName, Tool: "declare_repos"},
		{Server: ServerName, Tool: "request_branches"},
		{Server: ServerName, Tool: "branch_status"},
		{Server: ServerName, Tool: "rename_branch"},
		{Server: ServerName, Tool: "report_artifacts"},
		{Server: ServerName, Tool: "attach_proof"},
		{Server: ServerName, Tool: "list_operations"},
		{Server: ServerName, Tool: "cancel_operation"},
	}

	// SandboxToolIDs are the sandbox git tools.
	SandboxToolIDs = []harness.MCPTool{
		{Server: ServerName, Tool: "fetch_remote"},
		{Server: ServerName, Tool: "push_branch"},
	}
)
