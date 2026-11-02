// Package fakeprovider is a second agent provider for flow tests: it proves a provider registered
// next to claude drives every consumer with no caller change. Its structured and headless roles run
// in the test process; its interactive sessions run the fakeagent.Persona binary. The roles it does
// not play (MCP host, sandbox rules, env scrub) are the Claude provider's, embedded. It is neither
// a harness.HookHost nor a harness.UsageReporter.
package fakeprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
)

// ID is the provider's registry id.
const ID = "fake"

// StructuredCall is one recorded structured request.
type StructuredCall struct{ Model, System string }

// HeadlessCall is one recorded headless run.
type HeadlessCall struct {
	Model, Prompt, SessionID, Resume string
	Stage                            harness.StageValues
	// Context is the bytes of the agent context file the run was given; nil when none.
	Context []byte
	// App is the bytes of the app settings file the run was given and AppEnv its env; nil when none.
	App    []byte
	AppEnv []string
}

// base is the Claude provider seen through the roles the fake delegates: it hides Claude's hook and
// usage roles, which the fake does not play.
type base interface {
	harness.Provider
	Fail(kind error) error
	ScrubEnv(environ []string) []string
}

// Provider is the fake. Answers maps a substring of a structured call's system prompt to the output
// it returns; the first match answers.
type Provider struct {
	base
	Answers map[string]json.RawMessage

	mu         sync.Mutex
	structured []StructuredCall
	headless   []HeadlessCall
	renders    int
}

// New returns a fake whose structured calls are answered from answers.
func New(answers map[string]json.RawMessage) *Provider {
	return &Provider{base: claude.New(), Answers: answers}
}

// Structured returns the structured requests so far, in order.
func (p *Provider) Structured() []StructuredCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]StructuredCall(nil), p.structured...)
}

// Headless returns the headless runs so far, in order.
func (p *Provider) Headless() []HeadlessCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]HeadlessCall(nil), p.headless...)
}

// MCPConfigs is how many grant configs this provider rendered.
func (p *Provider) MCPConfigs() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renders
}

// MCPConfig renders as Claude does and counts the call: a run pinned to this provider renders here.
func (p *Provider) MCPConfig(servers []harness.MCPServer) ([]byte, error) {
	p.mu.Lock()
	p.renders++
	p.mu.Unlock()
	return p.base.MCPConfig(servers)
}

func (p *Provider) ID() string            { return ID }
func (p *Provider) Label() string         { return "Fake agent" }
func (p *Provider) Command() string       { return fakeagent.Persona }
func (p *Provider) ProductLabel() string  { return "Fake agent" }
func (p *Provider) HeadlessLabel() string { return fakeagent.Persona + " headless" }

func (p *Provider) ModelLabel(alias string) string { return alias }

// StageSettings: the fake takes none, so a step's values always fall to provider defaults.
func (p *Provider) StageSettings() []harness.StageSetting { return nil }

func (p *Provider) Models() harness.Catalog {
	return harness.Catalog{Aliases: []string{"fast", "deep"}, Default: "fast"}
}

func (p *Provider) Locate(_ context.Context, _ string) (string, error) {
	return exec.LookPath(fakeagent.Persona)
}

func (p *Provider) Available() error {
	_, err := p.Locate(context.Background(), "")
	return err
}

func (p *Provider) RunStructured(_ context.Context, model string, c harness.StructuredCall) (harness.StructuredResult, error) {
	p.mu.Lock()
	p.structured = append(p.structured, StructuredCall{Model: model, System: c.System})
	p.mu.Unlock()
	for system, out := range p.Answers {
		if strings.Contains(c.System, system) {
			return harness.StructuredResult{Output: out}, nil
		}
	}
	return harness.StructuredResult{}, &harness.Error{Kind: harness.ErrOutput, Msg: "fakeprovider: no answer for the system prompt"}
}

// RunHeadless records the spec, prints one line and ends the step through its MCP grant.
func (p *Provider) RunHeadless(_ context.Context, s harness.HeadlessSpec, h harness.Handler) (harness.Exit, error) {
	var ctxBytes []byte
	if s.ContextFile != "" {
		ctxBytes, _ = os.ReadFile(s.ContextFile)
	}
	call := HeadlessCall{Model: s.Model, Prompt: s.Prompt, SessionID: s.SessionID, Resume: s.Resume, Stage: s.Stage, Context: ctxBytes}
	if s.App != nil {
		call.App, _ = os.ReadFile(s.App.Files["config"])
		call.AppEnv = s.App.Env
	}
	p.mu.Lock()
	p.headless = append(p.headless, call)
	p.mu.Unlock()
	if h.OnLine != nil {
		h.OnLine(harness.Line{Stream: harness.StreamStdout, Text: "fake agent at work"})
	}
	args := map[string]any{"status": "done", "summary": "summary-fake"}
	if err := fakeagent.CallTool(s.MCPConfigs, agentmcp.ServerName, "finish_step", args); err != nil {
		return harness.Exit{Code: 1}, fmt.Errorf("fakeprovider: finish_step: %w", err)
	}
	if h.OnResult != nil {
		h.OnResult(harness.Result{End: harness.EndDone})
	}
	return harness.Exit{}, nil
}

func (p *Provider) ResumeLost(string) bool { return false }

func (p *Provider) InteractiveCommand(s harness.InteractiveSpec) harness.Launch {
	flag := "--session"
	if s.Resume {
		flag = "--resume"
	}
	cmd := fakeagent.Persona + " " + flag + " " + s.SessionID
	if s.ContextFile != "" {
		cmd += " " + fakeagent.PersonaContextFlag + " " + s.ContextFile
	}
	launch := harness.Launch{SessionID: s.SessionID}
	if s.App != nil {
		launch.Env = s.App.Env
		if f := s.App.Files["config"]; f != "" {
			cmd += " " + fakeagent.PersonaAppConfigFlag + " " + f
		}
	}
	launch.Command = cmd
	return launch
}

// SettingsSchema: the fake's section holds a model only.
func (p *Provider) SettingsSchema() []byte {
	return []byte(`{"type":"object","additionalProperties":false,"properties":{"model":{"type":"string","maxLength":100},"skip_hooks":{"type":"array","items":{"type":"string"}}}}`)
}

// RenderSettings writes the whole AgentSettings as one JSON file, so a test sees exactly what a
// provider receives.
func (p *Provider) RenderSettings(s harness.AgentSettings, _ harness.LaunchKind) (harness.NativeConfig, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return harness.NativeConfig{}, err
	}
	out := harness.NativeConfig{Files: []harness.NativeFile{{Role: "config", Name: "config.json", Data: b}}}
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Env = append(out.Env, k+"="+s.Env[k])
	}
	return out, nil
}

func (p *Provider) WithPrompt(command, prompt string) string {
	return command + " " + "'" + strings.ReplaceAll(prompt, "'", `'\''`) + "'"
}
