package bridge

import (
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// ScriptRunsService is this app's binding-name shim over scriptruns.Bound.
type ScriptRunsService struct {
	Runs *scriptruns.Bound
}

// ChannelScriptRunsChanged pushes one changed script run to every window.
const ChannelScriptRunsChanged = "kira:scriptRuns:changed"

// ChannelScriptRunLog pushes the log lines a smart run just stored: {runId, chunks}.
const ChannelScriptRunLog = "kira:scriptRunLog:appended"

func (s *ScriptRunsService) List(args scriptruns.ListArgs) ([]scriptruns.Run, error) {
	return s.Runs.List(args)
}

func (s *ScriptRunsService) Get(args scriptruns.IDArgs) (scriptruns.Run, error) {
	return s.Runs.Get(args)
}

func (s *ScriptRunsService) Stop(args scriptruns.IDArgs) error {
	return s.Runs.Stop(args)
}

func (s *ScriptRunsService) ResolveDir(args scriptruns.ResolveDirArgs) (scripts.Dir, error) {
	return s.Runs.ResolveDir(args)
}

func (s *ScriptRunsService) Preview(args scriptruns.RunArgs) (scriptruns.Preview, error) {
	return s.Runs.Preview(args)
}

func (s *ScriptRunsService) Start(args scriptruns.StartArgs) (scriptruns.Started, error) {
	return s.Runs.Start(args)
}

func (s *ScriptRunsService) Resume(args scriptruns.IDArgs) (scriptruns.Started, error) {
	return s.Runs.Resume(args)
}

func (s *ScriptRunsService) ResumeCommand(args scriptruns.ResumeCommandArgs) (scriptruns.ResumeLaunch, error) {
	return s.Runs.ResumeCommand(args)
}

func (s *ScriptRunsService) ReadLog(args scriptruns.ReadLogArgs) (scriptruns.LogPage, error) {
	return s.Runs.ReadLog(args)
}

func (s *ScriptRunsService) McpServers() (agentcfg.ServerList, error) {
	return s.Runs.McpServers()
}

func (s *ScriptRunsService) NextFires(args scriptruns.NextFiresArgs) ([]int64, error) {
	return s.Runs.NextFires(args)
}

func (s *ScriptRunsService) SchedulePreview(args scriptruns.ScheduleArgs) (scriptruns.Preview, error) {
	return s.Runs.SchedulePreview(args)
}

func (s *ScriptRunsService) RunScheduleNow(args scriptruns.ScheduleStartArgs) (scriptruns.Started, error) {
	return s.Runs.RunScheduleNow(args)
}

func (s *ScriptRunsService) ConfirmAccept(args scriptruns.ConfirmArgs) (scriptruns.Started, error) {
	return s.Runs.ConfirmAccept(args)
}

func (s *ScriptRunsService) ConfirmDecline(args scriptruns.IDArgs) error {
	return s.Runs.ConfirmDecline(args)
}

func (s *ScriptRunsService) RestoreState() (scriptruns.RestoreState, error) {
	return s.Runs.RestoreState()
}

func (s *ScriptRunsService) Restore(args scriptruns.RestoreArgs) (scriptruns.RestoreResult, error) {
	return s.Runs.Restore(args)
}

func (s *ScriptRunsService) RestoreDismiss() error { return s.Runs.RestoreDismiss() }
