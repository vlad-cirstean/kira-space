# P634 CI job sharding

Shard `go (Linux)` and `go-containers (Linux)` into matrix jobs over one split script, behind
aggregates with the same required-check names. Fix image-cache churn that made `go-containers` hit
its timeout. Check `ui`, `db-compat`, `checks` for cheap wins.

## Amendment (user decision)

`go-containers` stays ONE job named `go-containers (Linux)`: no shard matrix, no aggregate, no
`kibana`/`dbflow`/`sql` shards. The containers group in the table is one remainder row (`other`)
with every bundle; the job runs bare `containers` and `--bundles containers` (bare group lists all
its bundles). Kept from the design: pins-hash key, per-bundle restore-only caches, `cache-warm.yml`
`tc-images` saves, only pinned test images in each tar, `-timeout 7m` per package, `dbmcpflow` in the
containers group, `--check` in the job. Job timeout stays 15 min. `go (Linux)` keeps `adeflow1`,
`adeflow2`, `other` behind the aggregate; `go-split` prints only the `rest` matrix. Targets:
cache hit tests about 4-5 min, miss well under 12 min. Table rows and `go-containers-shard` text
below describe the superseded 4-shard design. adeflow has 190 top-level tests (95/95), not 191.

## Data (real, from CI)

Source: PR run 43628609545 (v0.3.13, PR #46), job and step timestamps via `actions_list`, per-package
times from `go test` `ok <pkg> <secs>` lines in job logs (`get_job_logs`). Runner: `ubuntu-24.04`,
4 vCPU, `go test` default `-p 4`. Not estimates.

### Where 24 min came from

`go-containers` attempt 1 (job 132549984752), 18:48:11 to 19:03:44, conclusion `cancelled`:

| step | time |
|---|---|
| setup | 72 s |
| image pull (cache miss) | 100 s |
| tests | 230 s |
| `docker save` all images | 458 s |
| cache save | 63 s |

Job hit `timeout-minutes: 15`. `pr-rerun-flaky.yml` reran it (attempt 2, job 132554374963, 7m57s, cache
hit). 15.5 + 8 = 24 min wall. Tests were 4 min both times. So the limit mattered: it turned a slow
save into a cancel plus full rerun.

Two root causes:

1. Cache key `docker-<os>-hashFiles(testsupport/*.go)` changes on any edit to testsupport.
   v0.3.13 (5ca6d7ac1) touched it, so the key missed.
2. Save step runs `docker save $(docker images --filter dangling=false ...)`: every image on the
   runner. Attempt 2's load log shows runner-preinstalled images in the tar
   (`ghcr.io/dependabot/dependabot-updater-core:latest`, `ghcr.io/github/github-mcp-server:latest`,
   `ghcr.io/github/gh-aw-*`), plus each test image twice (plain and `mirror.gcr.io` tag).
   That makes save 7.6 min and restore 56 s.

### `go-containers` per package (attempt 2, test step 4m13s wall)

| package | s | starts |
|---|---|---|
| adapters/kibana | 131 | elasticsearch, kibana |
| flows/dbflow | 105 | every kind |
| flows/dockerflow | 54 | `mirror.gcr.io` alpine, bash |
| adapters/mysqlfamily | 46 | mysql, mariadb |
| ipcfixture | 37 | mariadb, mysql, clickhouse, redis, localstack (sqs), kafka |
| adapters/clickhouse | 35 | clickhouse |
| adapters/kafka | 35 | kafka |
| adapters/postgres | 32 | postgres |
| adapters/testsupport | 32 | kafka, localstack |
| adapters/sqs | 20 | localstack |
| adapters/mongo | 15 | mongo |
| adapters/redis | 13 | redis |
| adapters/s3 | 7 | localstack |
| adapters/sqlite | 5 | none |

Fixture use found with `codegraph_explore` (testsupport `Start*`, `Prewarm`, `ImageFor` callers) and
confirmed by grep of `testsupport.Start*`/`testcontainers` in `*_test.go`.

### `go` per package (job 132549984878, 7m52s; setup 105 s, `bun run build` 10 s, tests 342 s)

Top: flows/adeflow 278, ade 75, flows/dbmcpflow 70, flows/memoryflow 58, flows/gitflow 41,
flows/termflow 32, flows/harnessflow 22, flows/grpcflow 20, flows/notifyflow 18, flows/githostflow 16,
flows/reviewflow 15, flows/appflow 12. Everything else under 11 s each, about 140 s in all.

Findings:

- flows/adeflow alone (278 s) bounds the job. Package-level sharding cannot beat it. It has 191
  top-level tests in 91 files, none `t.Parallel()`. Split it by test name.
- flows/dbmcpflow starts containers (`StartPostgres`, `StartMysql`, `StartMariadb`,
  `StartClickHouse`, `StartMongo`, `StartRedis`, own `tcpostgres.Run` in `mode_test.go`). The split
  regex misses it, so `go (Linux)` pulls those images from Docker Hub at test time. Move it to the
  containers group. It boots through `flowharness` (in-process `appwire.Build`, `fstest` mobile
  assets), as flows/dbflow does, so it needs no real frontend build.

### Other jobs (same run, attempt 1)

| job | wall | main steps |
|---|---|---|
| ui shards | 7m15 to 9m29 | setup about 102 s, playwright 298 to 424 s |
| checks (Linux) | 5m39 | setup 111 s, typecheck 160 s |
| checks (macOS) | 5m22 | setup 134 s, lint:go 85 s |
| db-compat min, max | 4m28 | setup 56 s, db-compat.sh 204 s |
| test-matrix groups | 2m to 4m | |

## Design

### Shard table in `scripts/go-test-split.sh`

One table, two groups (`containers`, `rest`), same group meaning as today. Each group has named shards
plus a remainder shard `other`. A package belongs to the first named-shard pattern that matches it,
else to `other`. So every package of a group runs in exactly one shard, by construction. Shards that
share one pattern are slices of one package, split by test name.

| group | shard | pattern (on import path) | slice | bundles | measured s |
|---|---|---|---|---|---|
| containers | kibana | `/internal/adapters/kibana(/\|$)` | | elastic | 131 |
| containers | dbflow | `/internal/flows/dbflow$` | | elastic sql kv stream | 105 |
| containers | sql | `/internal/flows/dbmcpflow$\|/internal/adapters/(mysqlfamily\|postgres\|clickhouse)$\|/internal/ipcfixture$` | | sql kv stream | 220 summed, -p 4 |
| containers | other | remainder | | kv stream flow | 181 summed, -p 4 |
| rest | adeflow1 | `/internal/flows/adeflow$` | 1/2 | | about 139 |
| rest | adeflow2 | `/internal/flows/adeflow$` | 2/2 | | about 139 |
| rest | other | remainder | | | about 500 summed, -p 4, longest ade 75 |

Group regex (containers vs rest) stays `PATTERN`, plus `|/flows/dbmcpflow`.

Comment above the table: source run id and per-shard seconds, so the next tuner sees the basis. Rule
in the comment: a new package that starts containers must match the containers group, or its fixtures
pull from Docker Hub in `go`.

Slices: `go test -list '.' <pkg>`, keep lines matching `^(Test|Fuzz|Example)`, `sort`, take index
`(i-1) mod n == k-1` (round robin), run `go test <args> -run '^(A|B|...)$' <pkg>`. Top-level names
only, so subtests still run. Union of slices equals full list. An empty slice prints a note and runs
nothing. A slice shard refuses caller args `-run`, `-list`, `-exec` (exit 2): they would break or
fake the slice.

### Script interface (POSIX sh, `set -eu`)

```
go-test-split.sh <containers|rest>[/<shard>] [go test args...]
go-test-split.sh --matrix <group>          # JSON array of shard names, no Go needed
go-test-split.sh --list <group>[/<shard>]  # packages, one per line; slices add "slice k/n: N tests"
go-test-split.sh --bundles containers/<shard>
go-test-split.sh --kinds <bundle...>       # pull-test-images.sh kinds of those bundles
go-test-split.sh --pins-hash               # 16 hex chars, no Go needed
go-test-split.sh --check
```

- Bare `containers` and `rest` keep today's meaning: whole group, no slicing. `bun run test:go:rest`,
  `bun run test:go:containers` and `cache-warm.yml` (`-run '^$' -exec true`) keep working unchanged.
- Unknown group or shard: usage on stderr, exit 2.
- Keep the `docker kernel` echo for the containers group.
- `--check` fails when a named shard matches no package (stale table), when containers and rest are
  not a partition of `go list ./...`, or when a `testimages` kind is in no bundle or in two.
- `--pins-hash`: sha256 of sorted `grep -hoE '[A-Za-z]*Image[A-Za-z]* *= *"[^"]+"'` over
  `apps/kira-space/internal/adapters/testsupport/*.go`, plus the alpine and bash pins grepped from
  `scripts/pull-test-images.sh`; first 16 hex. Changes only when a pin changes. `--check` also asserts
  the grep finds as many pins as `testimages` prints, so a renamed constant fails loudly.

### Image bundles

Five disjoint bundles. No image sits in two caches, so total cache size stays at the test images only
(smaller than today's tar, which carries runner images too).

| bundle | kinds |
|---|---|
| elastic | elasticsearch, kibana |
| sql | postgres, mysql, mariadb, clickhouse |
| kv | redis, mongo |
| stream | kafka, localstack |
| flow | `flow` pseudo-kind: `mirror.gcr.io/library/alpine:3.20`, `mirror.gcr.io/library/bash:5.2` |

Cache key: `tc-images-<os>-<bundle>-<pins-hash>`, path `~/tc-images/<bundle>.tar`.

`scripts/pull-test-images.sh [--print] [kind...]`:

- No kinds: every kind plus `flow`, as today.
- Kinds: only those (`go run .../cmd/testimages <kinds>`); `flow` pulls the two mirror-only images.
- `--print`: print exact refs to `docker save` (plain names for kinds, mirror names for `flow`), pull
  nothing. Keeps the flow pins in one file.
- Present images still print `present` and are skipped, so a run after a partial cache restore pulls
  only what is missing.

PR jobs restore only. `cache-warm.yml` saves bundles in main's scope, as `test-matrix` images do.
A PR that changes a pin misses that run and pulls the shard's kinds through the mirror (about 100 s
for every image today, in parallel, overlapped with compile wait). No save in PR jobs, so no save can
time a job out again.

### `pr.yml`

Replace jobs `go` and `go-containers` with:

1. `go-split` (name `go-split (Linux)`), `ubuntu-24.04`, `timeout-minutes: 5`. Checkout, then
   `echo "containers=$(sh scripts/go-test-split.sh --matrix containers)" >> "$GITHUB_OUTPUT"` and the
   same for `rest`. Job `outputs` expose both. Dynamic matrix: shard list lives only in the script,
   so pr.yml cannot omit a shard and silently drop its packages. Cost about 20 to 30 s before the
   shards start; go jobs stay off the critical path (ui about 9.5 min).
2. `go-shard` (name `go (Linux, ${{ matrix.shard }})`), `needs: go-split`, `timeout-minutes: 12`,
   `fail-fast: false`, `matrix.shard: ${{ fromJSON(needs.go-split.outputs.rest) }}`. Steps as today's
   `go` job (setup with `go-cache: go-rest`, `gtk: "true"`, git identity, inotify, `bun run build`),
   then `sh scripts/go-test-split.sh rest/${{ matrix.shard }} -timeout 7m`. Setup input
   `cache-save: ${{ matrix.shard == 'other' && 'true' || 'false' }}`: one saver per group keeps the
   PR-scope Go cache warm, as today's one job did, without four entries per push.
3. `go` (name `go (Linux)`): aggregate. `needs: [go-split, go-shard]`, `if: ${{ always() }}`,
   `timeout-minutes: 5`; fails unless both results are `success` (skipped shards after a failed
   `go-split` read `skipped`, so it fails).
4. `go-containers-shard` (name `go-containers (Linux, ${{ matrix.shard }})`), `needs: go-split`,
   `timeout-minutes: 12`, `fail-fast: false`, matrix from `outputs.containers`. Steps:
   1. checkout; placeholder `frontend/dist` and `dist-mobile` (keep step and its reason).
   2. `id: shard`: `bundles= <list> ` (pad with a space each side for `contains()`) from
      `--bundles containers/${{ matrix.shard }}`, and `pins` from `--pins-hash`.
   3. Five `actions/cache/restore@v6` steps, one per bundle, each
      `if: contains(steps.shard.outputs.bundles, ' <bundle> ')`.
   4. Background load of every restored `~/tc-images/*.tar` (nohup, pid file), before setup.
   5. setup: `bun: "false"`, `bindings: "false"`, `go-cache: go-ct`, `gtk: "true"`,
      `cache-save` as in `go-shard`.
   6. Background: wait for the load pid, then
      `sh scripts/pull-test-images.sh $(sh scripts/go-test-split.sh --kinds <bundles>)`; new pid file.
   7. git identity, inotify.
   8. `if: matrix.shard == 'other'`: `sh scripts/go-test-split.sh --check`.
   9. Wait for images, `timeout-minutes: 6`, then print the log.
   10. `sh scripts/go-test-split.sh containers/${{ matrix.shard }} -timeout 7m`,
       `TESTCONTAINERS_RYUK_DISABLED: "true"`.
   No save steps.
5. `go-containers` (name `go-containers (Linux)`): aggregate like `go`.

Update the header comment ("Only the aggregates `needs`...") to name the `go-split` producer too.

Timeouts: `-timeout 7m` per test binary (largest measured 131 s, so 3x headroom; a hang fails with
Go's goroutine dump, not a job kill). Job 12 min = setup about 2 + images about 1 + 7 + margin. The
setup step keeps its own 10-minute cap.

### `cache-warm.yml`

- `go-rest` and `go-containers` warm jobs: keep the compile-only steps; update comments ("Mirrors
  pr.yml `go (Linux, <shard>)`" etc.). Delete the `go-containers` job's three docker steps.
- New matrix job `tc-images` (name `warm Testcontainers images (Linux, ${{ matrix.bundle }})`),
  bundles `[elastic, sql, kv, stream, flow]`, `fail-fast: false`, `timeout-minutes: 30`, setup as
  `matrix-images` (`bun: "false"`, `bindings: "false"`, `gtk: "true"`). Compute pins, `lookup-only`
  restore with the PR key; on miss:
  `KINDS="$(sh scripts/go-test-split.sh --kinds <bundle>)"`,
  `sh scripts/pull-test-images.sh $KINDS`, `mkdir -p ~/tc-images`,
  `docker save $(sh scripts/pull-test-images.sh --print $KINDS) -o ~/tc-images/<bundle>.tar`,
  `du -h`, then `actions/cache/save@v6`.

Old `docker-Linux-*` entries are left to GitHub's 7-day eviction. Do not delete caches by hand.

### `pr-rerun-flaky.yml` and `pr-auto-merge.yml`: no edits

- Rerun logic never reads job names. It works per job id: superseded and timeout annotations are
  checked on each cancelled job, and `gh run rerun --failed` reruns failed and cancelled jobs plus
  their dependents. A failed shard fails its aggregate, so the run concludes `failure` and the rerun
  covers shard plus aggregate. Upstream `go-split` outputs from the first attempt are reused on rerun
  (GitHub keeps successful jobs' outputs). A timed-out shard (`cancelled`) also reruns via `--failed`.
- Auto-merge enables `gh pr merge --auto`. The ruleset gates on required checks by name. Names
  `go (Linux)` and `go-containers (Linux)` stay, now on the aggregates. No ruleset change. Never add
  `go-split (Linux)` or shard names to the ruleset.

### Cheap-win check (no change in P634)

- `checks (Linux)`, `checks (macOS)`, `db-compat`, `test-matrix`: all under 6 min, off the critical
  path after sharding. No change.
- `ui` shards become the critical path (up to 9m29). Two candidates, both declined here:
  - 10 shards: about 85 s off the slowest shard. Declined: job count. At start the run already has 19
    jobs (2 checks, 8 ui, ui-extra, visual, go-split, 4 test-matrix, 2 db-compat), and 7 shard jobs
    follow 30 s later, so 26 in all. On GitHub Free (20 concurrent jobs) 6 already queue until
    test-matrix groups finish (about 2 min). Two more ui shards would deepen that queue.
  - ui setup (102 s) installs GTK because `bindings: "true"` runs the cached `wails3` version
    check. Skipping it needs a setup-action design change. Measure first: the dispatched run below
    logs each setup sub-step's `duration_ms`. Record the GTK apt time in Result with a
    recommendation. Do not add a `SPEC.md` row; the orchestrator asks the user.

## Files

| file | change |
|---|---|
| `scripts/go-test-split.sh` | shard table, slicing, `--matrix`/`--list`/`--bundles`/`--kinds`/`--pins-hash`/`--check`; `dbmcpflow` into group regex |
| `scripts/pull-test-images.sh` | `[--print] [kind...]`, `flow` pseudo-kind |
| `.github/workflows/pr.yml` | `go-split`, `go-shard`, `go`, `go-containers-shard`, `go-containers`; header comment |
| `.github/workflows/cache-warm.yml` | `tc-images` bundle job; drop old docker steps; comments |
| `docs/PACKAGING.md` | §7 job table and intro sentence, cache-warm paragraph, ruleset step 1 parenthetical |
| `docs/DEV_ENVIRONMENT.md` | "Test speed on this box" bullet: shard usage, `dbmcpflow` now in containers |

`package.json`: no change (`test:go:rest`, `test:go:containers` keep group mode). `docs/PERF.md`
1168-1169: historical table, leave. `docs/ARCHITECTURE.md`: no CI job names to change (grep
`go-containers`, `go-test-split`, `docker-images` to confirm).

## Steps (Sonnet implementer)

Commit per group of related work, Conventional Commits.

1. `scripts/pull-test-images.sh`: kind args, `flow`, `--print`. Check:
   `sh scripts/pull-test-images.sh --print elasticsearch kibana flow` prints 4 refs. Commit
   `ci: pull only named Testcontainers kinds`.
2. `scripts/go-test-split.sh`: table and every mode above. Commit `ci: shard table in go-test-split`.
3. Dry run (placeholder `frontend/dist` and `dist-mobile` must exist locally; create them as the CI
   step does if missing, never commit them):
   - `--check` passes.
   - For each group, sorted union of `--list <group>/<shard>` over `--matrix <group>` equals
     `--list <group>`; containers plus rest equals `go list ./...`, no duplicate.
   - `--list rest/adeflow1` and `adeflow2` report 96 and 95 tests (191 total today).
   - `--matrix containers` prints `["kibana","dbflow","sql","other"]`.
   - `--bundles containers/dbflow` prints all four non-flow bundles.
   - Record outputs in Result.
4. Local runs, one heavy suite at a time (4-CPU sandbox):
   - `sh scripts/go-test-split.sh rest/adeflow1 -json` then `adeflow2 -json`; sum per-slice `Elapsed`
     of top-level tests. Both pass. Keep round robin and report the split. If one slice exceeds the
     other by more than 25%, say so in Result; orchestrator decides whether to rebalance.
   - `sh scripts/go-test-split.sh containers/sql` (proves `dbmcpflow` in the containers group).
   - `bun run test:go:rest` is not rerun in full; the two slices plus step 3 cover it.
5. `pr.yml` and `cache-warm.yml` as designed. Commit `ci: shard go and go-containers behind
   aggregates`.
6. Lint workflows: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7` from repo root (it runs
   shellcheck on `run:` blocks if `shellcheck` is on PATH; install it with apt if available). Fix
   every finding in touched files. `sh -n` both scripts.
7. Docs (`PACKAGING.md`, `DEV_ENVIRONMENT.md`), terse style. Commit `docs: CI go shards`.
8. Timing proof before the chapter PR (CI runs only on GitHub):
   1. Push `chapter/v0.3.14`.
   2. `gh workflow run pr.yml --ref chapter/v0.3.14`. It runs the branch's `pr.yml` (dispatch uses the
      ref's file). New `tc-images-*` keys miss in branch scope: this measures the miss path, the case
      that used to time out.
   3. `gh workflow run cache-warm.yml --ref chapter/v0.3.14 -f merge_sha=P634-branch-warm`. It saves
      bundles in branch scope.
   4. `gh workflow run pr.yml --ref chapter/v0.3.14` again: measures the hit path.
   5. Read jobs with `gh run view <id> --json jobs` (or `actions_list`). UI failures there are
      expected and out of scope. Any `go`/`go-containers` shard failure gets fixed and re-dispatched.
   Never use `--no-verify`; pre-push hook must pass.

## Verification (orchestrator)

- `git grep -n 'go-test-split.sh' .github` shows shard invocations; `fromJSON(needs.go-split` in pr.yml.
- `--check` and step 3 outputs in Result, with counts.
- Names: `git grep -n "name: go (Linux)$\|name: go-containers (Linux)$" .github/workflows/pr.yml` hits
  the aggregates.
- No `docker images --filter` save left: `git grep -n 'docker images --filter' .github` is empty.
- Dispatched runs: every go and go-containers shard green on both runs.

## Wall-time target

| check | before (run 43628609545) | target |
|---|---|---|
| `go (Linux)` | 7m52 | at most 6.5 min from run start (about 0.5 split, 2 setup, 0.5 compile, 3 test) |
| `go-containers (Linux)`, image cache hit | 7m57 | at most 6.5 min |
| `go-containers (Linux)`, image cache miss | 24 min (timeout plus rerun) | at most 7.5 min, no save, no rerun |
| PR run wall | 24 min | bound by ui shards, about 9.5 min |

## Result recording

`SPEC.md` P634 Result records, from both dispatched runs (run ids given):

- Per job: queue delay (`started_at` minus `created_at`), wall, test-step duration; aggregate end
  time from run start. Before column from run 43628609545 as in the target table.
- Per container shard: which bundles hit or missed, image wait time.
- adeflow slice split from step 4 and from CI.
- Queueing seen (26 jobs against the account's concurrency limit).
- ui setup GTK apt time and recommendation.

The chapter PR run is the final proof but lands after the close folds `SPEC.md`. The orchestrator
reports its go and go-containers timings in chat only; history files stay untouched.

## Out of scope

- Sharing image caches with `test-matrix` (`matrix-images-*`): different keys and image sets.
- `t.Parallel()` in adeflow: test-code change with shared-state risk.
- ui shard count and setup GTK change (see cheap-win check).
