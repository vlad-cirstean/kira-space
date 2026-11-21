# Batch 1: P719, P720, P723 as parallel streams

User request: run P719, P720 and P723 now as one batch. P716, P717, P718, P721 and P722 are not in
it and keep their table slots.

Discovery: `codegraph_explore` (index in `/home/user/kira-space` is stale, v0.3.17: `agentapi`,
`api_inputs.go`, `kept_script.go` missing), every fact re-read in the worktrees below.

## Streams

Base commit for all three: `7f9a49819` (`chapter/v0.3.23`, rebased onto `main`; `main` holds P715
from v0.3.22).

| Stream | Phase | Worktree | Branch | Plan |
|---|---|---|---|---|
| A | P719 one-time API input values | `/home/user/kira-space-v0.3.23-p719` | `stream/p719` | `plans/P719.md` |
| B | P720 API variables and environments UX | `/home/user/kira-space-v0.3.23-p720` | `stream/p720` | `plans/P720.md` |
| C | P723 remove PR flake rerun workflow | `/home/user/kira-space-v0.3.23-p723` | `stream/p723` | `plans/P723.md` |

Each stream: own Sonnet implementer, own commits, own hook runs. Design gate: P719 and P720 are
design phases (SPEC intro). `P719.md` records user approval of every decision. `P720.md` still says
"User approves this plan before implementation starts" and lists its open questions with defaults
only. Confirm user approval of P720 before Stream B's implementer starts.

## Overlap verdict: small

P719 and P720 share a word ("variables"), not a subsystem.

- P719 is the Studio MCP side: `dbmcp` approval answer and inputs, `agentapi` input planning,
  `keptvars` store (in-process memory), agent prompt text, the approval card.
- P720 is the API module side: `apifiles` config files and secrets, `ApiConfigService`, the
  `VariablesRepo` projection, tab kinds, the `studio/api/` views.
- P719 decision 5 settles it: only `inputs` (`{{kept.<name>}}`) are in P719; module `{{name}}`
  variables are P720.

One semantic seam, read-only from A's side. `agentapi` reads environment files through
`apifiles.EnvironmentRef`, `apifiles.EnvironmentFile`, `apifiles.Opened` and
`apifiles.SecretNameOf`, and the file keys `agent`, `agentHosts`, `agentAutoApproveReads`,
`agentReview` (`agentapi/types.go:189-197`, `policy.go:30-59`, `send.go:76,202`). P720 keeps the file
format and agent tools unchanged. Contract for the batch:

- B never changes the signature or meaning of those four `apifiles` symbols or those file keys.
  New code goes in new files (`variables_edit.go`) or new fields.
- A never edits anything under `internal/apifiles/`, `internal/apivars/`,
  `internal/storage/repos/variables.go` or `studio/api/`.
- Either stream needing to cross this line stops and reports; no split then, sequence B after A.

No shared Go package is edited by both. `dbmcp.ts` schema: A only. `ApiConfigService`: B only.
`shortcuts.ts`: B only (A adds no shortcut). API prompt card (`studio/approvals/`) and the variables
UI (`studio/api/`, `views/httprequest/`) share no component file.

## Live data

No collision.

- Kept values live in `keptvars.Store`, an in-process map with expiry; nothing on disk. One-time
  values (A) never enter it.
- Environment and collection variables live in `<stem>.environment.yaml` / `.collection.yaml`, the
  `api_*` projection and `secret_values` (module namespace). B's `lock` and duplicate copy write
  only there.
- Namespaces differ: `{{kept.<name>}}` versus `{{name}}`. Masking (`agentapi.newMask`) still takes
  environment secrets from `apifiles.Opened` plus kept entries plus A's one-time overlay, a union.
- Settings: A reads `dbMcp.keptRetentionMinutes` only; B adds no setting.
- Copy only: Settings "Studio MCP variables" (kept) and B's "Variables" tab are different surfaces.
  Neither renames the other.

## File ownership

Paths under `apps/kira-space/` unless they start with `packages/`, `internal/`, `docs/` or `.github/`.

### Stream A (P719)

| File | Change |
|---|---|
| `internal/agentapi/inputs.go`, `kept.go`, `types.go` | `InputDecl.Reuse`, `InputOnce`, overlay lookup, `Input.Once` |
| `internal/keptvars/store.go` | `Retention()` |
| `internal/dbmcp/approval.go`, `api_inputs.go`, `api_tools.go`, `kept_script.go` | answer modes, `Supplied`, keep split, wipe, `onceRead`, `HeldKeep.Blocked` |
| `internal/llmprompts/registry.go` and its goldens | `inputs` one-time text, `inputs.reuse` |
| `internal/flows/dbmcpflow/e2e_user_value_once_test.go` (new), `userinput_test.go`, `e2e_user_value_test.go`, `kept_script_test.go` | flow half |
| `tests/contract/e2e-user-value-once.json` (new) and contracts the edited dbmcp flows regenerate | contract |
| `packages/shared/domain/dbmcp.ts` | `apiInputSchema` `mode`, `keepMinutes`; keep `blocked` |
| `packages/workbench/src/state/approvals.ts`, `packages/workbench/src/approvals/approvalsQueries.ts` | `inputModes` |
| `frontend/src/studio/approvals/UserInputsPanel.vue`, `ApiResponseBody.vue` or `StagedKeepList.vue` | card, blocked keep line |
| `packages/workbench/src/components/SecretInput.vue` | only if `aria-invalid` needs styling: it already passes non-class attrs to the input |
| `tests/ui/studio/approvals-api-input-once.spec.ts` (new), `approvals-api-input.spec.ts` | UI half |
| `tests/visual/studio/approvals.spec.ts` and its snapshots dir | mixed-card shot |

