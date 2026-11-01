package harness

// Launch is how to start a TUI session.
type Launch struct {
	// Command is the shell command line, without its first prompt.
	Command string
	// Env is KEY=value entries the agent process gets; never sent over the bridge.
	Env []string
	// SessionID is the id the agent runs under, "" when only known after launch.
	SessionID string
}

// Interactive builds the launch of a TUI session.
type Interactive interface {
	InteractiveCommand(s InteractiveSpec) Launch
	// WithPrompt appends the first prompt after any trailing variadic flag.
	WithPrompt(command, prompt string) string
}

// InteractiveSpec describes one TUI launch. Bin defaults to the provider's Command.
type InteractiveSpec struct {
	Bin string
	// SessionID is the proposed id of a new session and the required id of a resume.
	SessionID string
	// Resume continues SessionID instead of starting it.
	Resume  bool
	AddDirs []string
	// MCPConfig is the path of a file holding MCPHost.MCPConfig's document; the provider passes it
	// as a flag or env.
	MCPConfig    string
	AllowedTools []string
	// Stage is the step's values for this provider; nil = defaults.
	Stage StageValues
}
