# P644 review round 2 findings

Base: tag `v0.3.13`. HEAD reviewed: `32e233ccf` (`chapter/v0.3.14`). Scope: `git diff v0.3.13..HEAD`.
Hardest look at commits since round 1 (`ac0231eac`): `4f2b3b964`, `3189cde66`, `3ea48dc98`,
`32e233ccf` (fixer), `ab4bcb600`, `4785ba432` (test run). Second pass: SVG validator, P636 splitter,
P634 CI, P638, submit core.

Counts: 0 high, 3 medium, 3 low, 2 nit.

## Verdict on round 1 fixes

- M1 fixed. `routeContinues` (`ade/route.go:245-260`) resolves the finishing status against the step's
  results; end, stop and forward-past-last pass with no draft. One gap: L5 (skipped steps).
- M2 fixed. `FilesVars.Draft` set only when an ok result continues (`detailsprompt.go:31-33`);
  manual steps (no results) get none, which is right (they get `ade.handoverLine`). Golden and budget
  updated. Nit N1 (duplicate clause in start message).
- M3 fixed. Idle branch re-checks under `s.mu` after the unlocked `agentLive` query; a touch sets
  `seen` under the lock, so the recheck catches it. A `touch=false` send that races the delete lands
  in a dead writer's buffer, which matches the stated semantics (no live agent, no writer). Lookup
  error keeps the writer: cost one indexed query per 30 min. Goroutine exits on dropped or replaced
  writer at its next idle tick (at most 30 min after archive). No leak, no deadlock.
- M4 (attach part) fixed. Cache cleared under `s.mu` on every `markStateDirty`; `load` takes the cache
  mutex, so shared use is race-free. A render that grabbed the old pointer renders pre-change facts,
  but the same change sent it a dirty signal, so a fresh render follows. `seed` writes `seeded` without
  the mutex; only per-call caches are seeded, so no race today. `RecentRunsOfTask` keeps
  `RunsOfTask`'s exact order (reversed) and review filter: no behaviour change, bounded read.
- L1 fixed. Lock order is `w.mu` then `s.mu` on every path (`writeStateLocked`, `sharedAttach`,
  `writeArchivedState`). `ArchiveTask` holds `taskMu` while waiting on `w.mu`; `renderState` takes no
  `taskMu`, so no deadlock. A writer made after archive fails `loadTaskCtx` (archived) and writes
  nothing.
- L2 fixed, but see M7 (truncated listing).
- L3 fixed. `filepath.Rel` after `Clean`: `..` after join (`<dir>/../t2/x`) gives `../t2/x`, refused;
  `..x` names a real child, accepted. A path through a symlinked ancestor (resolved form) stays
  absolute and is refused as before, never accepted wrongly; store's per-component lstat still guards
  symlinks inside the folder.
- L4 fixed. `taskFolderDenies(dirs...)` covers own and child folders on both launch paths.
- Nit fixed (golden uses `state`, `submit`). Doc still stale: N2.
- `ab4bcb600`, `4785ba432`: test-only, correct (`dropTabs` skips the discard prompt; `putAt` orders by
  ms then id).

## Parked items

- M4 board-notification scoping and `SetupByBranch` per render: acceptable, no open item needed. With
  the shared attach cache, git spawns per tick drop to one per repo. What stays per writer is about 15
  indexed SQLite reads plus a YAML marshal, and the byte-equal skip stops the write. That is bounded,
  linear in live writers, and local: a cost, not a defect.
- M5 blob GC: not acceptable as a Known open items entry. See M8.

## Functional correctness

### M6. Slice shard passes green when `go test -list` fails

- Where: `scripts/go-test-split.sh:103-107` (`slice_tests`), `:253-257`.
- Defect: `TESTS="$(slice_tests ...)"` takes the exit status of the pipeline's last stage (`awk`), so a
  failed `go test -list` (build error, `TestMain` setup failure, init panic) yields empty `TESTS`. The
  script then prints "has no tests, nothing to run" and exits 0.
- Scenario: `flowharness.Main` fails before `m.Run` (temp home, shim setup), or a Linux-only test-file
  compile error that macOS `lint:go` does not see. Both `go (Linux, adeflow1|2)` shards go green and
  the `go (Linux)` required check passes with zero adeflow tests run.
- Severity: medium (CI gate can pass with a whole package skipped).
- Fix: in the main path list first, `_raw="$(go test -list . "$PKGS")"` (a plain assignment, so
  `set -e` aborts on a failed listing), and have `slice_tests` filter `_raw`. Also exit 1 on an empty
  slice: a named slice shard always has tests.

### M7. Submit over the 500-file cap treats unlisted indexed files as deleted

- Where: `apps/kira-space/internal/taskfiles/submit.go:185-197` (`list` keeps the truncated listing),
  `:297-301` (rows not in `present` become targets), `:387-388` (`one` calls `gone`), `:476-478`
  (`decide` deletes the row on accept); `store/local.go:218-224` returns the first 500 entries.
- Defect: on `ErrTooManyFiles` the listing is partial, but every indexed path past the cap is processed
  as gone. The new `hasEntry` (L2 fix) also answers "no such file" for a named path that exists past
  the cap.
- Scenario: agent writes 520 files (screenshots, logs). The implicit finish submit reports the cap, then
  runs `decide` with `parsed == nil` for indexed files later in path order: own files are accepted as
  deletions (`h.Remove`, `tx.DeleteFile`), user-owned ones raise a delete approval for files still on
  disk. Proof files past the cap enter the unit as removed.
