# Handover: review to implement

## Summary

v1 stays alive behind deprecation headers and is unchanged otherwise. The remaining routes are payment methods and two proration endpoints. Money is integer cents everywhere on v2. The clock is injected; the flaky tests are fixed. Eleven of the fourteen routes serve v2 and replay correctly. This handover closes the review stage and opens implement.

## What changed

- `platform/problem/problem.go`: shared problem+json writer; the billing codes keep their numeric values.
- `api/v2/invoices.go`: invoice list, get and create on v2; create takes an idempotency header and replays the stored response on a repeat.
- `billing/store/replay.go`: stores response bodies by key for twenty-four hours and returns them on a repeat.
- `tests/contract/billing_v1_v2_test.go`: the contract tests, now run against both versions with one table of cases.
- `api/v2/proration.go`: proration for plan changes; computed in integer cents with half-even rounding.
- `billing/flags/flags.go`: billing-v2 flag read once per request; staff accounts are matched by group.
- `api/v1/deprecate.go`: adds the Deprecation and Sunset headers to every v1 route, nothing else changes.
- `tests/contract/replay_test.go`: one replay test per write endpoint.
- `ops/dashboards/billing.json`: two panels added: refund error rate and p95 latency per version.
- `billing/store/invoices.sql`: queries for the v2 invoice endpoints, reviewed for index use.
- `platform/clock/clock.go`: injected clock; tests and handlers take it from the request context.
- `docs/billing-v2.md`: route map from v1 to v2 with the deliberate differences listed.
- `api/v2/methods.go`: payment method add, default and remove; remove refuses the last method on an active plan.
- `api/v2/refunds.go`: refund create and list; a refund above the captured amount returns the shared problem+json error.

## Decisions and why

1. Money is integer cents in every payload. Why: floats were accepted by v1 and caused rounding disputes.
2. Do not touch the ledger export job. Why: another task owns it and the two must not change together.
3. Store replay bodies for a day, not forever. Why: storage cost grows with every write and clients retry within minutes.
4. Inject the clock instead of sleeping. Why: the refund and proration tests failed one run in twenty on slow runners.
5. Ship the flag per account group first. Why: staff can report problems without customers noticing.
6. Keep v1 alive behind headers until quarter end. Why: mobile apps update slowly and cannot be forced.
7. One problem+json shape for all errors. Why: mobile clients already match on its codes.

## Open risks

- **Blocking risk**: refund batches of three or more items reuse one key, so the second refund in a batch is replayed instead of applied; test_refund_batch reproduces it.
- The v1 deprecation headers were added to the redirect responses too, which one partner parses strictly.
- Payment method removal is not covered by a replay test because it deletes rather than creates.
- Replay storage was sized for staff traffic only; the full rollout may need a larger table.
- The dashboard panels were added by hand and are not yet part of the platform team's provisioning.
- test_refund and test_proration read the wall clock and failed in two of the last twenty runs.

## How verified

- Checked the dashboard panels against the local metrics endpoint with synthetic load.
- Sent each route to the v2 handler through the local gateway with the flag on and off.
- Contract suite green on v1 and v2: 212 cases each, run three times.
- Compared v1 and v2 responses on 40 recorded requests; only the documented differences remain.
- Ran the clock-injected tests two hundred times in a loop; no failure.
- Replay tests for all nine write endpoints pass and assert the stored body is returned byte for byte.

## Not touched

Nothing outside the files above changed. Generated files, vendored code and the other tasks' folders were left alone on purpose; if a diff shows them, the cause is a tool, not this stage.

## Next step guidance

- Review the replay store first; every write endpoint depends on its key handling.
- Check the problem codes against the mobile apps' mapping table before approving.
- Leave the v1 handlers alone except for the headers; a refactor there is out of scope.
- Run the whole suite three times, as the brief asks, and note any test that changes result.
