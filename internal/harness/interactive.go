package harness

// Launch is how to start a TUI session.
type Launch struct {
	// Command is the shell command line, without its first prompt.
	Command string
	// Env is KEY=value entries the agent process gets; never sent over the bridge.
	Env []string
	// SessionID is the id the agent runs under, "" when only known after launch.
	SessionID string
	// ConfigFile is the Kira-written provider config file a hook listener extends in place; "" = none.
	ConfigFile string
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
	// ContextFile is the absolute path of the app's agent context (Markdown) for this launch; ""
	// = none. Only the provider decides how its CLI gets it.
	ContextFile string
	// App is the app's agent settings rendered for this launch; nil = none. Only the provider maps it.
	App *AppConfig
	// Hermetic loads none of the user's own agent config; only the provider maps it.
	Hermetic bool
	// IsolationDir is a Kira-owned 0700 dir an Isolator prepared for this launch; "" none.
	IsolationDir string
	// Sandbox wraps the agent; nil runs it plain.
	Sandbox *SandboxExec
	// DisallowedTools are denied; a deny beats every allow.
	DisallowedTools []string
}
