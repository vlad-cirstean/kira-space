# v0.3.23 code review round 4: findings

Base: `5df2a7bfc` (same base as round 3). Head: `d2f019d7a`. Scope: `git diff 5df2a7bfc..HEAD`, round 3 fixes
included (f497fec91, 499430378, e056839b7, cae3e054d, 49e49aed3), P728 to P735 and the site stream.

Areas read: script engine read-back and shadow check (`jsengine/worker.go`, `bindings.go`, `shadow.go`,
`engine.go`), `dbmcp/kept_script.go`, `keptvars/script.go`, `keptvars/store.go`, API, SQL and kube script
flows (`api_script.go`, `query_script.go`, `kube_script.go`, `kube_tools.go`, `jsscript.go`), `agentapi/kept.go`
and `inputs.go`, `api_inputs.go`, `list_tool.go`, `render.go` hints, `script_tools.go`, `bridge/keptvars.go`,
`scripts/smart.go` stale grants, `ade/linkextract.go` and `foundlinks.go`, approvals dialog grouping and Deny
all, `approvalGroups.ts`, kept frontend (`TempVarRow`, `KeptValueView`, `StudioMcpKeptPane`, `keptQueries`),
`preparedValues.ts`, `kinds.ts` Sends fact, screenshot `trimTransparent` and `capturePhone`, `site/src/data/shots.ts`,
agent-api page, `llmprompts` registry, `docs/ARCHITECTURE.md` temporary-variable sections, i18n catalog
(37 added keys, all referenced), new tests against the tests policy.

Severity counts: important 1, pre_existing 1, nit 7.

## 1. Architecture, structure, maintainability, security

1. nit, docs drift and dead code: removed expiry cap still documented and half kept.
   `docs/ARCHITECTURE.md:3370` says a value a script derives from a secret or bound value gets `Put.NotAfter`
   via `keptRun.noteExpiry` and stays bound to the origin. P732 (31f2565f2) deleted `noteExpiry`; no caller sets
   `Put.NotAfter` (`keptvars/store.go:50-51`, `:229-230` are dead). Line 3381 of the same doc says the opposite
   (no taint or bound-origin rule applies to script output). `keptvars/entry.go:66-67` and `:71-72` still say a
   script-kept value carries the read entry's Origin, and `store.go:46` says Origin binds a `SourceUser` value
   only, while `api_script.go:351` now binds every script `secret.x` to the request origin
   (`kept_script.go:562-564`). `bridge/keptvars.go` `KeptEntryView.Origin` comment ("a user-supplied value goes
   to; "" for other kinds") is stale for the same reason. Failure: a reader trusts the doc and assumes derived
   copies expire with their source. Fix: drop `NotAfter` and its doc sentence, reword the Origin comments.

2. nit, docs drift: `docs/ARCHITECTURE.md:3384` says a store refusal reaches the agent as `notSaved`; the
   field is `tempNotSet` (`kept_script.go` `keptFailure` in `queryScriptEnvelope`, `kubeScriptEnvelope`, API
   result; line 3385 of the doc already says `tempNotSet`).

3. nit, misleading comment: `frontend/src/studio/approvals/kinds.ts:203-204` says a temporary secret goes only
   to the request host because "the backend refuses any other". The backend refuses only bound entries
   (`agentapi/kept.go` `checkUndeclared`, `e.Bound()`); a secret a SQL or kube transform set is committed with
   origin "" (`query_script.go:182`, `kube_tools.go:229`) and is sent to any host. The tooltip text itself
   (request host) is right; the stated reason is wrong and invites a wrong future change.

4. pre_existing, docs drift: `docs/ARCHITECTURE.md:3506` and `:3517` name an `explain_query` tool that does not
   exist at base or head; explain is `run_query` with `explain: true` (line 3308).

## 2. Functional correctness and business logic

5. important, read-back budget: saved values share the script's output cap, and values the script only read
   count against it. `jsengine/worker.go:135` sets the read-back budget to `st.limits.OutputBytes` and
   `:142-162` re-encodes every own key of `plain` and `secret`, injected values included. The API transform
   runs with `OutputBytes: plan.MaxResponseBytes` (`api_script.go:282`, default 65536, max 524288); a prepare
   script gets `DefaultOutputBytes` 1 MiB for all keys together. The store and the agent prompt promise up to
   16 writes of 1 MiB each (`keptvars/entry.go:28`, `llmprompts/registry.go:777`). Failure scenario: a SQL
   transform saves `plain.ids` (200 KB list, allowed under the 4 MiB SQL output cap); a later `api_request`
   transform that only reads `plain.ids` to filter the response fails after the request was sent with
   `transform script failed: result exceeds 65536 bytes ... Return less, or raise maxResponseBytes`
   (`api_script.go:297`), though its return value is tiny; above 512 KB no setting helps. The number in the
   text is the remaining budget (`encode(v, budget)`, `json.go:153`), not a configured limit, so it varies per
   run. Same cap hits a transform that writes one value larger than `maxResponseBytes`. Fix: give read-back
   its own budget tied to `keptvars.MaxValueBytes` per key and `maxKeptScriptNames`, with its own error text.

6. nit, `delete secret.x` is accepted and does nothing. `keptvars/script.go:87-91` classifies it as
   `OpDelete`; `kept_script.go:114-135` (`refuseSecretReads`) has no case for kind secret plus op delete, so a
   stored secret `x` passes; `kept_script.go:101-102` adds `x` to `mayKeep` as a secret the card lists as
   saved; the `secret` global starts empty (`kept_script.go:222`), so the diff sees no change and nothing is
   deleted or reported. Failure: agent believes it removed a secret, card says it saves one. Related: `delete
   plain.tok` on a secret `tok` gets the read refusal text "scripts cannot read secrets" (`kept_script.go:122-125`),
   which names the wrong rule. Fix: refuse `delete secret.x` with a fixed text, and give plain-delete of a
   secret its own text.

7. nit, `run_script` output hints. `script_tools.go:328-330` sets "output cut: narrow with lines or contains"
   whenever `len(lines) < log.Matched`, which is true for every finished run longer than the default 50-line
   tail; narrowing is the wrong advice there (the cut came from `lines`). On a start with `scriptId`,
   `from`, `lines` and `contains` are accepted by the schema and silently ignored (`script_tools.go:282`
   passes a nil query). Fix: name `lines` (raise it) or `from: head` when the line limit cut, and refuse the
   filter args on a start like `params` is refused on a read.

8. nit, site copy: `site/src/data/shots.ts` new entry `studio-api-variables` uses title and label "Api
   variables"; every other entry and the page text write "API".

## 3. Performance and resource efficiency

9. nit, full-value copies for metadata checks. `kept_script.go` `lookup` (`:176-181`) calls `Store.Get`, which
   copies the whole value under the store mutex (`store.go:267-268`). It runs per site in
   `refuseSecretReads` (`:117`), again in `addUse` (`:164`), per write in `checkSites` (`:304`, under `k.mu`)
   and per name in `bindings` (`:211`). With 16 names of 1 MiB and a few sites each, one call copies tens of
   MB only to read `Secret` and `ID`. Fix: a meta-only store read (or one snapshot per plan) for the checks;
   load values once in `bindings`.

Nothing else real found: the read-back diff, kind and site checks, staging order (refusals before staging),
masking of staged secrets in transform output and failure text, held-review commit points, `checkPrepared`,
input keeping, approvals grouping and group-scoped Deny all, found-link extraction, `trimTransparent` and the
new tests (parser and budget tests fit the tests policy; every new feature has a flow and UI pair).
