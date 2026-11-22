package claude

// Stubs of the bench flows: the kira-studio proof_save stub and the stub Space backends the frozen arm T
// and the real-server arm R serve over. Nothing here is the production feature.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// ---- shared proof model and validation (both arms) ----

type spikeStep struct {
	ID, Description, View string
	Rows                  [][]any // table
	ImageOK               bool    // image step with a decodable JPEG
	Text                  string  // diagram source or document data (Part 1b)
}

type spikeProof struct {
	Name, Summary, Reason string
	Steps                 []spikeStep
	Pending               bool
}

func decodeTable(raw []byte) ([][]any, error) {
	var t struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("table data is not valid JSON: %w", err)
	}
	if len(t.Columns) == 0 || len(t.Rows) == 0 {
		return nil, errors.New("table data needs columns and rows")
	}
	for i, r := range t.Rows {
		if len(r) != len(t.Columns) {
			return nil, fmt.Errorf("table row %d has %d cells, want %d", i, len(r), len(t.Columns))
		}
	}
	return t.Rows, nil
}

func checkJPEG(raw []byte) error {
	if _, err := jpeg.Decode(bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("image is not a valid JPEG: %w", err)
	}
	return nil
}

var proofViews = []string{"table", "document", "keyvalue", "chart", "diagram", "image"}

// ---- arm T: kira-studio proof_save stub with today's schema ----

// These mirror dbmcp's unexported proofSaveArgs so llmprompts.InputSchema yields today's schema.
type spikeSaveStep struct {
	Description     string         `json:"description,omitempty"`
	Query           string         `json:"query,omitempty"`
	Language        string         `json:"language,omitempty"`
	Source          string         `json:"source,omitempty"`
	View            string         `json:"view,omitempty"`
	Data            map[string]any `json:"data,omitempty"`
	KeepOrdinal     *int           `json:"keepOrdinal,omitempty"`
	ReplacesOrdinal *int           `json:"replacesOrdinal,omitempty"`
}

