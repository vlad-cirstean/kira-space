package harness

import (
	"context"
	"encoding/json"
)

// MCPHost renders MCP wiring in the provider's formats and lists the user's own MCP servers.
type MCPHost interface {
	// MCPConfig is the provider's config document for servers.
	MCPConfig(servers []MCPServer) ([]byte, error)
	// MCPToolName is one MCP tool's name in tool permission lists.
	MCPToolName(t MCPTool) string
	// ToolArgs renders a smart script's tool policy as built-ins and allowed tools.
	ToolArgs(p ToolPolicy) (tools, allowed []string)
	// UserMCPServers lists the MCP servers the agent's own CLI reports for dir, without secrets.
	UserMCPServers(ctx context.Context, dir string) ([]UserServer, error)
	// UserMCPPresence is UserMCPServers for a presence check of the want names: reused for longer
	// while it has them all, and with wait false it never blocks (known is false then).
	UserMCPPresence(ctx context.Context, dir string, want []string, wait bool) (servers []UserServer, known bool, err error)
	// MCPInstaller registers MCP servers with the agent's own CLI.
	MCPInstaller() MCPInstaller
}

// MCPServer is an http (URL, Headers) or stdio (Command, Args, Env) server.
type MCPServer struct {
	Name    string
	URL     string
	Headers map[string]string
	Command string
	Args    []string
	Env     map[string]string
}

// MCPTool names one tool of an MCP server.
type MCPTool struct{ Server, Tool string }

// ToolPolicy is a smart script's tools in Kira's tool vocabulary (Claude's built-in names).
type ToolPolicy struct {
	Builtins     []string
	BashPatterns []string
	MCP          []MCPTools // user servers and their ticked tools
	Extra        []string   // already-rendered names appended last (Kira tools)
}

// MCPTools is the ticked tools of one user server.
type MCPTools struct {
	Server string
	// AllTools grants every tool of the server; Tools is then empty.
	AllTools bool
	Tools    []string
}

// ToolNames renders ts in the provider's permission-list names.
func ToolNames(p MCPHost, ts ...MCPTool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, p.MCPToolName(t))
	}
	return out
}

// UserServer is one MCP server from the user's own agent config, described without its secrets.
type UserServer struct {
	Name string `json:"name"`
	// Transport is stdio, http or sse.
	Transport string `json:"transport"`
	// Target is the command's base name or the URL host. Never args, env or headers.
	Target string `json:"target"`
	// Status is the agent's own health report: connected, failed, needsAuth, pending or unknown.
	Status string `json:"status"`
}

// ErrServerGone means a named server is not in the agent's MCP list. Agent is the provider's
// display label.
type ErrServerGone struct{ Name, Agent string }

func (e ErrServerGone) Error() string {
	return "MCP server " + e.Name + " is no longer in your " + e.Agent + " config"
}

// MCPInstaller registers MCP servers with the agent's own CLI. Install never returns a Go error:
// an attempt is a value the pane renders.
type MCPInstaller interface {
	Status() InstallStatus
	Install(ctx context.Context, name, url, helperPath string) InstallResult
	InstallStdio(ctx context.Context, name, command string, args []string) InstallResult
	// Command and StdioCommand render the copy-paste text of the install, never executed.
	Command(name, url, helperPath string) string
	StdioCommand(name, command string, args []string) string
}

// JSONInstaller is an MCPInstaller that registers a server from the CLI's own JSON document,
// replacing a registration of the same name. extraEnv is added to the CLI's environment.
type JSONInstaller interface {
	InstallJSON(ctx context.Context, name string, server json.RawMessage, extraEnv []string) InstallResult
}

// InstallStatus is the pane's pre-click read; Install re-resolves everything itself.
type InstallStatus struct {
	// Path is "" when the agent CLI was not found at any probed location.
	Path string
	// Probed lists every path considered, in probe order.
	Probed []string
}

// InstallResult is Install's outcome, one of the Outcome* constants.
type InstallResult struct {
	Outcome string
	// Detail is a bounded single-line reason on OutcomeInstallFailed, never a token.
	Detail string
	Probed []string
}

const (
	// OutcomeInstalled: the CLI was found and registered the server.
	OutcomeInstalled = "installed"
	// OutcomeNotFound: no CLI at any probed location. Not a failure: the command is on screen to copy.
	OutcomeNotFound = "notFound"
	// OutcomeInstallFailed: the CLI was found but exited non-zero or could not be spawned.
	OutcomeInstallFailed = "installFailed"
)

// ActiveInstaller is an MCPInstaller that resolves the active provider's on every call, so a
// provider switch applies to the next click.
func ActiveInstaller(sel Selector) MCPInstaller { return activeInstaller{sel} }

type activeInstaller struct{ sel Selector }

func (a activeInstaller) inst() MCPInstaller { return a.sel.Active().MCPInstaller() }

func (a activeInstaller) Status() InstallStatus { return a.inst().Status() }

func (a activeInstaller) Install(ctx context.Context, name, url, helperPath string) InstallResult {
	return a.inst().Install(ctx, name, url, helperPath)
}

func (a activeInstaller) InstallStdio(ctx context.Context, name, command string, args []string) InstallResult {
	return a.inst().InstallStdio(ctx, name, command, args)
}

func (a activeInstaller) Command(name, url, helperPath string) string {
	return a.inst().Command(name, url, helperPath)
}

func (a activeInstaller) StdioCommand(name, command string, args []string) string {
	return a.inst().StdioCommand(name, command, args)
}
