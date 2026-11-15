package claude

// P643 Part 1 spike stubs: the two MCP surfaces the spike compares. Arm T is today's tools (the real
// agentmcp.Server over stub backends, plus a kira-studio proof_save stub with today's schema). Arm F
// is the planned surface (submit, finish_step without summary, merged branches and lease,
// cancel_operation) plus a task folder. Nothing here is the production feature.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
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

var mermaidKinds = []string{"flowchart", "graph", "sequenceDiagram", "classDiagram", "stateDiagram", "erDiagram", "gantt", "journey", "gitGraph", "mindmap", "timeline"}

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
	agentmcp.SpaceTools // unused methods panic: the spike flows never call them
	br                  spikeBranch
	children            []agentmcp.ChildTaskInfo
	chain               map[string][]agentmcp.ChainRow
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

func (s *spikeSpace) ListTasks(context.Context, agentmcp.SpaceCall, agentmcp.ListTasksArgs) (agentmcp.ListTasksResult, error) {
	return agentmcp.ListTasksResult{Tasks: s.children}, nil
}

func (s *spikeSpace) TaskChain(_ context.Context, _ agentmcp.SpaceCall, a agentmcp.TaskChainArgs) (agentmcp.TaskChainResult, error) {
	return agentmcp.TaskChainResult{Rows: s.chain[a.TaskID]}, nil
}

// SpawnTask approves at once and records the call; the reply carries the child id like today's tool.
func (s *spikeSpace) SpawnTask(_ context.Context, _ agentmcp.SpaceCall, a agentmcp.CreateTaskArgs) (agentmcp.CreateTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawned = append(s.spawned, a)
	return agentmcp.CreateTaskResult{Outcome: agentmcp.TaskCreated, TaskID: fmt.Sprintf("c-%d", len(s.spawned)), Title: a.Title,
		Next: "The child starts at its first stage with your brief as its first handover. Read its progress with task_chain."}, nil
}

func (s *spikeSpace) RunOutcomes(context.Context, string, agentmcp.OutcomeQuery) ([]agentmcp.RunOutcomeEntry, error) {
	return nil, nil
}

// ---- arm F: planned tools plus a task folder ----

type filesArm struct {
	root   string // task folder
	step   string // current step id
	mu     sync.Mutex
	seen   map[string]string // rel path -> sha last classified accepted or pending
	base   map[string]string // rel path -> sha of the accepted copy
	pend   map[string]bool
	ready  time.Time
	wtPath string
	finish []string
	kids   map[string]spikeChild // by folder slug, created through submit
	srv    *httptest.Server
	cons   bool   // consolidated layout: handovers.md and notes.md instead of one file each (Part 1b)
	handed string // cons: the handed sections of handovers.md as seeded
}

type spikeChild struct {
	ID, Title, Workflow, Brief string
	Repos                      []string
}

type submitArgs struct {
	Paths    []string `json:"paths,omitempty" jsonschema:"Files to submit, relative to the task folder. Omit to scan the whole folder."`
	Finished bool     `json:"finished,omitempty" jsonschema:"A session without a run: the stage is done, offer the move to the next stage."`
}

