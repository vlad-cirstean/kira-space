# P632 part 1: chapter non-UI full run

Tree: chapter/v0.3.13 after streams A, B, C merged. No Playwright suite run: the one regenerated
contract (`studio-console-redis`) has no UI reader.

## Result

Run: typecheck, `bun run lint`, `lint:go`, `test:unit` (2,290 tests, includes
`apps/kira-space/tests/unit`, packages, site and scripts specs), `site:build`, Go non-flow packages
and all flow packages with `KIRA_FLOW_COMPLETE=1 KIRA_FLOW_DOCKER=require`, `GOTMPDIR=/tmp/gotmp-m`.
Every check green on the final tree.

Failed and fixed:

- Typecheck: stale gitignored bindings lacked the `expect` field of the concurrent-edit guard. Fixed by
  regenerating bindings (`wails3 task common:generate:bindings`). No tracked change.
- Unit `grid-pending-by-primary-key` (3 tests): expected ops without the new `expect` field. Specs updated.
- F-A11, 39 studio unit tests, `useTabsStore` used before initialization. Cycle:
  `state/tabs` -> `tabKinds` -> `repo/state/repoLinks` -> `coderepos` -> `workspace` -> `repo/git/transport`
  -> `hostHandlers` -> `repoTabs` -> `terminalTabs` -> `tabs`. `terminalTabs.ts` read `useTabsStore` at
  module scope. Now passes a thunk, so the binding is read at call time. The cycle itself remains (stores
  reference each other lazily); only the eager read was the fault.
- Unit, uncovered by F-A11 fix (4 more specs ran for the first time): `tabs-save-*` failed in the studio
  dir. Eight specs overrode `tabsSave` on `studio/bridge/control`, a different object from `bridge/control`
  that `state/tabs.ts` uses, so an unmocked save wedged the shared save chain. Added
  `support/benignTabsSave.ts`, used by the console harness and six specs.
- `lint:go` (11 findings): gocognit in `TypeRoundTrip` and `KubeFake.log` (split into helpers), four
  `bytes.Equal`, one gofmt, two ineffassign, one prealloc, one unconvert.
- `TestGoConventions` G2: `flowharness/datastream.go` used literal `E_UNKNOWN` (not a real code). Now
  `ipcerr.CodeInternal`.
- `TestConsoleRedis`: ran statements in Go map order, so HGETALL and LRANGE sometimes ran before HSET and
  RPUSH and the contract recorded empty rows. Ordered slice, contract regenerated.
- `TestDbMcpEngineModes` (mysql, mariadb, clickhouse, varied by run, failed only beside other engine
  tests): `waitListed` matched the short EXPLAIN pre-check op, which ended before cancel landed.
  Now requires `elapsedMs >= 300`. Confirmed by a goroutine dump (read still running, cancel returned
  not found) and by a green `TestDbMcpEngine*` run.
- `internal/terminal` `bash_nohup`: not reproducible (40 runs under 12 busy loops with `-race`, full run
  green). The bound equalled `closeGracePeriod`, so a loaded shell taking 2 s to exit after SIGHUP tripped
  it. `closeGracePeriod` is now a var; the two Close tests widen it to 10 s and bound Close at 5 s, so a
  real stall (waiting for the slave) still fails and load does not.

Environment notes: the disk filled twice mid-run (shared host, 1 GB free). Failures that read
`no space left on device` (ipcfixture `TestFixture_MySQL`, openedfiles build, dbflow, dbmcpflow and
dockerflow builds) were not code faults. Freed the bun cache and the Go build cache, reran those
packages, all green.

Rerun status (final tree, `-count=1`): conventiontest, terminal, flowharness, adapters/testsupport, dbflow,
dbmcpflow, apiflow, adeflow, journeyflow, gitsession pass. Other flow packages passed in the first full
pass and no later commit touches their code. Second full run not needed: no fix touched shared code
beyond `flowharness/datastream.go`, whose package tests and the flow packages built on it reran.
