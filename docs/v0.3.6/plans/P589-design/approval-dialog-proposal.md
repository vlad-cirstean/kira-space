# Approval dialog: final design and implementation spec

Mock: `approval-dialog-proposal.html` and `approval-dialog-proposal-dark.png`. It shows all nine kinds:
- a Studio dialog with 8 items;
- a Space dialog with 2 items.

The app is dark-only, so `-light.png` is the same render.

Surface:
- `packages/workbench/src/approvals/ApprovalsDialog.vue`;
- per item `ApprovalItemCard.vue`;
- kind renderers through `registry.ts`, registered in `apps/kira-space/frontend/src/studio/approvals/kinds.ts` and `apps/kira-space/frontend/src/ade/v2/request/kinds.ts`.

## Why the grey line existed

- `ApprovalItemCard.vue:37` sets `border-l-[3px] border-border pl-3`, coloured from `connColorVar(target.color)` (`:23`, `:38`).
- With no colour it falls back to grey `border-border`.
- It is an accident, against `docs/ARCHITECTURE.md:53` (marks through `colorMarkClass`; no colour leaves the slot empty).
- `DbExemptionBody.vue:71` has a second bar (`border-l-2`).
- Both go away.

## Item anatomy

```
Operation  Target  where  ⚠ risk ⚠ risk                    m:ss left
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  (colour rule, only with a colour)
   content (unboxed, pl-3, data font)
   Label   value                                   (only data not shown above)
requester                              [option]  Deny  Approve
```

- **Item.** A flat section on `bg-dialog`, `py-3.5 flex flex-col gap-2`. Items are split by `divide-y divide-border` (one hairline); dialog body `px-4`. No card, box, fill or rounded rectangle inside the dialog.
- **Line 1, left.** One `flex flex-wrap items-baseline gap-x-2.5`:
  - **operation**: `text-kira-lg font-semibold`, tone colour: `text-warn` caution, `text-error` danger, `text-muted-foreground` neutral;
  - **target**: `text-kira-lg font-semibold text-fg`; `font-data` for host, repo and path targets;
  - **where**: `text-kira-md text-muted-foreground`;
  - **risk tags**: `text-kira-md text-warn-text`, 13px codicon `warning` in `text-warn`, 1-4 words, only when true.
- **Line 1, right.** Countdown only, `text-kira-sm text-subtle tabular-nums`, "{time} left".
- **Colour rule.**
  - Markup: `<span v-if="connBgClass(item.target.color)" :class="colorMarkClass('band', item.target.color)" />` right after line 1.
  - `band` exists (`packages/theme/src/connColor.ts:64`, `h-0.5 shrink-0`), and `ARCHITECTURE.md:53` lists it, so no new mark is needed.
  - As a block child of the flex column it spans the item width.
  - With no colour, nothing is drawn: `v-if`, not an empty slot, so no 2px gap.
  - Sources today: DB connection colour (db.query, db.exemption, db.response) and API environment colour. kube.read and script.run send no `Target.Color` (`internal/dbmcp/approval_item.go:146,154`), so they show no rule. Adding one is a backend follow-up, not part of this phase.
- **Content.** `pl-3`, `font-data text-kira-md`, no border or fill.
  - SQL keeps `MonacoHost` with a transparent background (`editor.background` = `--kira-bg-dialog`), no line numbers or gutter, no minimap, and auto height up to 12 lines.
  - Formatted/As sent: text tabs `text-kira-sm`, absolutely placed top-right of the content, active underlined in `accent`.
- **Details.** `KeyValueList` with `grid-cols-[72px_minmax(0,1fr)]`, `dt` `text-kira-sm text-subtle`, `dd` `text-kira-md`.
  - Pairs use a `font-data` grid with names muted.
  - A list over 3 rows shows 3, then a "+N more" text button: shadcn-vue `Collapsible`, closed by default.
- **Footer.**
  - Requester `text-kira-sm text-subtle`, truncated, left. Empty when the kind has no origin (proof.review).
  - Then the kind option: Review result checkbox, or the Open window text button.
  - Then Deny (`dialog-danger`) and Approve (`dialog-primary`), with per-kind labels kept.
- **Type.** 3 sizes (`text-kira-sm`, `text-kira-md`, `text-kira-lg`), 2 text weights (400, 600). The Button primitive keeps 500.
- **Dialog footer.** Deny all / Approve all unchanged. The countdown badge in the header goes; the count badge stays.

## Each datum once: moves since the current UI

