# v0.2.34

Branch `chapter/v0.2.34`. First `P` is 508 (highest in v0.2.33 is P507), last is P511.

Source: user request. Four phases. P511 is the last implementation phase. Close duty after P511: one full suite run (baselines re-record once), 1 code review round, hand-written close, PR. Each phase: Opus plan, Sonnet implementation.

| Phase | Title | Status |
|---|---|---|
| P508 | Flatter, consistent visual pass | Done |
| P509 Part 1 | Task details: free-form link add and edit | Done |
| P509 Part 2 | Task details: Main tab row grid | Done |
| P510 | Workflow inspector redesign, script sandbox, outside-repo steps | Done |
| P511 | Manual session steps get per-step sandbox | Done |

## Rows

### P508 Flatter, consistent visual pass

- Ask: v0.2.31 palette still reads inconsistent and busy. Make the whole app flatter and consistent across both apps and every shared package.
- Flat means: remove useless separators, dividers, delimiters, redundant borders and boxed containers. Separate with spacing, tone steps and typographic weight instead. Draw on modern minimalist open-source apps (Linear, Zed, Notion, shadcn/ui blocks).
- Colors: audit every surface, text, border, accent and status color. One coherent scale, no near-duplicate greys, no ad-hoc one-off hues. Everything via `packages/theme` tokens.
- Constraints: styling only. No component swaps, no features. Keep proportions and condensed density. Tailwind/shadcn tokens, no new scoped `<style>` blocks, no hard-coded colors outside tokens.
- Tests: no new tests. Baselines re-record once in the chapter's single full run, after `fc-match sans-serif` prints Inter.
- Plan: `plans/P508.md`, `plans/P508-iter2.md`.
- Result: Flatter palette on one token scale: surface and tone tokens, shared primitives and workbench, theme scale and greys folded to one set, buttons flattened, shadows, radius and dividers cut back, the old lint rules retired, test expectations updated. Styling only; no new tests. Baselines not re-recorded yet; they re-record once in the chapter's full run. Written afterwards from the commit log.

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
- Result: Manual steps with a session take a `sandbox` block (parser, writer, docs, `DefaultSandboxFor` off for script and manual). Without a session the key is refused. Migration 0069 adds `ade_session_sandbox` and rewrites started tasks' manual-step snapshots to off. `agentsandbox.Input.ReadFiles` lets the profile read launch files. `harness.InteractiveSpec` gains `Sandbox` and `DisallowedTools`; claude, opencode, fakeprovider and lean wrap the interactive launch. `LaunchStep` and Take over build the plan; `BoundService.AgentStarted` feeds the denial watch after the PTY opens. `Sessions[].sandbox` badge and `SessionSandbox` detail are on the wire; the inspector shows the Sandbox section for session steps, the TUI pane header shows the chip. Tests run: adeflow, ade, agentsandbox, harness, terminal, migrations, repos and llmprompts packages; all flows (stale contracts regenerated, prompt count 133); UI specs ade-v2-sandbox, workflows, flat-steps, launch, norepo, sessions (all pass); typecheck, lint, knip. Not run: the full suite (chapter close), real Mac Seatbelt check, real `claude` tests. Enforcement is macOS only; Linux gets flags and env only and says not enforced. Caveats: login shell rc files run before the wrapper, unconfined; a sandboxed TUI has no sandbox git tools and cannot push or fetch; `~/.claude` and keychain writes are denied, so sign-in from inside fails. Deviations: read-only repo roots use `ReadWorktrees`; with no writable worktree the task folder is writable; fixture manual steps stay at `write`; `ConfigDir` unset on the TUI policy (`ReadFiles` covers launch files).

## Results

- Full suite, run once after P511: Go `test:go:rest` 139 ok; containers 15 ok (Docker tests self-skipped); Studio UI 674; Space UI 1078; `test:unit` 1993; Space mobile 83 plus 1 skipped; ipc-frontend 7.
- Visual baselines re-recorded once, 53 images.
- Fixes from the run: i18n catalog exemption for key-by-parameter wrappers; step card `-mx-px` dropped; Workflows header buttons kept inside pane padding; automations tab test race (poll for terminal open).
- Code review round 1: 6 findings (M1, M2, L1 to L4), all fixed. Findings file dropped after the fixes landed.
- Not run: Docker-backed tests, `e2e-real`, `ui-timing`, `test:claude*`, proto/perf, real macOS Seatbelt check. The Seatbelt gap stays in ARCHITECTURE.md "Known open items".
