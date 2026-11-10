# Kira Studio history

Finished chapters, one `<!-- chapter <ch> -->` to `<!-- /chapter <ch> -->` block each, oldest first, in the files below.
Cite a chapter as `docs/HISTORY.md (<ch>)` or `docs/HISTORY.md (<ch> <phase>)`; this list maps it to its file.
Phase IDs are chapter-local before v1.6.
Deleted chapter files: `git show <commit>:docs/<chapter>/<file>` with the commit named in each chapter header.
Older chapters were distilled by `go run ./scripts/docshistory` (memory MCP `ingest_files`, one phase per input); each close since appends its block by hand (CLAUDE.md "Closing a chapter").

## Rules for a close

1. Limit: 1,500 lines per file in `docs/history/`.
2. Newest file = highest number in `docs/history/` = last index entry below.
3. Count its lines (`wc -l`). If it stays at or under 1,500 lines with your block appended, append there.
4. Otherwise create the next file `NN-<first chapter in file>.md` (next number), put the block there, add its index entry with chapter range.
5. A block never splits across files. A single block over 1,500 lines sits alone in its file; only then may a file exceed the limit.
6. Never move existing blocks between files.
7. Update the chapter range of the index entry for the file you touched.
8. No user request needed for any of this.

Example: newest file `06-v2.8.md` has 1,450 lines, new block has 100. Total 1,550 is over 1,500, so create `07-v3.0.md` (first chapter `v3.0`) and add its index entry `v3.0`. If the file has 1,185 lines instead: append, end its entry range at the new chapter.

## Index

- [`history/01-v1.md`](history/01-v1.md): v1 to v1.2
- [`history/02-v1.3.md`](history/02-v1.3.md): v1.3 to v1.8
- [`history/03-v1.9.md`](history/03-v1.9.md): v1.9
- [`history/04-v2.0.md`](history/04-v2.0.md): v2.0
- [`history/05-v2.1.md`](history/05-v2.1.md): v2.1 to v2.6
- [`history/06-v2.8.md`](history/06-v2.8.md): v2.8 to v0.3.7, plus "Fixes outside chapters" after v0.2.12. Newest file: a close appends here or opens the next.