| Kind | Datum | Was | Now |
|---|---|---|---|
| db.* | connection name | header + lede | L1 target |
| db.* | kind | header kind + lede | L1 where |
| db.query | class | badge + "Looks like …" | L1 operation. "Looks like {class}" risk only when it differs |
| api.request | host | header + URL + target line | L1 target. Content is path + query |
| api.request | environment | header location + target line | L1 where; its colour is the rule |
| api.request | query params | URL + Query table | content path |
| api.request | credential | credential line + Authorization header | Headers row. "Credential sent" risk flags it |
| api.request | content type | Body legend + header | Headers row |
| branch.request | task title | header + origin | L1 target |
| branch.request | step / source | origin + group label | footer |
| task.create | title | header + Title row | L1 target |
| task.create | orchestrator | origin + row | footer |
| script.run | cwd | header location + Folder line | L1 where |
| kube.read | cluster | header + lede | L1 target |
| all | operation | right badge | L1 left operation |
| all | countdown | header badge | L1 right |
| all | origin | "Requested by …" line | footer |

## Slot per field

Slots: Op, Tgt, Where, Risk (all line 1, left); Time (line 1, right); Rule; C content; D detail row; F footer; X dropped.

| Kind | Field | Slot |
|---|---|---|
| all | operation class + tone | Op |
| all | target.name | Tgt |
| all | target.location / kind | Where |
| all | target.color | Rule |
| all | target.readOnly | Risk "Read-only" (neutral colour) |
| all | expiresAt | Time |
| all | origin client, cwd / task, step, source | F |
| all | answer error | Risk (error colour) |
| db.query | connection name | Tgt |
| db.query | kind label, host, db / schema path | Where |
| db.query | mode / class | Op (Read, Write, DDL, Masked read) |
| db.query | statement | C |
| db.query | formatted / as sent | C tabs |
| db.query | plan estimate over threshold | Risk "~{count} rows" |
| db.query | plan issues | Risk "{severity}", max 2 then "+N" |
| db.query | declaredHigher | Risk "Looks like {class}" |
| db.query | detected equal to mode | X |
| db.query | enforcement | X |
| db.query | estimate under threshold | X |
| db.query | review result | F checkbox "Review result" |
| db.query | lede | X |
| db.exemption | connection | Tgt |
| db.exemption | kind, host, db | Where |
| db.exemption | operation | Op "Lift limit" (danger) |
| db.exemption | limit hit | Risk "Timeout {measured} / {max}" or "~{count} rows" |
| db.exemption | no-timeout note | X (implied by the timeout tag) |
| db.exemption | statement | C |
| db.exemption | agent's reason | D Reason |
| db.exemption | review result | F checkbox |
| db.exemption | lede, enforcement, detected, plan | X (plan estimate as Risk if over) |
| db.exemption | buttons | Keep the limit / Lift for this query |
| db.response | connection | Tgt |
| db.response | kind, host, db | Where |
| db.response | operation | Op "Result" |
| db.response | already ran | Risk "Already ran" |
| db.response | result rows | C (unboxed table, `font-data`, 3 rows + "+N more rows") |
| db.response | As sent view | C tabs |
| db.response | statement | D Statement (one line, truncated, Tooltip full) |
| db.response | lede | X |
| db.response | buttons | Withhold / Send |
| kube.read | cluster context | Tgt |
| kube.read | server · namespace | Where |
| kube.read | operation | Op (Discover, List, Describe, Logs) |
| kube.read | statement / object | C |
| kube.read | lede | X |
| script.run | script name | Tgt |
| script.run | cwd | Where |
| script.run | operation | Op "Run script" |
| script.run | command | C |
| script.run | description | D Note |
| script.run | params | D Params (pairs; secret → "secret · saved" / "secret · unset") |
| script.run | env | D Env (pairs) |
| script.run | lede | X |
| api.request | method | Op (tone from `methodTone`) |
| api.request | host | Tgt (`font-data`) |
| api.request | environment name | Where |
| api.request | environment colour | Rule |
| api.request | path + query | C first line |
| api.request | body / form | C |
| api.request | credential on non-read | Risk "Credential sent" |
| api.request | body truncated | Risk "Body truncated" |
| api.request | headers | D Headers |
| api.request | variables | D Variables |
| api.request | redirects, timeout | D Options "Redirects on/off · {seconds} s" |
| api.request | full URL, query table, target line, body type | X |
| api.request | buttons | Don't send / Send |
| proof.review | proof name | Tgt |
| proof.review | revision | Where "rev {n}" |
| proof.review | operation | Op "Edit" |
| proof.review | steps (ordinal, description, diff) | C |
| proof.review | reason | D Reason |
| proof.review | Open window | F text button |
| proof.review | buttons | Reject / Accept |
| branch.request | task title | Tgt |
| branch.request | operation + count | Op "Branches · {count}" |
| branch.request | item checkbox | C row |
| branch.request | repo | C row, `font-data` semibold |
| branch.request | branch | C row, `font-data` |
| branch.request | kind + base / from | C row tag ("new · from {base}", "checkout", "rename from {from}", "worktree") |
| branch.request | holder (adopt) | C row tag "held by {task}" / "held by main" |
| branch.request | holder worktree path | Tooltip on the tag |
| branch.request | dirty | C row Risk "Dirty worktree ({count})" |
| branch.request | main checkout detached | C row Risk "Detaches main" |
| branch.request | rename on remote | C sub-row checkbox "Also remote {upstream}" |
| branch.request | call source (more than one call) | C group label |
| branch.request | stacked branches | C row tag "+ {branches}" |
| branch.request | naming hint | X |
| branch.request | step / source | F |
| task.create | new title | Tgt |
| task.create | operation | Op "Create task" |
| task.create | brief | C (AdeNotesEditor readonly, unboxed) |
| task.create | workflow | D Workflow |
| task.create | repositories + base | D Repository ("{repo} from {base}") |
| task.create | parent orchestrator + step | F |
| unsupported kind | — | Risk "Unsupported" |

