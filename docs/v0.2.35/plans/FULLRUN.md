# Full suite run, v0.2.35

Resumable log. Env: 4 cores, CGO_ENABLED=1.

## Go (`rest` split: all except adapters, ipcfixture, dockerflow, dbflow)
- 142 packages ran; 1 failure.
- F1 `apps/kira-space/internal/flows/adeflow` TestAdeIDs: contract `ade-ids.json` held unmasked timestamps (addedAt, startedAt, createdAt, ...). Fix: mask the `*At` keys in the test, regenerate fixture. Commit "test: mask timestamps in ade-ids contract". Rerun x3 green.
- `apps/kira-studio/internal/flows/gitflow`: no such package in Studio; Space `flows/gitflow` passes (36.9s).
- F2 `apps/kira-studio/internal/flows/dockerflow` TestRestoreShutdownCasualty / TestRestoreDismissAndConflict flaked ~1/50: `kill` returned before the container stopped, a tick saw it running and swallowed the boot change. Fix: wait for stopped state in `restoreEnv.kill`. Commit "test: wait for killed container to stop in restore flows". Rerun: 10 min of -count=60 loop, zero failures (hit default 10m timeout, no test failed).
- Not run (real-container): internal/adapters/*, internal/ipcfixture (fail without containers: Kafka etc.), dbflow real DBs (ran, ok 6s).

- Env: `bun install --frozen-lockfile` needed (nanoid, uuid missing in node_modules); bindings regenerated (both apps) for AgentAccess. Then typecheck green, lint green, lint:dead green.
- F3 `apps/kira-studio/tests/unit/bridge-unwrap.spec.ts` sweep: apiConfigForm maps arg 3 before unwrap, threw TypeError with string args. Fix: pass array as arg 3. Unit: 2013 pass, 0 fail (194 files).
