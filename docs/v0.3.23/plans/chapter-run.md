# Chapter run (v0.3.23)

Head at start: `269b3ee83`. One full non-UI run.

## Ran, all green after fixes

- `bun run typecheck` (all 10 projects), `bun run lint` (biome + `lint:rules`), `bun run lint:dead` (knip).
- `bun run test:unit` (2409 pass), `packages/api-core` tests (277 pass), `bun run verify:packaging`.
- `go mod tidy -diff`, `go vet ./...`, `go build ./...`.
- `go test ./...` (root module and `apps/kira-space`), then `bun run test:flows` (30 packages ok).

## Failures found

1. `internal/conventiontest` `TestGoConventions`: G1 at `bridge/apiconfig.go:92`, a literal `ipcerr.BadRequest` message
   in `EditVariables`. Real defect. Fix: `ipcerr.Contract` (UI always sends 1-500 edits). Commit `497c0a1e7`.
2. `flows/adeflow` `TestAgentBranchCreateNoPrompt`: contract snapshot read `conflictCheck: "checking"`, the async
   check had not settled. Test race, seen under load in the full run; passed alone 3 times before the fix. Fix:
   wait until every row is `done` before the snapshot. Commit `bec514450`. Passed 3 repeats plus the
   `-p 1` flows run.
3. `flows/dockerflow` `TestStatusClasses/daemon-down` and `gitaskpass`
   `TestRunHelper_BrokerNeverAnswersExitsOneWithinItsOwnTimeout`: unix socket path too long under the
   per-worktree `GOTMPDIR` (scratchpad path). Environment only, not a code defect; pass with `GOTMPDIR=/tmp/gt`.

No `dbmcpflow` or other known flake recurred.

## Not run

- `golangci-lint` (`lint:go`): binary built with go1.25, repo targets 1.27.3, cannot load here. `go vet` and the repo
  rule scripts ran instead.
- Docker-dependent tests: no `dockerd` started, images absent and disk (~3 GB free) too small to pull. Container
  adapter and `dockerflow`/`dbmcpflow` container paths skip. Run in PR CI.
- Playwright `ui`, `visual`, `ui-timing`, screenshots; real `claude` tests; complete-suite container tests (out of scope).
- Packaging artifact checks A1/A3/A4/A5/N2/N3 (no `.app`/`.dmg`) and shellcheck (not installed).
- Second full suite: not needed; fixes touched one error call in `bridge` and one test.

## Review round 1

Base `c7534c9f0`; 8 findings (2 important, 6 nit), all real, all fixed. Commits `861ee60a4` (API variables),
`736267f7b` (F7), `0859f42c9` (F5, F6).

- R1-F1: unlock is no longer an `EditVariables` op. `ApiConfigService.UnlockVariables(edits, confirmed)` runs behind the
  app's one `localauth.Authorizer` (grace window shared with other reveals); `confirmed` counts only where the OS cannot
  authenticate, and only the gate's grant sets the store's per-edit flag. Outcomes `done`, `cancelled`,
  `confirmation-required`. `EditVariables` refuses an unlock op (E_CONTRACT). The Variables tab sends unconfirmed,
  shows its danger confirm on `confirmation-required`, sends again confirmed. Flow `TestApiVariablesMatrix` covers
  unconfirmed, declined prompt (no file change, confirmed ignored while OS auth is available) and confirmed; UI spec
  `api-variables-matrix.spec.ts` covers the unconfirmed-then-confirm path and the declined prompt.
- R1-F2: `useEditVariables` (a mutation held by the never-unmounting store) is gone. `edit` runs `editVariables` through
  the store's task action with a one-shot thunk that hands the edits over once and clears its reference, the same
  thunk idea `useAsyncAction` uses for typed secrets; no mutation keeps `variables`. No unit test added.
- R1-F3: `editVariablesLocked` keeps each written file's pre-edit text, reads the old value of overwritten secrets, and
  on a failed seal or save restores written files, puts old values back and deletes a new secret only when no file
  still references it. No test: a mid-batch save failure cannot be injected from the flow harness (root ignores
  file modes, no write seam in configfs).
- R1-F4: `lock` removes `agent` and `agentHosts`, so a shared plain variable locks; the user shares it again with hosts.
  Flow step `lockShared` in `TestApiVariablesMatrix`.
- R1-F5: `replayAdopted` notes `max(routed, result.end)` after the held bytes, so a second move of an idle adopted
  terminal no longer waits `terminalMoveSettle`.
- R1-F6: when `Hold` reports not live, `prepareTerminalMove` waits (up to `terminalMoveSettle`) for the exit event and
  sends `exited` with its code, never `running`. Flow subtest `exit before hold` (contract `Hold:exited`) and UI spec
  `a Runs terminal that exited before the hold leaves as exited, with its code`.
- R1-F7: `Registry.Open` tracks the pump (`OpenParams.out`) under the lock that registers the session; `trackOutput` is
  removed. No test: the window is a scheduling race that cannot be forced.
- R1-F8: `checkVariableEdits` reports an oversized plain value or description with their own texts
  (`backend.studio.apiFiles.valueTooLong.body`, `descriptionTooLong.body`); the secret value keeps its own.

Run: typecheck, `lint`, `lint:dead`, `go vet ./...` (root and app), `go test` of `apifiles`, `bridge`, `appwire`,
`localauth`, `apiflow`, i18n, `internal/terminal` and `windowflow` with `-race`; UI specs `api-variables-matrix`,
`secret-reveal`, `tab-window-move`: 35 passed.
