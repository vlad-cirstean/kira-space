package bridge

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// exposedScripts lists the MCP-enabled normal scripts of one stack in panel order.
func exposedScripts(repo *scripts.Repo) ([]scripts.CustomScript, error) {
	all, err := repo.List()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, rec := range all {
		if rec.Kind != scripts.KindSmart && rec.MCP != nil && rec.MCP.Enabled {
			out = append(out, rec)
		}
	}
	return out, nil
}

// isExposed reports whether a script is an MCP-enabled normal script now.
func isExposed(repo *scripts.Repo, scriptID string) (bool, error) {
	rec, err := repo.Get(scriptID)
	if err != nil || rec == nil {
		return false, err
	}
	return rec.Kind != scripts.KindSmart && rec.MCP != nil && rec.MCP.Enabled, nil
}

// scriptControl serves the script tools over both homes' stacks, in order: a list is the union, a
// call naming a script or run goes to the stack holding it.
type scriptControl struct{ stacks []*scriptruns.Service }

// NewScriptControl adapts the script-run services of every stack to dbmcp's script tools.
func NewScriptControl(stacks ...*scriptruns.Service) dbmcp.ScriptControl {
	return scriptControl{stacks: stacks}
}

// ofScript is the stack holding the script, the first stack when none does (its error surfaces).
func (c scriptControl) ofScript(id string) *scriptruns.Service {
	for _, svc := range c.stacks {
		if rec, err := svc.Scripts.Get(id); err == nil && rec != nil {
			return svc
		}
	}
	return c.stacks[0]
}

// ofRun is the stack holding the run, the first stack when none does.
func (c scriptControl) ofRun(id string) *scriptruns.Service {
	for _, svc := range c.stacks {
		if _, err := svc.Runs.Get(id); err == nil {
			return svc
		}
	}
	return c.stacks[0]
}

func (c scriptControl) Exposed() ([]dbmcp.ExposedScript, error) {
	var out []dbmcp.ExposedScript
	for _, svc := range c.stacks {
		recs, err := exposedScripts(svc.Scripts)
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			e := dbmcp.ExposedScript{ID: rec.ID, Name: rec.Name, Description: rec.MCP.Description, Run: rec.MCP.Run, Params: make([]dbmcp.ScriptParamInfo, 0, len(rec.Params))}
			for _, p := range rec.Params {
				e.Params = append(e.Params, dbmcp.ScriptParamInfo{
					Name: p.Name, Type: p.Type, Options: p.Options, Default: p.Default, Required: p.Required,
					Secret: p.Secret, Saved: p.Secret && slices.Contains(rec.SavedSecrets, p.Name),
				})
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// approvalValues maps env entries to approval values; a secret keeps no value.
func approvalValues(env []scriptruns.EnvVar, params bool) []dbmcp.ApprovalValue {
	out := make([]dbmcp.ApprovalValue, 0, len(env))
	for _, e := range env {
		name := e.Name
		if params {
			name = e.FromVar
		}
		v := dbmcp.ApprovalValue{Name: name, Value: e.Value, Secret: e.Secret, Saved: e.Saved, FromVar: e.FromVar}
		if e.Secret {
			v.Value = ""
		}
		out = append(out, v)
	}
	return out
}

func (c scriptControl) Plan(scriptID string, params map[string][]string) (dbmcp.ScriptPlan, error) {
	pv, err := c.ofScript(scriptID).PlanMCP(scriptruns.RunArgs{ScriptID: scriptID, Params: params})
	if err != nil {
		return dbmcp.ScriptPlan{}, err
	}
	return dbmcp.ScriptPlan{
		Hash: pv.Hash, Command: pv.Command, Cwd: pv.Dir.Path, Blocker: pv.Blocker, Missing: pv.Missing,
		Params: approvalValues(pv.Env, true), Env: approvalValues(pv.Env, false),
	}, nil
}

func (c scriptControl) Start(scriptID string, params map[string][]string, hash string) (string, error) {
	started, err := c.ofScript(scriptID).StartMCP(scriptruns.StartArgs{RunArgs: scriptruns.RunArgs{ScriptID: scriptID, Params: params}, Hash: hash})
	if err != nil {
		return "", err
	}
	return started.RunID, nil
}

func scriptRunOf(r scriptruns.Run) dbmcp.ScriptRun {
	out := dbmcp.ScriptRun{
		ID: r.ID, ScriptID: r.ScriptID, ScriptName: r.ScriptName, State: r.State, Trigger: string(r.Trigger),
		StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Headless: r.TerminalID == "",
	}
	if r.Outcome != nil {
		out.Reason, out.ExitCode = r.Outcome.Reason, r.Outcome.ExitCode
	}
	return out
}

func (c scriptControl) Run(runID string) (dbmcp.ScriptRun, bool, error) {
	run, err := c.ofRun(runID).Runs.Get(runID)
	if errors.Is(err, sql.ErrNoRows) {
		return dbmcp.ScriptRun{}, false, nil
	}
	if err != nil {
		return dbmcp.ScriptRun{}, false, err
	}
	return scriptRunOf(run), true, nil
}

func (c scriptControl) Wait(ctx context.Context, runID string) (dbmcp.ScriptRun, error) {
	run, err := c.ofRun(runID).Wait(ctx, runID)
	if err != nil {
		return dbmcp.ScriptRun{}, err
	}
	return scriptRunOf(run), nil
}

func (c scriptControl) Log(runID string, q dbmcp.ScriptLogQuery) (dbmcp.ScriptLog, error) {
	slice, err := c.ofRun(runID).Runs.QueryLog(runID, scriptruns.LogQuery{FromEnd: q.FromEnd, Limit: q.Limit, Contains: q.Contains})
	if err != nil {
		return dbmcp.ScriptLog{}, err
	}
	lines := make([]dbmcp.ScriptLogLine, 0, len(slice.Chunks))
	for _, ch := range slice.Chunks {
		lines = append(lines, dbmcp.ScriptLogLine{Seq: ch.Seq, Stream: ch.Stream, Text: ch.Text})
	}
	return dbmcp.ScriptLog{Lines: lines, Matched: slice.Matched, Truncated: slice.Truncated}, nil
}

type scriptRunControl struct{ stacks []*scriptruns.Service }

// NewScriptRunControl adapts the script-run services of every stack to dbmcp's operation tools:
// only runs of MCP-exposed scripts are listed or stopped, the way a hidden connection's operations
// are.
func NewScriptRunControl(stacks ...*scriptruns.Service) dbmcp.ScriptRunControl {
	return scriptRunControl{stacks: stacks}
}

func (c scriptRunControl) Live() ([]dbmcp.LiveRun, error) {
	var out []dbmcp.LiveRun
	for _, svc := range c.stacks {
		runs, err := svc.Runs.Live()
		if err != nil {
			return nil, err
		}
		exposed := map[string]bool{}
		for _, r := range runs {
			ok, seen := exposed[r.ScriptID]
			if !seen {
				if ok, err = isExposed(svc.Scripts, r.ScriptID); err != nil {
					return nil, err
				}
				exposed[r.ScriptID] = ok
			}
			if ok {
				out = append(out, dbmcp.LiveRun{ID: r.ID, ScriptName: r.ScriptName, State: r.State, Trigger: string(r.Trigger), StartedAt: r.StartedAt})
			}
		}
	}
	return out, nil
}

func (c scriptRunControl) Stop(id string) error {
	for _, svc := range c.stacks {
		if _, err := svc.Runs.Get(id); err == nil {
			return svc.Stop(id)
		}
	}
	return c.stacks[0].Stop(id)
}