type spikeSaveArgs struct {
	Name    string          `json:"name"`
	Summary string          `json:"summary,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Steps   []spikeSaveStep `json:"steps"`
}

type spikeListArgs struct {
	Name string `json:"name,omitempty"`
}

// studioStub holds the closed proofs of one arm-T run.
type studioStub struct {
	mu     sync.Mutex
	proofs map[string]*spikeProof // by lower-cased name: current closed revision
	edits  map[string]*spikeProof // pending revision
	srv    *httptest.Server
}

func newStudioStub(seed *spikeProof) *studioStub {
	st := &studioStub{proofs: map[string]*spikeProof{}, edits: map[string]*spikeProof{}}
	if seed != nil {
		st.proofs[strings.ToLower(seed.Name)] = seed
	}
	// The proof tools are gone from the product: arm T serves the frozen texts and schemas.
	var frozen benchBaseline
	raw, err := os.ReadFile(filepath.Join(benchFixtures, "tools-baseline.json"))
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(raw, &frozen); err != nil {
		panic(err)
	}
	mk := func() *mcp.Server {
		srv := mcp.NewServer(&mcp.Implementation{Name: "kira-studio", Version: "1"}, nil)
		for _, tl := range frozen.Roles["run-proof"]["kira-studio"] {
			switch tl.Name {
			case "proof_save":
				srv.AddTool(tl, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					var a spikeSaveArgs
					if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
						return errResult(err.Error()), nil
					}
					res, out, err := st.save(ctx, req, a)
					return studioReply(res, out, err)
				})
			case "proof_list":
				srv.AddTool(tl, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					var a spikeListArgs
					if err := json.Unmarshal(req.Params.Arguments, &a); err != nil {
						return errResult(err.Error()), nil
					}
					res, out, err := st.list(ctx, req, a)
					return studioReply(res, out, err)
				})
			}
		}
		return srv
	}
	srv := mk()
	st.srv = httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true}))
	return st
}

func toolErr(format string, a ...any) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}}}, nil, nil
}

func jsonOK(v any) (*mcp.CallToolResult, any, error) {
	raw, _ := json.Marshal(v)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

func (st *studioStub) save(_ context.Context, req *mcp.CallToolRequest, a spikeSaveArgs) (*mcp.CallToolResult, any, error) {
	var raw struct {
		Steps []struct {
			Data json.RawMessage `json:"data"`
		} `json:"steps"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &raw)
	name := strings.TrimSpace(a.Name)
	if name == "" || len(name) > 80 {
		return toolErr("name must be 1-80 characters.")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	old := st.proofs[strings.ToLower(name)]
	if old != nil && strings.TrimSpace(a.Reason) == "" {
		return toolErr("A closed proof named %q exists. Pass reason to change it.", name)
	}
	if old == nil && a.Reason != "" {
		return toolErr("reason edits a closed proof; no proof named %q exists.", name)
	}
	p := &spikeProof{Name: name, Summary: a.Summary, Reason: a.Reason}
	if old != nil {
		p.Summary = old.Summary
	}
	for i, s := range a.Steps {
		var out spikeStep
		switch {
		case s.KeepOrdinal != nil:
			if old == nil || *s.KeepOrdinal < 1 || *s.KeepOrdinal > len(old.Steps) {
				return toolErr("steps[%d].keepOrdinal %d is not an ordinal of the closed proof.", i, *s.KeepOrdinal)
			}
			out = old.Steps[*s.KeepOrdinal-1]
		default:
			if strings.TrimSpace(s.Description) == "" || !slices.Contains(proofViews, s.View) {
				return toolErr("steps[%d] needs description and a valid view.", i)
			}
			out = spikeStep{Description: s.Description, View: s.View}
			var data json.RawMessage
			if i < len(raw.Steps) {
				data = raw.Steps[i].Data
			}
			switch s.View {
			case "table":
				rows, err := decodeTable(data)
				if err != nil {
					return toolErr("steps[%d].data: %v", i, err)
				}
				out.Rows = rows
			case "image":
				var d struct{ Path, Base64 string }
				_ = json.Unmarshal(data, &d)
				var b []byte
				switch {
				case d.Path != "" && d.Base64 == "":
					if !filepath.IsAbs(d.Path) {
						return toolErr("steps[%d].data.path must be absolute.", i)
					}
					var err error
					if b, err = os.ReadFile(d.Path); err != nil {
						return toolErr("steps[%d].data.path: %v", i, err)
					}
				case d.Base64 != "" && d.Path == "":
					var err error
					if b, err = base64.StdEncoding.DecodeString(d.Base64); err != nil {
						return toolErr("steps[%d].data.base64 is not base64.", i)
					}
				default:
					return toolErr("steps[%d].data: exactly one of path or base64.", i)
				}
				if err := checkJPEG(b); err != nil {
					return toolErr("steps[%d].data: %v", i, err)
				}
				out.ImageOK = true
			default:
				if len(data) == 0 {
					return toolErr("steps[%d].data is required.", i)
				}
				out.Text = string(data)
			}
		}
		p.Steps = append(p.Steps, out)
	}
	if old == nil {
		st.proofs[strings.ToLower(name)] = p
		return jsonOK(map[string]any{"name": name, "status": "closed", "steps": len(p.Steps)})
	}
	p.Pending = true
	st.edits[strings.ToLower(name)] = p
	return jsonOK(map[string]any{"name": name, "status": "pending_review", "steps": len(p.Steps)})
}

