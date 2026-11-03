# P503 code review findings

Base `640871e5` (P493 review findings drop, v0.2.29). Tip `98358c2a`. Scope: `git diff 640871e5..HEAD`
(P495–P502, post-landing fixes, full-run fix `f98df9ec`). All fixer commits treated as unreviewed.

Counts: 0 High, 3 Medium, 4 Low.

Areas with no real finding: migration 0066 and `review_removed` schema (additive, FK cascade on);
`review.removed.*` param validation (repoId, refs, 1–1000 positive ids); session-id threading
(`prepareWithGrant` presets `RecordID`, `Tracker.Prepare` honours it, resume keeps the record id,
run grants carry the run's session); `UpdateArtifact` (bridge + engine validation, task ownership
check); resume action selection (`resumableSession`) and Start-new menu entry; branch row overflow
(every overflow kind outside `MENU_KINDS` is reachable through `fixItems`); `adeAgentsUi` leave
guard wiring in `App.vue`/`AdeShell.vue`; `adePanelTabs` typed tabs and pruning; card tag icons.

## gitsession/removed.go: capture marks a record checked after capturing nothing

**M1 — Medium.** `apps/kira-space/internal/gitsession/removed.go:61-106` (`captureCommitted`,
`captureWorking`), `:194-229` (`captureCommittedRecord`), `:264-285` (`captureWorkingRecord`).

Capture relies on parsed hunks. Hunks are nil whenever the delta is not a parsed text patch:
- `FileDelta`/`contentDelta` hit `MaxPatchBytes` (1 MiB) and return `BodyTooLarge` with no hunks
  (`incremental.go:415-438`). A reviewed text file over ~500 KiB that is rewritten, or one near
  the 1 MiB snapshot cap that is deleted, produces a patch over the cap.
- the current side became binary (`git diff` prints "Binary files differ", no hunks), or
  `ContentTooLarge` / binary on the working side (`captureWorking` only handles `cur.Kind` text and
  absent).
- the renamed-path branch skips silently when the new content is missing, binary or over
  `MaxSnapshotBytes` (`:213-226`).

In every case `DeletedRanges` returns nothing, no row is stored, then `SetRemovalChecked` records
the current oid/hash. The reviewed lines are never offered again for that current side, and a
later re-mark overwrites the snapshot. Failure scenario: agent replaces a reviewed 600 KiB
generated file wholesale; the review window shows no removed-since-review entry, though every
reviewed line left the branch.

Fix: when the delta is not a parsed text patch (`body.Kind == BodyTooLarge`, binary current, or
current absent), capture the record's full reviewed ranges (as `captureWorking` already does for
`ContentAbsent`), and for a text-to-text too-large patch fall back to the reviewed ranges or leave
`removal_checked` unset so a later pass retries. Apply the same to `markFileAt`/`markWorkingLocked`,
which call the same helpers before overwriting the snapshot.

## review.removed.list: heavy recompute on every working refetch; one failure hides everything

**M2 — Medium.** `apps/kira-space/frontend/src/ade/v2/review/useReviewRemoved.ts:58-59`,
`apps/kira-space/internal/gitsession/removed.go:142-192`.

`watch(() => working.dataUpdatedAt.value, invalidate)` refetches the removed list, undebounced,
on every successful uncommitted-list refetch. `dataUpdatedAt` changes even when data is identical.
The working list refetches on every stat-stamp change, agent hook and working mark. So while an
agent edits, each refetch runs `RemovedSinceReview`: `rangeFilesRecords` (name-status diff
merge-base..tip, `Records`, `blobOIDs`, and a `Touch` write), then `Records` again, `blobOIDs` again
over every reviewed record, then `Removed`. The working-origin capture it is meant to surface
already happened inside `ReviewWorking`; only the stored-row read is needed.

Same function: any range error (`ErrUnrelatedHistories`, a git failure) returns before
`e.review.Removed`, and `AdeReviewRemoved.vue` renders nothing on error. Stored entries, working
ones included, disappear until the range computes again.

