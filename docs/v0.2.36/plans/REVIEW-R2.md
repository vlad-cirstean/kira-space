# v0.2.36 code review, round 2

Base: `f0cd476d9` (same as round 1). Tip reviewed: `c055b8246`. Scope: `git diff f0cd476d9..HEAD`, minus generated files, snapshots, lockfiles and plan docs. Round-1 fixer commits `678067ff6..c055b8246` read line by line.

Nothing real found in: `ConditionOperators` export and `browseDepth`, `resultText` no-copy path, `heldRefusal` texts for `ApprovalHeldFull`/`ApprovalQueueFull`, `Connected` with `columns: true`, `Recheck` locking and entry store (generation guard holds), `recheckAfterApproval` table check on the exemption and browse paths, ClickHouse `isNameExpr` unwrap (parenthesised, nested, function forms), "All tables" untick, `Listing.Incomplete` wiring and i18n `{limit}`.

## Findings

### 1. Medium: `known` is name-only, so a new column sharing a name with any live column still reaches the agent

- Where: `apps/kira-studio/internal/maskreview/service.go:426-429`, `apps/kira-studio/internal/dbmcp/review_gate.go:37-60`.
- Defect: `Known` is one set of lowercased column names over every relation in scope. `settleResult` skips the refresh when every result name is in it. A column added outside MCP whose name exists on any other in-scope table counts as known, is not in `Withheld.Names`, and passes. ClickHouse column names are case-sensitive, so `SSN` beside a known `ssn` also passes.
- Failure: `customers.email` reviewed. A migration adds `users.email`. Within 30 s of the last check the agent runs `SELECT email FROM users`. `email` is known, no refresh, not withheld: unmasked values returned. Round-1 finding 2 is closed only for names that are new to the whole connection. `email`, `phone`, `name`, `address` are the common case.
- Fix: settle per relation. Take the statement's table refs (the P532 extraction, also run when the list is "all") and treat a result name as known only when it is a column of one of those relations in the last check. Otherwise refresh those relations' containers. Keep the name-only set as a fallback when the statement cannot be parsed, and refresh in that case.

### 2. Medium: every alias or computed column forces a full uncached column walk, on each call

- Where: `apps/kira-studio/internal/dbmcp/review_gate.go:50-60`, `apps/kira-studio/internal/dbmcp/tools.go:339-347` and `:414-417`, `apps/kira-studio/internal/maskreview/service.go:202-215`.
- Defect: an alias (`count(*)`, `total`, `x AS y`) is never in `known`, and nothing remembers that it survived a refresh. Each such call runs `Recheck`: one `schemaColumns` op per container in scope (up to 50), with refresh, each through the P531 throttle and each rewriting a `metadata_cache` row. A reviewed (held) call does it twice: once at render, once in `recheckHeld`. The 30 s freshness never applies to these calls.
- Failure: Postgres with 20 schemas, `throttle_per_sec` 1, column review on. `SELECT count(*) FROM orders` spends about 20 s in the column walk after the query, and other ops on that connection queue behind it. A slow container hits the context or the 30 s throttle wait, and the agent gets "Kira could not check its columns" after the statement already ran.
- Fix: refresh only the containers of the statement's tables (as in finding 1). Skip the refresh when the entry is a `columns: true` check younger than a few seconds. Record names that stayed unknown after a refresh on the entry, so the same alias does not refresh again until the entry is replaced.

### 3. Medium: post-execution refusals still tell the agent to rerun, or omit that the statement ran

- Where: `apps/kira-studio/internal/dbmcp/tools.go:419`, `:422`, `:440`, `:341-342` (via `reviewRefusal`, `review_gate.go:14-19`).
- Defect: round-1 finding 3 was fixed for two `heldRefusal` cases only. After execution:
  - `recheckHeld` says "changed while its result was in review; run the query again".
  - `recheckHeld` "could not be re-checked" and the resolve failure do not say not to rerun or that changes stay.
  - `settleResult` refusal after a write returns `reviewRefusal`, which never says the statement ran.
  - `ApprovalAbandoned` never says it ran.
