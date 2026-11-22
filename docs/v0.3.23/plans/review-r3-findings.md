# v0.3.23 code review round 3 findings

Base commit: `5df2a7bfc` (record v0.3.23 review round 2 fixes). Head commit: `3663770dd`.
Scope: `git diff 5df2a7bfc..HEAD`, P728 to P735 including merged stream P734 (presentation site) and every
fixer commit since base.

Severity: important (fix in this chapter), pre_existing (bug older than base, surfaced here), nit (small,
fix cheaply).

Areas read: `dbmcp` (kept_script, list_tool, kube_tools, kube_script, api_script, api_tools, query_script,
jsscript, approval_item, tools, kept_params, kept_tools), `jsengine` (bindings, shadow, worker read-back,
json round-trip), `keptvars` (script scan, refs, store, watch), `agentapi` (kept, resolve), `bridge/keptvars`,
`scripts/smart.go` stale grants, `apifiles` refactor, `ade` found links plus migration 0106, workbench
approvals (dialog, groups, store, queries), `studio/kept/*`, Settings temporary variables pane,
`formatScript`, `keptHighlight`, `ReviewTable`, screenshot `capture.ts` trim and `phone.ts`.

## 1. Architecture, structure, maintainability, security

Rule checks: changed Vue files are all `<script setup lang="ts">`, no scoped `<style>` blocks, no Options API,
no hand-rolled DOM listeners, timers, clipboard or storage in new code; server state stays in TanStack Query
(`keptQueries.ts`, `approvalsQueries.ts`); `useApprovalsStore` lost a concern (input modes), no store gained
one. No file under `githost/`, `issuetracker/`, `internal/docker` or `internal/harness/claude` changed (G5
untouched). Patch bump: no legacy-across-minor question. Secret paths checked and clean: read-back refusals
never quote values, staged secrets are masked from transform output, staged ids (`staged:`) are kept out of
`watchKept`, kept values insert in one pass (no re-scan for `{{...}}`), secret previews never leave Go.

1. nit. `internal/scripts/smart.go:350`: `studioMoved` maps `list_saved_variables` to `list` but not
   `list_kept_variables`. `list_saved_variables` existed only inside this chapter (P732, unreleased);
   `list_kept_variables` is the shipped name (tag v0.3.18 and base `5df2a7bfc`, `kept_tools.go:78`).
   Scenario: a smart script granting `kira-studio` / `list_kept_variables` fails validation with "use one of
   the current tools" instead of naming `list`. Add `list_kept_variables` (keep or drop the unreleased
   name). Category: maintainability.

2. nit. `apps/kira-space/internal/adapters/sqlparams.go:349-407`: `DecodeParam` refusals say "kept variable
   %q ..."; `dbmcp/kept_params.go:132-133` returns them verbatim to the agent. P735 moved every agent-facing
   text and error to "temporary variable". Scenario: `run_query` with `params: ["ids"]` where `ids` is an
   object gets "kept variable "ids" is an object ..." next to "temporary variable" wording everywhere else.
   Category: maintainability (agent-facing naming).

3. nit. `docs/ARCHITECTURE.md:3308`: says `list` takes `what` (connections, clusters, environments,
   scripts, saved). Code section is `temp` (`dbmcp/list_tool.go:25`, P735). Category: maintainability (doc
   drift).

## 2. Functional correctness and business logic

4. important. `packages/workbench/src/approvals/ApprovalsDialog.vue:55-61` with
   `packages/workbench/src/state/approvals.ts:86`: after an answer the dialog jumps to the first group, not
   the next item of the shown group. `group` resolves from `focused`; picking a tab sets `store.focusId` to
   that group's first item; answering it removes the item from `props.items`, so `focused` is undefined and
   `group` falls back to `groups[0]` (and `prune` nulls `focusId`). Scenario: two sessions A (older) and B,
   each with 3 prompts; the person picks tab B and approves B1; the dialog shows A1, not B2. The code
   comment (":50-51 answering moves to the next oldest of the same group") and plan P732 §8 ("After an
   answer the next item comes from the same group first") both promise the opposite. Fix: remember the
   shown group key (store or local ref) and fall back to its first remaining item before `groups[0]`.
   Category: functional.

5. important. `apps/kira-space/internal/keptvars/script.go:51-53` with
   `apps/kira-space/internal/dbmcp/kept_script.go:114-131` and `:216-218`: a script that reads a stored
   secret as `secret.x` is never refused; it gets `undefined`. `ScanScript` marks every `secret.x` mention
   as a write, `refuseSecretReads` refuses only `plain.x` of a secret and `secret.x` of a plain entry, and
   the `secret` global starts as `{}`. Scenario: a temporary secret `token` exists; the agent sends
   `api_request` with prepare `request.headers = [{name: "Authorization", value: "Bearer " + secret.token}];
   return request;`. Planning passes, the card lists `token` under the script's possible secret writes
   (`addMayKeep`), the user approves, and the server receives `Authorization: Bearer undefined`. After the run
   `secret` has no key, so nothing flags it. The same holds in a transform or a kube or run_query script.
   The `plain.x` case already refuses with "scripts cannot read secrets"; the `secret.x` read needs the same
   treatment, for example: after the run, refuse a stage whose source mentions `secret.x` for a name the
   store holds as secret while the read-back `secret` object lacks that key, or refuse at plan time a
   `secret.x` mention that ScanScript would classify as a read when scanned like `plain`. Category:
   functional (secret handling).

6. nit. `apps/kira-space/internal/dbmcp/approval_item.go:225`: `itemOf` maps `Client`, `Cwd` and `Session`
   but not `r.Origin.TaskID` / `StepID` into `approvals.Origin` (which has `Task` and `Step`). The new session
   tabs label each group with `approvalRequester` (`ApprovalsDialog.vue:64-75`), which for DB, API and kube
   kinds is `client · cwd · task · step`. Scenario: two agent sessions on the session server from the same
   client in the same folder show two tabs with identical labels; `approvalGroupKey`'s `task:` branch is
   also dead for these kinds. Mapping task and step (already in `ApprovalOrigin`) makes the tabs
   distinguishable. Category: functional (UI clarity).

Checked and clean: read-back diff (`diffGlobals`) is stable because every plain value in the store comes
from JS `JSON.stringify` (big integers round-trip through `number`/BigInt), so a read-only script never
produces a spurious write; `checkKinds`/`checkSites`/`checkLimits` run before any staging; `commit` re-checks
deleted ids; `deliverAPI` commits only after the review passes; `kubeClamp` reapplies after prepare;
`describe` path/columns merge keeps `allowedPath` gates on both branches; Deny all covers the shown group
only; stale-grant validation covers both Kira server names; found links merge last and never replace a
detected or stored artifact; dismissals cascade with the task.

## 3. Performance and resource efficiency

Nothing real. Found-link extraction is per task read with row, per-row and total caps; `ReviewTable` measures
at most 200 rows per result change; `trimTransparent` is test tooling only; per-run Masker construction and
store lookups are bounded by 16 names.
