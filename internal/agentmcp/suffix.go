package agentmcp

import "github.com/kirathecat/kira-studio/internal/llmprompts"

// FinishVars is the template data of the finish instruction for a step's results.
func FinishVars(results []ResultSpec) llmprompts.FinishVars {
	out := llmprompts.FinishVars{Implicit: IsImplicit(results), Results: make([]llmprompts.ResultVar, len(results))}
	for i, r := range results {
		out.Results[i] = llmprompts.ResultVar{ID: r.ID, Description: r.Description, OK: r.OK}
	}
	return out
}

// FinishInstruction is the finish instruction that ends every agent step prompt. It is not in the
// workflow YAML and cannot be removed.
func FinishInstruction(src llmprompts.Source, results []ResultSpec) string {
	return llmprompts.HeadlessFinishStepSuffix.Text(src, FinishVars(results))
}

// PlainFinishInstruction is FinishInstruction for a run with no Space grant, which ends with finish_step.
func PlainFinishInstruction(src llmprompts.Source, results []ResultSpec) string {
	v := FinishVars(results)
	v.Plain = true
	return llmprompts.HeadlessFinishStepSuffix.Text(src, v)
}

// SpaceNote tells an agent to create branches only through state branch.request.
func SpaceNote(src llmprompts.Source) string {
	return llmprompts.HeadlessSpaceSuffix.Text(src, llmprompts.NoVars{})
}

// RebaseReport ends every rebase run's prompt, edited ones included.
func RebaseReport(src llmprompts.Source) string {
	return llmprompts.HeadlessRebaseReportSuffix.Text(src, llmprompts.NoVars{})
}

// ScriptReport ends every smart script prompt, one-off edits included.
func ScriptReport(src llmprompts.Source) string {
	return llmprompts.HeadlessScriptReportSuffix.Text(src, llmprompts.NoVars{})
}
