# P716 design: MCP chaining, model A vs model B

Design phase. Compares both chaining models from `SPEC.md` P716, recommends one, plans each. User
decides. Base: `chapter/v0.3.23` at `8a09133ff`. Paths relative to `apps/kira-space/` unless rooted.

## Recommendation

Model A (no chain UI, kept variables, agent follow-up). Reasons:

1. User's own first instinct ("I think i'd keep it simpler"). A matches it; B adds a second
   navigation layer on top of every prompt.
2. Backend already does A's core today: `kept.set`/`kept.get`, `{{kept.x}}` and `params` reads,
   `list_kept_variables`, staged writes, preview and fingerprint reveal. A is mostly deletion plus
   small additions.
3. B is a strict superset of A. Every A phase is reused unchanged under B; B adds chain linking,
   retention of answered items and released results, and a dialog that waits for a step not yet
   sent. Picking A now leaves B open later; picking B now pays its cost up front.
4. B's "go back" needs results kept after the agent got them: new memory held per chain and a
   longer window for secret material to live in process. A keeps only what a script chose to keep.

Pick B if seeing the whole chain in one dialog matters more than fewer moving parts. B stays simple
for the user (same dialog, same footer); cost lands in backend and tests.

## Shots

Folder (throwaway, not committed):
`/tmp/claude-0/-home-user-kira-space/f4f0e293-574f-5470-bbb6-77e70816ad3a/scratchpad/p716-design/`.
Real frontend, app mocks, P715 style. `-end` files are same card scrolled to footer.

Scenario in every shot: (1) API `POST https://api.shop.example/v1/auth/session`, transform keeps
`session_token` (secret) and `account_id` (plain). (2) DB "shop prod" postgres, `orders` query with
param `account_id`, keeps `failed_order_ids`. (3) API `POST /v1/orders/retry`, header
`Bearer {{kept.session_token}}`, body `{{kept.failed_order_ids}}`. Kube: list pods keeps `pod`;
logs reads `pod` in prepare.

Before (current frontend, `ChainSteps` "Earlier step / This step"):

- `before-api-login.png`, `before-api-login-review.png`
- `before-db-followup.png`, `before-db-followup-end.png`
- `before-api-followup-secret.png`, `before-api-followup-secret-end.png`
- `before-kube-list.png`, `before-kube-logs-followup.png` (kube today: no script, no kept)

Model A:

- `A-01-api-login-keeps.png`: script card, "This script may keep:" with Secret/Plain badges.
- `A-02-api-login-review.png`: result review, "Will keep" rows; plain shows value, secret masked
  with Reveal.
- `A-03-db-followup.png`, `-end`: DB card, kept rows used with badges, no chain panel.
- `A-05-reveal-confirm.png`: fingerprint prompt fallback (confirm dialog where OS auth missing).
- `A-06-reveal-shown.png`: secret shown for 60 s after auth.
- `A-07-api-followup-secret.png`, `-end`: API card reading a secret and a plain kept variable.
- `A-08-api-followup-secret-revealed.png`: same card after reveal.
- `A-09-kube-list-keeps.png`, `A-10-kube-logs-followup.png`: kube cards, same rows as DB and API.
- `A-11-settings-kept.png`: Settings, MCP kept variables: kind badge, preview, reveal, clear.

Model B:

- `B-01-step1-prompt.png`: step strip "Step 1 of 1" over normal API card.
- `B-02-step1-review.png`: step 1 result review.
- `B-03-waiting.png`: dialog stays open, "Waiting for agent's next request".
- `B-04-step2-prompt.png`, `-end`: step 2 DB card; footer gains Back.
- `B-05-back-step1.png`, `-end`: read-only step 1 (approved, result, secret masked), Next.
- `B-06-step3-api.png`, `-end`: step 3 API card.
- `B-07-step4-kube.png`: step 4 kube card in same chain.

Prototype source: `prototype.patch` (669 lines, against this base) and `zz-p716-shots.spec.ts` in
same folder. Run with `P716_OUT=<dir> P716_MODE=before|proto`; B chain data travels in
`payload.__chain`. Prototype only; implementation rebuilds from this plan.

## Audit: chaining today

Backend, `internal/dbmcp/`:

