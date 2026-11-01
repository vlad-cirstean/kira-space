package terminal

import (
	"context"
	"errors"
	"slices"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// BoundService is the one Wails-bound terminal surface both apps embed. Each app's own
// bridge.TerminalService is `struct{ *terminal.BoundService }` — Wails' binding generator builds a
// call's FQN from the *registered* type's own package, promoted methods included, so each
// app keeps its own binding names while every method body here is shared. A separate type from
// Service (not new methods on it): Service also exports OpenWithCoalescedOutput and positional
// Write/Resize/Close, which embedding would bind too.
type BoundService struct {
	Emit     appevent.Emitter
	Registry *Registry
	// ComposeAgent, when set, rewrites a agent launch's command and supplies extra env before
	// spawn. Kira Space sets it (its own session tracker's Compose method); Kira Studio leaves
	// it nil, so its own Open is byte-identical to before this field existed. A func field, not a
	// method: Wails' binding generator only ever sees exported *methods* on the registered type, so
	// adding this never grows either app's own bound-call surface.
	ComposeAgent func(terminalID, cwd, command string) (string, []string, error)
	// AbortAgent undoes ComposeAgent's persisted side effects when Open fails after a successful
	// compose. Set together with ComposeAgent; a no-op for a terminalID it composed nothing for.
	AbortAgent func(terminalID string)
	// Arbiter, when set, decides who drives an agent session's input and size and sees its output.
	// Kira Space sets it (P212 Part 2, the phone terminal broker); Kira Studio leaves it nil, so
	// the window always drives. A field, not a method, so nothing new is bound.
	Arbiter Arbiter
	// Scripts, when set, records script launches: the host loads the stored script and its folder,
	// and learns how the session ended. A field, not a method, so nothing new is bound.
	Scripts ScriptLauncher
	// Agent resolves the provider whose CLI an agent launch needs.
	Agent harness.Selector
}

// ScriptLaunch is what ScriptLauncher.Begin resolved for one script run.
type ScriptLaunch struct {
	RunID   string
	Cwd     string
	Command string
	// Env is KEY=value entries the script's shell gets on top of the terminal's own.
	Env []string
}

// ScriptLauncher is the run store a script-launching Open reports to.
type ScriptLauncher interface {
	// Begin loads the script, prepares its folder and records a running run. launchToken, when set,
	// is the single-use token of a confirmed run dialog. An error is the caller's: a blocked
	// folder, an unknown script or a token that expired.
	Begin(scriptID, terminalID, launchToken string) (ScriptLaunch, error)
	// BeginAgent consumes the single-use token of an agent launch whose env the host holds and
	// returns that env. An error is the caller's: a token that expired or names another command.
	BeginAgent(token, command string) ([]string, error)
	// Failed records that the run's session did not spawn.
	Failed(runID string, err error)
	// Exited records how the run's session ended.
	Exited(terminalID string, code int, cause CloseCause)
	// Output sees each chunk of the run's session on the reader goroutine; it must not block.
	Output(terminalID string, b []byte)
}

// Arbiter lets one other party take over an agent terminal's input and size from its window.
// Terminals the arbiter does not know (plain shells, anything not opened as an agent) are always
// allowed.
type Arbiter interface {
	// Opened runs after an agent session spawned, with the window's size.
	Opened(terminalID string, cols, rows int)
	// Output sees each chunk on the reader goroutine; it must not block.
	Output(terminalID string, b []byte)
	// Exited runs once the session's exit event was sent.
	Exited(terminalID string)
	// AllowWrite reports whether the window's keystrokes reach the pty.
	AllowWrite(terminalID string) bool
	// AllowResize reports whether the window's size reaches the pty; either way the arbiter records
	// it as the size to return to.
	AllowResize(terminalID string, cols, rows int) bool
}

// svc is the internal/terminal.Service this bound type delegates its generic half to — built fresh
// per call rather than stored, since it is a stateless pair of pointers already held as Emit/Registry.
func (b *BoundService) svc() *Service {
	return &Service{Emit: b.Emit, Registry: b.Registry}
}

// ShutdownBound closes every live session — main.go's own teardown. A package function, not a
// method: Wails binds every exported method of the registered type, promoted ones included, and a
// bound Shutdown would let any window close every terminal.
func ShutdownBound(b *BoundService) {
	b.Registry.CloseAll()
}

// DefaultCwdResult is DefaultCwd's own wire shape — Path is "" when $HOME can't be resolved (P91
// §7.1: a missing home directory must not fail boot).
type DefaultCwdResult struct {
	Path string `json:"path"`
}

// DefaultCwd is the user's home directory, for an unscoped terminal launch — a repo-scoped terminal
// keeps using internal/gitsession's own worktree-cwd resolution.
func (b *BoundService) DefaultCwd() DefaultCwdResult {
	return DefaultCwdResult{Path: DefaultCwd()}
}

// OpenResult is Open's own wire shape.
type OpenResult struct {
	Shell string `json:"shell"`
}

// WriteArgs is Write's own wire shape.
type WriteArgs struct {
	TerminalID string `json:"terminalId"`
	// Data is base64 — keystrokes are not always valid UTF-8 (paste, Alt-meta, mouse reports).
	Data string `json:"data"`
}

// ResizeArgs is Resize's own wire shape.
type ResizeArgs struct {
	TerminalID string `json:"terminalId"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// CloseArgs is Close's own wire shape.
type CloseArgs struct {
	TerminalID string `json:"terminalId"`
}

// beginAgent consumes args.AgentLaunchToken and returns the env it holds.
func (b *BoundService) beginAgent(args OpenArgs) ([]string, error) {
	if b.Scripts == nil {
		return nil, ipcerr.Contract("agent launch tokens are not available")
	}
	return b.Scripts.BeginAgent(args.AgentLaunchToken, args.Command)
}

// Open validates args, spawns a new session and returns its resolved shell path. terminalId is
// client-supplied (the tab id) so the renderer subscribes to ChannelTerminal before this call
// returns — no output can race the subscription. The cwd check below is not a trust boundary and
// must not be read as one: the shell it starts is the user's own and can `cd` anywhere on its first
// line. It exists so a stale path fails with a clear E_BAD_REQUEST instead of a confusing exec error.
func (b *BoundService) Open(args OpenArgs) (OpenResult, error) {
	if err := ValidateOpen(args); err != nil {
		return OpenResult{}, err
	}
	// Agent-activity monitoring (the `--settings` flag, the hook env vars) left Kira Studio;
	// Agent stays — it is the keep-awake input, read from this registry's own live-
	// session count, no dependency on the hooks. Kira Space sets ComposeAgent below, so its
	// own agent launches are composed and tracked; Kira Studio leaves it nil, so its Open is
	// unchanged.
	agent := args.LaunchKind == LaunchKindAgent
	if err := b.requireAgent(agent); err != nil {
		return OpenResult{}, err
	}

	var scriptRun string
	var scriptEnv []string
	if args.ScriptID != "" {
		if b.Scripts == nil {
			return OpenResult{}, ipcerr.Contract("scripts are not available")
		}
		launch, err := b.Scripts.Begin(args.ScriptID, args.TerminalID, args.ScriptLaunchToken)
		if err != nil {
			return OpenResult{}, err
		}
		scriptRun = launch.RunID
		args.Cwd, args.Command, scriptEnv = launch.Cwd, launch.Command, launch.Env
		if len(args.Command) > MaxCommandBytes {
			b.Scripts.Failed(scriptRun, errors.New("command is too long"))
			return OpenResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.terminal.commandTooLong.body", nil)
		}
	}

	if args.AgentLaunchToken != "" {
		var err error
		if scriptEnv, err = b.beginAgent(args); err != nil {
			return OpenResult{}, err
		}
	}

	command, env := args.Command, scriptEnv
	composedAgent := agent && b.ComposeAgent != nil
	if composedAgent {
		composed, composedEnv, err := b.ComposeAgent(args.TerminalID, args.Cwd, command)
		if err != nil {
			return OpenResult{}, ipcerr.BadRequest(err.Error())
		}
		// ValidateOpen already checked args.Command alone; composing can grow it (the `--settings`
		// flag, a quoted prompt), so the same bound is rechecked here with the same message.
		if len(composed) > MaxCommandBytes {
			b.abortAgent(args.TerminalID)
			return OpenResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.terminal.commandTooLong.body", nil)
		}
		command, env = composed, append(slices.Clone(scriptEnv), composedEnv...)
	}

	tap, onExit := b.outputTaps(agent, scriptRun != "", args.TerminalID)
	var onEnd func(int, CloseCause)
	if scriptRun != "" {
		id := args.TerminalID
		onEnd = func(code int, cause CloseCause) { b.Scripts.Exited(id, code, cause) }
	}
	sess, err := b.svc().OpenWithCoalescedOutput(OpenParams{
		ID:        args.TerminalID,
		WindowKey: args.WindowKey,
		Cwd:       args.Cwd,
		Cols:      uint16(args.Cols),
		Rows:      uint16(args.Rows),
		Command:   command,
		Env:       env,
		Agent:     agent,
		OnEnd:     onEnd,
	}, args.WindowKey, args.TerminalID, tap, onExit)
	if err != nil {
		if composedAgent {
			b.abortAgent(args.TerminalID)
		}
		if scriptRun != "" {
			b.Scripts.Failed(scriptRun, err)
		}
		if errors.Is(err, ErrDuplicateSession) {
			return OpenResult{}, ipcerr.Contract("terminalId is already open")
		}
		if errors.Is(err, ErrRegistryClosed) {
			return OpenResult{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.edit.terminalWindowClosing.body", nil)
		}
		return OpenResult{}, ipcerr.InternalErr(err)
	}

	if agent && b.Arbiter != nil {
		b.Arbiter.Opened(args.TerminalID, args.Cols, args.Rows)
	}
	return OpenResult{Shell: sess.Shell()}, nil
}

// outputTaps builds the reader-goroutine tap and exit hook for an agent session (arbiter) and a script
// session (run tail); a session can be both.
func (b *BoundService) outputTaps(agent, script bool, id string) (tap func([]byte), onExit func()) {
	if agent && b.Arbiter != nil {
		tap = func(data []byte) { b.Arbiter.Output(id, data) }
		onExit = func() { b.Arbiter.Exited(id) }
	}
	if script {
		agentTap := tap
		tap = func(data []byte) {
			if agentTap != nil {
				agentTap(data)
			}
			b.Scripts.Output(id, data)
		}
	}
	return tap, onExit
}

func (b *BoundService) abortAgent(terminalID string) {
	if b.AbortAgent != nil {
		b.AbortAgent(terminalID)
	}
}

// Write decodes args.Data and forwards it to the pty. A no-op for an id with no live session
// (Registry.Write's own rule) — the renderer can have a keystroke in flight when a shell exits.
func (b *BoundService) Write(args WriteArgs) error {
	if b.Arbiter != nil && !b.Arbiter.AllowWrite(args.TerminalID) {
		return nil
	}
	return b.svc().Write(args.TerminalID, args.Data)
}

// Resize applies cols/rows to the real winsize (pty.Setsize) — a no-op for an id with no live
// session.
func (b *BoundService) Resize(args ResizeArgs) error {
	if b.Arbiter != nil && ValidDim(args.Cols) && ValidDim(args.Rows) && !b.Arbiter.AllowResize(args.TerminalID, args.Cols, args.Rows) {
		return nil
	}
	return b.svc().Resize(args.TerminalID, args.Cols, args.Rows)
}

// Close kills args.TerminalID's own session — idempotent, per Session.Close.
func (b *BoundService) Close(args CloseArgs) error {
	return b.svc().Close(args.TerminalID)
}

func (b *BoundService) requireAgent(agent bool) error {
	if !agent {
		return nil
	}
	_, err := b.Agent.Active().Locate(context.Background(), "")
	return err
}