## Files

Shared (`packages/workbench/src/approvals/`):
- `ApprovalItemCard.vue:23,36-118`: rewrite to the anatomy above.
  - Drop the `border-l` and inline style, the header badges and the `FieldDescription` origin.
  - Render `def.content`, `def.details` and `def.risks`.
- `registry.ts:8-17`: `ApprovalKindDef` becomes:
  - `operation(item)` (exists, now required);
  - `where?(item)`;
  - `risks?(item): string[]`;
  - `requester?(item)`;
  - `content: Component` (renamed from `body`);
  - `details?: Component`;
  - `footerAction?: Component`;
  - `labels?`, `individualOnly?` (kept);
  - `header?` removed.
- `ApprovalsDialog.vue:52-60`: body `px-4 divide-y divide-border`, item `first` prop kept.
- New `ApprovalDetails.vue`: 72px `KeyValueList` wrapper, plus `ApprovalPairs.vue` with a `Collapsible` "+N more" over 3 rows.
- `useCountdown.ts`: unchanged; the format moves to "{time} left".

Studio (`apps/kira-space/frontend/src/studio/approvals/`):
- `kinds.ts:36-90`: per-kind `operation`, `where`, `risks`, `requester`.
- `DbQueryBody.vue:51-78`: content only; plan, declaredHigher and enforcement move to `risks` or are dropped.
- `DbExemptionBody.vue:56-87`: content + Reason detail; drop the `:71` bar and the lede.
- `DbResponseBody.vue:18-37`: drop the lede and the Alert at `:33` (now a risk).
- `PendingResponse.vue:143-220`: unboxed table, 3 rows + "+N more rows".
- `KubeReadBody.vue:20-30`, `ScriptRunBody.vue:25-77`, `ApiRequestBody.vue:30-96` (path + query, not the URL), `ProofReviewBody.vue:15-36` (Open window moves to `footerAction`).
- `ApprovalStatement.vue:56-82`: text tabs; Monaco transparent with no gutter.
- `ApprovalPlanEvidence.vue`: delete (folded into `risks`).
- `ReviewResultOption.vue`: footer placement, label "Review result".
- Delete `ApiPairsTable.vue` and `ConnectionHeader.vue`. `KIND_LABEL` / `KIND_ACCENT_CLASS` move into `where`.
- `../editor/MonacoHost.vue`: props `transparent`, `lineNumbers=false`, `autoHeight`, if missing.

Space (`apps/kira-space/frontend/src/ade/v2/request/`):
- `kinds.ts:9-14`: operation, risks, requester.
- `BranchRequestBody.vue:118-178`: row grid (checkbox, repo, branch, tag, risk).
- `TaskCreateBody.vue:16-58`: brief + details.

Theme: no token or mark change. `colorMarkClass('band')` and `connBgClass` already exist.

## i18n (`packages/theme/src/i18n/en.json`)

