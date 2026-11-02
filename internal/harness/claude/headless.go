package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude/hooks"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/linewriter"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/procgroup"
)

// gracefulStopDelay is SIGTERM's grace window before SIGKILL; a var so a test can shorten it.
var gracefulStopDelay = 2 * time.Second

var killGroup = procgroup.Kill

// onStdout feeds one stdout line to the handler's callbacks.
func onStdout(h harness.Handler, line string, emit func(harness.Line)) {
	if h.OnRateLimits != nil {
		if rl, ok := parseRateLimits(line); ok {
			h.OnRateLimits(rl)
		}
	}
	if h.OnResult != nil {
		if r, ok := parseResult(line); ok {
			h.OnResult(r)
		}
	}
	for _, l := range parseLine(line) {
		emit(l)
	}
}

// settingSources is the --setting-sources value of a scope.
func settingSources(s harness.HeadlessSpec) string {
	if s.Isolated {
		if len(s.UserMCP) > 0 {
			return "user"
		}
		return ""
	}
	switch s.Config {
	case harness.ConfigUser:
		return "user"
	case harness.ConfigAll:
		return "user,project,local"
	}
	return ""
}

// userMCPSettings is the --settings value of an isolated run that loads user MCP servers: hooks off
// and only the granted and ticked servers allowed. Project .mcp.json servers never load, so a repo
// cannot supply the command behind a ticked name.
func userMCPSettings(s harness.HeadlessSpec) string {
	type server struct {
		Name string `json:"serverName"`
	}
	allowed := make([]server, 0, len(s.GrantMCP)+len(s.UserMCP))
	for _, n := range s.GrantMCP {
		allowed = append(allowed, server{n})
	}
	for _, n := range s.UserMCP {
		allowed = append(allowed, server{n})
	}
	b, _ := json.Marshal(struct {
		DisableAllHooks         bool     `json:"disableAllHooks"`
		AllowedMcpServers       []server `json:"allowedMcpServers"`
		EnableAllProjectServers bool     `json:"enableAllProjectMcpServers"`
		EnabledMcpjsonServers   []string `json:"enabledMcpjsonServers"`
	}{true, allowed, false, []string{}})
	return string(b)
}

// sandboxWords is the env and sandbox-exec wrapper put in front of the agent; none without a sandbox.
func sandboxWords(sb *harness.SandboxExec) []string {
	if sb == nil {
		return nil
	}
	words := []string{"/usr/bin/env"}
	for _, name := range sb.Unset {
		words = append(words, "-u", loginshell.Quote(name))
	}
	for _, kv := range sb.Env {
		words = append(words, loginshell.Quote(kv))
	}
	sbin := sb.Bin
	if sbin == "" {
		sbin = harness.DefaultSandboxBin
	}
	return append(words, loginshell.Quote(sbin), "-f", loginshell.Quote(sb.Profile))
}

// Script is the shell text that execs claude; run under the user's login shell it sees the same
// PATH as the TUI launch. The bearer token lives in the MCP config file, never on argv.
func Script(s harness.HeadlessSpec) string {
	bin := s.Bin
	if bin == "" {
		bin = "claude"
	}
	session := []string{"--session-id", loginshell.Quote(s.SessionID)}
	if s.Resume != "" {
		session = []string{"--resume", loginshell.Quote(s.Resume)}
	}
	words := append([]string{"exec"}, sandboxWords(s.Sandbox)...)
	words = append(words, bin, "-p", "--output-format", "stream-json", "--verbose")
	words = append(words, session...)
	for _, p := range s.MCPConfigs {
		words = append(words, "--mcp-config", loginshell.Quote(p))
	}
	if s.App != nil && s.App.Files["mcp"] != "" {
		words = append(words, "--mcp-config", loginshell.Quote(s.App.Files["mcp"]))
	}
	if s.Model != "" {
		words = append(words, "--model", loginshell.Quote(s.Model))
	}
	if s.MaxBudgetUSD > 0 {
		words = append(words, "--max-budget-usd", strconv.FormatFloat(s.MaxBudgetUSD, 'f', -1, 64))
	}
	words = append(words, stageArgs(s.Stage, loginshell.Quote)...)
	if s.ContextFile != "" {
		words = append(words, appendSystemPromptFileFlag, loginshell.Quote(s.ContextFile))
	}
	words = append(words, "--setting-sources", loginshell.Quote(settingSources(s)))
	if s.App != nil && s.App.Files["settings"] != "" {
		words = append(words, "--settings", loginshell.Quote(s.App.Files["settings"]))
	}
	if s.Isolated {
		if len(s.UserMCP) > 0 {
			words = append(words, "--settings", loginshell.Quote(userMCPSettings(s)))
		} else {
			words = append(words, "--strict-mcp-config")
		}
		words = append(words, "--permission-prompts", "none")
	}
	if len(s.AddDirs) > 0 {
		words = append(words, "--add-dir")
		for _, d := range s.AddDirs {
			words = append(words, loginshell.Quote(d))
		}
	}
	if s.Tools != nil {
		words = append(words, "--tools", loginshell.Quote(strings.Join(s.Tools, ",")))
	}
	if len(s.AllowedTools) > 0 {
		words = append(words, "--allowedTools")
		for _, t := range s.AllowedTools {
			words = append(words, loginshell.Quote(t))
		}
	}
	if len(s.DisallowedTools) > 0 {
		words = append(words, "--disallowedTools")
		for _, t := range s.DisallowedTools {
			words = append(words, loginshell.Quote(t))
		}
	}
	return strings.Join(words, " ")
}

