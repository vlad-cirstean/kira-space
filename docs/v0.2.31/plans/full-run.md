# P505 full run

Tip before run: `fa68d188`. Container-backed tests not run (no Docker).

## Ran

- `bun run lint`, `lint:dead`, `lint:go`, `go vet ./...`, `bun run typecheck` (15 projects): clean.
- `bun run test:unit`: 1993 pass, 0 fail.
- `go test ./...`: all packages ok, 0 fail. Docker-backed tests skip without a daemon.
- Studio Playwright, `ui` + `ui-clipboard` + `ipc-frontend` + `visual`: 702 pass.
- Space Playwright, `ui` + `mobile-ios` + `mobile-android` + `visual`: 1172 pass, 1 skipped (in-code skip, not new).

## Finding: visual projects were blind

`toHaveScreenshot` uses Playwright's default per-pixel `threshold` 0.2, no `maxDiffPixels`. The
palette shift (bg `24,24,24` to `22,22,24`) sits under it, so old baselines passed against the
redesign. `--update-snapshots=all` rewrote all 47 PNGs (21 Studio, 26 Space), so none matched
byte for byte. Thresholds unchanged.

Fix: baselines re-recorded with `fc-match sans-serif` = Inter. Viewed `console` and
`plan-branches-pending`: readable, mask stays magenta. Rerun: Studio 21 pass, Space 26 pass.
Commit `3f0a683a`.

## Not run

- Container-backed Go, `e2e-real`, ipc fixture regeneration, `test:claude*`, `ui-timing`, perf, site check (no Docker / opt-in).
- No fix touched shared code: no second full run.