- `kept_script.go`: `keptRun` :26, `keptRunFor` :58, cap `maxKeptScriptNames=16` :22. `get` :189
  refuses a secret read in prepare. `set` :251 has no secret option: secret only by taint (read a
  secret, everything kept turns secret; `commit` :331 stores `Secret: k.secretRead`). `drop` :279.
  `stagedViews` :294 gives no preview for a secret. `checkPrepared` :378, `keptSourceOf` :447.
- `kept_tools.go`: `KeptStore` :16, `keptScope` :33, `listKeptVariables` :56 (note text :67),
  `addKeptTools` :72, `keptGoneError` :79, `watchKept` :112.
- API: `api_tools.go` `keptRunFor` :144, `watchKept` :286; `api_script.go` `commit` :349;
  `api_review.go` `apiReviewFor` :52 (secret read forces review), `held.Keep` :89;
  `api_inputs.go` `keepInputs` :112.
- DB: `query_script.go` `queryKeptNotAllowed` :26-28, `keptRunFor` :45, secret/bound refusal :59,
  `stagedViews` :160, `reviewResponse` :192, `commit` :202; `kept_params.go` :129-130 ("secrets
  bind into api_request only, never into SQL"); `tools.go` `reviewResponse` :420, `watchKept` :718;
  `limits_gate.go` :145.
- Kube: `kube_tools.go` (306 lines) `kubeRun` :118, `addKubeTools` :283. No script, no kept.
- `jsscript.go` `scriptsOn` :52.
- Store: `internal/keptvars/store.go` `Put` :41 (Secret, Origin, NotAfter); `script.go`
  `ScanScript` :20 reads literal `kept.(get|set)("name"` at plan time. Session-scoped, TTL
  `dbMcp.keptRetentionMinutes` (default 60).
- Held results: `approval.go` `maxHeldResultBytes = 8 << 20` :46, `maxHeldTotalBytes = 64 << 20`
  :47. Result review is a second queue item (`api.response`/`db.response`) fetched by
  `DbMcpService.PendingResponse`.
- Queue: `internal/approvals/queue.go` `Put` :141, `Remove` :155, `Answer` :178. Answered items are
  removed; nothing keeps them.
- Reveal: `internal/bridge/keptvars.go` `KeptVarsService.Reveal` :168 behind `localauth.Gated`;
  OS prompt reason :151 "show a value you supplied to an agent request." (input-only wording).

Agent text, `internal/llmprompts/registry.go`:

- `DbmcpInstructions` :763 never mentions kept variables.
- `DbmcpToolRunQuery` :828, `DbmcpToolAPIRequest` :895, `DbmcpToolRunQueryScripts` :901,
  `DbmcpToolListKept` :910, `api_request` `inputs` :933, `script` :936,
  `DbmcpToolAPIRequestScripts` :943.
- Kube tool entries :961, :967, :975, :989, :1002: no kept mention.
- Budgets `internal/llmprompts/testdata/budget.tsv` (`dbmcp.instructions` 1196, `api_request`
  1171).

Frontend:

- `frontend/src/studio/approvals/ChainSteps.vue` (75 lines): derives "Earlier step" rows from each
  kept use's `source.request`. Used in `DbQueryBody.vue` :7/:19, `ApiRequestBody.vue` :19/:75,
  `DbExemptionBody.vue` :7/:19. Strings `studio.dbMcp.approval.chain.*` in
  `packages/theme/src/i18n/en.json`.
- Already in place: `KeptVariablesPanel.vue`, `KeptVariableRow.vue` (preview, secret reveal),
  `kept/KeptValueView.vue` (staged value hides reveal), `StagedKeepList.vue`,
  `TransformScriptBlock.vue` ("This script may keep:"), `kinds.ts` risk line
  `studio.kept.risk.keeps.status` (~:365, ~:422), Settings `StudioMcpKeptPane.vue`,
  `KeptEntryRow.vue`, `packages/workbench/src/approvals/ApprovalsDialog.vue`,
  `ApprovalItemActions.vue`, `packages/shared/domain/dbmcp.ts` `transformViewSchema` :129,
  `kubeReadPayloadSchema` :205.

Tests:

- Flow: `internal/flows/dbmcpflow/e2e_kept_chain_test.go`, `e2e_api_review_chain_test.go`,
  `e2e_db_script_chain_test.go`, `kept_script_test.go`, `kube_test.go`, `e2e_user_value_test.go`.
- UI: `tests/ui/studio/e2e-kept-chain.spec.ts` :64-67 asserts chain rows;
  `tests/ui/studio/credential-reveal.spec.ts` is the reveal pattern.
- Visual: `tests/visual/studio/approvals-prompt.spec.ts` :75-84 'script card with its chain',
  baseline `approvals-script-chain.png`.
- Real claude: `tests/claude/dbmcp_test.go:345` `TestDbMcpKeptChainThroughClaude` dictates the
  script, so it never proves unprompted use. `tests/claude/kube_test.go:17`. `bun run
  test:claude:db` runs `TestDbMcp|TestKubeMcpThroughClaude|...`.

Gap list (both models): no explicit plain/secret choice; no reveal of a staged secret during
review; reveal reason wording covers inputs only; no kind badge on may-keep, kept, settings rows or
risk line; kube lacks scripts and kept; agent not told at connect time.

## Comparison

User effort per request. A: one prompt per request, same as any other prompt; result review where
the tool already reviews. Three-step scenario: 3 approvals, plus reviews. B: same approvals and
reviews, plus dialog stays open between steps (waiting state), plus optional Back/Next. Equal
clicks on happy path; B adds a waiting screen and navigation to learn.

Secrets security. A: secret lives only in kept store until TTL or clear; agent sees name and kind,
never value; reveal behind `localauth.Gated`. B: same, plus answered items and released results
retained for "go back"; retained result may hold raw secret material before transform masked it,
so B must store only post-transform views (secrets masked) and drop them with the chain. More data
in memory, longer.

AI awareness and prompt changes. Same in both: instructions, DB/API/kube tool descriptions and
`list_kept_variables` say a script can keep a value plain or secret, a follow-up request reads it,
secret values never reach the agent. B adds nothing agent-facing unless an explicit chain id is
chosen (then a `chain` arg on every chaining tool, plus budget).

Backend change size and deletions. A: add `kept.setSecret` and secret-aware `stagedViews`; new
gated reveal over held result; badge data in views; reason text; kube script plus kept (P718).
Deletes `ChainSteps.vue`, its 3 call sites, `chain.*` strings, chain asserts, visual baseline. B:
all of A, plus chain linking, retained-step store with caps and expiry, new bridge calls (list
chain steps, read past step), queue change so answered items stay visible, dialog waiting state.
Estimate: A ~600-900 lines changed, net delete on frontend; B adds ~1,200-1,800 more.

Test split. A: flow test (plain plus secret kept, follow-up reads both, secret absent from AI
output); UI spec (same scenario, preview, fingerprint on secret, chain panel gone from DB and API);
real claude test with no dictated script. B: all of A plus flow tests for linking and retention
(cap, expiry, released result masked) and UI spec for forward/back and waiting.

Risk. A: low; behavior agent already has, plus one store flag. Main risk: agent ignores kept vars;
covered by real claude test. B: medium-high. Unrelated agent call in middle of a chain: which chain?
Agent never sends step 2: how long does dialog wait? Memory under many chains. Back while new item
queued. Each needs a rule and a test.

Legacy drop (v0.3.23 is a minor bump from v0.3.22). Both: delete `ChainSteps` outright, no flag, no
compat. Kept store is in-memory and session-scoped, so no migration. Taint-derived secret stays
(read-a-secret-keeps-secret) beside explicit `setSecret`; nothing legacy to keep. B adds no
persisted data either.

## Decisions under both models

- Secret syntax: `kept.setSecret("name", value)`. Literal name lets `ScanScript` badge the card at
  plan time. Shots show `kept.set(name, value, { secret: true })`; implementation uses `setSecret`
  (options object is not scannable without a parser change).
- `kept.set` stays plain unless script read a secret (taint rule unchanged).
- Origin binding: API-made secret binds to its request origin (today's `Origin`). DB and kube
  secrets bind into `api_request` only, as today (`kept_params.go` :129).
- Staged secret reveal: new `KeptVarsService.RevealStaged(itemID, name)` behind `localauth.Gated`,
  reading held result's staged value; same 60 s view as `useKeptReveal`.
- Reveal reason text: "show a secret value kept for an agent request." covers inputs and kept.
- Badges: shadcn-vue `Badge` `warn` for Secret, `info` for Plain, on may-keep row, staged row,
  kept row, Settings row; risk line appends "(secret)".

## Plan: model A

Phase split (renumber per CLAUDE.md on approval; parts keep P716 number):

1. P716 Part 1: remove chain UI. Delete `ChainSteps.vue`, its uses in `DbQueryBody.vue`,
   `ApiRequestBody.vue`, `DbExemptionBody.vue`, `studio.dbMcp.approval.chain.*` in
   `packages/theme/src/i18n/en.json` (and other locales), chain asserts
   `e2e-kept-chain.spec.ts` :64-67, visual case `approvals-prompt.spec.ts` :75-84 and baseline
   `approvals-script-chain.png`. User asked for this as its own phase.
2. P716 Part 2: secret and plain kept variables. Backend `kept_script.go` (`setSecret`,
   `stagedViews` secret entry masked), `internal/keptvars/script.go` (`ScanScript` regex adds
   `setSecret`), `bridge/keptvars.go` (`RevealStaged`, reason text), `packages/shared/domain/
   dbmcp.ts` (kind on staged and may-keep views). Frontend `TransformScriptBlock.vue`,
   `StagedKeepList.vue`, `kept/KeptValueView.vue`, `KeptVariableRow.vue`, `KeptEntryRow.vue`,
   `kinds.ts`, `en.json`. Tests: flow `e2e_kept_chain_test.go` (extend: plain plus secret, follow-up
   reads both, secret absent from AI output), UI spec `e2e-kept-chain.spec.ts` (preview plain,
   fingerprint on secret, no chain panel).
3. P716 Part 3: agent awareness. `internal/llmprompts/registry.go` (`DbmcpInstructions`,
   `run_query`, `api_request`, `list_kept_variables`, kube entries), `testdata/budget.tsv` if
   needed, `kept_tools.go` note :67 (add kind and kube). Real claude test: rewrite
   `TestDbMcpKeptChainThroughClaude` so prompt names task only (login, then query with the
   account), agent must choose `kept.set`/`setSecret` and follow-up on its own; run
   `bun run test:claude:db`.
4. Kube: P718 adopts scripts and kept in kube tools with Part 2's API (`kube_tools.go`, kube
   prompt cards). No kube chain panel. P718's prepare-key table and prompt-suffix draft stay.

File ownership: Part 1 frontend and tests only; Part 2 owns `kept_script.go`, `keptvars/`,
`bridge/keptvars.go`, `dbmcp.ts`, kept frontend files; Part 3 owns `registry.go`, `budget.tsv`,
`tests/claude/dbmcp_test.go`. `en.json` shared by Parts 1 and 2 (additive hunks). Order 1, 2, 3:
Part 3 text names `setSecret`, so it follows Part 2.

## Plan: model B

All of model A's parts, then:

5. P716 Part 4: chain linking and retention. Link by kept lineage: a request that reads a kept
   variable joins the chain of that variable's `source.request`; a request reading none starts a new
   chain. No agent-facing arg, no budget change. Retain per chain: answered prompt payload and
   post-transform result view only (secrets masked, never raw response), capped by existing
   `maxHeldResultBytes` per step and `maxHeldTotalBytes` across chains, oldest chain evicted first;
   chain expires with last kept variable's TTL. Files: new `internal/dbmcp/chain.go`,
   `approval.go` (retain on answer), `internal/approvals/queue.go` (answered items stay listable as
   read-only), bridge `ListChainSteps(chainID)`, `ReadChainStep(chainID, step)`. Flow tests:
   linking, unrelated call starts own chain, cap eviction, expiry, released result masked.
6. P716 Part 5: dialog step navigation. `ApprovalsDialog.vue` step strip ("Step n of m"), Back and
   Next in `ApprovalItemActions.vue`, read-only past-step view reusing each body component in
   display mode, waiting state after last step answered (closes on dismiss; reopens on next queued
   step; no timeout of its own). Queue counter and Deny all count only open items. UI spec: same
   scenario, forward, back, waiting, review per step.

B owns `approval.go`, `queue.go`, `ApprovalsDialog.vue`, `ApprovalItemActions.vue` on top of A's
files. Parts 4 and 5 follow 1 to 3; 5 depends on 4's bridge calls.

## Open questions for user

1. A or B.
2. Under B, "go back" after release: show masked post-transform result (proposed) or prompt only?
3. Under B, waiting state: keep dialog open until dismissed (proposed) or close and reopen on next
   step?
4. Secret syntax `kept.setSecret` vs options object (shots used options object).
