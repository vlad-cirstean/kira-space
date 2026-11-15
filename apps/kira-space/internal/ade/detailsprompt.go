package ade

import (
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// filesVars is the task-files fragment of a prompt: the folder, the step's draft file and, for a
// provider that loads MCP tools lazily, the sentence that loads exactly the session's tools. Zero
// when the board has no files root.
func (b *TaskBoard) filesVarsFor(taskID, stepID, agentID string, run, orchestrator bool) llmprompts.FilesVars {
	agent, err := b.agentFor(agentID)
	if err != nil {
		agent = nil
	}
	return b.filesVars(taskID, stepID, agent, run, orchestrator)
}

func (b *TaskBoard) filesVars(taskID, stepID string, agent harness.Provider, run, orchestrator bool) llmprompts.FilesVars {
	if b.files == nil {
		return llmprompts.FilesVars{}
	}
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return llmprompts.FilesVars{}
	}
	v := llmprompts.FilesVars{Folder: dir, Draft: draftRel(stepID)}
	if l, ok := agent.(harness.ToolLoader); ok {
		v.Load = l.LoadHint(sessionTools(run, orchestrator)...)
	}
	return v
}

// sessionTools are the Kira tools a headless run (finish_step too) or an interactive session carries.
func sessionTools(run, orchestrator bool) []harness.MCPTool {
	var tools []harness.MCPTool
	if run {
		tools = append(tools, agentmcp.FinishStep)
	}
	tools = append(tools, agentmcp.RunOutcome)
	tools = append(tools, agentmcp.SpaceToolIDs...)
	if orchestrator {
		tools = append(tools, agentmcp.OrchestratorToolIDs...)
	}
	return tools
}
