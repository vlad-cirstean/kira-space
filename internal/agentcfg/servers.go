package agentcfg

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// agentLabel names the app's settings in a blocked run.
const agentLabel = "Kira agent settings"

// Server list sources.
const (
	SourceApp   = "app"
	SourceAgent = "agent"
)

// ServerSource is where an isolated run's ticked MCP servers come from: the app's settings when
// hermetic, else the agent CLI's own list.
type ServerSource struct {
	Store    *Store      // nil: CLI only
	Hermetic func() bool // nil: true
}

// ServerList is a picker's servers and where they came from.
type ServerList struct {
	Source  string               `json:"source"` // "app" | "agent"
	Servers []harness.UserServer `json:"servers"`
}

// Ticked is what a run passes for its ticked servers.
type Ticked struct {
	MCPConfig string   // <Dir>/<Stem>-app.mcp.json, 0600; "" none
	UserMCP   []string // CLI mode only
	Secrets   []string // for the run's Masker
	Release   func()
}

func (s ServerSource) app() bool {
	return s.Store != nil && (s.Hermetic == nil || s.Hermetic())
}

// List returns every server in the last good settings (a tick is explicit, so `enabled` is
// ignored), or the agent CLI's list for dir.
func (s ServerSource) List(ctx context.Context, p harness.Provider, dir string) (ServerList, error) {
	if s.app() {
		out := ServerList{Source: SourceApp, Servers: []harness.UserServer{}}
		if set, ok := s.Store.Settings(); ok {
			for _, name := range sortedKeys(set.MCPServers) {
				out.Servers = append(out.Servers, serverRow(name, set.MCPServers[name]))
			}
		}
		return out, nil
	}
	out := ServerList{Source: SourceAgent, Servers: []harness.UserServer{}}
	servers, err := p.UserMCPServers(ctx, dir)
	if servers != nil {
		out.Servers = servers
	}
	return out, err
}

// serverRow is a picker row: transport and target only, never args, env or headers. Kira never
// health-checks a server, so Status is "".
func serverRow(name string, m MCPServer) harness.UserServer {
	row := harness.UserServer{Name: name, Transport: "stdio"}
	if m.URL != "" {
		row.Transport = "http"
		if u, err := url.Parse(m.URL); err == nil {
			row.Target = u.Host
		}
		return row
	}
	row.Target = filepath.Base(m.Command)
	return row
}

// Missing is the first ticked name absent from the source ("" none); known false means not
// checked (the CLI list is not ready and wait is false).
func (s ServerSource) Missing(ctx context.Context, p harness.Provider, dir string, names []string, wait bool) (missing string, known bool, err error) {
	if len(names) == 0 {
		return "", true, nil
	}
	if s.app() {
		set, _ := s.Store.Settings()
		for _, n := range names {
			if _, ok := set.MCPServers[n]; !ok {
				return n, true, nil
			}
		}
		return "", true, nil
	}
	have, known, err := p.UserMCPPresence(ctx, dir, names, wait)
	if err != nil || !known {
		return "", false, err
	}
	for _, n := range names {
		if !slices.ContainsFunc(have, func(u harness.UserServer) bool { return u.Name == n }) {
			return n, true, nil
		}
	}
	return "", true, nil
}

// Gone is the error for a ticked name Missing reported, naming where the source looked.
func (s ServerSource) Gone(p harness.Provider, name string) harness.ErrServerGone {
	if s.app() {
		return harness.ErrServerGone{Name: name, Agent: agentLabel}
	}
	return harness.ErrServerGone{Name: name, Agent: p.Label()}
}

// Wire returns what a run passes: an app MCP file (hermetic) or the names for UserMCP (CLI). The
// app file holds the resolved servers; Release removes it.
func (s ServerSource) Wire(ctx context.Context, l Launch, names []string) (Ticked, error) {
	none := Ticked{Release: func() {}}
	if len(names) == 0 {
		return none, nil
	}
	if !s.app() {
		none.UserMCP = names
		return none, nil
	}
	set, _ := s.Store.Settings()
	picked := Settings{MCPServers: map[string]MCPServer{}}
	for _, n := range names {
		m, ok := set.MCPServers[n]
		if !ok {
			return none, s.Gone(l.Provider, n)
		}
		m.Enabled = nil
		picked.MCPServers[n] = m
	}
	resolve, values, err := s.Store.resolver(ctx, set)
	if err != nil {
		return none, err
	}
	as, err := picked.agentSettings(l.Provider.ID(), resolve)
	if err != nil {
		return none, fmt.Errorf("agent settings: %w", err)
	}
	doc, err := l.Provider.MCPConfig(as.MCP)
	if err != nil {
		return none, fmt.Errorf("agent settings: %w", err)
	}
	app, files, err := s.Store.writeFiles(l, harness.NativeConfig{Files: []harness.NativeFile{{Role: "mcp", Name: "mcp.json", Data: doc}}})
	if err != nil {
		return none, err
	}
	s.Store.track(files, true)
	return Ticked{
		MCPConfig: app.Files["mcp"], Secrets: *values,
		Release: func() { files.remove(); s.Store.track(files, false) },
	}, nil
}
