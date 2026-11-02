package fakeagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// OpenCode is the name the fake plays the opencode CLI under.
const OpenCode = "opencode"

// OpenCodeSessions is the file in the fake dir holding the sessions the fake created, as a JSON
// array in the shape of `opencode session list --format json`.
const OpenCodeSessions = "opencode-sessions.json"

// OpenCodeMCPFixture is the file under $XDG_CONFIG_HOME/opencode that the fake's `mcp list` prints
// from: a JSON object of server name to {"url": ...} or {"command": ...}. The fake plays the CLI, so
// it may read it; Kira code never does.
const OpenCodeMCPFixture = "kira-fixture-mcp.json"

type openCodeSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Updated   int64  `json:"updated"`
	Created   int64  `json:"created"`
	ProjectID string `json:"projectId"`
	Directory string `json:"directory"`
}

// runOpenCode plays opencode: `session list`, `mcp list`, `run --format json` and the TUI.
func runOpenCode(scen Scenario, dir string, args []string) int {
	if len(args) >= 2 && args[0] == "session" && args[1] == "list" {
		return openCodeSessionList(dir, args)
	}
	if len(args) >= 2 && args[0] == "mcp" && args[1] == "list" {
		return openCodeMCPList(dir, args)
	}
	cwd, _ := os.Getwd()
	n := nextCount(dir, OpenCode)
	stem := filepath.Join(dir, fmt.Sprintf("%s-%d", OpenCode, n))
	writeFile(stem+".cwd", []byte(cwd))
	recordOpenCodeEnv(stem)
	recordOpenCodeContext(stem)
	recordAppEnv(stem)
	if cfg := os.Getenv("OPENCODE_CONFIG"); cfg != "" {
		if raw, err := os.ReadFile(cfg); err == nil {
			writeFile(stem+".config", raw)
		}
	}
	writeFile(stem+".args", []byte(strings.Join(args, "\n"))) // marker: the call is fully recorded
	if len(args) > 0 && args[0] == "run" {
		return openCodeRun(scen, dir, stem, cwd, args)
	}
	resume := argAfter(args, "-s")
	if resume == "" {
		addOpenCodeSession(dir, cwd)
	}
	f, err := os.OpenFile(stem+".prompt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		return 1
	}
	defer f.Close()
	_, _ = io.Copy(f, os.Stdin)
	return 0
}

// recordOpenCodeEnv keeps the OPENCODE_*, KIRA_* and XDG_CONFIG_HOME variables the call started
// with, one Go-quoted entry per line, as <stem>.env.
func recordOpenCodeEnv(stem string) {
	var lines []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "KIRA_FAKE_") {
			continue
		}
		if strings.HasPrefix(kv, "OPENCODE") || strings.HasPrefix(kv, "KIRA_") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") {
			lines = append(lines, strconv.Quote(kv))
		}
	}
	sort.Strings(lines)
	writeFile(stem+".env", []byte(strings.Join(lines, "\n")))
}

// recordOpenCodeContext keeps the bytes of the file named by `instructions` in
// OPENCODE_CONFIG_CONTENT as <stem>.context and its `mcp` block as <stem>.appmcp. The fake plays the CLI, so it may read it.
func recordOpenCodeContext(stem string) {
	var doc struct {
		Instructions []string        `json:"instructions"`
		MCP          json.RawMessage `json:"mcp"`
	}
	if json.Unmarshal([]byte(os.Getenv("OPENCODE_CONFIG_CONTENT")), &doc) != nil {
		return
	}
	if len(doc.MCP) > 0 {
		writeFile(stem+".appmcp", doc.MCP)
	}
	for _, path := range doc.Instructions {
		if raw, err := os.ReadFile(path); err == nil {
			writeFile(stem+".context", raw)
			return
		}
	}
}

