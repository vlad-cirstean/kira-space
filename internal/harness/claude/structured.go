package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/internal/agentproc"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

const (
	structuredBudget  = "0.50"
	structuredTimeout = 120 * time.Second
)

// readFilesTools are the built-ins a ReadFiles call may use.
var readFilesTools = []string{"Read", "Grep"}

// structuredArgs is the isolation contract. Not --bare: it forces ANTHROPIC_API_KEY auth and never
// reads OAuth/keychain, which breaks a subscription login.
func structuredArgs(model string, c harness.StructuredCall, mcpConfig string, allowed []string) []string {
	budget := c.Budget
	if budget == "" {
		budget = structuredBudget
	}
	var tools []string
	if c.ReadFiles {
		tools = readFilesTools
	}
	args := []string{"-p", "--model", model}
	if mcpConfig == "" && len(tools) == 0 {
		args = append(args, "--safe-mode")
	}
	args = append(args, "--setting-sources", "", "--strict-mcp-config")
	if len(tools) > 0 {
		// --restricted confines Read and Grep to the working dir and drops code-running tools. Probed in
		// P274: without it Read opens an absolute path outside cwd under --safe-mode. Not --safe-mode:
		// it is kept off for tool calls, as for MCP calls.
		joined := strings.Join(tools, ",")
		args = append(args, "--restricted", "--tools", joined, "--allowedTools", joined)
		if c.MaxTurns > 0 {
			args = append(args, "--max-turns", strconv.Itoa(c.MaxTurns))
		}
	} else {
		args = append(args, "--tools", "")
	}
	if mcpConfig != "" {
		// --safe-mode disables --mcp-config servers too (measured in P211), so an MCP call runs
		// without it; --strict-mcp-config still keeps every other server out.
		args = append(args, "--mcp-config", mcpConfig, "--allowedTools", strings.Join(allowed, ","))
	}
	return append(args,
		"--disable-slash-commands", "--no-session-persistence", "--permission-prompts", "none",
		"--output-format", "json", "--json-schema", string(c.Schema),
		"--system-prompt", c.System, "--max-budget-usd", budget)
}

type structuredResult struct {
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	NumTurns         int             `json:"num_turns"`
}

// RunStructured drives the claude CLI headless with every source of ambient context switched off:
// only the system prompt and the stdin document are visible to the model.
func (p *Provider) RunStructured(ctx context.Context, model string, c harness.StructuredCall) (harness.StructuredResult, error) {
	path, _, found := p.find()
	if !found {
		return harness.StructuredResult{}, errNotFound
	}
	var mcpConfig string
	var allowed []string
	if len(c.MCPServers) > 0 {
		raw, err := p.MCPConfig(c.MCPServers)
		if err != nil {
			return harness.StructuredResult{}, fmt.Errorf("claude: mcp config: %w", err)
		}
		mcpConfig = string(raw)
		for _, t := range c.MCPTools {
			allowed = append(allowed, p.MCPToolName(t))
		}
	}
	dir, err := os.MkdirTemp("", "kira-memory-*")
	if err != nil {
		return harness.StructuredResult{}, fmt.Errorf("claude: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	for name, data := range c.Files {
		if err := renameio.WriteFile(filepath.Join(dir, filepath.Base(name)), data, 0o600); err != nil {
			return harness.StructuredResult{}, fmt.Errorf("claude: seed temp dir: %w", err)
		}
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = structuredTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, runErr := toolexec.RunIOTracked(runCtx, path, structuredArgs(model, c, mcpConfig, allowed), dir, ScrubSessionEnv(os.Environ()), []byte(c.Input), agentproc.Track)
	if runErr != nil && runCtx.Err() != nil {
		if ctx.Err() != nil {
			return harness.StructuredResult{}, ctx.Err()
		}
		return harness.StructuredResult{}, errTimeout
	}
	return parseStructured(out, runErr)
}

func parseStructured(out []byte, runErr error) (harness.StructuredResult, error) {
	var res structuredResult
	parsed := json.Unmarshal(out, &res) == nil
	spent := harness.StructuredResult{Usage: harness.Usage{CostUSD: res.TotalCostUSD, Turns: res.NumTurns}}
	if parsed && res.Subtype == "error_max_budget_usd" {
		return spent, errBudget
	}
	if parsed && res.Subtype == "error_max_turns" {
		return spent, errMaxTurns
	}
	if runErr != nil || (parsed && res.IsError) {
		detail := res.Result
		var execErr *toolexec.ExecError
		if errors.As(runErr, &execErr) {
			detail += " " + execErr.Stderr
		}
		return spent, classifyFailure(detail, runErr)
	}
	if !parsed || len(res.StructuredOutput) == 0 || string(res.StructuredOutput) == "null" {
		return spent, errOutput
	}
	spent.Output = res.StructuredOutput
	return spent, nil
}

func classifyFailure(detail string, runErr error) error {
	low := strings.ToLower(detail)
	switch {
	case strings.Contains(low, "unknown option") || strings.Contains(low, "unknown flag"):
		return errOutdated
	case strings.Contains(low, "login") || strings.Contains(low, "log in") || strings.Contains(low, "api key") ||
		strings.Contains(low, "authenticat") || strings.Contains(low, "oauth"):
		return errAuth
	case strings.Contains(low, "usage limit") || strings.Contains(low, "limit reached"):
		return errUsageLimit
	case strings.Contains(low, "rate limit") || strings.Contains(low, "rate_limit") || strings.Contains(low, "429") ||
		strings.Contains(low, "overloaded") || strings.Contains(low, "529"):
		return errRateLimited
	}
	msg := strings.TrimSpace(detail)
	if msg == "" && runErr != nil {
		msg = runErr.Error()
	}
	return fmt.Errorf("Claude Code failed: %s", toolexec.FirstLineBounded(msg, 300))
}
