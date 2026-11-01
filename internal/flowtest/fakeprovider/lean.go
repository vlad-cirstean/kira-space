package fakeprovider

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
)

// LeanID is the lean provider's registry id.
const LeanID = "fake-lean"

// LeanEnv is the env entry every lean launch carries; the persona records it.
const LeanEnv = fakeagent.EnvPersonaConfig + "=lean-config"

// Lean is a provider with only the roles a minimal agent needs: no structured or headless role, no
// hooks, no usage. Its launches carry env, and its agent assigns its own session id, which
// FindSession reads back. MCP wiring and sandbox rules are Claude's.
type Lean struct {
	harness.MCPHost
	harness.Sandboxed
}

// NewLean returns the lean provider.
func NewLean() *Lean {
	c := claude.New()
	return &Lean{MCPHost: c, Sandboxed: c}
}

func (*Lean) ID() string            { return LeanID }
func (*Lean) Label() string         { return "Lean agent" }
func (*Lean) Command() string       { return fakeagent.Persona }
func (*Lean) ProductLabel() string  { return "Lean agent" }
func (*Lean) HeadlessLabel() string { return fakeagent.Persona + " headless" }

func (*Lean) ModelLabel(alias string) string { return alias }

func (*Lean) StageSettings() []harness.StageSetting { return nil }

func (*Lean) Models() harness.Catalog {
	return harness.Catalog{Aliases: []string{"lean"}, Default: "lean"}
}

func (*Lean) Locate(_ context.Context, _ string) (string, error) {
	return exec.LookPath(fakeagent.Persona)
}

func (p *Lean) Available() error {
	_, err := p.Locate(context.Background(), "")
	return err
}

// InteractiveCommand starts a session with no id up front; a resume names the id it continues.
func (*Lean) InteractiveCommand(s harness.InteractiveSpec) harness.Launch {
	l := harness.Launch{Command: fakeagent.Persona + " " + fakeagent.LeanArg, Env: []string{LeanEnv}}
	if s.Resume {
		l.Command += " --resume " + s.SessionID
		l.SessionID = s.SessionID
	}
	return l
}

func (*Lean) WithPrompt(command, prompt string) string { return command + " " + prompt }

// FindSession returns the newest session the persona recorded in cwd since q.Since.
func (*Lean) FindSession(_ context.Context, q harness.SessionQuery) (string, error) {
	f, err := os.Open(filepath.Join(os.Getenv(fakeagent.EnvDir), fakeagent.LeanSessions))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	var id string
	var newest int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var s fakeagent.LeanSession
		if json.Unmarshal(sc.Bytes(), &s) != nil || s.Cwd != q.Cwd || s.StartedAt < q.Since.UnixMilli() {
			continue
		}
		if s.StartedAt >= newest {
			id, newest = s.ID, s.StartedAt
		}
	}
	return id, sc.Err()
}
