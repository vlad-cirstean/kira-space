package scriptruns

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Bound is the Wails-bound half; each app's bridge.ScriptRunsService embeds it, so binding names
// stay app-local. It forwards to Service and maps errors to the wire.
type Bound struct{ Svc *Service }

type ListArgs struct {
	Limit int `json:"limit"`
	// TaskID, when set, lists only the runs started for that ADE task.
	TaskID string `json:"taskId"`
}

type IDArgs struct {
	ID string `json:"id"`
}

func (b *Bound) List(args ListArgs) ([]Run, error) {
	return ipcerr.InternalResult(b.Svc.List(args.Limit, args.TaskID))
}

func (b *Bound) Get(args IDArgs) (Run, error) {
	if args.ID == "" {
		return Run{}, ipcerr.Required("id")
	}
	run, err := b.Svc.Get(args.ID)
	if IsCallerError(err) {
		return Run{}, ipcerr.T(ipcerr.CodeNotFound, "backend.core.scriptRun.runNotFound.body", nil)
	}
	if err != nil {
		return Run{}, ipcerr.InternalErr(err)
	}
	return run, nil
}

func (b *Bound) Stop(args IDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	err := b.Svc.Stop(args.ID)
	if IsCallerError(err) {
		return ipcerr.T(ipcerr.CodeNotFound, "backend.core.scriptRun.runNotFound.body", nil)
	}
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	return nil
}

// ResolveDir previews a script's folder; an empty ScriptID answers the app home.
func (b *Bound) ResolveDir(args ResolveDirArgs) (scripts.Dir, error) {
	dir, err := b.Svc.ResolveDir(args.ScriptID)
	if IsCallerError(err) {
		return scripts.Dir{}, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
	}
	if err != nil {
		return scripts.Dir{}, ipcerr.InternalErr(err)
	}
	return dir, nil
}

type ResolveDirArgs struct {
	ScriptID string `json:"scriptId"`
}

// Preview resolves a run the way Start would, without starting it.
func (b *Bound) Preview(args RunArgs) (Preview, error) {
	if args.ScriptID == "" {
		return Preview{}, ipcerr.Required("scriptId")
	}
	pv, err := b.Svc.Preview(args)
	if err != nil {
		return Preview{}, ipcerr.Wrap(err)
	}
	return pv, nil
}

// Start runs what Preview showed: a smart script runs headless, a normal one returns the token its
// terminal tab opens with.
func (b *Bound) Start(args StartArgs) (Started, error) {
	if args.ScriptID == "" {
		return Started{}, ipcerr.Required("scriptId")
	}
	started, err := b.Svc.Start(args)
	if err != nil {
		return Started{}, ipcerr.Wrap(err)
	}
	return started, nil
}

// Resume continues a smart run a restart interrupted, as a new run.
func (b *Bound) Resume(args IDArgs) (Started, error) {
	if args.ID == "" {
		return Started{}, ipcerr.Required("id")
	}
	started, err := b.Svc.Resume(args.ID)
	if IsCallerError(err) {
		return Started{}, ipcerr.T(ipcerr.CodeNotFound, "backend.core.scriptRun.runNotFound.body", nil)
	}
	if err != nil {
		return Started{}, ipcerr.Wrap(err)
	}
	return started, nil
}

// ResumeCommandArgs names the smart run to continue in a terminal.
type ResumeCommandArgs struct {
	RunID string `json:"runId"`
}

// ResumeCommand is the launch that continues a smart run's conversation in the agent's TUI.
func (b *Bound) ResumeCommand(args ResumeCommandArgs) (ResumeLaunch, error) {
	if args.RunID == "" {
		return ResumeLaunch{}, ipcerr.Required("runId")
	}
	l, err := b.Svc.ResumeCommand(args.RunID)
	if IsCallerError(err) {
		return ResumeLaunch{}, ipcerr.T(ipcerr.CodeNotFound, "backend.core.scriptRun.runNotFound.body", nil)
	}
	if err != nil {
		return ResumeLaunch{}, ipcerr.Wrap(err)
	}
	return l, nil
}

type ReadLogArgs struct {
	ID       string `json:"id"`
	AfterSeq int    `json:"afterSeq"`
}

