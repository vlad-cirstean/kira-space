# v0.3.20

Chapter v0.3.20, branch `chapter/v0.3.20`, tag `v0.3.20`. First phase `P694` (highest history phase: `P693`).

| # | Phase | Status |
|---|-------|--------|
| P694 | Agent prompts: when to write proofs, where the task Details live | Planned |
| P695 | Task files: watch on disk, submit narrowed, errors in state, no human diff for latest AI edits | Planned |
| P696 | Dialogs: remove redundant buttons (audit every dialog) | Planned |

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

## Streams

P696 is independent and can run as its own stream in parallel. P694 then P695 are sequential unless the
P695 plan proves no shared file. Cap 3 concurrent streams (`docs/WORKING_AGREEMENT.md`).
