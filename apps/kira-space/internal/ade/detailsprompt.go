package ade

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
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
	v := llmprompts.FilesVars{Folder: dir, Draft: draftRel(stepID), Proofs: taskfiles.PromptProofLines(b.proofIndex(taskID))}
	if l, ok := agent.(harness.ToolLoader); ok {
		v.Load = l.LoadHint(sessionTools(run, orchestrator)...)
	}
	return v
}

// sessionTools are the Kira tools a stage needs at once: a headless run's finish_step and submit, an
// interactive session's submit, the orchestrator's create_task. The suffix names the others.
func sessionTools(run, orchestrator bool) []harness.MCPTool {
	var tools []harness.MCPTool
	if run {
		tools = append(tools, agentmcp.FinishStep)
	}
	tools = append(tools, agentmcp.Submit)
	if orchestrator {
		tools = append(tools, agentmcp.CreateTask)
	}
	return tools
}
