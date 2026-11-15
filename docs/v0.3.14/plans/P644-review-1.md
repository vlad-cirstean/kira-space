# P644 review round 1 findings

Base: tag `v0.3.13`. HEAD reviewed: `11680917ea69d9a5d6c6e2696343cf3cd63afadf` (`chapter/v0.3.14`).
Scope: `git diff v0.3.13..HEAD` (142 commits), by area per `SPEC.md` P634–P643. Most effort on P643.

Counts: 0 high, 5 medium, 4 low, 1 nit.

## SubmitForFinish verdict

`SubmitForFinish` (`apps/kira-space/internal/ade/detailsmcp.go:87-114`), called from `finishRun`
(`internal/agentmcp/mcp.go:400`), refuses an ok `step.finish` with no `handovers/draft-<step>.md`.
Plan line 363 and Q7 (line 1705) say: refuse ok with no draft *while route continues*. No proof-only
exception exists in plan. So refusal itself is correct, proof-only stages included: next stage needs
handover. Two defects sit around it: M1 (route-continues test missing) and M2 (prompt never names
draft, so every headless stage pays one refusal).

## Functional correctness

### M1. ok finish refused even when route ends

- Where: `apps/kira-space/internal/ade/detailsmcp.go:87-114`; caller `internal/agentmcp/mcp.go:382-425`.
- Defect: refusal condition is `draft == "" && ok && !noStageRun(runID)`; it never checks whether ok
  result's route continues.
- Scenario: last stage of a task, or ok result with `Next == "end"`, finishes ok with no draft.
  Refused, agent writes a draft nobody reads, retries. Plan says this case passes and
  `finishRun` uses `args.Reason` as summary (mcp.go:410 already does that).
- Severity: medium.
- Fix: pass result status into `SubmitForFinish(ctx, call, status string, ok bool)`; resolve status
  against current step's results (`stepResultsOf(tc, cur)`, `adewire.StepResult.Next`); skip refusal
  when `Next == RouteEnd` or route resolves past last step. `ResultSpec` in agentmcp lacks `Next`, so
  resolve on ade side.

### M2. Headless step prompt never names draft path

- Where: commit `c0f2960cf` removed `Draft` from headless files fragment,
  `apps/kira-space/internal/ade/detailsprompt.go`; golden
  `internal/llmprompts/testdata/golden/ade.step.prompt.files.txt`.
- Defect: commit message claims "stage handover line and refused step.finish already name the draft",
  but `AdeHandoverLine` (`internal/llmprompts/registry.go:1270-1276`) renders only in manual, review
  and start messages, never in headless step prompt.
- Scenario: every headless stage run with continuing route: ok `step.finish` refused, agent writes
  draft, calls again. One extra round-trip plus tokens per stage, every run. Fakes in
  `runengine_test.go` (`fakeFinish`) now always take refusal path, so tests hide the cost.
