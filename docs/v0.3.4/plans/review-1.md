# v0.3.4 code review round 1

Base: `aaacdfaf0` (tag `v0.3.3`). Head: `3e792e331`. Scope: `git diff aaacdfaf0..HEAD`, generated files and
`docs/HISTORY.md` content skipped.

## Covered

- P569 proof image steps: `internal/proof/image.go`, `internal/proof/service.go`,
  `internal/storage/repos/proof.go`, migration 0092, bridge `proof.go`/`proofshots.go` SaveJpeg,
  `ProofImage.vue`, `ProofStepView.vue`, `menuactions.go`, `shell/menu.go` ItemAction, dbmcp endpoint.
- P570 agent-chosen branch base: `internal/ade/branchrequest.go` (planNamed, planNew, agentBase,
  baseNameRefusal, createApproved).
- P571 orchestrator tasks: `internal/ade/taskcreate.go`, `internal/ade/orchestrator.go`,
  `internal/ade/runnote.go`, `internal/ade/handover.go`, `internal/ade/runs.go` grant and advance
  paths, `internal/ade/launches.go`, `internal/agentmcp/orchestrator.go`,
  `internal/storage/repos/adetask.go`, `internal/storage/repos/adehandover.go`, migration 0093 plus
  testdata schema, `adeflow/orchestrator_test.go`, frontend board/next-pass/plan-model changes.
- P572 About link: `appupdate.RepoURL`, `AboutPane`, `useAppInfo`.
- P573 docs history: `scripts/docshistory` changes.
- Full-run fixes (`full-run.md` commits).

Paths below are under `apps/kira-space/` unless they start with `internal/` at repo root.

## No findings

- Frontend rules: every new `.vue` is `<script setup lang="ts">`, no `<style>` block, shadcn-vue
  primitives, Pinia stores, TanStack Query (`useNextPass` invalidates board and handover keys).
- CLI credentials: nothing new touches CLI credential material.
- Migrations 0092, 0093: indexes recreated, self-FK names `ade_tasks`, testdata schema matches.
- P570 base validation: base goes through `branchNameRefusal` (check-ref-format, leading `-` refused)
  and resolves only against inventory or planner names. No git argument injection.
- P571 grant and depth: orchestrator grant set server-side from task kind (`runs.go:871-873`,
  `launches.go:432`), withheld from rebase and review runs (`runs.go:823-825`); `CreateChildTask`
  checks in one transaction that parent is a live, parentless orchestrator, so depth stays 1.
  `create_task` goes through the approvals queue; no bypass found.
- P572, P573: none.

## A. Child finish mirror

### F1 (medium, functional) Duplicate `child` row in parent chain when child's last step is automatic

`internal/ade/runs.go:1241-1246`, `internal/ade/runs.go:1289`, `internal/ade/handover.go:123-178`,
`internal/storage/repos/adehandover.go:334-350`, `internal/storage/repos/adetask.go:1294-1332`.

Run end calls `recordRunHandoverLocked`. Past the last automatic step it targets `entryDone` and calls
`RecordRunHandover` (mirrors when `ToStepID == done`) or `MirrorFinish` when there is no text. Then
`advanceLocked` gets `actComplete` and calls `SetEntryStep(taskID, entryDone, nil, nil)`, which mirrors
again whenever `stepID == entryDone`, even with nil handoff.

Scenario: child workflow `implement (agent) -> test (script)`. Test run passes. Parent chain gets two
`child` rows: one with the handover body (or "finished without text"), one with empty body and empty
`from_step_name`. Parent UI shows duplicate finish entry; `task_chain` hands orchestrator agent the
duplicate. `TestOrchestratorChildChain` (`adeflow/orchestrator_test.go:340-400`) finishes only via
`StepDone` on a manual last step, so this path is untested.

Fix: mirror in one place only, for example skip `SetEntryStep`'s mirror when handoff is nil and the
run path already mirrored, or dedupe per finish. Add flow test with automatic last step.

## B. Proof image

### F2 (medium, performance) `proof_save` path steps bypass body cap: unbounded retention and decode cost

`internal/proof/service.go:310-324`, `internal/proof/image.go:40-73`,
`internal/storage/repos/proof.go:380-400`.

8 MiB body cap does not cover image steps given by `data.path`. `Save` vets all steps (up to 100)
before any `AddStep`, keeping each ~3 MiB payload in `vet`. 32 MiB session cap
(`ProofMaxSessionBytes`) applies only in `AddStep`, so one call holds ~300 MiB before refusal at
around step 11. Each step also runs a full `jpeg.Decode` (`image.go:73`). 25 MP progressive 4:4:4
JPEG of uniform content fits under 3 MiB but makes Go `image/jpeg` allocate ~300 MB of coefficient
blocks plus ~75 MB YCbCr image per decode. 100 such paths (same file repeated works) cost minutes of
CPU and GB-scale allocation; no ctx cancellation.

Fix: sum payload against `ProofMaxSessionBytes` (minus stored bytes) during vetting and refuse early.
Bound decode cost: lower pixel cap for progressive files, or replace full decode with bounded
integrity check. Pass ctx and check it between steps.

### F3 (low, security) `readImageFile` errors probe files outside agent sandbox

`internal/proof/image.go:61-71`, `internal/proof/image.go:100-116`.

Kira process reads `data.path`, not the agent's sandboxed shell. Error text tells apart: unreadable
or missing, over cap (with exact size in KiB), not JPEG, dimensions (with width and height).
Seatbelt-sandboxed agent learns whether a path it cannot read exists, its size and its type. P569
plan claims agent cannot probe beyond its own shell; this contradicts it.

Fix: one uniform message for every path-source failure before JPEG validation passes; omit size and
dimensions for path sources.

## C. Orchestrator `list_tasks`

### F4 (low, performance) `ListTasks` N+1 queries

`internal/ade/orchestrator.go:157-177`, `internal/ade/orchestrator.go:190`.

Per child (up to 100): `fillChildStep` calls `loadTaskCtx`, which re-reads `GetTask`, `BranchesLive`
(every live branch in space), `RepoConfig.List`, workflow and runs; loop also calls `LatestHandover`.
Orchestrator with 100 children triggers ~500 queries and 100 full live-branch scans per call;
agent may poll it.

Fix: load `BranchesLive` and `RepoConfig` once per call, batch latest handovers per child set.

### F5 (low, functional) Archived child reports raw step id

`internal/ade/orchestrator.go:186`.

Archived, unfinished child gets `info.Step = t.EntryStepID`; live child gets step name
(`stepName(tc, cur)`). Orchestrator agent sees `step: "s3f9..."` for one and `step: "Review"` for
another, cannot match archived child to a workflow step by name.

Fix: resolve name from task's workflow snapshot (`taskWorkflow`), fall back to id.

## Counts

High 0, medium 2, low 3.
