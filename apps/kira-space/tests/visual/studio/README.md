# `tests/visual/` — pixel-diff regression (v1.4 P6)

Six specs: one canonical at-rest screenshot each for the workbench shell, the data grid, the SQL
console, the connection dialog and the Schema (DDL) editor, plus `settings.spec.ts` (P110 I2-1b)
alone contributing 8, one per Settings pane. Reuses `tests/ui/`'s own fixtures/support (the built
bundle, both mocked wire planes) — see `docs/HISTORY.md (v1.4 P6)` for the full design
record.

## Baselines are regenerated only from the `ui` CI job's own environment

A baseline PNG (the `*-snapshots/` directories Playwright creates beside each spec) is only ever
captured or updated via `bun run test:visual:update:studio` run inside `.github/workflows/pr.yml`'s `ui`
job — `ubuntu-24.04`, the exact `bunx playwright install webkit` + apt-package set that job
installs — or a container that matches it byte-for-byte. **Never from a local macOS run.** WKWebView
(what a real packaged build and a macOS dev machine both use) renders different glyph
hinting/antialiasing than WebKitGTK (what CI uses), independent of any font choice — a baseline
captured on one is not assertable against the other.

## A failing run

`test-results/` (Playwright's own default location on a screenshot mismatch) holds the actual,
expected and diff PNGs for whatever failed. Look there first, before assuming a genuine visual
regression — a font substitution, a not-yet-applied CI image update, or a real product change can
all produce a diff, and the PNGs make which one obvious at a glance.