type fileResult struct {
	Path    string `json:"path"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

type finishArgs struct {
	Status          string   `json:"status" jsonschema:"How the step ended: done, failed or needs_input."`
	Reason          string   `json:"reason,omitempty" jsonschema:"Why it failed, or what you need. Required when status is not done."`
	ConflictedFiles []string `json:"conflictedFiles,omitempty" jsonschema:"Files that still conflict, relative to the repo."`
	LastGitError    string   `json:"lastGitError,omitempty" jsonschema:"Last git error output you saw."`
	Tried           string   `json:"tried,omitempty" jsonschema:"What you tried before giving up."`
}

type branchesArgs struct {
	Action      string   `json:"action" jsonschema:"declare, request, rename, change or wait."`
	Repo        string   `json:"repo,omitempty" jsonschema:"Task repo: nickname, name or id. For rename, change and wait."`
	Repos       []string `json:"repos,omitempty" jsonschema:"declare: repos the work touches."`
	Branch      string   `json:"branch,omitempty" jsonschema:"wait: the branch to wait for."`
	Branches    []string `json:"branches,omitempty" jsonschema:"request: branch names you choose, one per repo, as repo=name."`
	From        string   `json:"from,omitempty" jsonschema:"rename and change: the current branch."`
	To          string   `json:"to,omitempty" jsonschema:"rename and change: the new branch."`
	WaitSeconds int      `json:"waitSeconds,omitempty" jsonschema:"wait: block until the branch setup ends, up to this many seconds (0 to 600)."`
}

type leaseArgs struct {
	Action string `json:"action" jsonschema:"acquire, release or status."`
	Queue  string `json:"queue,omitempty" jsonschema:"The lease queue name."`
}

type cancelArgs struct {
	OpID string `json:"opId" jsonschema:"Operation id from state.yaml operations."`
}

func newFilesArm(root, step, wtPath string, ready time.Time, withCreateTask bool) *filesArm {
	fa := &filesArm{root: root, step: step, seen: map[string]string{}, base: map[string]string{}, pend: map[string]bool{}, ready: ready, wtPath: wtPath}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			fa.base[rel] = fileSHA(p)
		}
		return nil
	})
	src := llmprompts.Defaults()
	srv := mcp.NewServer(&mcp.Implementation{Name: "kira-space", Title: "Kira Space", Version: "1"}, &mcp.ServerOptions{
		Instructions: "Kira Space tools for this task. The task's files are in its task folder: read README.md there first, and state.yaml for the task's state. Change task content by editing the files, then call submit. Create branches and worktrees only with branches; you name each branch, the user approves. Never git branch or git worktree. Finishing replies list every artifact.",
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "submit", Description: "Submit your edits to this task's files. Kira accepts each file, holds it for the user's approval, or rejects it, and answers per file: accepted, pending, rejected, invalid (with line), restored (read-only file), ignored. Empty paths scans the whole task folder.",
		InputSchema: mustSchema[submitArgs]()}, fa.submit)
	mcp.AddTool(srv, &mcp.Tool{Name: "finish_step", Description: llmprompts.HeadlessToolFinishStep.Text(src, llmprompts.NoVars{}) + " Submits your files first; the step's handover is handovers/draft-<step>.md, write it before this call.",
		InputSchema: mustSchema[finishArgs]()}, fa.finishStep)
	// Merged tools carry the registry text of every tool they replace, so schema size is realistic.
	txt := func(ps ...interface {
		Text(llmprompts.Source, llmprompts.NoVars) string
	}) string {
		var parts []string
		for _, p := range ps {
			parts = append(parts, p.Text(src, llmprompts.NoVars{}))
		}
		return strings.Join(parts, "\n")
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "branches", Description: "Manage the task's branches and worktrees; pick one action. wait blocks until a branch's worktree setup ends, no polling; setup state is also in state.yaml.\ndeclare: " +
		txt(llmprompts.HeadlessToolDeclareRepos) + "\nrequest: " + txt(llmprompts.HeadlessToolRequestBranches) + "\nrename: " + txt(llmprompts.HeadlessToolRenameBranch) + "\nchange: " + txt(llmprompts.HeadlessToolChangeBranch),
		InputSchema: mustSchema[branchesArgs]()}, fa.branches)
	mcp.AddTool(srv, &mcp.Tool{Name: "lease", Description: "Pick one action. Held leases and queue positions are also in state.yaml.\nacquire: " + txt(llmprompts.HeadlessToolAcquireLease) + "\nrelease: " + txt(llmprompts.HeadlessToolReleaseLease) + "\nstatus: " + txt(llmprompts.HeadlessToolLeaseStatus),
		InputSchema: mustSchema[leaseArgs]()}, func(context.Context, *mcp.CallToolRequest, leaseArgs) (*mcp.CallToolResult, any, error) {
		return toolErr("no lease queues in this spike.")
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "cancel_operation", Description: "Stop one running operation of this task by id; the list is in state.yaml operations. " + txt(llmprompts.HeadlessToolOperations),
		InputSchema: mustSchema[cancelArgs]()}, func(context.Context, *mcp.CallToolRequest, cancelArgs) (*mcp.CallToolResult, any, error) {
		return toolErr("no such operation.")
	})
	if withCreateTask { // an orchestrator keeps create_task unless the flow replaces it with folders (6 tools, plan section 9)
		mcp.AddTool(srv, &mcp.Tool{Name: "create_task", Description: llmprompts.HeadlessToolCreateTask.Text(src, llmprompts.NoVars{}),
			InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[agentmcp.CreateTaskArgs](src, llmprompts.HeadlessToolCreateTask.ID(), map[string]any{
				"repos": llmprompts.MaxVars{Max: 10}, "brief": llmprompts.MaxVars{Max: agentmcp.MaxHandoverBytes},
			}), "title", "workflow", "brief")}, func(context.Context, *mcp.CallToolRequest, agentmcp.CreateTaskArgs) (*mcp.CallToolResult, any, error) {
			return toolErr("not needed in this flow.")
		})
	}
	fa.srv = httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true}))
	return fa
}

func mustSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](nil)
	if err != nil {
		panic(err)
	}
	return s
}

func fileSHA(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

var readOnlyFiles = []string{"README.md", "task.yaml", "state.yaml"}

// scan classifies every file of the folder (or paths) the way the planned submit would.
func (fa *filesArm) scan(paths []string) []fileResult {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	var rels []string
	if len(paths) == 0 {
		_ = filepath.WalkDir(fa.root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(fa.root, p)
				rels = append(rels, rel)
			}
			return nil
		})
	} else {
		rels = paths
	}
	for rel := range fa.base { // deleted files
		if _, err := os.Stat(filepath.Join(fa.root, rel)); err != nil && !slices.Contains(rels, rel) && len(paths) == 0 {
			rels = append(rels, rel)
		}
	}
	slices.Sort(rels)
	var out []fileResult
	checkedProofs := map[string]bool{}
	for _, rel := range rels {
		rel = filepath.Clean(rel)
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
			out = append(out, fileResult{rel, "rejected", "path leaves the task folder"})
			continue
		}
		abs := filepath.Join(fa.root, rel)
		sha := fileSHA(abs)
		if sha == fa.base[rel] && sha != "" {
			continue // unchanged
		}
		parts := strings.Split(rel, string(filepath.Separator))
		switch {
		case slices.Contains(readOnlyFiles, rel):
			if sha == "" {
				out = append(out, fileResult{rel, "restored", "read-only file deleted; restored"})
			} else {
				out = append(out, fileResult{rel, "restored", "read-only file edited; restored"})
			}
		case fa.cons && rel == "handovers.md":
			out = append(out, fa.checkCons(abs, rel, sha))
		case rel == "notes.md" || parts[0] == "notes":
			fa.pend[rel] = true
			out = append(out, fileResult{rel, "pending", "user note: waiting for the user's approval"})
		case parts[0] == "handovers" && len(parts) == 2 && strings.HasPrefix(parts[1], "draft-") && strings.HasSuffix(parts[1], ".md"):
			step := strings.TrimSuffix(strings.TrimPrefix(parts[1], "draft-"), ".md")
			switch {
			case sha == "":
				fa.seen[rel] = ""
				out = append(out, fileResult{rel, "accepted", "draft removed"})
			case step != fa.step:
				out = append(out, fileResult{rel, "rejected", fmt.Sprintf("The task is in step %s, not %s.", fa.step, step)})
			case fileSize(abs) > 16<<10:
				out = append(out, fileResult{rel, "invalid", "over 16 KiB"})
			default:
				fa.seen[rel], fa.base[rel] = sha, sha
				out = append(out, fileResult{rel, "accepted", ""})
			}
		case parts[0] == "handovers":
			fa.pend[rel] = true
			out = append(out, fileResult{rel, "pending", "handed handover: waiting for the user's approval"})
		case rel == "links.yaml":
			var v any
			if b, _ := os.ReadFile(abs); yaml.Unmarshal(b, &v) != nil {
				out = append(out, fileResult{rel, "invalid", "not valid YAML"})
			} else {
				fa.seen[rel], fa.base[rel] = sha, sha
				out = append(out, fileResult{rel, "accepted", ""})
			}
		case parts[0] == "children" && len(parts) == 3:
			slug := parts[1]
			if checkedProofs["c/"+slug] {
				continue
			}
			checkedProofs["c/"+slug] = true
			out = append(out, fa.checkChild(slug))
		case parts[0] == "proofs" && len(parts) == 3:
			slug := parts[1]
			if checkedProofs[slug] {
				continue
			}
			checkedProofs[slug] = true
			out = append(out, fa.checkProof(slug)...)
		default:
			out = append(out, fileResult{rel, "ignored", "not a Details file"})
		}
	}
	return out
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

type proofFront struct {
	Kira    string `yaml:"kira"`
	Name    string `yaml:"name"`
	Summary string `yaml:"summary"`
	Reason  string `yaml:"reason"`
	Steps   []struct {
		ID          string `yaml:"id"`
		Description string `yaml:"description"`
		View        string `yaml:"view"`
		Data        string `yaml:"data"`
		Inline      any    `yaml:"inline"` // Part 1c: step data in proof.md itself
	} `yaml:"steps"`
}

// readProof parses and validates one proof folder; the rel field names the failing file.
func readProof(dir string) (*spikeProof, string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "proof.md"))
	if err != nil {
		return nil, "proof.md", errors.New("proof.md is missing")
	}
	txt := strings.ReplaceAll(string(raw), "\r\n", "\n")
	rest, ok := strings.CutPrefix(txt, "---\n")
	front, _, found := strings.Cut(rest, "\n---")
	if !ok || !found {
		return nil, "proof.md", errors.New("line 1: proof.md must start with --- frontmatter and close it with ---")
	}
	var f proofFront
	if err := yaml.Unmarshal([]byte(front), &f); err != nil {
		return nil, "proof.md", fmt.Errorf("frontmatter: %v", err)
	}
	if f.Kira != "proof/1" || strings.TrimSpace(f.Name) == "" || len(f.Steps) == 0 {
		return nil, "proof.md", errors.New("frontmatter needs kira: proof/1, name and at least one step")
	}
	p := &spikeProof{Name: f.Name, Summary: f.Summary, Reason: f.Reason}
	seen := map[string]bool{}
	for i, s := range f.Steps {
		if s.ID == "" || seen[s.ID] {
			return nil, "proof.md", fmt.Errorf("steps[%d].id is empty or duplicate", i)
		}
		seen[s.ID] = true
		if strings.TrimSpace(s.Description) == "" || !slices.Contains(proofViews, s.View) {
			return nil, "proof.md", fmt.Errorf("step %s needs description and a valid view", s.ID)
		}
		var b []byte
		name := s.Data
		if s.Inline != nil {
			name = "proof.md"
			switch v := s.Inline.(type) {
			case string:
				b = []byte(v)
			default:
				var jerr error
				if b, jerr = json.Marshal(v); jerr != nil {
					return nil, name, fmt.Errorf("step %s: inline data: %v", s.ID, jerr)
				}
			}
		} else {
			if s.Data == "" || strings.Contains(s.Data, "/") || strings.HasPrefix(s.Data, ".") {
				return nil, "proof.md", fmt.Errorf("step %s: data must name a file in the proof folder", s.ID)
			}
			var rerr error
			if b, rerr = os.ReadFile(filepath.Join(dir, s.Data)); rerr != nil {
				return nil, s.Data, fmt.Errorf("step %s: data file %s is missing", s.ID, s.Data)
			}
		}
		var err error
		st := spikeStep{ID: s.ID, Description: s.Description, View: s.View}
		switch s.View {
		case "table":
			if st.Rows, err = decodeTable(b); err != nil {
				return nil, name, fmt.Errorf("step %s: %v", s.ID, err)
			}
		case "image":
			if err := checkJPEG(b); err != nil {
				return nil, name, fmt.Errorf("step %s: %v", s.ID, err)
			}
			st.ImageOK = true
		case "diagram":
			first, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
			if (s.Inline == nil && !strings.HasSuffix(s.Data, ".mmd")) || !slices.ContainsFunc(mermaidKinds, func(k string) bool { return strings.HasPrefix(first, k) }) {
				return nil, name, fmt.Errorf("step %s: a diagram is a .mmd file whose first line starts with a Mermaid diagram type", s.ID)
			}
			st.Text = string(b)
		case "document":
			var d struct {
				Documents []any `json:"documents"`
			}
			if json.Unmarshal(b, &d) != nil || len(d.Documents) == 0 {
				return nil, name, fmt.Errorf("step %s: document data is JSON {documents:[{body:{...}}]}", s.ID)
			}
			st.Text = string(b)
		}
		p.Steps = append(p.Steps, st)
	}
	return p, "", nil
}

func (fa *filesArm) checkProof(slug string) []fileResult {
	dir := filepath.Join(fa.root, "proofs", slug)
	rel := filepath.Join("proofs", slug, "proof.md")
	p, bad, err := readProof(dir)
	if err != nil {
		return []fileResult{{filepath.Join("proofs", slug, bad), "invalid", err.Error()}}
	}
	edit := false
	for k := range fa.base {
		if strings.HasPrefix(k, filepath.Join("proofs", slug)+string(filepath.Separator)) {
			edit = true
		}
	}
	var changed bool
	hashAll := func() map[string]string {
		m := map[string]string{}
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			m[filepath.Join("proofs", slug, e.Name())] = fileSHA(filepath.Join(dir, e.Name()))
		}
		return m
	}
	now := hashAll()
	for k, v := range now {
		if fa.base[k] != v {
			changed = true
		}
	}
	if edit && changed && strings.TrimSpace(p.Reason) == "" {
		return []fileResult{{rel, "invalid", "This proof is accepted: set reason in proof.md frontmatter to say what changes and why."}}
	}
	if edit && changed {
		for k, v := range now {
			fa.seen[k] = v
		}
		fa.pend[rel] = true
		return []fileResult{{rel, "pending", "edit of an accepted proof: waiting for the user's approval"}}
	}
	for k, v := range now {
		fa.base[k], fa.seen[k] = v, v
	}
	return []fileResult{{rel, "accepted", "new proof"}}
}

func (fa *filesArm) submit(_ context.Context, _ *mcp.CallToolRequest, a submitArgs) (*mcp.CallToolResult, any, error) {
	res := fa.scan(a.Paths)
	raw, _ := json.Marshal(map[string]any{"results": res, "finished": a.Finished})
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

func (fa *filesArm) finishStep(_ context.Context, _ *mcp.CallToolRequest, a finishArgs) (*mcp.CallToolResult, any, error) {
	if a.Status != "done" && a.Status != "failed" && a.Status != "needs_input" {
		return toolErr("status must be one of: done, failed, needs_input")
	}
	if a.Status == "done" && fa.cons && fa.consDraft() == "" {
		return toolErr("Write the ## draft-%s section at the end of handovers.md, then call finish_step.", fa.step)
	}
	if a.Status == "done" && !fa.cons && fileSHA(filepath.Join(fa.root, "handovers", "draft-"+fa.step+".md")) == "" {
		return toolErr("Write handovers/draft-%s.md, then call finish_step.", fa.step)
	}
	res := fa.scan(nil)
	fa.mu.Lock()
	fa.finish = append(fa.finish, a.Status)
	fa.mu.Unlock()
	raw, _ := json.Marshal(map[string]any{"ack": "Step reported. Stop now.", "results": res})
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

func (fa *filesArm) branches(_ context.Context, _ *mcp.CallToolRequest, a branchesArgs) (*mcp.CallToolResult, any, error) {
	if a.Action != "wait" {
		return toolErr("action %s is not available in this spike.", a.Action)
	}
	if a.Repo == "" || a.Branch == "" || a.WaitSeconds < 0 || a.WaitSeconds > 600 {
		return toolErr("wait needs repo, branch and waitSeconds 0 to 600.")
	}
	if d := time.Until(fa.ready); d > 0 {
		time.Sleep(min(d, time.Duration(a.WaitSeconds)*time.Second))
	}
	state, note := "ready", "setup done"
	if time.Now().Before(fa.ready) {
		state, note = "running", "installing dependencies"
	} else {
		sp := filepath.Join(fa.root, "state.yaml")
		if b, err := os.ReadFile(sp); err == nil {
			_ = os.WriteFile(sp, bytes.Replace(b, []byte("state: running, note: installing dependencies"), []byte("state: ready, note: setup done"), 1), 0o644)
			fa.mu.Lock()
			fa.base["state.yaml"] = fileSHA(sp)
			fa.mu.Unlock()
		}
	}
	raw, _ := json.Marshal(map[string]any{"repo": a.Repo, "branch": a.Branch, "worktree": fa.wtPath, "setup": map[string]string{"state": state, "note": note}})
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}, nil, nil
}

func (fa *filesArm) close() { fa.srv.Close() }

// ListArtifacts feeds finish_step's reply: the spike task has none.
func (s *spikeSpace) ListArtifacts(context.Context, string) (agentmcp.TaskArtifacts, error) {
	return agentmcp.TaskArtifacts{}, nil
}

// checkChild validates a drafted child folder and, like an approved create_task, creates the child.
func (fa *filesArm) checkChild(slug string) fileResult {
	dir := filepath.Join(fa.root, "children", slug)
	rel := filepath.Join("children", slug)
	if k, ok := fa.kids[slug]; ok {
		return fileResult{rel, "rejected", "child " + k.ID + " already created; its folder is final"}
	}
	var y struct {
		Title    string `yaml:"title"`
		Kind     string `yaml:"kind"`
		Workflow string `yaml:"workflow"`
		Repos    []struct {
			Repo string `yaml:"repo"`
			Base string `yaml:"base"`
		} `yaml:"repos"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "task.yaml"))
	if err != nil {
		return fileResult{filepath.Join(rel, "task.yaml"), "invalid", "task.yaml is missing"}
	}
	if err := yaml.Unmarshal(b, &y); err != nil {
		return fileResult{filepath.Join(rel, "task.yaml"), "invalid", err.Error()}
	}
	if t := strings.TrimSpace(y.Title); t == "" || len([]rune(t)) > 200 || y.Workflow == "" || len(y.Repos) > 10 || (y.Kind != "task" && y.Kind != "") {
		return fileResult{filepath.Join(rel, "task.yaml"), "invalid", "needs title (1-200 characters), workflow, kind task, at most 10 repos"}
	}
	brief, err := os.ReadFile(filepath.Join(dir, "brief.md"))
	if err != nil || strings.TrimSpace(string(brief)) == "" || len(brief) > 16<<10 {
		return fileResult{filepath.Join(rel, "brief.md"), "invalid", "brief.md is missing, empty or over 16 KiB"}
	}
	if fa.kids == nil {
		fa.kids = map[string]spikeChild{}
	}
	c := spikeChild{ID: fmt.Sprintf("c-%d", len(fa.kids)+1), Title: y.Title, Workflow: y.Workflow, Brief: string(brief)}
	for _, r := range y.Repos {
		c.Repos = append(c.Repos, r.Repo)
	}
	fa.kids[slug] = c
	for _, f := range []string{"task.yaml", "brief.md"} {
		fa.base[filepath.Join(rel, f)] = fileSHA(filepath.Join(dir, f))
	}
	preview := string(brief)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return fileResult{rel, "created", "child " + c.ID + " created. Brief preview: " + preview}
}