func readOpenCodeSessions(dir string) []openCodeSession {
	var out []openCodeSession
	if raw, err := os.ReadFile(filepath.Join(dir, OpenCodeSessions)); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// addOpenCodeSession records a new session for cwd and returns its id.
func addOpenCodeSession(dir, cwd string) string {
	all := readOpenCodeSessions(dir)
	now := time.Now().UnixMilli()
	s := openCodeSession{
		ID: fmt.Sprintf("ses_fake%02d", len(all)+1), Title: "New session", Updated: now, Created: now,
		ProjectID: "global", Directory: cwd,
	}
	raw, _ := json.Marshal(append(all, s))
	writeFile(filepath.Join(dir, OpenCodeSessions), raw)
	return s.ID
}

func openCodeSessionList(dir string, args []string) int {
	n := nextCount(dir, "opencode-session-list")
	writeFile(filepath.Join(dir, fmt.Sprintf("opencode-session-list-%d.args", n)), []byte(strings.Join(args, "\n")))
	cwd, _ := os.Getwd()
	out := []openCodeSession{}
	for _, s := range readOpenCodeSessions(dir) {
		if sameDir(s.Directory, cwd) {
			out = append(out, s)
		}
	}
	raw, _ := json.Marshal(out)
	fmt.Println(string(raw))
	return 0
}

// openCodeMCPList prints the fixture's servers in `opencode mcp list` shape.
func openCodeMCPList(dir string, args []string) int {
	n := nextCount(dir, "opencode-mcp-list")
	writeFile(filepath.Join(dir, fmt.Sprintf("opencode-mcp-list-%d.args", n)), []byte(strings.Join(args, "\n")))
	cfgHome := os.Getenv("XDG_CONFIG_HOME")
	if cfgHome == "" {
		cfgHome = filepath.Join(os.Getenv("HOME"), ".config")
	}
	var servers map[string]struct {
		URL     string `json:"url"`
		Command string `json:"command"`
	}
	if raw, err := os.ReadFile(filepath.Join(cfgHome, "opencode", OpenCodeMCPFixture)); err == nil {
		_ = json.Unmarshal(raw, &servers)
	}
	fmt.Println("┌  MCP Servers")
	fmt.Println("│")
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := servers[name]
		fmt.Printf("•  ✓ %s connected\n", name)
		fmt.Printf("|      %s\n", cmpStr(s.URL, s.Command))
		fmt.Println("│")
	}
	fmt.Printf("└  %d server(s)\n", len(servers))
	return 0
}

// openCodeGrant is the OpenCode-shaped MCP config of the call, as a Claude-shaped inline document
// callTool reads.
func openCodeGrant() string {
	raw, err := os.ReadFile(os.Getenv("OPENCODE_CONFIG"))
	if err != nil {
		return "{}"
	}
	var doc struct {
		MCP map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcp"`
	}
	_ = json.Unmarshal(raw, &doc)
	servers := map[string]serverEntry{}
	for name, e := range doc.MCP {
		servers[name] = serverEntry{Type: "http", URL: e.URL, Headers: e.Headers}
	}
	out, _ := json.Marshal(map[string]any{"mcpServers": servers})
	return string(out)
}

func openCodeEvent(typ, session string, extra map[string]any) {
	ev := map[string]any{"type": typ, "timestamp": time.Now().UnixMilli()}
	if session != "" {
		ev["sessionID"] = session
	}
	for k, v := range extra {
		ev[k] = v
	}
	emitJSON(ev)
}

// openCodeRun plays `run --format json`: the prompt arrives on stdin and the scripted action
// (the scenario's OpenCode map, keyed like Claude's) ends the run. "lost" is the resume of an
// unknown id; "done" and the other finish names call finish_step through the config's grant.
func openCodeRun(scen Scenario, dir, stem, cwd string, args []string) int {
	prompt, _ := io.ReadAll(os.Stdin)
	writeFile(stem+".prompt", prompt)
	repo := filepath.Base(filepath.Dir(cwd))
	actions := scen.OpenCode[repo]
	if len(actions) == 0 {
		actions = scen.OpenCode["*"]
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(stem), OpenCode+"-"))
	action := Action{Name: "done"}
	if len(actions) > 0 {
		action = actions[min(n, len(actions))-1]
	}
	resume := argAfter(args, "-s")
	if action.Name == "lost" {
		openCodeEvent("error", "", map[string]any{"error": map[string]any{
			"name": "NotFoundError", "data": map[string]any{"message": "Session not found: " + resume},
		}})
		return 1
	}
	id := resume
	if id == "" {
		id = addOpenCodeSession(dir, cwd)
	}
	openCodeEvent("step_start", id, nil)
	if code, done := runSideEffects(action, dir, cwd, []string{"--mcp-config", openCodeGrant()}); done {
		return code
	}
	finish := func(status string) int {
		_, err := callTool([]string{openCodeGrant()}, nil, agentmcp.ServerName, "finish_step", map[string]any{"status": status, "summary": "summary-" + status, "reason": "summary-" + status})
		if err != nil {
			fmt.Fprintln(os.Stderr, "finish_step:", err)
			return 3
		}
		openCodeEvent("text", id, map[string]any{"part": map[string]any{"text": "finished: " + status}})
		openCodeEvent("step_finish", id, nil)
		return 0
	}
	if rid, ok := strings.CutPrefix(action.Name, "result:"); ok {
		return finish(rid)
	}
	switch action.Name {
	case "done", "needs_input", "failed":
		return finish(action.Name)
	case "fail":
		fmt.Fprintln(os.Stderr, cmpStr(action.Stderr, "fake opencode: scripted failure"))
		return 1
	case "sleep":
		time.Sleep(time.Hour)
	}
	openCodeEvent("step_finish", id, nil)
	return 0
}

func sameDir(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return a == b
}