func (st *studioStub) list(_ context.Context, _ *mcp.CallToolRequest, a spikeListArgs) (*mcp.CallToolResult, any, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if a.Name != "" {
		p := st.proofs[strings.ToLower(strings.TrimSpace(a.Name))]
		if p == nil {
			return toolErr("no proof named %q.", a.Name)
		}
		type brief struct {
			Ordinal     int    `json:"ordinal"`
			Description string `json:"description"`
			View        string `json:"view"`
		}
		var out []brief
		for i, s := range p.Steps {
			out = append(out, brief{i + 1, s.Description, s.View})
		}
		return jsonOK(map[string]any{"name": p.Name, "steps": out})
	}
	var out []map[string]any
	for _, p := range st.proofs {
		_, pending := st.edits[strings.ToLower(p.Name)]
		out = append(out, map[string]any{"name": p.Name, "steps": len(p.Steps), "pendingReview": pending})
	}
	return jsonOK(out)
}

func (st *studioStub) close() { st.srv.Close() }

// ---- arm T: stub backends of the real agentmcp.Server ----

type spikeBranch struct {
	Repo, Branch, Worktree string
	ReadyAt                time.Time
}

type spikeSpace struct {
	agentmcp.SpaceTools // unused methods panic: the bench flows never call them
	br                  spikeBranch
	children            []frozenChildTaskInfo
	chain               map[string][]frozenChainRow
	mu                  sync.Mutex
	spawned             []agentmcp.CreateTaskArgs
}

func (s *spikeSpace) info(wait time.Duration) agentmcp.BranchInfo {
	if wait > 0 {
		if d := time.Until(s.br.ReadyAt); d > 0 {
			time.Sleep(min(d, wait))
		}
	}
	state, note := "ready", "setup done"
	if time.Now().Before(s.br.ReadyAt) {
		state, note = "running", "installing dependencies"
	}
	return agentmcp.BranchInfo{Repo: s.br.Repo, Branch: s.br.Branch, Worktree: s.br.Worktree, Setup: agentmcp.SetupInfo{State: state, Note: note}, Next: ""}
}

func (s *spikeSpace) BranchStatus(_ context.Context, _, _, _ string, wait time.Duration) (agentmcp.BranchInfo, error) {
	return s.info(wait), nil
}

func (s *spikeSpace) TaskInfo(context.Context, string, bool) (agentmcp.TaskInfo, error) {
	b := s.br.Branch
	bi := s.info(0)
	return agentmcp.TaskInfo{Title: "Orders ledger", Workflow: "default", Step: "implement",
		Repos: []agentmcp.TaskRepo{{Repo: s.br.Repo, CodeRepoID: "r-1", Branch: &b, Worktree: s.br.Worktree, Setup: bi.Setup}}}, nil
}

func (s *spikeSpace) ListTasks(context.Context, agentmcp.SpaceCall, frozenListTasksArgs) (frozenListTasksResult, error) {
	return frozenListTasksResult{Tasks: s.children}, nil
}

func (s *spikeSpace) TaskChain(_ context.Context, _ agentmcp.SpaceCall, a frozenTaskChainArgs) (frozenTaskChainResult, error) {
	return frozenTaskChainResult{Rows: s.chain[a.TaskID]}, nil
}

// SpawnTask approves at once and records the call; the reply carries the child id like today's tool.
func (s *spikeSpace) SpawnTask(_ context.Context, _ agentmcp.SpaceCall, a agentmcp.CreateTaskArgs) (agentmcp.CreateTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawned = append(s.spawned, a)
	return agentmcp.CreateTaskResult{Outcome: agentmcp.TaskCreated, TaskID: fmt.Sprintf("c-%d", len(s.spawned)), Title: a.Title,
		Next: "The child starts at its first stage with your brief as its first handover. Read its progress with task_chain."}, nil
}

func (s *spikeSpace) RunOutcomes(context.Context, string, frozenOutcomeQuery) ([]frozenRunOutcomeEntry, error) {
	return nil, nil
}

// ListArtifacts feeds finish_step's reply: the bench task has none.
func (s *spikeSpace) ListArtifacts(context.Context, string) (agentmcp.TaskArtifacts, error) {
	return agentmcp.TaskArtifacts{}, nil
}
