package claude

// The baseline generator. It drives today's real agentmcp.Server through the spike stubs, so it
// stops compiling once Parts 3 to 5 change those tools: delete it with the spike code then. The
// frozen output, testdata/details-bench/tools-baseline.json, is what arm T serves.

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// TestDetailsBenchFreeze writes the baseline from today's real agentmcp.Server and the proof stub.
// Free (no claude). Run once before any tool changes: KIRA_BENCH_FREEZE=<file>.
func TestDetailsBenchFreeze(t *testing.T) {
	out := os.Getenv("KIRA_BENCH_FREEZE")
	if out == "" {
		t.Skip("set KIRA_BENCH_FREEZE to the baseline file to write")
	}
	spike1bOn = true
	defer func() { spike1bOn = false }()
	base := benchBaseline{Roles: map[string]map[string][]*mcp.Tool{}}
	for _, role := range benchRoles {
		r := &spikeRun{t: t, flow: role.flow, arm: "T", lib: loadSpikeLib(t)}
		cfg, _, done := r.setup(t, flowByIDs(spikeFlows1b, role.flow))
		var conf struct {
			MCPServers map[string]struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(mustRead(t, cfg), &conf); err != nil {
			t.Fatal(err)
		}
		base.Roles[role.name] = map[string][]*mcp.Tool{}
		for name, sv := range conf.MCPServers {
			tools, err := listTools(t, sv.URL, sv.Headers)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
			base.Roles[role.name][name] = tools
		}
		done()
	}
	base.FinishAck = llmprompts.HeadlessFinishStepAck.Text(llmprompts.Defaults(), llmprompts.NoVars{})
	raw, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
