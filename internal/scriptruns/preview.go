package scriptruns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// maxSmartRuns is how many headless smart runs go at once.
const maxSmartRuns = 3

// RunArgs is what the run dialog sends to Preview and Start.
type RunArgs struct {
	ScriptID string              `json:"scriptId"`
	Params   map[string][]string `json:"params"`
	// Prompt is a one-off prompt body for this run, never saved; nil uses the saved body.
	Prompt *string `json:"prompt"`
	// TaskID and BranchID run the script for an ADE task and branch (Kira Space); "" for none.
	TaskID   string `json:"taskId"`
	BranchID string `json:"branchId"`
	// ListTasks fills Needs.Tasks even when no ADE variable is used (a schedule's task picker).
	ListTasks bool `json:"listTasks"`
}

// EnvVar is one variable the run gets. A secret's Value is empty here. FromVar names the param the
// value comes from, "" for a fixed one.
type EnvVar struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Secret  bool   `json:"secret"`
	FromVar string `json:"fromVar"`
	// Saved says a secret's value comes from the script's saved value, not the dialog.
	Saved bool `json:"saved"`
}

// Preview is what a run would do, exactly as Start does it.
type Preview struct {
	Kind string `json:"kind"`
	// Missing lists required params without a value.
	Missing []string `json:"missing"`
	// Blocker, when set, says why Run is disabled.
	Blocker string `json:"blocker"`
	// SecretIssue says a saved secret value could not be read, so the run asks for it.
	SecretIssue string      `json:"secretIssue"`
	Dir         scripts.Dir `json:"dir"`
	// Body is the saved prompt template; a one-off edit starts from it.
	Body string `json:"body"`
	// Prompt is the body being used, resolved, as parts.
	Prompt []scripts.Part `json:"prompt"`
	// Suffix is what Kira adds after the prompt.
	Suffix       string   `json:"suffix"`
	Env          []EnvVar `json:"env"`
	Command      string   `json:"command"`
	Model        string   `json:"model"`
	MaxBudgetUSD float64  `json:"maxBudgetUsd"`
	Timeout      string   `json:"timeout"`
	Tools        []string `json:"tools"`
	Allowed      []string `json:"allowedTools"`
	MCP          []string `json:"mcpServers"`
	Hash         string   `json:"hash"`
	// Needs lists what to pick before the run can resolve; ADE is the task and branch it resolved, nil without a task.
	Needs Needs   `json:"needs"`
	ADE   *RunADE `json:"ade"`
}

// StartArgs is RunArgs plus the hash of the preview the user confirmed.
type StartArgs struct {
	RunArgs
	Hash string `json:"hash"`
}

// TerminalStart tells the window how to open a normal script's terminal tab.
type TerminalStart struct {
	Token string `json:"token"`
	Cwd   string `json:"cwd"`
}

// Started is Start's answer: a smart run's id, or the terminal a normal script opens.
type Started struct {
	RunID    string         `json:"runId"`
	Terminal *TerminalStart `json:"terminal"`
}

type planned struct {
	script  scripts.CustomScript
	dir     scripts.Dir
	params  []RunParam
	envList []string
	// mask hides every secret value the run gets; nil when it has none.
	mask *secrets.Masker
	// sent is the full prompt text a smart run sends: the resolved body, then the suffix.
	sent    string
	smart   scripts.Smart
	mcp     []string
	preview Preview
	// release drops the board's worktree claim a run holds; nil when none.
	release func()
	args    RunArgs
	opts    planOpts
	// trigger overrides runTrigger when the caller says what started the run.
	trigger Trigger
	// resume is the Claude session a resumed smart run continues, resumedFrom the run it continues.
	resume, resumedFrom string
	// agent is the provider a smart run runs under, pinned at plan time; provider is its id.
	agent    harness.Provider
	provider string
	// headless is agent's Headless role, nil when it has none (the preview then carries a blocker).
	headless harness.Headless
	// timeout and timeoutText bound a headless normal run.
	timeout     time.Duration
	timeoutText string
}