// ReadLog returns a smart run's log lines after AfterSeq.
func (b *Bound) ReadLog(args ReadLogArgs) (LogPage, error) {
	if args.ID == "" {
		return LogPage{}, ipcerr.Required("id")
	}
	return ipcerr.InternalResult(b.Svc.Runs.ReadLog(args.ID, args.AfterSeq))
}

// McpServers lists the MCP servers the agent's CLI reports.
func (b *Bound) McpServers() ([]harness.UserServer, error) {
	return ipcerr.InternalResult(b.Svc.McpServers(context.Background()))
}

// NextFiresArgs asks for the next fire instants of a cron expression.
type NextFiresArgs struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
	Count    int    `json:"count"`
}

// NextFires answers the next Count (default 3) fire instants as unix milliseconds.
func (b *Bound) NextFires(args NextFiresArgs) ([]int64, error) {
	count := args.Count
	if count <= 0 {
		count = 3
	}
	fires, err := scripts.NextFires(args.Cron, args.Timezone, time.UnixMilli(b.Svc.now()), count)
	if err != nil {
		return nil, ipcerr.BadRequest(err.Error())
	}
	out := make([]int64, 0, len(fires))
	for _, f := range fires {
		out = append(out, f.UnixMilli())
	}
	return out, nil
}

// ScheduleArgs names a scheduled script; Secrets fill its secret params for this call only.
type ScheduleArgs struct {
	ScriptID string              `json:"scriptId"`
	Secrets  map[string][]string `json:"secrets"`
}

// SchedulePreview resolves the run a script's schedule would start.
func (b *Bound) SchedulePreview(args ScheduleArgs) (Preview, error) {
	if args.ScriptID == "" {
		return Preview{}, ipcerr.Required("scriptId")
	}
	pv, err := b.Svc.SchedulePreview(args.ScriptID, args.Secrets)
	if err != nil {
		return Preview{}, ipcerr.Wrap(err)
	}
	return pv, nil
}

// ScheduleStartArgs is ScheduleArgs plus the hash of the preview the user confirmed.
type ScheduleStartArgs struct {
	ScheduleArgs
	Hash string `json:"hash"`
}

// RunScheduleNow starts a script's scheduled run at once.
func (b *Bound) RunScheduleNow(args ScheduleStartArgs) (Started, error) {
	if args.ScriptID == "" {
		return Started{}, ipcerr.Required("scriptId")
	}
	started, err := b.Svc.RunScheduleNow(args.ScriptID, args.Hash, args.Secrets)
	if err != nil {
		return Started{}, ipcerr.Wrap(err)
	}
	return started, nil
}

// ConfirmArgs answers a waiting run: its id, the preview hash and the secret values asked.
type ConfirmArgs struct {
	RunID   string              `json:"runId"`
	Hash    string              `json:"hash"`
	Secrets map[string][]string `json:"secrets"`
}

// ConfirmAccept starts a waiting scheduled run.
func (b *Bound) ConfirmAccept(args ConfirmArgs) (Started, error) {
	if args.RunID == "" {
		return Started{}, ipcerr.Required("runId")
	}
	started, err := b.Svc.ConfirmAccept(args.RunID, args.Hash, args.Secrets)
	if err != nil {
		return Started{}, ipcerr.Wrap(err)
	}
	return started, nil
}

// ConfirmDecline ends a waiting scheduled run without running it.
func (b *Bound) ConfirmDecline(args IDArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	b.Svc.ConfirmDecline(args.ID)
	return nil
}

// RestoreState lists the runs the last quit or crash ended, planned against their scripts now.
func (b *Bound) RestoreState() (RestoreState, error) {
	state, err := b.Svc.RestoreState()
	if err != nil {
		return RestoreState{}, ipcerr.Wrap(err)
	}
	return state, nil
}

// Restore starts the runs the user confirmed from that offer.
func (b *Bound) Restore(args RestoreArgs) (RestoreResult, error) {
	res, err := b.Svc.Restore(args)
	if err != nil {
		return RestoreResult{}, ipcerr.Wrap(err)
	}
	return res, nil
}

// RestoreDismiss forgets the offer.
func (b *Bound) RestoreDismiss() error {
	b.Svc.RestoreDismiss()
	return nil
}
