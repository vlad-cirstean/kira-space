package harness

import "encoding/json"

// SettingsHost is a provider that takes the app's agent settings. Optional role.
type SettingsHost interface {
	// SettingsSchema is the JSON Schema (2020-12) of the provider's section in settings.yaml. It must
	// be self-contained: no $ref or $defs, it is embedded under the provider's id.
	SettingsSchema() []byte
	// RenderSettings renders s natively for one launch kind. It writes nothing.
	RenderSettings(s AgentSettings, kind LaunchKind) (NativeConfig, error)
	// KiraAdds lists, as display text, what Kira adds to every launch besides the settings.
	KiraAdds() []string
}

// LaunchKind is what a launch is for.
type LaunchKind int

const (
	// LaunchInteractive is a TUI session.
	LaunchInteractive LaunchKind = iota
	// LaunchHeadless is an unattended run.
	LaunchHeadless
)

// AgentSettings is the app's settings for one launch of one provider.
type AgentSettings struct {
	// Env values are resolved, or verbatim references for the settings view.
	Env map[string]string
	// MCP are the enabled servers.
	MCP []MCPServer
	// Allow and Deny are tool permissions in Kira's tool vocabulary.
	Allow, Deny []string
	// Hooks are the common hooks minus the provider's skipped ones.
	Hooks []Hook
	// Section is the provider's own section, references resolved the same way as the rest.
	Section json.RawMessage
}

// Hook is one user hook command. Kira's own hooks are never in this list.
type Hook struct {
	Name, Matcher, Command string
	Event                  EventKind
	TimeoutSec             int
}

// NativeConfig is what a provider receives for one launch, before Kira writes any file.
type NativeConfig struct {
	Files []NativeFile
	// Env is KEY=value; it may carry secrets.
	Env []string
	// Notes are settings that are not applied, for the settings view.
	Notes []string
}

// NativeFile is one provider document. Role is the provider's own key ("mcp", "settings"). Inline
// means the provider takes the bytes through the environment, so Kira writes no file for it.
type NativeFile struct {
	Role, Name string
	Data       []byte
	Inline     bool
}

// AppConfig is NativeConfig after Kira wrote its files (0600). Env and Inline may carry secrets: never
// return them from a bound method, never log them.
type AppConfig struct {
	// Files maps a role to its absolute path.
	Files map[string]string
	// Inline maps a role to its bytes.
	Inline map[string][]byte
	Env    []string
}

// SettingsHostOf is p's SettingsHost role, if it has one.
func SettingsHostOf(p Provider) (SettingsHost, bool) {
	h, ok := p.(SettingsHost)
	return h, ok
}