// RunHeadless starts claude, feeds the prompt on stdin and streams parsed output to h until it exits.
func (p *Provider) RunHeadless(ctx context.Context, spec harness.HeadlessSpec, h harness.Handler) (harness.Exit, error) {
	if spec.Timeout <= 0 {
		return harness.Exit{}, errors.New("claude: HeadlessSpec.Timeout must be positive")
	}
	if spec.Isolated && spec.App != nil {
		return harness.Exit{}, errors.New("claude: an isolated run takes no app settings")
	}
	if _, err := p.Locate(ctx, spec.Bin); err != nil {
		var ie *ipcerr.Error
		if errors.As(err, &ie) {
			return harness.Exit{}, errors.New(ie.Message)
		}
		return harness.Exit{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	argv := loginshell.BuildArgv(shell, login, Script(spec))
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(slices.Clone(cmd.Env), hooks.MCPToolTimeoutEnv)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = strings.NewReader(spec.Prompt)
	stopEscalate := procgroup.GracefulCancel(cmd, gracefulStopDelay, killGroup)

	var mu sync.Mutex
	emit := func(l harness.Line) {
		if h.OnLine != nil {
			h.OnLine(l)
		}
	}
	cmd.Stdout = linewriter.New(func(line string) {
		mu.Lock()
		defer mu.Unlock()
		onStdout(h, line, emit)
	})
	cmd.Stderr = linewriter.New(func(line string) {
		mu.Lock()
		defer mu.Unlock()
		if strings.TrimSpace(line) != "" {
			emit(harness.Line{Stream: harness.StreamStderr, Text: line})
		}
	})

	if err := cmd.Start(); err != nil {
		return harness.Exit{}, err
	}
	if h.OnStart != nil {
		h.OnStart(cmd.Process.Pid)
	}
	waitErr := cmd.Wait()
	stopEscalate()
	cmd.Stdout.(*linewriter.Writer).Flush()
	cmd.Stderr.(*linewriter.Writer).Flush()

	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	cancelled := !timedOut && ctx.Err() != nil
	if (timedOut || cancelled) && cmd.Process != nil {
		_ = killGroup(cmd.Process.Pid, syscall.SIGKILL)
	}
	exit := harness.Exit{TimedOut: timedOut, Cancelled: cancelled}
	if waitErr == nil {
		return exit, nil
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(waitErr, &exitErr):
		exit.Code = exitErr.ExitCode()
		return exit, nil
	case errors.Is(waitErr, exec.ErrWaitDelay) && cmd.ProcessState != nil:
		exit.Code = cmd.ProcessState.ExitCode()
		return exit, nil
	case timedOut || cancelled:
		return exit, nil
	}
	return harness.Exit{}, waitErr
}

// ScrubEnv drops the variables that tie a child to the Claude Code session running this process.
func (*Provider) ScrubEnv(environ []string) []string { return ScrubSessionEnv(environ) }
