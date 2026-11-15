# Handover: implement to review

## Summary

The admin page shows the report and nothing else; it never writes. The remaining work is the greeting banner on the admin page and the review fixes listed below. Migrations are in place and reversible for one release. Reconcile compares orders and ledger lines per day and reports mismatches by cause. The posting path now writes integer cents inside one transaction per batch. This handover closes the implement stage and opens review.

## What changed

- `internal/ledger/clock.go`: posting time comes from an injected clock so tests stop reading the wall clock.
- `docs/ledger.md`: describes posting, rounding and the reconcile command with one worked example.
- `internal/orders/export.go`: the nightly export reads orders in id order and streams, no longer loads the day in memory.
- `internal/ledger/accounts.go`: account lookup is cached per batch; the cache dies with the batch, never across requests.
- `internal/recon/match.go`: matching works on (order id, total in cents); ties break on the older ledger line.
- `internal/orders/status.go`: status transitions validated against a table; unknown transitions return a typed error.
- `db/migrations/0043_ledger_cents.sql`: totals column converted from numeric(12,2) to bigint cents, backfilled in chunks of 5000.
- `scripts/seed_orders.sh`: seeds 400 orders across three days for local reconcile runs.
- `internal/ledger/post.go`: posting now runs inside one transaction per order batch; a failed line rolls the whole batch back.
- `cmd/ledgerctl/main.go`: new subcommand reconcile with --day and --dry-run, exits 2 when mismatches remain.
- `internal/ledger/rounding.go`: half-even rounding moved here from the reporting layer so every writer shares it.
- `db/migrations/0042_ledger_idx.sql`: index on ledger_lines(order_id, posted_at) to keep matching under a second per day.
- `web/admin/ledger/Mismatches.vue`: admin table of mismatches with a day picker; reads the report endpoint only.
- `internal/recon/report.go`: report groups mismatches by cause and prints the ten largest per cause.

## Decisions and why

1. Put rounding in one package. Why: three writers rounded differently and the report disagreed with the ledger.
2. Match on id and total, not on timestamps. Why: the two systems stamp times seconds apart and the clocks are not synchronised.
3. Leave refunds out of this task. Why: they post to a second ledger that another team owns; the interface is documented in docs/ledger.md.
4. Keep the old numeric column for one release. Why: a rollback must be possible without restoring from backup.
5. Fail the whole batch on one bad line. Why: a half-posted batch is harder to repair than a rejected one.
6. Reconcile per day, not per order. Why: a day is the unit finance closes, and per-order runs made the report too noisy to read.
7. Store totals as integer cents end to end. Why: floats drifted by one cent on about one order in two thousand during the dry run.

## Open risks

- **Blocking risk**: Orders edited after the nightly export match against their old total until the next run.
- The admin table does not paginate; it is fine up to a few hundred mismatches and slow beyond.
- Rounding differences of exactly half a cent are resolved toward the even cent; finance has not signed this off.
- Large days above fifty thousand orders were only tested with synthetic data, not with a production copy.
- Orders edited after the nightly export are matched against their old total until the next run.

## How verified

- Compared the nightly export before and after the change on one day: identical bytes except the header.
- Opened the admin page with an empty and a full report; both render, the day picker keeps its value.
- Applied both migrations to a copy of staging and rolled them back once; row counts matched.
- Edited three totals by hand: the command reported exactly those three and exited with 2.
- Ran the reconcile command on the seeded days: 400 orders, 0 mismatches, exit code 0.
- Package tests for ledger, orders and recon pass; the posting tests run on a fake clock.

## Not touched

Nothing outside the files above changed. Generated files, vendored code and the other tasks' folders were left alone on purpose; if a diff shows them, the cause is a tool, not this stage.

## Next step guidance

- Check the reconcile exit codes against what the nightly job expects before wiring it into the schedule.
- If the cents backfill is slow, run it in chunks and say so in the handover; do not skip it.
- Start with the posting path in internal/ledger/post.go; the other changes depend on its transaction boundary.
- Keep the new tests on the fake clock; a real sleep makes the suite flaky on slow runners.
- Do not widen the matching key; every extra column made the false-mismatch rate worse in the dry run.
