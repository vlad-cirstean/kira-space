package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/linewriter"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/procgroup"
)

// gracefulStopDelay is SIGTERM's grace window before SIGKILL; a var so a test can shorten it.
var gracefulStopDelay = 2 * time.Second

var killGroup = procgroup.Kill

const maxResultText = 1 << 10

func refuse(msg string) error {
	return &harness.Error{Kind: harness.ErrUnsupported, Msg: msg}
}

// scope is which of the user's config layers a run loads.
type scope struct{ user, project bool }

func scopeOf(s harness.HeadlessSpec) scope {
	if s.Isolated {
		return scope{user: len(s.UserMCP) > 0}
	}
	return scope{user: s.Config != harness.ConfigNone, project: s.Config == harness.ConfigAll}
}

// Script is the shell text that execs opencode; run under the user's login shell it sees the same
// PATH as the TUI launch. The prompt goes on stdin: a positional message waits on stdin when it is
// not a terminal (probe P-2). Tokens live in the config file, never on argv.
func Script(s harness.HeadlessSpec) string {
	bin := s.Bin
	if bin == "" {
		bin = "opencode"
	}
	words := []string{"exec"}
	if sb := s.Sandbox; sb != nil {
		words = append(words, "/usr/bin/env")
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
		words = append(words, loginshell.Quote(sbin), "-f", loginshell.Quote(sb.Profile))
	}
	words = append(words, bin, "run", "--format", "json")
	if !scopeOf(s).project {
		words = append(words, "--pure")
	}
	if s.Resume != "" {
		words = append(words, "-s", loginshell.Quote(s.Resume))
	}
	if s.Model != "" && s.Model != defaultModel {
		words = append(words, "-m", loginshell.Quote(s.Model))
	}
	if s.Dir != "" {
		words = append(words, "--dir", loginshell.Quote(s.Dir))
	}
	return strings.Join(words, " ")
}