- Severity: medium (cost, every run).
- Fix: put draft path back as one clause in finish suffix ("Before an ok step.finish, write your
  handover to handovers/draft-<step>.md"), only when route continues. Not a separate "then call
  submit" order (that caused F3b detour).

### M3. state.yaml writer stops after 30 min idle while agent still reads file

- Where: `apps/kira-space/internal/ade/statefile.go:231-268` (`markStateDirty` creates writer only
  when `touch`), `:296-327` (`runStateWriter` idle exit at `stateIdle` = 30 min);
  `docs/ARCHITECTURE.md:4086`.
- Defect: writer exits after 30 min with no touch; later board, lease and child changes reach no
  writer (`touch=false` never starts one), so state.yaml goes stale while agent still works.
- Scenario: interactive session or orchestrator idles (user away, long Bash, waiting on children), then
  reads `state.yaml` with Read (no MCP call, so no touch). Children status, setup, leases are 30+
  min old. ARCHITECTURE.md claim "agents read it only while working, so each launch refreshes it"
  is false for sessions and orchestrators.
- Severity: medium.
- Fix: in idle branch, keep writer while task has a live run or open session; or let
  `markStateDirty(touch=false)` start a writer when task has a live agent. Fix ARCHITECTURE entry.

### L1. writeArchivedState races in-flight live write

- Where: `apps/kira-space/internal/ade/statefile.go:398-434` vs `:361-396`.
- Defect: `writeArchivedState` neither takes `w.mu` nor stops writer goroutine.
- Scenario: archive lands while writer has rendered live document; writer's rename lands after
  archived write, so archived task shows live state.yaml (until next change, which never comes).
- Severity: low.
- Fix: take `w.mu` and stop/mark writer archived before writing.

### L2. submit naming a missing path answers silently "accepted 0"

- Where: `apps/kira-space/internal/taskfiles/submit.go:337-399` (`one`), `:340-342`, `:351-352`,
  `:414-417` (`gone`).
- Defect: named path with no file and no row returns empty `FileResult`; `r.add` drops empty `Path`.
- Scenario: agent submits `notes/plan.md` but wrote `notes/plans.md`. Reply says accepted 0, no
  error; agent believes submit worked.
- Severity: low.
- Fix: when path was named explicitly, return invalid "no such file in task folder".

### L3. Absolute path inside task folder rejected

- Where: `apps/kira-space/internal/taskfiles/submit.go:274-305` (`targets`).
- Defect: absolute path is answered invalid "not a path inside the task folder" even when it is
  inside.
- Scenario: agent passes path from its own Write call (absolute); submit refuses; retry costs a turn.
- Severity: low.
- Fix: strip task-folder prefix after `filepath.Clean`, then apply existing checks.

### Nit. Stale golden Load hint

- Where: `internal/llmprompts/golden_test.go:68`.
- Defect: golden input uses `mcp__kira-space__finish_step`; production `sessionTools` gives `submit`
  and `state`. Golden pins a tool name agents never see.
- Fix: use production tool names.

## Performance

### M4. Board-wide change renders every live state writer, with git spawns

- Where: `apps/kira-space/internal/ade/boardcache.go:161-182` (`notifyBoard` → `markStateFor(allChanged)`
  → `markAllStateDirty`); `leases.go:133`; `statefile.go:495-548` (`renderState`).
- Defect: 63 unscoped `notifyBoard()` sites (refresh passes included, `board.go:982`, `:985`) dirty
  every live writer; each render runs `spaceInfo` → `newAttachCache` → `BranchInventory` (one git
  spawn per repo, `worktreedir.go:159-187`), plus `SetupByBranch` (whole space), `RepoConfig.List` and
  unbounded `RunsOfTask`.
- Scenario: 10 live tasks, 5 repos, board refresh tick: 50 git spawns plus 10 whole-space queries per
  150 ms window. Byte-equal skip saves the write, not the render.
- Severity: medium.
- Fix: share one attach cache per coalesced tick across writers; scope board notifications that
  cannot change a task's state.yaml; bound `RunsOfTask` with a limit query.

### M5. Blob store never collected

- Where: `apps/kira-space/internal/taskfiles/store/blobs.go` (`Remove` has no GC caller);
  `proofunit.go:293-297` (blobs Put before transaction); plan §3 line 190; `docs/ARCHITECTURE.md:3598`.
- Defect: plan says blobs are "collected after accept and at archive"; nothing collects. Put-before-tx
  also orphans blobs on failed transactions.
- Scenario: proofs with screenshots resubmitted over months: `<home>/task-blobs` grows without bound,
  deleted and archived tasks included.
- Severity: medium (disk, plan item dropped without Known open items entry).
- Fix: sweep blobs unreferenced by `proof_steps`/`task_files` after accept, delete and archive (or at
  start); else add Known open items entry naming it.

## Architecture, security, maintainability

### L4. Child task folders added to AddDirs without deny rules

- Where: `apps/kira-space/internal/ade/taskfolder.go:77-107` (`withTaskFolder`,
  `withTaskFolderArgs`); `:62-69` (`taskFolderDenies`); `sandbox.go:85-89`, `sandbox_tui.go:148-151`.
- Defect: child folders join AddDirs, but `taskFolderDenies` covers own folder only (state.yaml,
  links.yaml, task.yaml).
- Scenario: with sandbox off, orchestrator Edit/Write on child's `state.yaml` passes tool permission;
  writer overwrites it, so damage is transient. Sandbox on: Allow covers own dir only, so no write.
- Severity: low.
- Fix: emit same deny set for each child folder.

### Clean or proportionally covered

- Store symlink and hard-link handling (`taskfiles/store/local.go`): per-component lstat walk, Read
  checks `os.SameFile`. `WriteAtomic` (227-262) has narrow lstat-to-rename TOCTOU; requires an attacker
  inside task folder racing Kira, not worth a change.
- branch.request detach (`branchrequest.go:435-524`): 10 min wait under 35 min `MCPCallTimeout`, late
  answers recorded, `Close` abandons all, `withdrawOwnerBranchCalls` clears detached calls. Clean.
- Migrations 0101, 0102: versioned. Clean. `PurgeTaskless` on every start is deliberate (Q16), cheap.
  `taskfiles/migrate.go` per-start marker queries: cost low, no finding.
- CLI-credential rule: no credential reads in diff. Clean.
- Mobile approvals (`internal/mobileweb/approvals.go`): phone kinds only, cwd/location/worktree
  stripped, answer limited to pending phone kinds, 50-ID cap. Clean.
- Frontend: all 33 changed `.vue` files `<script setup lang="ts">`, no `<style>`, no `defineComponent`;
  `useElementSize`, shadcn Dialog/Alert, TanStack hooks, Pinia `tabClose` store. Clean.
- Light coverage, not called clean: SVG validator, P636 splitter, P634 CI changes.
