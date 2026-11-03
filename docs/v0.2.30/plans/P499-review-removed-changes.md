# P499 plan: review shows reviewed changes removed since review

Source: `docs/v0.2.30/SPEC.md` row P499. Base: `chapter/v0.2.30` at `fb865ddd`. All paths under
`apps/kira-space/` unless rooted otherwise. One phase, no Parts: storage, wire and UI land as one
order-dependent sequence (UI reads the wire, wire reads storage). One sequential implementer.

## What loses reviewed content today

Committed review (`review_file` + `review_range`, `internal/gitreview/store.go`):

1. File leaves the range diff (reverted to base, or added then deleted). `rangeFilesRecords`
   (`internal/gitsession/incremental.go:545`) lists only `changes`; the record stays in the DB, unseen.
2. Reviewed lines deleted inside a still-listed file. `ProjectRanges` (`internal/gitreview/project.go:16`)
   drops them on every read; nothing shows them.
3. Next mark of the same path. `markFileAt` (`incremental.go:771`) calls `Store.Put`, whose upsert replaces
   `content`, ranges and `reviewed_at`. Removed reviewed lines of the old snapshot are gone for good.

Uncommitted review (`review_working_file`, `internal/gitsession/working_review.go`): **yes, same loss.**

- `markWorkingLocked` (`working_review.go:164`) projects old ranges through `projectWorkingRanges`, then
  `PutWorking` overwrites the snapshot. Reviewed worktree lines deleted since are lost.
- `ade.ReviewWorking` (`internal/ade/review.go:249`) asks `WorkingReviewStates` only for dirty paths. A
  reviewed uncommitted file reverted to HEAD leaves the list; its record sits unseen.
- Promotion (`PromoteWorkingReview`) is not a loss: landed lines move to the committed review,
  `keepUnlandedWorking`/`rebaseWorkingRecord` keep the same snapshot. Capture runs after promotion, so it
  only sees lines that never landed.

So uncommitted marks are in scope (origin `working`).

## Definition

A removed entry is one contiguous block of reviewed snapshot lines that the snapshot-to-current delta
deletes (`porcelain.LineDel`, by `OldLine`). Current = branch tip at the record's path (or the rename
target, see below) for `commit`; worktree file for `working`. Reviewed = `recordRanges(rec)` /
`workingRecordRanges(rec)` (a `full` record expands to every line).