Add:
- `workbench.approvals.left.status` "{time} left";
- `workbench.approvals.more.label` "+{count} more";
- `workbench.approvals.moreRows.label` "+{count} more rows";
- risk tags under `workbench.approvals.risk.*`:
  - `readOnly` "Read-only";
  - `rows` "~{count} rows";
  - `looksLike` "Looks like {class}";
  - `timeout` "Timeout {measured} / {max}";
  - `alreadyRan` "Already ran";
  - `credentialSent` "Credential sent";
  - `bodyTruncated` "Body truncated";
  - `dirty` "Dirty worktree ({count})";
  - `detachesMain` "Detaches main";
  - `unsupported` "Unsupported";
- operation tags:
  - `studio.dbMcp.approval.op.{liftLimit,result,runScript}`;
  - `studio.proof.approval.op.edit`;
  - `space.ade.branchRequest.op.label` "Branches · {count}";
  - `space.ade.request.taskCreate.op.label` "Create task";
- row tags `space.ade.branchRequest.tag.{new,checkout,rename,worktree,heldBy,heldByMain,stacked}`;
- detail labels `studio.dbMcp.approval.detail.{reason,statement,headers,variables,options,params,env,note}`;
- `studio.dbMcp.approval.api.options.value` "Redirects {state} · {seconds} s";
- `studio.dbMcp.approval.reviewResult.short` "Review result";
- `studio.proof.approval.rev.label` "rev {n}".

Remove once unused:
- `studio.dbMcp.approval.{read,write,ddl,unknown,other}.body`;
- `studio.dbMcp.approval.enforcement.*`;
- `studio.dbMcp.approval.detected.label`;
- `studio.dbMcp.approval.declaredHigher.body`;
- `studio.dbMcp.approval.estimate.body`;
- `studio.dbMcp.approval.exemption.{lede.body,noTimeout.body,limit.label}`;
- `studio.dbMcp.preview.{lede,requested,ran}.body`;
- `studio.dbMcp.approval.kube.*.body`;
- `studio.dbMcp.approval.script.lede.body`, `script.folder.label`;
- `studio.dbMcp.approval.api.{credential.label,body.label,variables.label,options.label}`;
- `space.ade.branchRequest.{item.*,naming.hint,stacked.hint,dirty.hint,mainCheckout.hint,source.*}`;
- `workbench.approvals.origin.status`;
- `workbench.approvals.unsupported.body`;
- `common.expires.status` only if no other caller (grep first).

## Specs and snapshots

Keep every testid on its logical element:

| Testid | Element |
|---|---|
| `approval-target-name` | Tgt |
| `approval-target-kind` | Where |
| `approval-operation` | Op |
| `approval-origin` | F requester |
| `approval-countdown` | Time |
| `approval-error` | error Risk |
| `approval-deny-*`, `approval-approve-*` | buttons |
| `api-approval-url` | content path line |
| `api-approval-target` | Where |
| `ade-branch-request-holder-*` | row tag |
| `ade-branch-request-dirty-*` | row risk |
| `db-mcp-approval-plan-rows`, `db-mcp-approval-declared-higher`, `db-mcp-exemption-limit`, `db-mcp-preview-ran` | risk tags |

Update:
- Visual: `apps/kira-space/tests/visual/studio/approvals.spec.ts` (`approvals-dialog-visual-linux.png`; `approvals-toast-visual-linux.png` unchanged). Regenerate in PR CI.
- Screenshots: `apps/kira-space/tests/screenshots/build/dbPrompt.shot.ts`.
- UI specs whose text assertions change:
  - `studio/approvals-api.spec.ts:30-35`: URL text becomes path + query; the credential line becomes the "Credential sent" tag;
  - `studio/dbmcp-approval.spec.ts:25-31`;
  - `studio/dbmcp-approval-mode.spec.ts`, `dbmcp-review-mode.spec.ts`: enforcement and masked text gone; masked shows as Op "Masked read";
  - `studio/dbmcp-approval-script.spec.ts`: Folder moves to Where;
  - `studio/dbmcp-preview.spec.ts:50`;
  - `studio/approvals-exemption.spec.ts`: lede and no-timeout removed;
  - `studio/kubernetes-mcp.spec.ts:86`;
  - `studio/connection-mcp-{timeout,masked,expensive}.spec.ts`;
  - `tests/ui/approvals.spec.ts`;
  - `ade-v2-branch-request.spec.ts:42`: source text moves to the footer;
  - `ade-v2-branch-adopt.spec.ts:61-69`;
  - `ade-v2-branch-rename.spec.ts`;
  - `ade-v2-orchestrator.spec.ts`;
  - `prompts.spec.ts`;
  - `copy-coverage.spec.ts`: keys added and removed.
- Flow tests (Go) are unaffected: the payloads do not change.
