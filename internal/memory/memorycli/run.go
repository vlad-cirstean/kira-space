// Package memorycli is the `memory-mcp` subcommand body: the stdio MCP server over the shared
// memory.db, run by Claude Code per session.
package memorycli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/mcpcall"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/embed"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
	"github.com/kirathecat/kira-studio/internal/memory/mcpserver"
)

// OpsRecorder builds the tools/call hook for a server of project (import mode when importMode), plus
// the func that releases it. A nil hook records nothing.
type OpsRecorder func(project string, importMode bool) (mcpcall.Hook, func())

// Run returns the process exit code. promptsPath is the app's llm-prompts.json: the gate, reconcile
// and server texts it overrides apply here too. agents picks the provider and models its model per call. Diagnostics go to stderr: stdout is the protocol.
func Run(args []string, promptsPath string, agents harness.Selector, models memory.ModelSource, ops OpsRecorder) int {
	fs := flag.NewFlagSet("memory-mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	importRef := fs.String("import-ref", "", "attribute every stored memory to this bulk-import file id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *importRef != "" {
		if !ids.IsUUID(*importRef) {
			fmt.Fprintln(os.Stderr, "kira-memory: --import-ref must be a UUID")
			return 2
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := memory.OpenDefault()
	defer store.Close()
	embedder := embed.NewClient(embed.ClientOptions{Spec: embed.Default, Home: memory.Home()})
	var prompts llmprompts.Source = llmprompts.Defaults()
	if promptsPath != "" {
		prompts = llmprompts.NewStore(promptsPath)
	}
	agent := memory.NewAgentRunner(agents, models)
	svc := memory.NewService(store, agent, memory.ServiceOptions{Embedder: embedder, Prompts: prompts})
	defer svc.Close()
	opts := mcpserver.Options{ImportRef: *importRef, Prompts: prompts, Project: detectProject(ctx)}
	if ops != nil {
		hook, closeOps := ops(opts.Project, *importRef != "")
		defer closeOps()
		opts.OnToolCall = hook
	}
	if *importRef == "" {
		opts.Distill = importer.Distiller{Runner: agent, Prompts: prompts}
		ing := &lazyIngest{store: store, available: agent.Available}
		defer ing.Close()
		opts.Ingest = ing
	}
	if id := os.Getenv("CLAUDE_CODE_SESSION_ID"); memory.ValidSession(id) {
		opts.SessionID = id
	}
	if err := mcpserver.RunStdio(ctx, svc, opts); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "kira-memory:", err)
		return 1
	}
	return 0
}

// detectProject is the project key of the working directory, empty outside a git repository.
func detectProject(ctx context.Context) string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	key, err := memory.DetectProject(ctx, dir)
	if err != nil {
		return ""
	}
	return key
}