- Severity: medium (silent index and cache loss).
- Fix: carry `truncated` from `list`; when set, skip the rows-not-present loop, and in `one`/`hasEntry`
  fall back to `st.Stat(rel)` before treating a path as gone or missing.

### M8. Blob GC still missing (M5 carried)

- Where: `apps/kira-space/internal/taskfiles/store/blobs.go:43-92` (`Remove` has no caller);
  `proofunit.go:293-297`; `docs/ARCHITECTURE.md` Known open items, first entry.
- Defect: plan P643 §3 says blobs are collected after accept and at archive. The fixer moved it to Known
  open items instead. Working agreement: a dropped in-scope plan item is narrowed scope nobody agreed
  to; Known open items is for real limitations, and work genuinely out of scope becomes a named
  `SPEC.md` phase, not an entry. Blob GC is inside P643's scope.
- Scenario: as round 1: resubmitted screenshots and deleted proofs grow `<home>/task-blobs` without
  bound, archived tasks included.
- Severity: medium.
- Fix (small): a sweep at start and after archive: live set = `task_files.accepted_sha256` plus every
  `sha256` in `proof_steps` payloads; delete unreferenced blobs whose mtime is older than 1 h. The age
  gate covers the put-before-commit window; make `Put` refresh mtime (`os.Chtimes`) when the blob
  exists so a reused old blob cannot be swept mid-submit. Then drop the open item. Only the user can
  instead narrow the plan.

### L5. `continues` ignores skipped steps

- Where: `apps/kira-space/internal/ade/route.go:233-241`.
- Defect: `routeIndex(plan, idx, res.Next) < len(plan)` counts a following step with `Def.Skip`, but
  `walkPath` (`steps.go:249-269`) passes over skipped steps.
- Scenario: workflow `implement`, `pr` with `pr` skipped. `implement`'s ok `done` effectively ends the
  task, yet the prompt names the draft and `SubmitForFinish` refuses the finish until the agent writes
  a draft nobody reads: the M1 cost on every such run.
- Severity: low.
- Fix: after computing the target index, advance while `t < len(plan) && plan[t].Def.Skip`, then compare
  to `len(plan)`.

### L6. SVG `<style>` check is per text chunk and resets on any end tag

- Where: `apps/kira-space/internal/proof/svg.go:81-92` (`token`), `:113`.
- Defect: `checkCSS` runs per `CharData` token, and `inStyle` clears on any `EndElement`.
  `<style>@imp<!---->ort url(http://x)</style>` splits into two clean chunks; `<style><g/>@import
  url(http://x)</style>` is unchecked after `</g>`.
- Scenario: a proof SVG with an external stylesheet passes validation. Harmless today: the only
  renderer is `<img>` (`ProofImage.vue`), which blocks external loads and scripts. The validator's own
  contract ("refuses url() to anything but a fragment or data image") is broken, and any future inline
  use would trust it.
- Severity: low (defense in depth).
- Fix: track style depth, buffer all text inside `<style>` and run `checkCSS` once at `</style>`; refuse
  child elements inside `<style>`.

## Nits

- N1. Start message with an agent current step now says the draft twice: `headless.space.files` Draft
  clause and `ade.handoverLine` (`internal/llmprompts/registry.go:858`, `:1239-1241`). Tokens only.
  Fix: drop `Draft` when the template also renders `ade.handoverLine`, or leave and accept.
- N2. `docs/ARCHITECTURE.md:4073` says the Claude load hint names "`finish_step` and `submit` for a
  run, `submit` for a session"; `sessionTools` (`ade/detailsprompt.go:42-48`) gives `submit`, `state`
  (plus `create_task`). Fix the sentence.

## Second pass, clean

- P636 splitter (`packages/shared/domain/sqlSplit.ts`): directives only at a real line start (spans
  skip multi-line strings and comments), quoted and backslash tokens follow the `mysql` client, compound
  block tracking is off under a custom delimiter, comment-only chunks dropped only with
  `delimiterDirective`. `format.ts` rebuild keeps directives and per-statement terminators. Clean.
- P634 CI: `go` aggregate fails on skipped or failed shards; `--check` proves partition and bundle
  coverage; per-bundle restore with background load then pull. Only defect is M6.
- P638 (`apifiles/project.go`, `store.go`): skipped items keep last good rows and order; edits refused
  while items are skipped. Clean.
- P635, P637, P639-P642: UI and fixture work, round 1 coverage stands; no new risk found.

## Tests for the idle-writer and archive-race paths

Required. Both are concurrency logic (ordering between the writer goroutine, the idle timer, the
archive write and agent touches), which the working agreement's test bar names. No test exercises
`runStateWriter`, `agentLive` or `writeArchivedState` today. Minimal set, internal tests in
`apps/kira-space/internal/ade`:

1. `TestStateWriterIdleKeepsLiveAgent`: make `stateIdle` a package var a test lowers (for example
   50 ms). Touch a task with a running session row, wait past idle, assert the writer is still in
   `stateFiles.writers`; mark the session stopped, wait, assert the writer is gone.
2. `TestArchiveWaitsForInFlightRender`: block a render inside `renderState` through a fake
   `Statuses.List` that waits on a channel, call `writeArchivedState` in a goroutine, release the render,
   then assert `state.yaml` shows `status: archived` and a later `markStateDirty` writes nothing. Run
   under `-race`.
