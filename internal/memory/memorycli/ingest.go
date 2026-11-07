package memorycli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/kirapaths"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/memory/embed"
	"github.com/kirathecat/kira-studio/internal/memory/importer"
)

// RunIngest is the `memory-ingest` subcommand body: the detached import runner, one per memory.db.
// It exits 0 at once when another runner holds the store's lock, and after the store has been idle.
// promptsPath is the app's llm-prompts.json; agents picks the provider and models its model per
// call. Diagnostics go to stderr, which the launcher appends to the runner log.
func RunIngest(args []string, promptsPath string, agents harness.Selector, models memory.ModelSource) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "memory-ingest takes no arguments")
		return 2
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "memory-ingest:", err)
		return 1
	}
	store := memory.OpenDefault()
	defer store.Close()
	var prompts llmprompts.Source = llmprompts.Defaults()
	if promptsPath != "" {
		prompts = llmprompts.NewStore(promptsPath)
	}
	agent := memory.NewAgentRunner(agents, models)
	svc := memory.NewService(store, agent, memory.ServiceOptions{
		Embedder: embed.NewClient(embed.ClientOptions{Spec: embed.Default, Home: memory.Home()}), Prompts: prompts,
	})
	defer svc.Close()
	runner, err := importer.NewRunner(store, importer.RunnerOptions{
		Agent: importer.ModelAgent{Runner: agent, Executable: exe, SpaceHome: kirapaths.SpaceHome(), Prompts: prompts},
		Docs:  svc,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "memory-ingest:", err)
		return 1
	}
	switch err := runner.Run(ctx); {
	case errors.Is(err, importer.ErrRunnerBusy):
		return 0
	case err != nil:
		fmt.Fprintln(os.Stderr, "memory-ingest:", err)
		return 1
	}
	return 0
}