- Unreviewed lines never produce an entry (keeps today's correct behaviour).
- Reviewed line rewritten in place (del + add) counts as removed: the reviewed text is gone.
- Non-text record (`binary`/`tooLarge`, state `full`) whose current side is absent: one whole-file entry, no
  text. Non-text current side with a text record: skip for `working` (cannot diff); for `commit` the fast
  tier still yields hunks only for text, so skip there too.
- `absent` record (reviewed as deleted): nothing to lose, skip.
- Delta not parsed (`BodyTooLarge` fast tier, no hunks): skip. Known open item (see Docs).
- Rename: a record under `old` whose path has a renamed change `old -> new` with no record at `new` diffs
  the snapshot against `tip:new`, not against an absent `tip:old`. Without this every renamed reviewed file
  would read as fully removed.

Dismissed rows stay (text cleared) so capture subtracts them: a dismissed block never comes back. Rows
cascade with their `review_session`, so the idle sweep and `Purge` bound storage.

## 1. Storage (`internal/gitreview`)

### Migration `migrations/0006_p499_removed.sql`, registered in `migrations/embed.go` as `{Version: 6, Name: "p499_removed", ...}`

```sql
CREATE TABLE review_removed (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id      INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  origin          TEXT    NOT NULL CHECK (origin IN ('commit', 'working')),
  path            TEXT    NOT NULL,
  reviewed_at     INTEGER NOT NULL,  -- source record's reviewed_at, unix millis: one review event
  reviewed_at_sha TEXT    NOT NULL,  -- commit: reviewed_at_sha; working: head_sha
  snapshot_key    TEXT    NOT NULL,  -- commit: blob_oid; working: content_hash
  content_kind    TEXT    NOT NULL,
  start_line      INTEGER NOT NULL,  -- snapshot coordinates, inclusive; 0 for a non-text whole-file row
  end_line        INTEGER NOT NULL,
  content_bytes   INTEGER NOT NULL,  -- UNCOMPRESSED length; 0 when content is NULL
  content         BLOB,              -- flate(lines); NULL for non-text and once dismissed
  removed_at      INTEGER NOT NULL,  -- unix millis of capture
  dismissed_at    INTEGER            -- NULL while shown
);
CREATE INDEX review_removed_source ON review_removed (session_id, origin, path, reviewed_at);

ALTER TABLE review_file ADD COLUMN removal_checked TEXT NOT NULL DEFAULT '';
ALTER TABLE review_working_file ADD COLUMN removal_checked TEXT NOT NULL DEFAULT '';
```

No FK to `review_file`: a row must outlive the record it came from. `removal_checked` holds the current-side
identity (tip blob oid, or worktree content hash) last checked for removal; list-time capture skips a record
whose current identity equals it, so a refresh costs no diff once settled. `normalize.go`'s NFC sweep stays
untouched, same as P418's working tables: every new row's path comes from git or the record.

### `store.go` / `working.go` edits

- `FileRecord.RemovalChecked string`, `WorkingRecord.RemovalChecked string`; `Records()`,
  `WorkingRecords()` select the column. `Record()`/`WorkingRecord()` need not.
- `Put` and `PutWorking`: both upsert arms set `removal_checked = ''` (a new snapshot was never checked).
  Without this the `ON CONFLICT DO UPDATE` keeps the stale value.
- `SetRemovalChecked(ctx, repoID, branch, origin, path string, reviewedAt time.Time, key string) error`:
  `UPDATE review_file|review_working_file SET removal_checked = ? WHERE session_id = ? AND path = ? AND reviewed_at = ?`.
  The `reviewed_at` guard makes it a no-op after a concurrent re-mark.

### New `removed.go`

- `RemovedSource{Origin, Path string; ReviewedAt time.Time; ReviewedAtSHA, SnapshotKey string; ContentKind ContentKind}`.
  Origin reuses `OriginCommit`/`OriginWorking` (`comments.go`).
- `CaptureRemoved(ctx, repoID, branch string, src RemovedSource, ranges []LineRange, snapshot []byte) (int, error)`:
  one tx. `sessionID`; no session means no-op. Read `start_line,end_line` of every row (dismissed or not)
  matching `(session, origin, path, reviewed_at, snapshot_key)`. `fresh := Subtract(Normalize(ranges), existing)`.
  Insert one row per fresh range: text `SnapshotLines(snapshot, r)`, `Compress`ed, `content_bytes = len`.
  Idempotent, so two concurrent captures of one source insert each block once (SQLite serialises writers;
  the read happens inside the tx).
- `CaptureRemovedFile(ctx, repoID, branch string, src RemovedSource) (int, error)`: same tx shape; inserts
  one `start_line = end_line = 0`, `content NULL` row only when no row for the source exists.
- `Removed(ctx, repoID, branch string) ([]Removed, error)`: undismissed rows ordered `path, origin, start_line`,
  text `Decompress`ed. `Removed{ID int64; Origin, Path, ReviewedAtSHA string; ReviewedAt, RemovedAt time.Time; ContentKind ContentKind; Range *LineRange; Text []byte}`
  (`Range` nil for a whole-file row). Empty slice, never nil, when no session.
- `DismissRemoved(ctx, repoID, branch string, ids []int64) (int, error)`:
  `UPDATE review_removed SET dismissed_at = ?, content = NULL, content_bytes = 0 WHERE session_id = ? AND dismissed_at IS NULL AND id IN (...)`.
  Scoped by session: an id of another branch dismisses nothing. Returns rows affected.
- `SnapshotLines(snapshot []byte, r LineRange) []byte`: lines `r.Start..r.End` (1-based), each with its
  own trailing `\n` as stored. Line splitting matches gitsession `countLines` (a final line without `\n`
  counts).

### Range arithmetic (`ranges.go` / `project.go`)

- `Intersect(a, b []LineRange) []LineRange` in `ranges.go`. Replace the body of gitsession
  `intersectRanges` (`working_review.go:340`) with a call, so one implementation remains.
- `DeletedRanges(ranges []LineRange, hunks []porcelain.DiffHunk) []LineRange` in `project.go`:
  `Intersect(Normalize(ranges), Normalize(old lines of every LineDel line))`.
- Unit test `TestDeletedRanges` (table) in `project_test.go`: no hunks; pure insertion; deletion inside,
  straddling and outside a range; del+add replace; whole-file deletion; two hunks. This is the only new
  `gitreview` unit test (row allows tests only for new range arithmetic).

## 2. Capture (`internal/gitsession`, `internal/ade`)

New file `gitsession/removed.go`:

- `captureCommitted(ctx, branch string, rec gitreview.FileRecord, snapshot []byte, hunks []porcelain.DiffHunk, currentAbsent bool) error`:
  - text record: `DeletedRanges(recordRanges(rec), hunks)`; non-empty: `CaptureRemoved` with
    `SnapshotKey = rec.BlobOID`, `ReviewedAtSHA = rec.ReviewedAtSHA`.
  - `binary`/`tooLarge` record, `State == "full"`, `currentAbsent`: `CaptureRemovedFile`.
- `markFileAt` (`incremental.go:790`): inside the `found` branch, after `FileDelta`, call
  `captureCommitted(ctx, branch, existingRec, snapshot, delta.Hunks, delta.CurrentOID == "")` before `Put`.
  Already under `Store.Lock`. Covers `MarkFile`, `MarkFiles` and working promotion (all reach `markFileAt`).
- `RemovedSinceReview(ctx, base, branch string) (ReviewRemovedResult, error)`:
  1. `res, _, err := e.rangeFilesRecords(ctx, base, branch)` for tip and changes (reuses its errors,
     including `ErrUnrelatedHistories`). `records := e.review.Records(...)`.
  2. Current path per record: its own, or `ch.Path` for a renamed change whose `OriginalPath` is the record
     and `renameSource(records, ch) != ""`.
  3. `oids := e.blobOIDs(ctx, tip, currentPaths)` (one batch; missing reads `""`).
  4. Per record where the record has reviewed lines (or is non-text full), `oid != rec.BlobOID`, and
     `oid != rec.RemovalChecked`: take `e.review.Lock(repo, branch, rec.Path)`, re-read `Record` (snapshot),
     skip when `ReviewedAt` changed. Same path: `FileDelta(ctx, branch, path, rec, snapshot, tip)`. Rename
     (text only): `readCurrentContent(tip, newPath)` + `contentDelta(snapshot, cur)`. `captureCommitted`, then
     `SetRemovalChecked(..., OriginCommit, rec.Path, rec.ReviewedAt, oid)`.
  5. Return `e.review.Removed(...)` mapped to wire entries.
- Working, in `working_review.go`:
  - `projectWorkingRanges` also returns the hunks it computed (nil when content hash matches); keep callers
    compiling.
  - `markWorkingLocked`: when `found`, capture before `PutWorking`: text record + text `cur`:
    `DeletedRanges(workingRecordRanges(rec), hunks)`; text record + `cur` absent: all reviewed ranges;
    non-text full record + `cur` absent: `CaptureRemovedFile`. `SnapshotKey = rec.ContentHash`,
    `ReviewedAtSHA = rec.HeadSHA`, origin `working`.
  - `CaptureWorkingRemoved(ctx, branch string, read func(path string) (WorkingContent, error)) error`: every
    working record (not only dirty paths) with reviewed lines, `cur.Hash != rec.ContentHash` and
    `cur.Hash != rec.RemovalChecked`: lock path, re-read `WorkingRecord`, skip on changed `ReviewedAt`,
    capture as above, `SetRemovalChecked(..., OriginWorking, ..., cur.Hash)`. `cur.Hash` of absent is `""`;
    use `wireHash`-style token `"absent"` for the checked key so absent is distinguishable from unchecked.
  - Shared capture logic for one working record lives in one helper used by both call sites.
- `internal/ade/review.go` `ReviewWorking`: after `w.promote(ctx)`, call
  `entry.CaptureWorkingRemoved(ctx, w.branch, current)` with the same caching `current` closure. Non-dirty
  paths read through `w.current` like any other path.

## 3. Wire

gitrpc (method names follow `review.comment.*`):

- `gitrpc/wire.go`: `ReviewRemovedListParams{RepoID, Branch, Base}` (json `repoId`, `branch`, `base`);
  `ReviewRemovedDismissParams{RepoID, Branch string; IDs []int64 json:"ids"}`;
  `ReviewRemovedDismissResult{Dismissed int json:"dismissed"}`.
- `gitsession/removed.go` wire result: `ReviewRemovedResult{Entries []ReviewRemovedEntry json:"entries"}` (nonNil),
  `ReviewRemovedEntry{ID int64 "id"; Origin string "origin"; Path string "path"; ReviewedAt int64 "reviewedAt"; ReviewedAtSHA string "reviewedAtSha"; ContentKind string "contentKind"; Range *gitreview.LineRange "range"; Text *string "text"; Truncated bool "truncated"; RemovedAt int64 "removedAt"}`.
  `Text` over 64 KiB returns cut at a line boundary with `Truncated: true` (constant `maxRemovedTextBytes`,
  bounds one list response; full text stays stored).
- `gitrpc/incremental.go`: `handleReviewRemovedList` (validation copied from `review.files`: repoId
  non-empty, `validRefArg` branch and base) and `handleReviewRemovedDismiss` (repoId, branch; `ids` non-empty,
  each `> 0`, at most 1000, else `ipcerr.Contract`). Errors through `mapDetailError`.
- `gitrpc/handlers.go`: `"review.removed.list"`, `"review.removed.dismiss"` in `requestHandlers`.
- `bridge/gitservice_review.go`: `GitReviewRemovedListArgs`/`ReviewRemovedList`,
  `GitReviewRemovedDismissArgs`/`ReviewRemovedDismiss` via `callGit`.
- `flowharness/boundgit.go`: both methods in the bound table.
- Bindings: `wails3 task common:generate:bindings` (never hand-typed flags; `docs/DEV_ENVIRONMENT.md`).
  Commit the regenerated `frontend/bindings/**` files.
- `frontend/src/repo/git/boundGit.ts`: `'review.removed.list'`, `'review.removed.dismiss'` to
  `GitService.ReviewRemovedList/Dismiss`.
- `packages/git-ipc/src/contract.ts`: `ReviewRemovedEntry` interface (`origin: 'commit' | 'working'`,
  `contentKind: 'text' | 'binary' | 'tooLarge'`, `range: LineRange | null`, `text: string | null`), the two
  method entries with doc comments. Export from `index.ts` if the barrel lists contract types.

## 4. UI: ADE review window

Renders in the ADE review window (`frontend/src/ade/v2/review/`), not `packages/git-ui`
`ReviewFilesPane`. Reasons: working entries exist only in ADE; the window already runs server state through
TanStack Query (`useReviewWorking`, `useChangeRequestReview` call `gitCall`), shadcn-vue and Tailwind;
git-ui has no TanStack Query and is a separately mounted app. git-ui stays untouched.

- `useReviewRemoved.ts` (TanStack Query):
  - `useQuery` key `['reviewRemoved', gitRepoId, branch, base]`; `queryFn` awaits `useGitRepoHold(...).ready()`
    then `gitCall('review.removed.list', { repoId, branch, base })`; `refetchOnWindowFocus: true`.
  - Invalidate on: `onGitRepoEvent('repo.changed', ...)` `refsChanged`, debounced with VueUse
    `useDebounceFn(..., TIMING.fileSettle)` (`useReviewHead` pattern); `onReviewRepaint` `kind: 'mark'` for
    this repo and branch, either scope (a mark may capture); `useReviewWorking(branchId).dataUpdatedAt`
    change (ADE `ReviewWorking` captures working entries). Listener cleanup via `tryOnScopeDispose`.
  - `useMutation` dismiss: `gitCall('review.removed.dismiss', { repoId, branch, ids })`; `onSuccess`
    removes the ids from cached data with `setQueryData`, then invalidates.
- `AdeReviewRemoved.vue` (`<script setup lang="ts">`, Tailwind only, no `<style>`): hidden when no entries.
  shadcn `Collapsible` (default open) titled "Removed since review" with the count. Per entry
  (`data-testid="ade-review-removed-entry"`): path (`-path`), shadcn `Badge` "Uncommitted" for `working`
  (`-origin`), "Reviewed <relative time>" (`-reviewed`; reuse the app's existing relative-time formatter if
  one exists, else VueUse `useTimeAgo`), line range or "Whole file", the text in a `<pre>` with
  snapshot line numbers, deleted-line tone from existing diff tokens, `max-h` with own scroll (`-text`),
  "N more lines not shown" when `truncated`, and a "Not stored" note for a non-text entry. Dismiss: shadcn
  `Button` ghost icon with `Tooltip` (`-dismiss`). Section testid `ade-review-removed`. A dismiss error
  shows an inline shadcn `Alert`.
- `AdeReviewWindow.vue`: left `<section>` becomes a flex column: `AdeReviewFiles` `min-h-0 flex-1`, then
  `AdeReviewRemoved` `shrink-0 max-h-[40%] overflow-auto border-t`. `AdeReviewFiles.vue` unchanged.
- i18n `packages/theme/src/i18n/en.json`: new keys only under `space.ade.review.removed.*` (title, count,
  reviewed, lines, wholeFile, notStored, truncated, uncommitted, dismiss label/tooltip, dismiss error,
  aria). Follow existing suffix conventions; `check-catalog --unused` must pass.
- No Pinia store: no shared client state, only server state.

## 5. Tests (split at IPC)

Backend flow, `internal/flows/reviewflow/removed_test.go`, `TestReviewRemovedSinceReview`:

1. main: `a.txt` 5 lines, `b.txt`. feature: `new.txt` (3 lines) added; `a.txt` gains lines 6-9; `b.txt`
   gains one line.
2. Mark `new.txt` whole file; mark `a.txt` range 6-8. `b.txt` stays unreviewed.
3. External commit on feature: delete `new.txt`; drop `a.txt` lines 6-8 (line 9 stays, so `a.txt` stays
   listed); revert `b.txt` to main (unreviewed content leaves the diff).
4. Re-mark `a.txt` whole file **before any list** (proves capture before overwrite).
5. `review.removed.list`: exactly two entries, `new.txt` range 1-3 with its full text, `a.txt` range 6-8
   with those lines; `reviewedAt` equals each mark's `reviewedAt`; no `b.txt` entry.
6. List again: same ids, no duplicates. Contract `git-review-removed`, key `git:review.removed.list#removed`
   (mask `reviewedAt`, `reviewedAtSha`, `removedAt`).
7. Dismiss both ids: `dismissed == 2`; contract `git:review.removed.dismiss`. List: empty; contract
   `git:review.removed.list#dismissed`. Mark `a.txt` once more and list: still empty (never comes back).
8. `badRequest` for empty `ids` and a zero id.

Working flow, `internal/flows/adeflow/review_removed_test.go`, `TestReviewWorkingRemoved` (setup copied
from `review_working_review_test.go`):

1. Uncommitted edits: `c.txt` with 4 new lines, `d.txt` edited. Mark both whole via `ReviewWorkingMark`.
2. Worktree: delete `c.txt` lines 2-3; restore `d.txt` to HEAD (no longer dirty).
3. `ReviewWorking`, then git stream `review.removed.list`: `working` entries for `c.txt` 2-3 and the
   reverted `d.txt` lines, with text. Call `ReviewWorking` again: no duplicates.
4. Re-mark `c.txt` after a further deletion: the newly deleted reviewed lines appear (capture in
   `markWorkingLocked`).
5. Contract `git-review-removed-working`, key `git:review.removed.list#working`.

Coverage: run `internal/flows/coverage`; if bound names need an explicit call, add one to
`internal/flows/gitflow/bound_review_test.go` (how `ReviewMarkMany` is covered there).

UI spec, new `tests/ui/ade-v2-review-removed.spec.ts` (`reviewTest`, `reviewControl`, `GIT` from
`support/reviewHunkDiff`):

- `review.removed.list` `__sequence`: commit entries from `git-review-removed` `#removed` plus the working
  entry from `git-review-removed-working`, then the list minus the dismissed id.
- Section shows; both paths, reviewed label, line range and text visible; working entry has the badge.
- Click dismiss on one entry: `gitStreamParams(page, 'review.removed.dismiss')` equals
  `[{ repoId, branch, ids: [id] }]`; that entry hides, the other stays.
- Default empty list: section absent.
- `tests/ui/support/gitStreamMock.ts`: `ReviewRemovedList: 'review.removed.list'`,
  `ReviewRemovedDismiss: 'review.removed.dismiss'`. `tests/ui/support/gitUiPortFixtures.ts`: defaults
  `{ entries: [] }` and `{ dismissed: 0 }`, so every other review spec still boots.

## 6. Docs

`docs/ARCHITECTURE.md`: the review.db table line (now seven tables, `review_removed`, 0006) plus one
sentence on capture points and dismissal. "Known open items": a reviewed file whose delta exceeds
`MaxPatchBytes` (no parsed hunks) captures no removed lines.

## Fast checks

- `go build ./...`, `go vet ./apps/kira-space/...`, `bun run lint:go`.
- `go test ./apps/kira-space/internal/gitreview/ ./apps/kira-space/internal/gitsession/ ./apps/kira-space/internal/gitrpc/`.
- `go test ./apps/kira-space/internal/flows/reviewflow -run TestReviewRemovedSinceReview`, `./internal/flows/adeflow -run TestReviewWorkingRemoved`,
  `./internal/flows/coverage`; first with `KIRA_CONTRACT=write`, then without.
- `bun run typecheck`, `bun run lint`.
- Playwright (command in `docs/DEV_ENVIRONMENT.md`): `ade-v2-review-removed.spec.ts`, plus
  `ade-v2-review.spec.ts` and `ade-v2-review-working.spec.ts` (left-pane layout and new default fixture).
  Full suite waits for the chapter's single run.

## Commits (Conventional Commits, each a passing hook)

1. `feat: review.db keeps removed reviewed lines` (migration, store, ranges, unit test).
2. `feat: capture reviewed lines removed since review` (gitsession, ade).
3. `feat: review.removed list and dismiss` (gitrpc, bridge, flowharness, bindings, git-ipc, boundGit, flow
   tests, contracts).
4. `feat: removed-since-review section in review window` (composable, component, window, i18n, UI spec, mocks).
5. `docs: review_removed in architecture`.

## File ownership (for a stream split)

P499 owns, and no other v0.2.30 row names:

- `internal/gitreview/`: `migrations/0006_p499_removed.sql`, `migrations/embed.go`, `store.go`, `working.go`,
  `removed.go` (new), `ranges.go`, `project.go`, `project_test.go`.
- `internal/gitsession/`: `incremental.go`, `working_review.go`, `removed.go` (new).
- `internal/ade/review.go` (only `ReviewWorking`).
- `internal/gitrpc/`: `wire.go`, `incremental.go`, `handlers.go`.
- `internal/bridge/gitservice_review.go`, `internal/flowharness/boundgit.go`.
- `frontend/bindings/**` (regenerated; gitservice, bridge/gitrpc/gitsession models).
- `frontend/src/repo/git/boundGit.ts`.
- `frontend/src/ade/v2/review/`: `useReviewRemoved.ts` (new), `AdeReviewRemoved.vue` (new),
  `AdeReviewWindow.vue`.
- `packages/git-ipc/src/contract.ts` (and `index.ts` only if the barrel needs the type).
- `internal/flows/reviewflow/removed_test.go`, `internal/flows/adeflow/review_removed_test.go` (new),
  `internal/flows/gitflow/bound_review_test.go` (only if coverage needs it).
- `tests/contract/git-review-removed.json`, `tests/contract/git-review-removed-working.json` (new).
- `tests/ui/ade-v2-review-removed.spec.ts` (new), `tests/ui/support/gitStreamMock.ts`,
  `tests/ui/support/gitUiPortFixtures.ts`.
- `docs/ARCHITECTURE.md`: review.db paragraph and one Known open items entry only.

Shared:

- `packages/theme/src/i18n/en.json`: P499 adds keys only under `space.ade.review.removed.*`. P495–P498 must
  not add or remove keys there. Different JSON regions; a rebase conflict means a misplaced key.
- `docs/ARCHITECTURE.md`: shared only if another stream's plan also edits it; then that edit must stay
  outside the review.db paragraph and Known open items entry above, or the split is off.

Files P495–P502 touch per SPEC, none in P499's list: `board/actions.ts`, `board/menu.ts`,
`plan/useTaskMenu.ts`, `panel/AdeTaskPanel.vue`, `panel/AdeTaskTab.vue`, `plan/AdeTaskCard.vue`,
`AdeShelfRow`, `AdeActionCell.vue`, `AdeBranchRow.vue`, `mobile/state/useTaskActions.ts`,
`sessions/AdeTaskLaunchMenu.vue`, `packages/workbench/src/automations/**`, `shell/AdeShell.vue`,
`context/AdeContextPage.vue`, `settings/AdeSettingsPage.vue`, `state/adeContextUi.ts`,
`state/adeSettingsUi.ts`, `adeBoardUi`, ADE task-panel specs, adewire types and ADE task storage (P501).
P499 changes no adewire type and no ADE board storage, so it does not overlap P501's backend work either.

Ordering: P499 depends on no other v0.2.30 row and none depends on it. Bindings regeneration is the one
cross-cutting step: rows P495–P498 are frontend-only or reuse existing IPC per SPEC, so none regenerates
bindings. If one does, the two regenerations conflict only in generated files; regenerate again after
rebase.
