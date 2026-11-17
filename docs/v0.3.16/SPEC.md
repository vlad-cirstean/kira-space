# v0.3.16

Release retention deletes whole releases instead of their assets; git tags stay. Phases from `P656`
(highest shipped: `P655`, `docs/history/07-v0.3.15.md`). Branch `chapter/v0.3.16`.

| # | Phase | Status |
|---|-------|--------|
| P656 | Release retention deletes whole releases, never tags | Planned |

## P656 Release retention deletes whole releases, never tags

Retention (`scripts/releaseretention`, `internal/releaseline`, `.github/workflows/release-retention.yml`,
`docs/PACKAGING.md` §7, `docs/ARCHITECTURE.md`) today deletes release assets only. New rule: a
release that fails the keep rule is deleted whole (`DELETE repos/{repo}/releases/{id}`). Git tags are
never deleted: no `git/refs` call, no `--cleanup-tag`. Keep rule unchanged: each minor's `.0` (or
first release), last patch of each minor, last 5 patches of the current minor, `/releases/latest`.
Expected on the live listing (latest `v0.3.15`): delete `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`,
`v0.3.10`; keep `v0.3.3`, `v0.3.11` to `v0.3.15`. Check updater and `install.sh` behaviour against a
deleted release.

Plan: `plans/P656.md`.

Result: Done. Retention deletes each non-kept release whole through `releases/{id}`; tags stay. `releaseline.Release`
gains `ID`; `Delete` is one entry per release; TSV is `release id, tag`; workflow Apply rewritten; asset-less
releases failing the keep rule are deleted too. Guard grep (`git/refs`, `cleanup-tag`, `releases/assets`,
`release delete`) has no hits. Tests: `go test ./scripts/releaseretention/ ./internal/releaseline/` pass. Live read-only
dry run (latest `v0.3.15`) deletes exactly `v0.3.4`, `v0.3.5`, `v0.3.6`, `v0.3.9`, `v0.3.10` and keeps `v0.3.3`,
`v0.3.11` to `v0.3.15`: `total: 11 releases, 5 to delete (5 assets, 270 MB); tags kept`. `actionlint` not installed;
Apply script passes `bash -n`. Updater and `install.sh` unchanged. User step after merge: dispatch `Release retention`
with `dry_run=true`, confirm the 5 releases, then keep or set `RELEASE_RETENTION=apply`.
