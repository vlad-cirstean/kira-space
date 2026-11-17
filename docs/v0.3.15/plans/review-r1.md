# v0.3.15 code review, round 1

Base commit: `7c65b1f5` (v0.3.14 merge; no earlier review session in this chapter).
Head commit: `37f681c9`.
Scope: `git diff 7c65b1f5..37f681c9`, phases P646 to P654. Reviewer read changed files plus one-hop
callers (updater, retention, metrics/agentproc, prompt dialogs, configsvc removal, smart step,
task-list frontend, tests).

Severity counts: High 1, Medium 1, Low 5.

## Findings

### R1-1 High: prompt chords reuse editor chords, so a focus steal answers a prompt

- Files: `packages/shared/domain/shortcuts.ts:69-73` (approve reuses `form.submit` Mod+Enter;
  `prompt.deny` is Mod+Backspace), `packages/workbench/src/state/promptGate.ts:8,27-31`,
  `packages/workbench/src/approvals/ApprovalsDialog.vue:44-60`,
  `packages/theme/src/components/ui/dialog/DialogContent.vue:75-81`. Same chords in
  `PairingRequestDialog.vue`, `GitCredentialDialog.vue`, `ProofReadyPrompt.vue`, `UpdateDialog.vue`.
- Defect: the typing gate only waits `PROMPT_QUIET_MS` (4 s) after the last key. Then the dialog
  opens and moves focus to its root, even though focus was still in the editor. Mod+Enter is also
  `view.run` (`shortcuts.ts:42`) and `form.submit` app-wide. Mod+Backspace is `grid.deleteRows` and
  `tree.delete` on macOS (`shortcuts.ts:66,100`). P649 asked that typing never approves or denies;
  the two-key chord is the very chord users press in the surface they were in.
- Failure scenario: user writes SQL in the query editor, reads it for 5 s, presses Cmd+Enter to run.
  An agent's db.query or task.create approval opened during the pause and took focus; Cmd+Enter
  approves it unread. Same for a user who selects grid rows, pauses, then presses Cmd+Backspace: an
  agent request is denied.
- Fix: arm prompt chords only after the dialog has been visible a short time and only for a chord
  whose modifier keydown happened after open (track a keydown of Meta/Control inside the dialog, or
  ignore chords until a keyup follows open). Alternatively give approve and deny chords no other
  surface uses (Mod+Enter and Mod+Backspace both collide), and keep the `Kbd` labels in step.

### R1-2 Medium: pre-P653 installs still jump minors; the script can step them

- Files: `scripts/install.sh:146-151,221-248`, `docs/ARCHITECTURE.md:4089` (Known open item).
- Defect: apps up to 0.3.14 run the fresh `install.sh` from `main` without `--tag`, so it installs
  `releases/latest`. Once 0.4.0 ships, any such install jumps to 0.4.x and skips 0.3.last's
  migrations, which the "Minor bumps drop legacy code" rule then no longer covers. The open item says
  the server cannot fix it without duplicating the rule in shell. But the script already parses
  release JSON with JXA (`parse-release.js`), and self-update mode can read the installed bundle's
  `CFBundleShortVersionString`. The old app accepts any `staged <tag>` line (pre-P653 `handleLine`
  checks the prefix only), so the script may pick the tag.
- Failure scenario: a user on 0.3.12 who skips updates during v0.3.15 and v0.3.16 clicks Update after
  v0.4.0 ships. The app installs v0.4.x; v0.4.0 migrations assume 0.3.last's schema and either fail
  or corrupt state.
- Fix: in self-update mode (`--wait-pid` set) with no `--tag`, list `/releases`, apply the same
  stepping rule in the JXA helper (own line's top patch, else next line's lowest), and stage that
  tag. Keep `releaseline` the source of truth with a fixture test that runs both on the same
  listing, or at minimum land this before v0.4.0 is tagged and drop the open item.

### R1-3 Low: retention eligibility differs from the updater's

- File: `scripts/releaseretention/plan.go:72-74`.
- Defect: retention finds each line's first and last release over `len(r.Assets) > 0`. The updater
  (`releaseline.NextStep`) only counts releases whose platform asset is `uploaded` and non-empty
  (`HasAsset`). The header says both share one stepping rule. `release.yml` verifies one uploaded
  asset today, so they agree now, but a hand-edited release (asset re-upload, extra file, failed
  upload left in state `new`) splits them.
- Failure scenario: v0.4.9 is published with only a checksum file or a failed dmg. Retention keeps
  v0.4.9 as "last patch of minor" and deletes v0.4.8's dmg. The updater skips v0.4.9, so an install
  on v0.4.0 steps straight to v0.5.0 and misses v0.4's patch migrations.
- Fix: pass the asset name (`-asset kira-space-macos-arm64.dmg`) and use
  `releaseline.HasAsset(r, asset)` as the `Lines` filter, the same predicate `NextStep` uses.

