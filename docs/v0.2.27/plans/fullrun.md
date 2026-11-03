# Chapter full run (P459-P469)

Base: `da7e580`. Run once on `chapter/v0.2.27`; targeted reruns after each fix.

## Ran

- `bun install`, `bun run setup`: ok, bindings regenerated, no diff.
- `bun run lint` (biome + lint:rules incl. check-docs), `bun run typecheck`: clean.
- `lint:dead` (knip), `lint:go`, `go vet ./...`: 1 knip + 3 golangci findings, fixed.
- `bun run test:unit`: 1 fail, fixed.
- Go `test:go:rest` (all flows incl. space/studio, `ade`, `adeflow`): 1 fail, fixed.
- `-race -timeout 25m` on `ade/...` + `flows/adeflow`: pass (321 s, 1017 s). No timeout at that limit.
- UI Space `ui` + `visual`: 1044 pass, 3 fail, fixed.
- UI Studio `ui` + `ui-clipboard` + `visual` + `ipc-frontend`: 740 pass, 0 fail.

## Failures and fixes

- `graphChunkFrame.{bin,json}` stale at contract 57, code at 58: regenerated (`KIRA_CONTRACT=write`).
- knip: `TASK_TARGET` exported, unused outside `progress.ts`: unexported.
- golangci: `startRun` gocognit 31 (extracted `stepRepo`), `ineffassign` in `launches.go`, unused `azure.orgName` (deleted).
- `space-ops-ade.json` `record#child` drift: P468 nanoid worktree folder differs from branch slug. Test now masks `<folder-id>`; fixture regenerated.
- `ade-v2-flat-steps`, `ade-v2-run`: P469 renamed the `done` card action to `advance` ("Next stage"). Specs use `advance`.
- Visual `ade-plan-branches`: not a font issue (`fc-match sans-serif` is Inter). P461 removed the card plus button; the only changed pixels are that glyph. Re-recorded `plan-branches-stacked` only.

## Suspects that passed in the full run

`ade-v2-task-launch`, `agent-provider`, `ade-v2-archive`, `ui-leftovers`, `ade-v2-sessions-menu`, `ade-v2-branch-request`, `git-credential-relay`: no failure, no rerun needed.

## Not run

- `test:go:containers` (adapters, Docker/DB flows): chapter touched none.
- `test:ui:space-mobile`, `test:e2e-real:*`, `ui-timing`, `*:complete` flow suites, `test:claude*`.

## Left

Nothing open. No second full rerun: fixes touched specs, one fixture, and Go lint cleanups covered by targeted reruns.
