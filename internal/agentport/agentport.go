// Package agentport moves a provider's native user-level agent config into the app's shared settings
// and context files (Import) and renders them back into the provider's files (Export). It is the one
// package allowed to open ~/.claude.json, ~/.claude/settings.json and opencode.json, only inside an
// Import or Export call a user clicked, decoding only the keys it maps. A literal credential goes
// straight into the sealed secret store and settings.yaml keeps a reference; export writes env
// references only. docs/ARCHITECTURE.md "CLI credentials: never touched" states the exception.
package agentport

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// Provider ids with a porter.
const (
	providerClaude   = "claude"
	providerOpenCode = "opencode"
)

// File roles.
const (
	roleSettings = "settings"
	roleContext  = "context"
	roleMCP      = "mcp"
	roleConfig   = "config"
)

// Size caps of what Import reads.
const (
	maxSettingsFile = 4 << 20
	maxClaudeJSON   = 64 << 20
	maxContextFile  = 4 << 20
)

// Options builds a Service.
type Options struct {
	Settings  *agentcfg.Store
	Context   *agentctx.Store
	Secrets   *agentcfg.SecretStore
	Providers *harness.Registry
	// Env reads CLAUDE_CONFIG_DIR and XDG_CONFIG_HOME; nil means the login shell, then the process
	// environment.
	Env func(string) (string, bool)
	// Home is the user's home directory; "" means os.UserHomeDir.
	Home string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Service is the bound surface: preview and apply of an import or an export per provider.
type Service struct {
	o        Options
	sessions *sessions
}

// New builds a Service.
func New(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o, sessions: newSessions(o.Now)}
}

// Close drops every preview.
func (s *Service) Close() error {
	s.sessions.clear()
	return nil
}

func (s *Service) home() string {
	if s.o.Home != "" {
		return s.o.Home
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// env reads names from the login shell first, then the process environment.
func (s *Service) env(ctx context.Context, names ...string) map[string]string {
	out := map[string]string{}
	if s.o.Env != nil {
		for _, n := range names {
			if v, ok := s.o.Env(n); ok {
				out[n] = v
			}
		}
		return out
	}
	got, _ := loginshell.ProbeEnv(ctx, names)
	for _, n := range names {
		if v, ok := got[n]; ok {
			out[n] = v
		} else if v, ok := os.LookupEnv(n); ok {
			out[n] = v
		}
	}
	return out
}

// target is one native file a provider imports from or exports to.
type target struct {
	Role string
	Path string
}

// claudeTargets are Claude's user-level files.
func (s *Service) claudeTargets(ctx context.Context) (files []target, configDir string) {
	configDir = s.env(ctx, "CLAUDE_CONFIG_DIR")["CLAUDE_CONFIG_DIR"]
	dir := configDir
	jsonPath := filepath.Join(s.home(), ".claude.json")
	if dir != "" {
		jsonPath = filepath.Join(dir, ".claude.json")
	} else {
		dir = filepath.Join(s.home(), ".claude")
	}
	return []target{
		{roleSettings, filepath.Join(dir, "settings.json")},
		{roleContext, filepath.Join(dir, "CLAUDE.md")},
		{roleMCP, jsonPath},
	}, configDir
}

// openCodeConfigNames are the user config file names OpenCode loads, winner first (Q-P454-OC1,
// verified on 1.19.5: with opencode.json and opencode.jsonc both present opencode.jsonc wins).
var openCodeConfigNames = []string{"opencode.jsonc", "opencode.json", "config.json"}

// openCodeTargets are OpenCode's user-level files. The config file is the winning existing one, else a
// new opencode.json.
func (s *Service) openCodeTargets(ctx context.Context) []target {
	base := s.env(ctx, "XDG_CONFIG_HOME")["XDG_CONFIG_HOME"]
	if base == "" {
		base = filepath.Join(s.home(), ".config")
	}
	dir := filepath.Join(base, "opencode")
	cfg := filepath.Join(dir, "opencode.json")
	for _, n := range openCodeConfigNames {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			cfg = filepath.Join(dir, n)
			break
		}
	}
	return []target{{roleConfig, cfg}, {roleContext, filepath.Join(dir, "AGENTS.md")}}
}

func (s *Service) targets(ctx context.Context, provider string) ([]target, string, bool) {
	switch provider {
	case providerClaude:
		t, dir := s.claudeTargets(ctx)
		return t, dir, true
	case providerOpenCode:
		return s.openCodeTargets(ctx), "", true
	}
	return nil, "", false
}

// display is path with the home directory as ~.
func (s *Service) display(path string) string {
	if h := s.home(); h != "" {
		if rel, err := filepath.Rel(h, path); err == nil && rel != ".." && !filepath.IsAbs(rel) && len(rel) < len(path) && rel[:min(2, len(rel))] != ".." {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return path
}

// neutralEvent is the settings.yaml event name of a provider event kind.
func neutralEvent(k harness.EventKind) (string, bool) { return agentcfg.NeutralEvent(k) }

// PortFile is one native file of a provider.
type PortFile struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// PortProvider is a provider with an Import and an Export.
type PortProvider struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	Files []PortFile `json:"files"`
}

// List names the registered providers with a porter and where their native files are.
func (s *Service) List(ctx context.Context) []PortProvider {
	out := []PortProvider{}
	for _, id := range []string{providerClaude, providerOpenCode} {
		if !s.has(id) {
			continue
		}
		p, _ := s.o.Providers.Get(id)
		files, _, _ := s.targets(ctx, id)
		pp := PortProvider{ID: id, Label: p.Label(), Files: []PortFile{}}
		for _, t := range files {
			_, err := os.Stat(t.Path)
			pp.Files = append(pp.Files, PortFile{Role: t.Role, Path: s.display(t.Path), Exists: err == nil})
		}
		out = append(out, pp)
	}
	return out
}
