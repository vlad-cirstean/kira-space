# Full run, chapter v0.2.30

Base tip aedef1c2, tree clean. Sonnet 5.5 orchestrated run; `fc-match sans-serif` = Inter.

## Ran

- `bash scripts/setup.sh`, `bun run build:space-mobile`.
- `bun run lint`: pass. `bun run lint:dead` (knip): pass. `bun run lint:go`: 0 issues.
- `bun run typecheck` (all 15 projects): pass, 0 errors.
- `bun run test:unit`: 1993 pass, 0 fail, 193 files.
- `go vet ./...`: clean. `go test ./...`: 156 packages ok, 0 fail (82 without test files).
- Studio `ui`: 672 pass.
- Space `ui`: 1062 pass, 1 fail on first run; 1063 pass after fix.
- Studio `visual` + `ui-clipboard`: 23 pass; `ipc-frontend`: 7 pass.
- Space `visual` 26 pass, `mobile-ios` + `mobile-android` 83 pass.


## Failure

`ade-v2-review-removed` "dismissing an entry hides it and keeps the rest": expected 5 entries, got 4
before the dismiss click. Cause: `useReviewRemoved` also refetches when `useReviewWorking`
resolves (`dataUpdatedAt` watch). Under load that refetch consumed the mock's `__sequence`
first entry, so the list showed the post-dismiss answer early. Product behaviour correct; mock was
call-count dependent.

Fix `f98df9ec`: `gitStreamMock.ts` gains `__afterCall` (answer `before` until a method has been
requested, then `after`); spec uses it for `review.removed.dismiss`. Verified: 15/15 lone,
40/40 with `--repeat-each=40 --workers=8` under 8 busy loops, and the 4 other specs using
`gitStreamMock` (121 tests) pass. Change is additive test support; no second full run needed.

## Baselines

None re-recorded: all visual specs pass unchanged (P500, P501, P502 included).

## Not run

- Container-backed Go tests, `test:go:containers`, complete flow suites, `e2e-real`: no Docker.
- `test:claude*`: opt-in, token cost.
- `ui-timing`: not part of the ask.
