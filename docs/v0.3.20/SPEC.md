# v0.3.20

Chapter v0.3.20, branch `chapter/v0.3.20`, tag `v0.3.20`. First phase `P694` (highest history phase: `P693`).

| # | Phase | Status |
|---|-------|--------|
| P694 | Agent prompts: when to write proofs, where the task Details live | Planned |
| P695 | Task files: watch on disk, submit narrowed, errors in state, no human diff for latest AI edits | Planned |
| P696 | Dialogs: remove redundant buttons (audit every dialog) | Planned |
| P697 | Required services: repo scripts that must be running for a task | Planned |
| P698 | Toasts: drop operation progress and success toasts the ops panel and task already show | Planned |

P694 note: partly implemented. Resume from commit `b4a4f7808` and stash `P694-wip`; the plan's file and
test list is the source of truth. Chapter rebases onto `main` after PR #52 (`chapter/v0.3.19`) merges,
before its own PR opens.

## P695

Watch task files on disk. `submit` is used only to trigger an action in Kira, or is merged into the step
finish call. On any validation error, Kira writes the error into state (the agent's state file section).
The AI is instructed to check that file and section regularly. As long as the AI edits the latest note,
handover or proof, no human diff is shown (diff review only when something else changed).

Planner note: overlaps P694 on `apps/kira-space/internal/taskfiles/` (`readme.go`, `submit.go`,
`proofunit.go`), `internal/llmprompts/registry.go`, the finish and files prompts, and `ade/statefile.go`.
Run after P694 or give a plan file-ownership table. P694's proof line wording must stay consistent with
the narrowed submit.

## P696

Many dialogs have useless buttons, for example Cancel next to No, while every dialog also has the X close
button. Audit all dialogs (Kira Space and shared packages: shadcn-vue dialogs, confirm dialogs, step
dialogs) and fix each: no Cancel beside No, no duplicate dismiss actions where the X exists, keep buttons
that carry a distinct outcome. Deliverable: audit list (dialog, buttons before and after) in the plan, a
guard in `scripts/check-ui-primitives.sh` if a rule is mechanically checkable, updated UI specs and visual
baselines. Independent of P694 and P695 (frontend dialogs only); plan lists any overlapping files.

## P697

Repos can have dependent scripts, usually long-running processes (dev servers, local databases,
watchers). Before work on a task starts, Kira checks whether each of the repo's required scripts is
already running. If one is not, Kira asks the user to run it when starting work on that task.

Name: working title was "dependent scripts". "Required services" chosen; alternatives considered:
Companion services, Task prerequisites.

Open points for the planner (not decided here, except Detection):

- Declaration: how a repo declares which scripts are required, and where (repo config, the existing repo
  scripts under `internal/scripts` and the Scripts module, Repos config in ade).
- Detection (decided): only scripts Kira started count. "Already running" means that script's run
  (a Kira-started run record) is still running. No process scan, no port check, no health probe. A
  service started outside Kira is not detected, so the user is asked. No credential a driven CLI
  owns is touched.
- Trigger: when the check fires (task start, first session launch, resume) and what the ask looks like
  (a dialog; follow P696's dialog button rules: no redundant Cancel beside No when X exists).
- Modes: behaviour on Start, manual and headless runs, and on server builds.
- Memory: whether the choice is remembered per task or per repo.
- Ownership: whether Kira stops services it started when the task finishes (default no, ask).

## P698

Deleting worktrees, and probably other operations, shows a persistent toast that later flips to success.
Drop it. Progress is visible in the ops panel and in the task itself, so the toast is redundant.

Scope for the planner (list, not decided here): audit every toast in the app (Kira Space frontend and
shared packages: toast calls, sonner/shadcn-vue toaster usage, persistent or loading toasts that later
flip to success, success toasts for operations the ops panel or the task already shows). Build the table:
toast site, operation, where progress and result are already visible, keep or drop. Drop the redundant
ones. Keep toasts that carry information nowhere else shows: errors with no other surface, results of
operations with no ops-panel or task entry.

Check that failures still surface somewhere visible (ops panel entry, task state or a kept error toast)
and that no operation loses its only failure signal. Candidate guard in `scripts/check-ui-primitives.sh`
if a rule is mechanical. Remove copy catalog keys that become unused (dead-key guards).

## Streams

P696 is independent and can run as its own stream in parallel. P694 then P695 are sequential unless the
P695 plan proves no shared file. Cap 3 concurrent streams (`docs/WORKING_AGREEMENT.md`).

P697 is independent of P694 and P695 (prompts and task files) and overlaps P696 only on dialog
conventions. Plan it once a stream slot frees (cap 3 concurrent).

P698 is frontend only and independent of P694, P695 and P697. Overlaps P696 only on the shared dialog and
toast conventions and possibly the Ops panel files. Plan it once a stream slot frees (cap 3 concurrent).
