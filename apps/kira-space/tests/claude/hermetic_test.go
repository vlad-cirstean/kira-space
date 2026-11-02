package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initServers returns the MCP server names the last system init line of the tee lists, and the
// text of the last result line.
func initServers(t *testing.T, tee string) (servers []string, result string) {
	t.Helper()
	f, err := os.Open(tee)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var m struct {
			Type       string `json:"type"`
			Subtype    string `json:"subtype"`
			Result     string `json:"result"`
			MCPServers []struct {
				Name string `json:"name"`
			} `json:"mcp_servers"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		switch {
		case m.Type == "system" && m.Subtype == "init":
			servers = nil
			for _, s := range m.MCPServers {
				servers = append(servers, s.Name)
			}
		case m.Type == "result":
			result = m.Result
		}
	}
	return servers, result
}

// TestHermeticSession: a hermetic ADE agent step sees project config and not the user's, and a
// control run with hermetic off sees both, so the hermetic case cannot pass vacuously.
func TestHermeticSession(t *testing.T) {
	const prompt = "Reply with every word in your instructions that starts with USER-MARK or PROJECT-MARK, nothing else."
	for _, hermetic := range []bool{true, false} {
		name := "hermetic"
		if !hermetic {
			name = "control"
		}
		t.Run(name, func(t *testing.T) {
			rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
			userMark := fmt.Sprintf("USER-MARK-%08x", rnd.Uint32())
			projMark := fmt.Sprintf("PROJECT-MARK-%08x", rnd.Uint32())
			f := newAdeFixture(t, map[string]string{
				"CLAUDE.md":                   projMark + "\n",
				".mcp.json":                   `{"mcpServers":{"proj-probe":{"command":"sh","args":["-c","sleep 30"]}}}` + "\n",
				".claude/settings.local.json": `{"enabledMcpjsonServers":["proj-probe"]}` + "\n",
			})
			hookMarker := filepath.Join(f.Home, "user-hook-ran")
			writeFile(t, filepath.Join(f.Home, ".claude", "CLAUDE.md"), userMark+"\n")
			writeFile(t, filepath.Join(f.Home, ".claude", "settings.json"),
				fmt.Sprintf(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"touch %s"}]}]}}`, hookMarker)+"\n")
			f.seedClaudeJSON(t)
			cfg := map[string]any{}
			raw, _ := os.ReadFile(filepath.Join(f.Home, ".claude.json"))
			_ = json.Unmarshal(raw, &cfg)
			cfg["mcpServers"] = map[string]any{"user-probe": map[string]any{"command": "sh", "args": []string{"-c", "sleep 30"}}}
			out, _ := json.Marshal(cfg)
			writeFile(t, filepath.Join(f.Home, ".claude.json"), string(out))
			if !hermetic {
				f.SetHermetic(t, false)
			}
			step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 2m\n    prompt: " + prompt + " Then call finish_step with status done and summary ok.\n"
			f.saveWorkflow(t, "flow", step+manualStepYAML)
			run := f.startRun(t, f.createTask(t, "flow"), "feat/hermetic")
			log := f.runLog(t, run.ID)
			if run.State != "done" {
				t.Fatalf("run = %+v, want done (login works); log:\n%s", run, tail(log, 1500))
			}
			servers, result := initServers(t, f.tee)
			has := func(n string) bool {
				for _, s := range servers {
					if s == n {
						return true
					}
				}
				return false
			}
			_, hookRan := os.Stat(hookMarker)
			seen := strings.Contains(result+log, userMark)
			if hermetic {
				if !has("proj-probe") || !has("kira-ade") || has("user-probe") {
					t.Fatalf("MCP servers = %v, want proj-probe and kira-ade, not user-probe", servers)
				}
				if seen || hookRan == nil {
					t.Fatalf("user config leaked: marker seen %v, hook ran %v; servers %v; result %q; log:\n%s", seen, hookRan == nil, servers, result, tail(log, 2500))
				}
				if !strings.Contains(result+log, projMark) {
					t.Fatalf("project marker %s missing from the reply; log:\n%s", projMark, tail(log, 1500))
				}
				return
			}
			if !has("user-probe") || !seen {
				t.Fatalf("control: servers = %v, user marker seen = %v, want the user config loaded", servers, seen)
			}
		})
	}
}
