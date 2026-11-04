# v0.2.34

Branch `chapter/v0.2.34`. First `P` is 508 (highest in v0.2.33 is P507), last is P511.

Source: user request. Four phases. P511 is the last implementation phase. Close duty after P511: one full suite run (baselines re-record once), 1 code review round, hand-written close, PR. Each phase: Opus plan, Sonnet implementation.

| Phase | Title | Status |
|---|---|---|
| P508 | Flatter, consistent visual pass | Done |
| P509 Part 1 | Task details: free-form link add and edit | Done |
| P509 Part 2 | Task details: Main tab row grid | Done |
| P510 | Workflow inspector redesign, script sandbox, outside-repo steps | Done |
| P511 | Manual session steps get per-step sandbox | Todo |

## Rows

### P508 Flatter, consistent visual pass

- Ask: v0.2.31 palette still reads inconsistent and busy. Make the whole app flatter and consistent across both apps and every shared package.
- Flat means: remove useless separators, dividers, delimiters, redundant borders and boxed containers. Separate with spacing, tone steps and typographic weight instead. Draw on modern minimalist open-source apps (Linear, Zed, Notion, shadcn/ui blocks).
- Colors: audit every surface, text, border, accent and status color. One coherent scale, no near-duplicate greys, no ad-hoc one-off hues. Everything via `packages/theme` tokens.
- Constraints: styling only. No component swaps, no features. Keep proportions and condensed density. Tailwind/shadcn tokens, no new scoped `<style>` blocks, no hard-coded colors outside tokens.
- Tests: no new tests. Baselines re-record once in the chapter's single full run, after `fc-match sans-serif` prints Inter.
- Result: pending.

### P509 Part 1 Task details: free-form link add and edit

- Ask: Details panel items editable like Notion. One add line, type or paste, filed under the right category. No multi-step forms or dialogs for the common case.
- Plan: `plans/P509-part1.md`.
- Result: Backend classifies. `AddLink` and `EditLink` replace `AddArtifact` and `UpdateArtifact`; rules in `ade/linkclassify.go`. Stored links may be branches (migration 0067 widens the kind check). Details tab gets one add line, in-place free-text edit of stored rows and a kind menu. Flow test and UI spec cover the same add, edit and recategorise scenario.

### P509 Part 2 Task details: Main tab row grid

- Ask: Main tab rows look different and misaligned. One shared layout (columns, alignment, spacing, type scale, icon size).
- Plan: `plans/P509-part2.md`.
- Result: Shared task-panel row component. Fields, branches, workflow and Details rows sit on it; task panel dot uses `rounded-kira-sm`. UI spec asserts rows share one grid. `--radius-kira-xs` alias and its twMerge entry deleted at integration.

### P510 Workflow inspector redesign, script sandbox, outside-repo steps

- Ask: Redesign the workflow step inspector (flat rows, sandbox section). Script steps get a sandbox, default off. Agent, script and manual-session steps may run outside the repo (`outside_repo`, task folder, no worktree). Later tab shows before Plan.
- Plan: `plans/P510.md`.
- Result: Parser and wire accept `sandbox` on script steps and `outside_repo` on agent, script, manual-session steps. Script steps run behind the sandbox profile when access is not off. Outside-repo steps target the task folder once. Migration 0068 rewrites started tasks' script-step snapshots to the off sandbox; `decorateTask` hashes the re-marshalled snapshot so added wire fields never flag outdated. Inspector rebuilt with sections and a no-repo node chip. Flow tests and UI specs added. Migration renumbered from 0067 at integration (collision with P509 Part 1).

### P511 Manual session steps get per-step sandbox

- Ask: Manual session steps get the per-step sandbox config too. Sandbox disableable per step (access `off`, no wrapping) for agent, script and manual steps; inspector exposes `off` for manual sessions; a test asserts it.
- Plan: `plans/P511.md`.
- Result: pending.