### R1-4 Low: retention runs the build with the write token in its environment

- File: `.github/workflows/release-retention.yml:48-76`.
- Defect: `GH_TOKEN` (`contents: write`) is set for the whole step, so `go run
  ./scripts/releaseretention` and its module download run with the write token readable. The planner
  needs no token; only the two `gh api` reads and the delete loop do.
- Failure scenario: a compromised or typo-squatted module pulled by `go run` (cache off) reads
  `GH_TOKEN` and can delete releases or push tags for the job's lifetime.
- Fix: build and run the planner in a step without `GH_TOKEN`, and set the token only on the fetch
  and apply steps (or pass `GH_TOKEN` inline to each `gh` call).

### R1-5 Low: Agents CPU misses short-lived subprocesses

- Files: `internal/metrics/sampler.go:141-142,183`, `internal/agentproc/agentproc.go`.
- Defect: CPU is a delta between two 5 s ticks per pid; a pid absent from the previous tick
  contributes nothing. Agent tool calls (git, rg, node, test runners) mostly live under 5 s, so their
  CPU never reaches Agents. P650 asked for agents and their subprocesses.
- Failure scenario: an agent runs `go test ./...` in many short child processes; the status bar shows
  Agents near 0 % while the machine is busy.
- Fix: also add reaped children's CPU of each tracked root (Linux `/proc/<pid>/stat` cutime and
  cstime; darwin `proc_pid_rusage` gives child totals via `RUSAGE_INFO` `ri_child_user_time`), taken
  as a per-root delta. Or record the limit in ARCHITECTURE next to the Agents bullet (line 1671).

### R1-6 Low: stale doc comment on explain approval

- File: `apps/kira-space/internal/dbmcp/tools.go:798-802`.
- Defect: `explainQuery` doc says the prompt "carries the composed statement"; since P649 it carries
  the agent SQL with `Explain: true` (`tools.go:776-777`).
- Failure scenario: a later change trusts the comment and drops the composed-statement read assertion
  as redundant, or a reviewer misjudges what the user approved.
- Fix: reword to "prompt raises the approval with the agent's SQL marked as an explain".

### R1-7 Low: imports in the middle of `ProofReadyPrompt` script

- File: `apps/kira-space/frontend/src/studio/proof/ProofReadyPrompt.vue:40-41`.
- Defect: two `import` lines sit after executable statements. ES hoists them, so it works, but every
  other component keeps imports at the top and Biome's organize-imports cannot sort them there.
- Failure scenario: a reader adds a use of `formatShortcut` above line 40 assuming it is not imported
  and adds a duplicate import, or an import sorter skips the block.
- Fix: move both imports into the top import block.

## Dimensions

- Architecture, structure, security: R1-1, R1-2, R1-3, R1-4, R1-6, R1-7. configsvc and ConfigFileBar
  removal is clean: no Go, TS, Vue or bindings reference remains, the only file opener left is the
  proof folder reveal, and no persisted state needs migration (the dropped
  `kira.ade.hiddenRepoIds` local key is inert). Release retention deletes only through
  `releases/assets/{id}`, never tags, release pages or `releases/latest`; workflow inputs reach shell
  only through env; `install.sh --tag` and `Installer.Stage` both validate the tag shape.
- Functional correctness: R1-1, R1-2, R1-3. Smart-step change checked: `smartStepOf` reads the same
  `wf.Steps[].SmartScript` that `workflowSteps` builds `stepDef` from, the finish summary falls back
  to the ok reason (`agentmcp.finishRun`), and sessions and non-smart runs keep the draft refusal; no
  path found where a non-smart step skips the required handover. `NextStep` stepping matches the
  spec cases; agentproc ref-counting and untrack timing (after Wait, or terminal unregister) are
  sound, with the one-tick pid-reuse window documented.
- Performance and resource efficiency: R1-5 only (accuracy, not cost). The extra darwin
  `proc_pidinfo` per pid every 5 s and the per-tick tree walk are linear and cheap; the update check
  pages at most 10 requests per 6 h.
- Frontend rules: new components are `<script setup lang="ts">`, use Tailwind utilities, shadcn-vue
  primitives (Popover, DropdownMenu, Badge, Button, Kbd), VueUse (`useMouse`, `useRafFn`,
  `refAutoReset`, `useActiveElement`, `useEventListener`), Pinia (`promptGate` holds one concern,
  `adeBoardUi.rowEdit` fits its board-UI concern) and TanStack mutations. No violation found.
- Tests: decision-heavy code has table tests (`releaseline`, retention `plan`, `groupProcesses`);
  features have backend flow halves (`appflow/update_test.go`, `appflow/metrics_test.go`,
  `adeflow` smart-step subtest, `dbmcpflow/mode_test.go`) and UI halves (`update-dialog`,
  `window-chrome`, `prompts-typing`, `approvals-payload`, `ade-v2-row-actions`). No gap against the
  Tests rules found.
