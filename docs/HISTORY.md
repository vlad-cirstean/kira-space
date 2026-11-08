# Kira Studio history

Finished chapters, one `<!-- chapter <ch> -->` to `<!-- /chapter <ch> -->` block each, oldest first, in the files below.
Cite a chapter as `docs/HISTORY.md (<ch>)` or `docs/HISTORY.md (<ch> <phase>)`; this list maps it to its file.
A chapter block always stays whole in one file; no file is cut inside a block.
Phase IDs are chapter-local before v1.6.
Deleted chapter files: `git show <commit>:docs/<chapter>/<file>` with the commit named in each chapter header.
Older chapters were distilled by `go run ./scripts/docshistory` (memory MCP `ingest_files`, one phase per input); each close since appends its block by hand (CLAUDE.md "Closing a chapter").

- [`history/01-v1.md`](history/01-v1.md): v1 to v1.2
- [`history/02-v1.3.md`](history/02-v1.3.md): v1.3 to v1.8
- [`history/03-v1.9.md`](history/03-v1.9.md): v1.9
- [`history/04-v2.0.md`](history/04-v2.0.md): v2.0
- [`history/05-v2.1.md`](history/05-v2.1.md): v2.1 to v2.8
- [`history/06-v0.2.10.md`](history/06-v0.2.10.md): v0.2.10 onward, plus "Fixes outside chapters" after v0.2.12. Newest file: a close appends here.
