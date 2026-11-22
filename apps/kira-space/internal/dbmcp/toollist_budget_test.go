package dbmcp

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	budgetKube    struct{ KubeReader }
	budgetOps     struct{ OpsControl }
	budgetScripts struct{ ScriptControl }
	budgetAPI     struct{ APIRequester }
	budgetKept    struct{ KeptStore }
	budgetJS      struct{ ScriptEngine }
	budgetOp      struct{ OpRunner }
)

const (
	// toolListTokenBudget caps what a client loads on connect: instructions plus every tool, as the
	// repo estimates tokens (ceil(runes/3)). Lower it with the texts; never raise it to fit.
	toolListTokenBudget = 4000
	toolDescriptionMax  = 300
)

// TestToolListWithinBudget guards the full tool set (every feature on) against growth.
func TestToolListWithinBudget(t *testing.T) {
	s := &Server{cfg: Config{
		Kube: budgetKube{}, Ops: budgetOps{}, Scripts: budgetScripts{}, API: budgetAPI{}, Kept: budgetKept{}, JS: budgetJS{}, ScriptOps: budgetOp{},
		APISettings: func() APISettings { return APISettings{Enabled: true, Scripts: true} },
	}}
	srv := s.buildMCPServer()
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "budget", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	est := func(n int) int { return int(math.Ceil(float64(n) / 3)) }
	total := est(len([]rune(cs.InitializeResult().Instructions)))
	for _, tool := range list.Tools {
		if n := len([]rune(tool.Description)); n > toolDescriptionMax {
			t.Errorf("%s: description is %d chars, over %d", tool.Name, n, toolDescriptionMax)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		total += est(len([]rune(tool.Description))) + est(len(schema)) + est(len(tool.Name))
	}
	if total > toolListTokenBudget {
		t.Errorf("tool list is %d tokens, over the %d budget", total, toolListTokenBudget)
	}
	t.Logf("tool list: %d tools, %d tokens", len(list.Tools), total)
}
