package harness

import (
	"context"
	"time"
)

// Headless runs the agent unattended and streams what it does.
type Headless interface {
	// RunHeadless starts the agent, feeds Prompt on stdin and streams parsed output to h until it
	// exits.
	RunHeadless(ctx context.Context, s HeadlessSpec, h Handler) (Exit, error)
	// ScrubEnv drops the variables that tie a child to an agent session running this app.
	ScrubEnv(environ []string) []string
	// ResumeLost reports whether a stderr line says the resumed conversation does not exist.
	ResumeLost(line string) bool
}

// HeadlessSpec is one headless launch.
type HeadlessSpec struct {
	// Bin is the CLI to run (a test points it at a fake); "" means the provider's.
	Bin string
	Dir string
	// Prompt goes to stdin: variadic flags would swallow a positional prompt, and stdin keeps it off argv.
	Prompt string
	// SessionID is the proposed id; a SessionLocator provider reports its own through OnSessionID.
	SessionID string
	// Resume, when set, continues that session instead of starting SessionID; the id stays the same.
	Resume string
	// MCPConfigs are the provider config files naming the MCP servers; the first is Kira's grant.
	MCPConfigs []string
	Config     ConfigScope
	// Isolated loads no settings file, only the MCPConfigs servers, and denies instead of
	// prompting; it overrides Config. With UserMCP the user's own settings source loads too,
	// bounded to the GrantMCP and UserMCP servers, hooks off.
	Isolated bool
	// UserMCP names servers from the user's own agent config that the agent loads itself. Kira never
	// reads their definitions.
	UserMCP []string
	// GrantMCP names the servers in MCPConfigs (Kira's grants).
	GrantMCP []string
	// Model is the model to run; "" omits it.
	Model string
	// MaxBudgetUSD caps the run's cost; 0 omits it.
	MaxBudgetUSD float64
	// Tools are the built-ins that exist for the run; nil omits the restriction, empty allows none.
	Tools []string
	// AllowedTools run without a prompt; the caller adds the finish_step tool.
	AllowedTools []string
	// DisallowedTools are denied; a deny beats every allow.
	DisallowedTools []string
	// AddDirs are extra directories the run may read and edit.
	AddDirs []string
	// Env is the full child environment; nil means os.Environ().
	Env []string
	// Sandbox wraps the agent in a sandbox runner; nil runs it plain.
	Sandbox *SandboxExec
	// Stage is the step's values for this provider; nil = defaults.
	Stage   StageValues
	Timeout time.Duration
	// ContextFile is the absolute path of the app's agent context (Markdown) for this launch; ""
	// = none. Only the provider decides how its CLI gets it.
	ContextFile string
}

// ConfigScope is which of the user's agent config layers a run loads.
type ConfigScope int

const (
	// ConfigNone loads none.
	ConfigNone ConfigScope = iota
	// ConfigUser loads the user's own config only.
	ConfigUser
	// ConfigAll loads the user's, the project's and the local config.
	ConfigAll
)

// DefaultSandboxBin is macOS's profile runner.
const DefaultSandboxBin = "/usr/bin/sandbox-exec"

// SandboxExec is the wrapper put in front of the agent. The login shell sources the user's
// profile first, so the overrides go on an env command that runs after it.
type SandboxExec struct {
	// Bin defaults to DefaultSandboxBin (a test points it at a shim).
	Bin     string
	Profile string
	// Env are K=V overrides; Unset are names removed from the environment.
	Env   []string
	Unset []string
}

// Log streams, matching the ade_log_chunks stream values.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
	StreamEvent  = "event"
)

// Line is one log line.
type Line struct {
	Stream string
	Text   string
}

// Handler receives the parsed output. Its callbacks may run on any goroutine, never concurrently
// with each other.
type Handler struct {
	// OnStart receives the child's pid right after it starts.
	OnStart func(pid int)
	OnLine  func(Line)
	// OnRateLimits receives the account's rate-limit windows.
	OnRateLimits func(RateLimits)
	// OnResult receives the run's closing result.
	OnResult func(Result)
	// OnSessionID receives the id the agent assigned the conversation, once. Only a SessionLocator
	// provider calls it; Claude runs under the proposed id.
	OnSessionID func(id string)
}

// Exit is how the process ended.
type Exit struct {
	Code      int
	TimedOut  bool
	Cancelled bool
}

// RateWindow is one rate-limit window; UsedPercent is 0..100 and ResetsAt Unix seconds.
type RateWindow struct {
	UsedPercent float64
	ResetsAt    int64
}

// RateLimits holds the windows an event carried; either may be nil.
type RateLimits struct {
	FiveHour *RateWindow
	SevenDay *RateWindow
}

// ResultEnd is how the agent says the run ended.
type ResultEnd int

const (
	EndDone ResultEnd = iota
	EndBudget
	EndMaxTurns
	EndError
)

// Result is the run's closing report.
type Result struct {
	End ResultEnd
	// CostUSD is nil when the report carried no cost.
	CostUSD *float64
	Denials []string
	// Text is the final message, cut to a bounded size.
	Text string
}
