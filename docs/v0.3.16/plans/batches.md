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

## Batch 4: P666

Batch 4 = P666 (single stream, starts after batch 3 landed). Plan written against the tree with
batches 1 to 3 included.
