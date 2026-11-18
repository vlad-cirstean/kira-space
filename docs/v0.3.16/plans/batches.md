# Batches

Grouping per `docs/WORKING_AGREEMENT.md` Phase loop (Batch) and Streams. Batches run in order.
Batch 2 starts only after batch 1 landed. Each stream: own git worktree off `chapter/v0.3.16`, own Opus
plan (`plans/P<n>.md`), own Sonnet implementer. Concurrent streams capped at 3.

## Batch 1: P657, P658, P659

| Stream | Phase |
|--------|-------|
| A | P657 |
| B | P658 |
| C | P659 |

No ordering dependency between streams. Shared files likely: i18n catalogs, registries, route tables.
Each stream's planner fills the per-stream file-ownership table in its plan and lists each shared
file with that stream's additive hunk.

Landing: once every stream is implemented and verified, rebase each onto `chapter/v0.3.16`. Resolve
conflicts only in listed shared files; stop on any other.

## Batch 2: P660, P661, P662

| Stream | Phase |
|--------|-------|
| A | P660 |
| B | P661 |
| C | P662 |

Starts after batch 1 landed. Plans written against the tree with batch 1 included.

Known overlap: P660 and P662 may both touch app shell and module keep-alive. P662 root cause may
relate to P660. Planners of P660 and P662 must list every shared file in their ownership tables, with
each stream's edit. If overlap proves large or semantic, or the P662 cause turns out to be P660's,
no split: run those two sequentially, P660 first. P661 expected independent; its planner confirms.

Landing: as in batch 1.

## Later: P660 Part 2

Split from P660. Runs after batch 2 landed, as its own phase (plan `plans/P660-part2.md`), before
or with a later batch. Depends on P660 Part 1's `useViewStateStore` and `stateKey` seam.

## Batch 3: P663, P664, P665

| Stream | Phase |
|--------|-------|
| A | P663 |
| B | P664 |
| C | P665 |

Starts after batch 2 landed. Plans written against the tree with batches 1 and 2 included. No
ordering dependency between streams. Shared files likely: app shell, shortcut registry, i18n
catalogs. Each stream's planner fills the per-stream file-ownership table in its plan and lists each
shared file with that stream's additive hunk.

Landing: as in batch 1.

## Batch 4: P666, P667, P668

| Stream | Phase |
|--------|-------|
| A | P666 |
| B | P667 |
| C | P668 |

Starts after batch 3 landed. Plans written against the tree with batches 1 to 3 included. Planners
fill the per-stream file-ownership tables and list each shared file with that stream's additive hunk.

Ownership: P667 owns the new card-radio theme component. P668 must not touch `ResetDialog.vue`,
`CherryPickDialog.vue`, `RevertDialog.vue` or `AdeWorkflowReviewPanel.vue`. Both may touch the theme
package index and i18n catalogs, additive hunks only. P666 expected independent; its planner
confirms.

Landing: as in batch 1.

## Batch 5: P669, P670, P671

| Stream | Phase |
|--------|-------|
| A | P669 |
| B | P670 |
| C | P671 |

Starts after batch 4 landed. Plans written against the tree with batches 1 to 4 included. Planners
fill the per-stream file-ownership tables and list each shared file with that stream's additive hunk.
P670 implementation waits for user confirmation if its plan flags a lost capability.

Landing: as in batch 1.

## Batch 6: P672

Sequential: Part 1 analysis, then Parts 2..9 (cut in `P672.md`; Part 2 first, optional batch pairs {5, 6} and {7, 8} on user request). Starts after batch 5 landed because it touches every
module and must see P667/P668 and the other changes.