// planOpts tunes plan: claimed says the caller already holds the worktree claim, so the busy
// check skips it; headless says the run is unattended, so a normal script also needs the branch free.
type planOpts struct {
	claimed, headless bool
	// informational marks a plan that only reports (a preview, the restore offer): the MCP presence
	// check never waits on a cold `claude mcp list`. Start re-checks without this.
	informational bool
	// provider pins a resumed smart run to the provider of its conversation; empty means the active one.
	provider string
}

func (s *Service) getenv(k string) string {
	if s.Getenv != nil {
		return s.Getenv(k)
	}
	return os.Getenv(k)
}

// plan resolves a run the one way Preview shows it and Start executes it.
func (s *Service) plan(ctx context.Context, args RunArgs, o planOpts) (*planned, error) {
	rec, err := s.Scripts.Get(args.ScriptID)
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	if rec == nil {
		return nil, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
	}
	given, saved, issue, err := s.fillSaved(*rec, args.Params)
	if err != nil {
		return nil, err
	}
	vals, missing, err := scripts.ParamValues(rec.Params, given)
	if err != nil {
		return nil, ipcerr.BadRequest(err.Error())
	}
	p := &planned{script: *rec, dir: scripts.ResolveDir(*rec, s.Home), args: args, opts: o}
	pv := Preview{
		Kind: rec.Kind, Missing: nonNilStrings(missing), Dir: p.dir, Env: []EnvVar{}, MCP: []string{}, Tools: []string{},
		Allowed: []string{}, Prompt: []scripts.Part{}, Needs: Needs{Tasks: []TaskChoice{}, Branches: []BranchChoice{}},
	}
	pv.Blocker = p.dir.Blocker
	pv.SecretIssue = issue
	rendered := map[string]string{}
	var secretVals []string
	for _, v := range vals {
		pv.Env = append(pv.Env, EnvVar{Name: scripts.EnvName(v.Name), Value: ifNot(v.Secret, v.Env), Secret: v.Secret, FromVar: v.Name, Saved: v.Secret && saved[v.Name]})
		p.envList = append(p.envList, scripts.EnvName(v.Name)+"="+v.Env)
		shown := v.Env
		if v.Secret {
			shown = scripts.SecretMask
			secretVals = append(secretVals, v.Env)
		} else {
			rendered[v.Name] = v.Rendered
		}
		p.params = append(p.params, RunParam{Name: v.Name, Value: shown})
	}
	p.mask = secrets.NewMasker(secretVals...)
	if rec.Kind != scripts.KindSmart {
		pv.Command = rec.Command
		if err := s.planADE(args, p, &pv, rec.Command, rendered); err != nil {
			return nil, err
		}
		p.preview = finishPreview(pv, p)
		return p, nil
	}

	smart := *rec.Smart
	p.smart = smart
	body := rec.Command
	pv.Body = body
	if args.Prompt != nil {
		body = strings.TrimSpace(*args.Prompt)
		if blocker := checkPromptEdit(body, rec.Params); blocker != "" && pv.Blocker == "" {
			pv.Blocker = blocker
		}
	}
	if err := s.planADE(args, p, &pv, body, rendered); err != nil {
		return nil, err
	}
	pv.Prompt = scripts.Compose(body, rendered)
	pv.Suffix = agentmcp.ScriptReport(s.llmPrompts())
	agent, err := s.agentFor(o.provider)
	if err != nil {
		return nil, providerGone(err)
	}
	p.agent, p.provider = agent, agent.ID()
	if h, ok := harness.HeadlessOf(agent); ok {
		p.headless = h
	} else if pv.Blocker == "" {
		pv.Blocker = (&harness.MissingRole{Agent: agent.Label(), Role: "smart scripts"}).Error()
	}
	extraTools := []harness.MCPTool{agentmcp.FinishStep}
	if args.TaskID != "" {
		pv.Suffix = agentmcp.SpaceNote(s.llmPrompts()) + "\n\n" + pv.Suffix
		extraTools = append(append(extraTools, agentmcp.RunOutcome), agentmcp.SpaceToolIDs...)
	}
	p.sent = scripts.PlainText(pv.Prompt) + "\n\n" + pv.Suffix
	pv.Model, pv.MaxBudgetUSD, pv.Timeout = smart.Model, smart.MaxBudgetUSD, smart.Timeout
	pv.Tools, pv.Allowed = agent.ToolArgs(smart.ToolPolicy(harness.ToolNames(agent, extraTools...)))
	for _, c := range smart.MCP {
		pv.MCP = append(pv.MCP, c.Server)
	}
	p.mcp = pv.MCP
	if pv.Blocker == "" {
		pv.Blocker = agentBlocker(agent, smart.Model, smart.MaxBudgetUSD)
	}
	if pv.Blocker == "" {
		pv.Blocker = s.smartBlocker(ctx, agent, pv.MCP, o.informational)
	}
	p.preview = finishPreview(pv, p)
	return p, nil
}