Fix: on working refetch, use the debounced `invalidateSoon` and skip refetch when the working data
is unchanged (compare `data`, not `dataUpdatedAt`). In `RemovedSinceReview`, reuse the records
`rangeFilesRecords` already loaded (its second return), skip the commit capture pass when the tip
equals the last checked tip, and on a range error still return `e.review.Removed` (log the capture
error) so stored entries stay visible.

## AdeLinkGroups.vue: any URL holding a key-shaped token routes as Jira

**M3 — Medium.** `apps/kira-space/frontend/src/ade/v2/details/AdeLinkGroups.vue:97-134`, with
`apps/kira-space/frontend/src/ade/v2/jira.ts:1-6`.

`parseJira` matches `[A-Z][A-Z0-9]+-\d+` anywhere in the input. `addValue` acts on `jira.key`
before looking at the URL. On a task with no Jira link (`:100`), pasting
`https://github.com/org/repo/tree/PROJ-42-fix`, a Confluence page `.../pages/123/ADR-12+Title`, or
any URL with `UTF-8`/`SHA-256` in it sets the task's Jira field to that key with that URL. On a
task that has a Jira link (`:123`), the same URLs are stored as `issue` artifacts instead of being
offered to the host slot or stored as a link the backend kinds itself.

Fix: treat a URL as Jira only when it is a Jira issue URL (path `/browse/<KEY>` or a
`selectedIssue=<KEY>` query, or the host of the task's existing Jira link). Keep the bare-key path
for non-URL input. Every other URL goes to the host slot / `link` artifact path.

## handover.go: merged run handover drops the draft author

**L1 — Low.** `apps/kira-space/internal/ade/handover.go:143-149`.

When a session's draft exists and run summaries are appended, the handed row gets
`run.SessionID`; `applyHandoff` deletes the draft row. The interactive session that wrote the
draft body loses its link in Notes and the timeline, though its text leads the body.
Fix: keep `draft.SessionID` when the draft body is non-empty (it is the first author), or only set
`run.SessionID` when there is no draft body.

## Dead or stale code

**L2 — Low.**
- `apps/kira-space/internal/gitsession/working_review.go:345-348`: `intersectRanges` is now a
  one-line wrapper of `gitreview.Intersect`. Call `gitreview.Intersect` at `:493` and delete it.
- `apps/kira-space/frontend/src/ade/v2/artifacts/AdeArtifactList.vue:17,43`: its only caller,
  `stage/AdeStageDialog.vue:125`, always passes `compact`, so the non-compact title is dead. That
  caller also still passes `readonly`, which the component no longer declares; it falls through as
  a stray `readonly` attribute on the root div. Drop the `compact` prop and title, and the
  `readonly` attribute at the call site.

## Repeated per-render derivations in templates

**L3 — Low.**
- `apps/kira-space/frontend/src/ade/v2/panel/AdeStepBlock.vue:297-305`: `target(rl.run.branchId)`
  runs 4 times per run line, each a `rows.find` plus `rows.some`. Fold it into the `runs` computed
  (`:96-99`) so each line carries its target once.
- `apps/kira-space/frontend/src/ade/v2/panel/AdeMainBranches.vue:102-154`: `prs.get(row.id)!` is
  repeated 14 times with non-null assertions. Build a computed list of `{ row, pr }` (or a small
  PR-line component taking `pr`) and bind once.

## AdeLinkGroups.vue: paste submit has no in-flight guard

**L4 — Low.** `apps/kira-space/frontend/src/ade/v2/details/AdeLinkGroups.vue:136-153`.

Enter while a submit is still awaiting runs `submit` again on the same text. URL artifacts dedupe
on `(task_id, url)`, but a bare Jira key is stored with `url = ''` (`:104-112`), so a double Enter
stores two identical `issue` rows. Fix: wrap `submit` in `useAsyncAction` (as `useTaskAction`
does) and ignore calls while pending.
