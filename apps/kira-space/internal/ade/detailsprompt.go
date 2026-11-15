package ade

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// filesVars is the task-files fragment of a prompt: the folder and, for a
// provider that loads MCP tools lazily, the sentence that loads exactly the session's tools. Zero
// when the board has no files root.
func (b *TaskBoard) filesVarsFor(taskID, agentID string, orchestrator bool) llmprompts.FilesVars {
	agent, err := b.agentFor(agentID)
	if err != nil {
		agent = nil
	}
	return b.filesVars(taskID, agent, orchestrator)
}

func (b *TaskBoard) filesVars(taskID string, agent harness.Provider, orchestrator bool) llmprompts.FilesVars {
	if b.files == nil {
		return llmprompts.FilesVars{}
	}
	dir, err := b.filesDir(taskID)
	if err != nil || dir == "" {
		return llmprompts.FilesVars{}
	}
	v := llmprompts.FilesVars{Folder: dir, Proofs: taskfiles.PromptProofLines(b.proofIndex(taskID))}
	if l, ok := agent.(harness.ToolLoader); ok {
		v.Load = l.LoadHint(sessionTools(orchestrator)...)
	}
	return v
}

// sessionTools are the Kira tools a stage needs at once: submit and state, plus the orchestrator's
// create_task. A Space grant has no other tool.
func sessionTools(orchestrator bool) []harness.MCPTool {
	tools := []harness.MCPTool{agentmcp.Submit, agentmcp.State}
	if orchestrator {
		tools = append(tools, agentmcp.CreateTask)
	}
	return tools
}