// agentBlocker is why agent cannot run a script with model and a cost cap: a cap it cannot enforce,
// or a model outside its catalog (the registry holds every provider's aliases, so a script may name
// one this agent lacks).
func agentBlocker(agent harness.Provider, model string, budgetUSD float64) string {
	switch {
	case budgetUSD > 0 && !harness.CapsCost(agent):
		return i18n.T("backend.core.agent.noCostCap.body", map[string]string{"agent": agent.Label()})
	case model != "" && !agent.Models().Valid(model):
		return i18n.T("backend.core.agent.modelUnknown.body", map[string]string{"agent": agent.Label(), "model": model})
	}
	return ""
}

// fillSaved adds the saved value of each secret param the dialog gave none for; a typed value wins.
// saved names the params filled this way. issue says a saved value could not be read, which counts
// as absent.
func (s *Service) fillSaved(rec scripts.CustomScript, typed map[string][]string) (given map[string][]string, saved map[string]bool, issue string, err error) {
	given, saved = typed, map[string]bool{}
	var want []string
	for _, p := range rec.Params {
		if p.Secret && slices.Contains(rec.SavedSecrets, p.Name) && !hasValue(typed[p.Name]) {
			want = append(want, p.Name)
		}
	}
	if len(want) == 0 {
		return given, saved, "", nil
	}
	vals, unreadable, err := s.Scripts.SecretValues(rec.ID)
	if err != nil {
		return nil, nil, "", ipcerr.InternalErr(err)
	}
	given = make(map[string][]string, len(typed)+len(want))
	maps.Copy(given, typed)
	var bad []string
	var detail string
	for _, n := range want {
		if v, ok := vals[n]; ok {
			given[n], saved[n] = []string{v}, true
		} else if e, ok := unreadable[n]; ok {
			bad, detail = append(bad, n), e.Error()
		}
	}
	if len(bad) > 0 {
		issue = i18n.T("backend.core.scriptRun.savedSecretUnreadable.body", map[string]string{"names": strings.Join(bad, ", "), "detail": detail})
	}
	return given, saved, issue, nil
}

func hasValue(vs []string) bool { return len(vs) > 0 && vs[0] != "" }

