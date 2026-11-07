package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory"
)

const (
	finalizeBaseTimeout = 5 * time.Minute
	finalizeBatchTime   = 45 * time.Second
	finalizeMaxTimeout  = 45 * time.Minute
	finalizeBaseBudget  = 1.00
	finalizeBatchBudget = 0.15
	finalizeMaxBudget   = 5.00
	mcpServerName       = "kira-memory"
)

var finalizeTools = []harness.MCPTool{
	{Server: mcpServerName, Tool: "store_memory"},
	{Server: mcpServerName, Tool: "search_memories"},
	{Server: mcpServerName, Tool: "memory_history"},
}

type finalizeDoc struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Chunks int    `json:"chunks"`
}

type finalizeProv struct {
	Origin    string `json:"origin"`
	SourceTag string `json:"sourceTag,omitempty"`
	Scope     string `json:"scope"`
	Project   string `json:"project,omitempty"`
}

type finalizeWire struct {
	Document   finalizeDoc  `json:"document"`
	Provenance finalizeProv `json:"provenance"`
	// Challenge is false for an external source nobody can answer: the store accepts instead of asking.
	Challenge bool        `json:"challenge"`
	Facts     []FinalFact `json:"facts"`
}

// mcpServer is the one MCP server the finalize agent may use: this binary in import mode, so
// every memory it stores is attributed to the file.
func mcpServer(exe, spaceHome, fileID string, extraEnv map[string]string) harness.MCPServer {
	env := map[string]string{"KIRA_SPACE_HOME": spaceHome}
	for k, v := range extraEnv {
		env[k] = v
	}
	return harness.MCPServer{Name: mcpServerName, Command: exe, Args: []string{"memory-mcp", "--import-ref", fileID}, Env: env}
}

func finalizeLimits(facts int) (time.Duration, string) {
	batches := math.Ceil(float64(facts) / factsPerBatch)
	timeout := min(finalizeBaseTimeout+time.Duration(batches)*finalizeBatchTime, finalizeMaxTimeout)
	budget := min(finalizeBaseBudget+finalizeBatchBudget*batches, finalizeMaxBudget)
	return timeout, fmt.Sprintf("%.2f", budget)
}

func (a ModelAgent) Finalize(ctx context.Context, in FinalizeInput) (FinalizeOutput, error) {
	input, err := json.Marshal(finalizeWire{
		Document:   finalizeDoc{Path: in.Path, Title: in.Title, Chunks: in.ChunkCount},
		Provenance: finalizeProv{Origin: in.Prov.Origin, SourceTag: in.Prov.SourceTag, Scope: in.Prov.Scope, Project: in.Prov.Project},
		Challenge:  in.Prov.Origin != memory.OriginExternal, Facts: in.Facts,
	})
	if err != nil {
		return FinalizeOutput{}, err
	}
	timeout, budget := finalizeLimits(len(in.Facts))
	res, err := a.Runner.RunResult(ctx, memory.Call{
		System: llmprompts.MemoryImportFinalizeSystem.Text(a.prompts(), llmprompts.FinalizeVars{MaxItems: memory.MaxItems}), Input: string(input), Schema: llmprompts.MemoryImportFinalizeSystem.Schema(),
		MCPServers: []harness.MCPServer{mcpServer(a.Executable, a.SpaceHome, in.FileID, a.Env)}, MCPTools: finalizeTools,
		Timeout: timeout, Budget: budget,
	})
	out := FinalizeOutput{CostUSD: res.CostUSD}
	if err != nil {
		return out, err
	}
	var parsed struct {
		Unresolved []UnresolvedFact
		Dropped    []DroppedFact
	}
	if err := json.Unmarshal(res.Output, &parsed); err != nil {
		return out, a.Runner.Fail(harness.ErrOutput)
	}
	out.Unresolved, out.Dropped = parsed.Unresolved, parsed.Dropped
	return out, nil
}