// consDraft returns the body of the current step's draft section in handovers.md (Part 1b layout).
func (fa *filesArm) consDraft() string {
	b, _ := os.ReadFile(filepath.Join(fa.root, "handovers.md"))
	return consDraftOf(string(b), fa.step)
}

func consDraftOf(content, step string) string {
	_, rest, ok := strings.Cut(content, "## draft-"+step)
	if !ok {
		return ""
	}
	_, body, _ := strings.Cut(rest, "\n")
	return strings.TrimSpace(body)
}

// checkCons classifies handovers.md: the handed sections stay as seeded, the draft section is the agent's.
func (fa *filesArm) checkCons(abs, rel, sha string) fileResult {
	b, _ := os.ReadFile(abs)
	txt := string(b)
	switch {
	case !strings.HasPrefix(txt, fa.handed):
		return fileResult{rel, "rejected", "handed sections are read-only; only the draft section at the end is yours"}
	case consDraftOf(txt, fa.step) == "":
		if strings.Contains(txt[len(fa.handed):], "## draft-") {
			return fileResult{rel, "rejected", fmt.Sprintf("The task is in step %s: name your draft section ## draft-%s.", fa.step, fa.step)}
		}
		fa.seen[rel], fa.base[rel] = sha, sha
		return fileResult{rel, "accepted", "draft removed"}
	case len(txt)-len(fa.handed) > 16<<10:
		return fileResult{rel, "invalid", "draft over 16 KiB"}
	}
	fa.seen[rel], fa.base[rel] = sha, sha
	return fileResult{rel, "accepted", ""}
}