func ifNot(secret bool, v string) string {
	if secret {
		return ""
	}
	return v
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// checkPromptEdit answers why a one-off prompt cannot run, "" when it can.
func checkPromptEdit(body string, params []scripts.Param) string {
	if body == "" {
		return i18n.T("backend.core.scriptRun.promptEmpty.body", nil)
	}
	if utf8.RuneCountInString(body) > scripts.MaxPrompt {
		return i18n.T("backend.core.scriptRun.promptTooLong.body", map[string]string{"max": strconv.Itoa(scripts.MaxPrompt)})
	}
	for _, p := range params {
		if p.Secret && containsVarName(body, p.Name) {
			return i18n.T("backend.core.scriptRun.promptSecret.body", map[string]string{"name": p.Name})
		}
	}
	return ""
}

func containsVarName(text, name string) bool {
	for _, used := range scripts.VarsUsed(text, []scripts.Param{{Name: name}}) {
		if used == name {
			return true
		}
	}
	return false
}

// smartBlocker answers why a smart run cannot start now: the run cap, or a user MCP server that is
// gone. The check lists servers from the app home, like the picker: runs never load a folder's project
// `.mcp.json` servers, so a name only that file defines does not count.
func (s *Service) smartBlocker(ctx context.Context, agent harness.Provider, mcp []string, informational bool) string {
	s.mu.Lock()
	running := len(s.smart)
	s.mu.Unlock()
	if running >= maxSmartRuns {
		return i18n.T("backend.core.scriptRun.smartCap.body", map[string]string{"max": strconv.Itoa(maxSmartRuns)})
	}
	if len(mcp) == 0 {
		return ""
	}
	have, known, err := agent.UserMCPPresence(ctx, s.Home, mcp, !informational)
	if err != nil {
		return err.Error()
	}
	if !known {
		return ""
	}
	for _, name := range mcp {
		found := false
		for _, h := range have {
			found = found || h.Name == name
		}
		if !found {
			return harness.ErrServerGone{Name: name, Agent: agent.Label()}.Error()
		}
	}
	return ""
}

func finishPreview(pv Preview, p *planned) Preview {
	pv.Hash = hashOf(pv, p)
	return pv
}

// hashOf fingerprints what the user confirmed: kind, folder, what is sent, and how it runs.
func hashOf(pv Preview, p *planned) string {
	type envKV struct{ Name, Value string }
	fixed := make([]envKV, 0, len(pv.Env))
	for _, e := range pv.Env {
		fixed = append(fixed, envKV{e.Name, e.Value})
	}
	adeKey := ""
	if pv.ADE != nil {
		adeKey = pv.ADE.TaskID + "/" + pv.ADE.BranchID
	}
	b, _ := json.Marshal(struct {
		Kind, Cwd, Mode, Ade, Sent, Model, Timeout, Command string
		Budget                                              float64
		Tools, Allowed, MCP                                 []string
		Env                                                 []envKV
	}{pv.Kind, pv.Dir.Path, pv.Dir.Mode, adeKey, p.sent, pv.Model, pv.Timeout, pv.Command, pv.MaxBudgetUSD, pv.Tools, pv.Allowed, pv.MCP, fixed})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Preview resolves a run without starting anything.
func (s *Service) Preview(args RunArgs) (Preview, error) {
	p, err := s.plan(s.lifeCtx(), args, planOpts{informational: true})
	if err != nil {
		return Preview{}, err
	}
	return p.preview, nil
}

// Start runs what Preview showed. A script that changed since the preview is refused.
func (s *Service) Start(args StartArgs) (Started, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	p, err := s.plan(s.lifeCtx(), args.RunArgs, planOpts{})
	if err != nil {
		return Started{}, err
	}
	var taskID, branchID string
	if a := p.preview.ADE; a != nil {
		taskID, branchID = a.TaskID, a.BranchID
	}
	if live, err := s.Runs.Running(p.script.ID, taskID, branchID); err != nil {
		return Started{}, ipcerr.InternalErr(err)
	} else if live != nil {
		return Started{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.alreadyRunning.body", map[string]string{"name": p.script.Name})
	}
	if p.dir.Mode == scripts.DirModeWorktree && p.preview.Blocker == "" && len(p.preview.Missing) == 0 &&
		(p.script.Kind == scripts.KindSmart || p.dir.Pending) {
		if p, err = s.claimWorktree(p, p.script.Kind == scripts.KindSmart); err != nil {
			return Started{}, err
		}
	}
	started, err := s.startPlanned(p, args.Hash)
	if err != nil && p.release != nil {
		p.release()
	}
	return started, err
}

// claimWorktree creates the branch's missing worktree and takes the board's claim on it. keep holds
// the claim until the run ends (p.release); otherwise it drops at once, since a normal terminal
// script only needs the worktree to exist. The run is planned again against the created worktree.
func (s *Service) claimWorktree(p *planned, keep bool) (*planned, error) {
	release, err := s.ADE.ClaimWorktree(context.Background(), p.preview.ADE.BranchID, p.script.Name)
	if err != nil {
		return nil, ipcerr.BadRequest(err.Error())
	}
	if !keep {
		release()
		return s.plan(s.lifeCtx(), p.args, p.opts)
	}
	o := p.opts
	o.claimed = true
	np, err := s.plan(s.lifeCtx(), p.args, o)
	if err != nil {
		release()
		return nil, err
	}
	np.release, np.trigger = release, p.trigger
	return np, nil
}

func (s *Service) startPlanned(p *planned, hash string) (Started, error) {
	if err := checkStart(p, hash); err != nil {
		return Started{}, err
	}
	if p.script.Kind == scripts.KindSmart {
		return s.startSmart(p)
	}
	return s.startTerminal(p)
}