- Failure: write mode with review. `INSERT INTO orders ...` commits, the user approves the result, a pending column appeared meanwhile; the agent reads "run the query again" and inserts the row twice.
- Fix: every text returned after execution says the statement ran, adds `ranSuffix`, and says not to rerun. For a read, "run the query again" may stay. `settleResult` gets a post-execution variant of `reviewRefusal`.

### 4. Low: the round-1 fixes have weak or no tests

- Where: `apps/kira-studio/internal/dbmcp/settle_result_test.go:85-93`; `apps/kira-studio/internal/flows/dbmcpflow/maskreview_test.go`; `apps/kira-studio/internal/adapters/postgres/tablerefs_test.go`.
- Defect:
  - `TestSettleResultHeldRecheck` never calls `recheckHeld`. It hand-builds `rendered` with an empty withheld set and checks that a fake returning `ssn` differs, which holds by construction.
  - The round-1 finding 2 scenario (column added outside MCP, then `SELECT *` within 30 s) has no flow test through the real `maskreview.Service`. Only fakes cover it.
  - No test covers `recheckAfterApproval` refusing after an allow-list change.
  - The Postgres conformance suite has no case for a user overload of a built-in name (round-1 finding 5).
- Fix: drive `recheckHeld` in the unit test, or drop it. Add one dbmcpflow case: review on, add a column out of band, query, expect it withheld. Add a `public.format(int)` case to `TestPostgres_TableProbe`.

### 5. Low: Postgres refuses common calls on databases with extensions, with a refusal the agent cannot act on

- Where: `apps/kira-studio/internal/adapters/postgres/tablerefs.go:400-415`; text in `apps/kira-studio/internal/dbmcp/tables_gate.go:101`.
- Defect: any function name that also exists outside `pg_catalog` is refused. `citext` installs `max`, `min`, `replace`, `strpos`, `split_part` and `regexp_*` in `public`. PostGIS and others add more. The refusal says "refused: function max (it can reach tables the list cannot check)". It does not say that `pg_catalog.max(...)` passes, although the walker accepts a `pg_catalog`-qualified call as built in.
- Failure: allow list on, `citext` installed. `SELECT max(id) FROM orders` is refused. The agent has no way to learn the fix.
- Fix: keep the check (exempting extension functions would admit `dblink`). Add to the refusal: "a function of this name also exists in schema X; call `pg_catalog.<name>` to use the built-in".

### 6. Low: ClickHouse IN-function list holds invented names and misses the `*IgnoreSet` forms

- Where: `apps/kira-studio/internal/adapters/clickhouse/tablerefs.go:203-207`.
- Defect: `innullin` and `notinnullin` are not ClickHouse functions. The analyzer's IN-name set (`isNameOfInFunction`) also includes `nullInIgnoreSet`, `notNullInIgnoreSet` and their global variants, which are missing here.
- Failure: if the server resolves a bare second argument of `globalNullInIgnoreSet(id, secret)` as a table, it is a membership oracle like round-1 finding 1. Not confirmed against a real server.
- Fix: match `^(global)?(not)?(null)?in(ignoreset)?$` on the lowercased name, drop the invented entries, add a parse test. Confirm one `*IgnoreSet` form against the real-container suite.

### 7. Low: unqualified user-defined operators bypass the Postgres function probe

- Where: `apps/kira-studio/internal/adapters/postgres/tablerefs.go:276-282`.
- Defect: this is round-1 finding 5 for operators. An `A_Expr` with a one-part name is accepted without any probe. A user operator in a `search_path` schema runs its backing function, which may read an unlisted table.
- Failure: earlier DDL created `public.===(int, int)` backed by a SQL function that reads `secret`. `SELECT 1 === 2` passes the gate.
- Fix: collect unqualified operator names and refuse when `pg_operator` joined to `pg_namespace` has that `oprname` outside `pg_catalog`. Use the same probe query as functions.

### 8. Low: dead `truncated` field after the switch to `incomplete`

- Where: `apps/kira-studio/internal/maskreview/service.go:57`, `:91`, `:295`; `packages/shared/domain/maskReview.ts:24`; `apps/kira-studio/tests/contract/dbmcp-mask-review.json:28`.
- Defect: `entry.truncated` is never set, so `Listing.Truncated` equals `Incomplete != ""`. The UI now reads `incomplete` only.
- Fix: remove `entry.truncated`, `Listing.Truncated` and the zod field, then regenerate the contract.