### Stream B (P720)

| File | Change |
|---|---|
| `internal/apifiles/variables_edit.go` (new), `lifecycle.go`, `import.go` | `EditVariables`, duplicate copies secrets, `secretsUnset` |
| `internal/bridge/apiconfig.go` | `ApiConfigService.EditVariables` |
| `internal/storage/repos/variables.go`, `internal/storage/model/variables.go`, `internal/storage/model/tabs.go` | `secretSet`, `agent`; `variable-set` kind restore |
| `internal/flows/apiflow/variables_matrix_test.go` (new), `variables_test.go`, `variables_form_test.go`, `agentaccess_test.go` | flow half |
| `tests/contract/api-variables-matrix.json` (new), contracts of the edited api flows | contract |
| `packages/shared/domain/variables.ts`, `packages/shared/domain/apiConfig.ts` | new fields, import report |
| `packages/shared/domain/shortcuts.ts` | `grid.rename` (F2) |
| `frontend/src/studio/api/**` (new `VariablesView.vue`, `VariablesMatrix.vue`, `VariableCell.vue`, `EnvironmentColumnMenu.vue`, `VariableRowMenu.vue`, `RequestVariablesPanel.vue`, `state/variablesMatrix.ts`; edits and deletes per `P720.md` §5) | matrix UI |
| `frontend/src/studio/views/httprequest/HttpRequestView.vue`, `views/grpcrequest/GrpcRequestView.vue` | unresolved Button, popover |
| `frontend/src/studio/shortcuts/studioCommands.ts` | drop `api.variables` |
| `frontend/src/state/tabDomain.ts`, `frontend/src/state/tabKinds.ts` | drop `variable-set`, restore mapping |
| `tests/ui/studio/api-variables-matrix.spec.ts` (new) and the specs `P720.md` §6 rewrites or removes (`tests/ui/module-actions.spec.ts` sits in `tests/ui/`, not `tests/ui/studio/`), `tests/ui/studio/support/apiRows.ts` | UI half |
| `tests/visual/studio/variables.spec.ts` (new), `tests/visual/studio/http-request-view.spec.ts-snapshots/` | baselines |

### Stream C (P723)

| File | Change |
|---|---|
| `.github/workflows/pr-rerun-flaky.yml` | delete |
| `docs/PACKAGING.md` | §7 status line, "Chapter PR to release" steps |

### Shared files

Each edit is an additive hunk in a region the other stream does not touch.

| File | A | B | C |
|---|---|---|---|
| `packages/theme/src/i18n/en.json` | add `studio.kept.input.*`; retire the old "Kira keeps these values" hint key | add `studio.api.variables.*`; prune keys of deleted `studio/api/` files only | none |
| `docs/ARCHITECTURE.md` | "User-supplied values (P679)" (~line 3375): one-time bullets | "Api config files (P473)" (~line 3720): `EditVariables`, duplicate copies secrets, `variable-set` restore mapping | none |
| `tests/ui/copy-coverage.spec.ts` | only if the new card copy needs a case: new step in the approval dialog test | rewrite the variables steps | none |
| `internal/appwire/dbmodule.go` | only if `Retention()` needs wiring: one line in the kept store block | only if `EditVariables` needs a new dependency: one line in the API services block | none |
| `docs/v0.3.23/SPEC.md` | own `## P719` section Result only | own `## P720` section Result only | own `## P723` section Result only |
| Wails bindings (`frontend/bindings/`, generated, untracked) | no bridge signature change (approval options cross as `Record<string, unknown>`) | regenerate | none |

Rules:

- B's catalog pruning removes only keys whose last caller B deleted. Never a `studio.kept.*` or
  `studio.approvals.*` key.
- No stream edits the phase table (lines 14-23 of `SPEC.md`): rows 19, 20 and 23 sit within three
  lines of each other and would conflict. The orchestrator marks status after landing.

## Ordering dependency

None between A, B and C. Confirmed:

- C touches only CI config and `docs/PACKAGING.md`, which neither A nor B edits.
- A and B share no Go package and no frontend component; their single seam (above) is read-only
  from A and frozen for B.
- Neither plan consumes anything the other adds: A uses no new `apifiles` call, B uses no new
  `dbmcp`/`keptvars` call.

Outside the batch, not blocking it: P716 and P717 sit earlier in the table and touch the files A
edits (`kept_script.go`, `api_tools.go`, `registry.go`, `studio/approvals/`). The user put P719
ahead of them for this batch. P716's plan then plans against a tree with P719 landed. SPEC P719 also
asks for one-time wording in `DbmcpInstructions`, which P716 adds. `P719.md` covers the tool and arg
texts only, so P716 must carry the one-time sentence into the instructions it writes.

## Landing

Once every stream is implemented and verified in its own worktree, rebase onto `chapter/v0.3.23`:

1. C first: smallest, no shared file.
2. A next.
3. B last: largest diff, so its rebase resolves the `en.json` and `ARCHITECTURE.md` hunks once.

Any order works. A conflict outside the shared table above means the ownership boundary was wrong:
stop. After all three land, on the chapter branch: regenerate bindings, `bun run lint` (catalog
`--unused` sees both streams' keys together), `bun run typecheck`, `go build ./...`. Then remove the
three worktrees and branches and push.

C takes effect only after this chapter's PR merges: `workflow_run` workflows run from `main`, so the
old rerun workflow still reruns this chapter's failed PR runs.