// mergeMCP writes the mcp entries of every config in paths into one file under dir.
func mergeMCP(dir string, paths []string) (string, error) {
	merged := map[string]json.RawMessage{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		var doc struct {
			MCP map[string]json.RawMessage `json:"mcp"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return "", err
		}
		maps.Copy(merged, doc.MCP)
	}
	out, err := json.Marshal(map[string]any{"mcp": merged})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "kira-agent-config.json")
	return path, renameio.WriteFile(path, out, 0o600)
}

// redirectConfigHome makes a config home under dir that links every entry of the real one except
// OpenCode's own, so git and other tools keep their config while user MCP servers, the global
// AGENTS.md and other OpenCode config drop out. Login lives in the data dir and stays.
func redirectConfigHome(dir string, environ []string) (string, error) {
	real := ""
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "XDG_CONFIG_HOME="); ok {
			real = v
		}
	}
	if real == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		real = filepath.Join(home, ".config")
	}
	target := filepath.Join(dir, "xdg")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return target, nil
		}
		return "", err
	}
	for _, e := range entries {
		if e.Name() == "opencode" {
			continue
		}
		if err := os.Symlink(filepath.Join(real, e.Name()), filepath.Join(target, e.Name())); err != nil {
			return "", err
		}
	}
	return target, nil
}

// runEnv is the child environment of a run and the scratch dir to remove after it.
func (p *Provider) runEnv(ctx context.Context, s harness.HeadlessSpec) (env []string, scratch string, err error) {
	var users []harness.UserServer
	if s.Isolated && len(s.UserMCP) > 0 {
		if users, err = p.UserMCPServers(ctx, s.Dir); err != nil {
			return nil, "", err
		}
	}
	doc, err := permissionDoc(s, users)
	if err != nil {
		return nil, "", err
	}
	if s.ContextFile != "" {
		doc[instructionsKey] = []string{s.ContextFile}
	}
	if s.App != nil {
		if err := withAppConfig(doc, s.App); err != nil {
			return nil, "", err
		}
	}
	content, err := json.Marshal(doc)
	if err != nil {
		return nil, "", err
	}
	scratch, err = os.MkdirTemp("", "kira-opencode-")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(scratch)
		}
	}()
	base := s.Env
	if base == nil {
		base = os.Environ()
	}
	sc := scopeOf(s)
	env = p.ScrubEnv(base)
	if !sc.user {
		env = dropEnv(env, layerEnv)
		cfgHome, err := redirectConfigHome(scratch, base)
		if err != nil {
			return nil, scratch, err
		}
		env = append(env, "XDG_CONFIG_HOME="+cfgHome)
	}
	if !sc.project {
		env = append(env, "OPENCODE_DISABLE_PROJECT_CONFIG=1", "OPENCODE_DISABLE_CLAUDE_CODE=1")
	}
	switch len(s.MCPConfigs) {
	case 0:
	case 1:
		env = append(env, "OPENCODE_CONFIG="+s.MCPConfigs[0])
	default:
		path, err := mergeMCP(scratch, s.MCPConfigs)
		if err != nil {
			return nil, scratch, err
		}
		env = append(env, "OPENCODE_CONFIG="+path)
	}
	env = append(env, "OPENCODE_CONFIG_CONTENT="+string(content))
	return env, scratch, nil
}

// runState folds the stdout events into the run's session id and closing result.
type runState struct {
	// mu serialises every handler callback of the run, stdout and stderr alike.
	mu        *sync.Mutex
	resume    string
	gotID     bool
	errText   string
	cost      *float64
	lastText  string
	emit      func(harness.Line)
	idReports func(string)
}

func (r *runState) stdout(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev, ok := parseEvent(line)
	if !ok {
		if strings.TrimSpace(line) != "" {
			r.emit(harness.Line{Stream: harness.StreamStdout, Text: line})
		}
		return
	}
	if ev.SessionID != "" && !r.gotID && harness.ValidSessionID(ev.SessionID) {
		r.gotID = true
		if ev.SessionID != r.resume {
			r.idReports(ev.SessionID)
		}
	}
	if ev.Cost != nil {
		r.cost = ev.Cost
	}
	if ev.Text != "" {
		r.lastText = ev.Text
	}
	if ev.Err != "" && r.errText == "" {
		r.errText = ev.Err
	}
	for _, l := range ev.Lines {
		r.emit(l)
	}
}

// RunHeadless starts opencode, feeds the prompt on stdin and streams parsed output to h until it
// exits. Kira's tool grants never use --auto, and nothing in the run may prompt.
func (p *Provider) RunHeadless(ctx context.Context, spec harness.HeadlessSpec, h harness.Handler) (harness.Exit, error) {
	if spec.Timeout <= 0 {
		return harness.Exit{}, errors.New("opencode: HeadlessSpec.Timeout must be positive")
	}
	if spec.Isolated && spec.App != nil {
		return harness.Exit{}, errors.New("opencode: an isolated run takes no app settings")
	}
	if spec.MaxBudgetUSD > 0 {
		return harness.Exit{}, refuse("OpenCode can't cap a run's cost.")
	}
	if spec.Model != "" && !Catalog.Valid(spec.Model) {
		return harness.Exit{}, refuse("OpenCode has no model " + spec.Model + " to choose here.")
	}
	if _, err := p.Locate(ctx, spec.Bin); err != nil {
		var ie *ipcerr.Error
		if errors.As(err, &ie) {
			return harness.Exit{}, errors.New(ie.Message)
		}
		return harness.Exit{}, err
	}
	env, scratch, err := p.runEnv(ctx, spec)
	if scratch != "" {
		defer func() { _ = os.RemoveAll(scratch) }()
	}
	if err != nil {
		return harness.Exit{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	argv := loginshell.BuildArgv(shell, login, Script(spec))
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = strings.NewReader(spec.Prompt)
	stopEscalate := procgroup.GracefulCancel(cmd, gracefulStopDelay, killGroup)

	start := time.Now()
	var mu sync.Mutex
	emit := func(l harness.Line) {
		if h.OnLine != nil {
			h.OnLine(l)
		}
	}
	st := &runState{mu: &mu, resume: spec.Resume, emit: emit, idReports: func(id string) {
		if h.OnSessionID != nil {
			h.OnSessionID(id)
		}
	}}
	cmd.Stdout = linewriter.New(st.stdout)
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
	var exitErr *exec.ExitError
	switch {
	case waitErr == nil:
	case errors.As(waitErr, &exitErr):
		exit.Code = exitErr.ExitCode()
	case errors.Is(waitErr, exec.ErrWaitDelay) && cmd.ProcessState != nil:
		exit.Code = cmd.ProcessState.ExitCode()
	case timedOut || cancelled:
	default:
		return harness.Exit{}, waitErr
	}
	p.finishRun(ctx, spec, h, st, exit, start, timedOut || cancelled)
	return exit, nil
}

// finishRun reports a late session id the stream never carried, then the closing result.
func (p *Provider) finishRun(ctx context.Context, spec harness.HeadlessSpec, h harness.Handler, st *runState, exit harness.Exit, start time.Time, ended bool) {
	if h.OnSessionID != nil && !st.gotID && spec.Resume == "" {
		id, err := p.FindSession(context.WithoutCancel(ctx), harness.SessionQuery{Cwd: spec.Dir, Since: start})
		if err == nil && id != "" {
			h.OnSessionID(id)
		}
	}
	if h.OnResult == nil || ended {
		return
	}
	r := harness.Result{End: harness.EndDone, CostUSD: st.cost, Text: st.lastText}
	if st.errText != "" || exit.Code != 0 {
		r.End = harness.EndError
	}
	if st.errText != "" {
		r.Text = st.errText
	}
	if len(r.Text) > maxResultText {
		r.Text = strings.ToValidUTF8(r.Text[:maxResultText], "") + "…"
	}
	h.OnResult(r)
}
