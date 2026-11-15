# P643 analysis: one `create` + edit + `submit` pattern for every created resource

Opus analysis for the user's question, no code. Base `f1aad9aae` (chapter/v0.3.14). Plan under test:
`P643-details-on-disk.md` sections 7 to 9 and 16. Evidence: `P643-spike-results.md` (Parts 1, 1b,
1c), spike code under `apps/kira-space/tests/claude/details_spike*_test.go`, raw transcripts in the
session scratchpad (`d1b-main`, `d1c`, `dumpfull`, `dump`, `d1b`, `d1b-f5`: 105 runs re-counted
here), current code through `codegraph_explore`.

Question: replace `proof` (content in one call) and `create_task` (blocking approval) with one
`create {kind, args}` tool. Kira writes a valid skeleton (folder, headers, slug, paths), the agent
edits, then calls `submit`.

Verdict: reject. Keep the plan as is. Details in section 6.

## 1. Inventory: what an agent creates or changes

CONTENT = file-shaped, file is truth (plan Decisions). STATE = SQLite, never driven by a file edit
(binding user decision). "Fit" = can follow create + edit + submit without breaking a decision.

| Resource | Today (code) | Plan | Class | Fit | Why |
|---|---|---|---|---|---|
| User note | UI `AddNote`; agent none | `notes/*.md`, agent edit = prompt with reason | CONTENT | no value | agent rarely creates one (writer matrix: agent R); name free (`noteRe` accepts any `[A-Za-z0-9._-]` name); body only |
| Link, Jira slot, PR slot | `report_artifacts` (`agentmcp/space.go`, `ade/artifacts.go`) | `links.yaml` entry plus `submit` | CONTENT | no | an entry is not a file; one file per task; nothing to scaffold |
| Report artifact (kind proof attach) | `report_artifacts` kind proof | removed with attach (Q10) | - | - | gone |
| Handover draft | `handover` tool (`RecordHandover`), `finish_step` summary | `handovers/draft-<step>.md` plus `finish_step` or `submit {finished}` | CONTENT | negative | path fixed and named in the prompt; a pre-created file turns a 1-call `Write` into `Read` plus `Write` (section 2) |
| Handed handover | Kira on stage move, child brief (`createChild`) | Kira writes numbered file; agent edit = prompt | CONTENT (Kira-made) | no | agent never creates one |
| Proof, new | kira-studio `proof_save` one call (`proof.Service.Save`) | `proof` tool writes `proofs/<slug>/proof.md` and runs submit (`taskfiles.Request.Tool = "proof"`) | CONTENT | yes, technically | cost: section 2 |
| Proof, edit | `proof_save` with reason, ordinals from `proof_list` | `proof` patch by step id, or Read plus Edit plus `submit` | CONTENT | n/a | resource exists; create has nothing to do |
| Proof image | base64 in payload | file beside `proof.md`; `proof` copies `image` path in | CONTENT | n/a | binary, copied, never scaffolded |
| Child task | `create_task`: validate (`planChildTask`), `task.create` approval, blocks up to 30 min (`SpawnTask`), `createChild` | unchanged (user decision) | STATE (task row, workflow, repos, branches, plan day, approval) plus one CONTENT field (brief = child's first handed handover) | brief only | title, workflow, repos are state; driving them from `children/<name>/task.yaml` is state driven by a file edit |
| Branches declare, request, rename, change | 4 tools, request/rename/change block 30 min | `branches {action}` | STATE | no | binding decision |
| Lease acquire, release | 2 tools | `lease {action}` | STATE | no | binding decision; calls are the heartbeat |
| Stage finish | `finish_step` | kept, implicit submit | STATE | no | status, reason are state signals |
| Session finished signal | `handover {finished}` | `submit {finished}` argument | STATE | no | argument, never a file field |
| Operations list, cancel | `operations` | list in `state.yaml`, `cancel_operation` | STATE | no | binding decision |
| Run outcome, lease status read | `run_outcome`, `lease_status` | `state.yaml` read | STATE (read) | no | read-only mirror |
| `README.md`, `task.yaml`, `state.yaml` | none | Kira-only, restored | STATE mirror | no | `Kind.KiraOwned()` in `taskfiles/classify.go` |

Result: of 16 rows, 2 are content an agent creates from nothing with a non-fixed path: new proof and
child task (and the child is mostly state). Create cannot apply to edits, state or Kira-owned files.
"One pattern for every resource" covers 2 resources, not every one.

Resources already on disk or at a known path without a create:

- Draft handover: path `handovers/draft-<step>.md` is in the step prompt (`headless.space.files`).
  An agent writes it with `Write` (new file, no Read): 11 of 11 `Write` calls on new files in the
  transcripts succeeded with no prior Read. A Kira-made skeleton would force a Read first.
- `links.yaml`: one file per task, Kira writes it at migration and on UI edits. An agent appends an
  entry. Create would allocate a path that is already fixed.
- Notes: user content, `notes/<created>-<id>.md`; an agent edits with a reason, it does not create.
- Existing proofs: README and prompt `Proofs:` lines name the path (spike 1c A). Edit path only.

## 2. Claude Code read-before-edit and the per-resource call count

Rule, as documented in this harness's own tool descriptions: Edit "You must Read the file in this
conversation before editing, or the call will fail"; Write "Overwriting an existing file you
haven't Read will fail". A new file needs no Read.

Transcripts (105 runs, Claude Code 2.1.325), every Edit or Write call:

| Call | Prior read of that file | Count | Failed |
|---|---|---|---|
| Write, new file | none needed | 11 | 0 |
| Edit after Read tool | Read | 8 | 0 |
| Edit or Write after Bash `cat` only | Bash | 9 | 0 |
| Edit with no visible read | none seen | 1 | 0 |
| "has not been read" errors | | 0 | |

So a Bash `cat` satisfied the check in 2.1.325, and agents often skip Edit for Bash writes (shell
heredoc or `sed -i`: F3 FL 3 of 3 runs, F6 FL 3 of 3, F2 FP 3 of 3), which no read check covers.

Calls per new resource (excluding ToolSearch and the final answer, each 1 turn in every arm):

| Path | Calls | Notes |
|---|---|---|
| `proof` tool (plan) / `proof_save` (T) | 1 | F2 T: 3 turns total |
| file Write (new) plus `submit` | 2 | needs the format: FP read README plus AGENTS first (2 more) |
| create plus Read plus Edit (or Write) plus `submit` | 4 | Edit or Write on the skeleton needs a read |
| create plus Bash heredoc overwrite plus `submit` | 3 | skeleton discarded unread; create bought only the path |
| create with initial content plus `submit` | 2 | the `proof` tool under another name, plus a second call |
| create with initial content that also submits | 1 | literally the `proof` tool |

Skeleton embedded in the create reply: an MCP tool result does not enter Claude Code's file-read
state (filled by its own read tools; no run had a case to observe). Assume the Read stays, or the
agent overwrites with Bash. Either way the skeleton text is paid twice (reply, then Read echo or
rewrite) and adds context.

Turn cost, measured (sonnet, realistic fixtures, about 42k context):

| Flow | T cost, turns | Files arm cost, turns | Marginal USD per extra turn |
|---|---|---|---|
| F2 new proof | 0.0638, 3 | FP 0.0837, 6 | 0.0066 |
| F3 proof edit | 0.0666, 5 | FL 0.0854, 7 | 0.0094 |
| F6 two children | 0.0669, 4 | FL 0.0912, 7 | 0.0081 |

Part 1b's 0.015 to 0.02 USD per turn is median run cost over turns (includes the 37k first-turn
cache write); the marginal cost of one more turn is 0.007 to 0.009 USD, about 10 to 14 percent of
a T run. Each turn also re-sends the context (cache read of about 42k tokens).

Estimates for create-first (K), from the measured turn structure:

| Flow | K best path | K turns | K cost | K/T cost | K/T final context |
|---|---|---|---|---|---|
| F2 new proof | ToolSearch, create, Bash overwrite, submit, answer | 5 | 0.078 to 0.082 | 1.22 to 1.28 | about 1.01 (skeleton in reply, about 0.5k) |
| F2, Read path | ToolSearch, create, Read, Write, submit, answer | 6 | 0.084 to 0.091 | 1.31 to 1.42 | about 1.02 |
| F3 proof edit | create not used; Read, Edit, submit (FA, FP measured) | 7 to 8 | 0.079 to 0.083 | 1.18 to 1.24 | 0.99 to 1.00 |
| F6, creates parallel | ToolSearch, create x2, Read x2, Write x2, submit, answer | 6 | 0.081 to 0.085 | 1.21 to 1.27 | 1.03 to 1.07 |
| F6, creates serial | as above, creates one per turn | 7 to 8 | 0.089 to 0.103 | 1.33 to 1.54 | 1.05 to 1.09 |

Every K estimate fails the cost gate (1.2; F6 used 1.1 in the spike). F2 best case is at best equal
to FP (1.31 measured), because FP already writes the whole proof in one Bash call; create only
replaces FP's two doc reads with one create call and its reply. Read-path K is worse than FP.

## 3. Child task creation as create plus submit

Design (for comparison only):

- `create {kind: "child", title, workflow, repos}`: validates as `planChildTask` does, writes
  `children/<name>/brief.md` (skeleton with header) and a draft row; replies with the path.
- Agent writes the brief (Read plus Write, or Bash heredoc).
- `submit`: reads the brief, raises `task.create` (same payload, phone included), and on approval
  runs `createChild`. Title, workflow, repos stay in create args (state, never in a file), so only
  the brief is a file. A `task.yaml` in the draft would put state behind a file edit: refused by the
  binding decision.

Against the dropped D-Q2 variant F and F6 data:

- Variant F (agent writes `children/<name>/task.yaml` plus `brief.md`, `submit` creates): F6 FL
  1.36, FF 1.40 cost; context 1.06 to 1.09; 6 to 7 turns against T 4. Create-first adds one create
  call per child and removes the README or AGENTS reads (FL read README plus AGENTS in 3 of 3 runs).
  Net estimate 1.21 to 1.54 (table above). Still over 1.1 and over 1.2 in most paths.
- Blocking: `SpawnTask` blocks the call up to 30 min and withdraws the item when the call's context
  ends (`taskAbandoned`). Q6 says no `submit` item blocks. Create plus submit must pick one:
  (a) `submit` blocks for child items: breaks Q6 and makes a mixed submit (draft handover plus child)
  wait 30 min for unrelated files; (b) `submit` returns `pending`: the approval outlives the call, so
  the in-memory `taskReqs` queue needs a persisted draft (the dropped `task_child_drafts`) and the
  agent no longer gets the child id in the same call (the `create_task` reply carries `taskId` and
  `next`). Both reopen what the user closed.
- The brief is already content in today's flow: `createChild` writes it as the child's first handed
  handover, and the plan writes that as a file (Part 3). Nothing is gained by staging it in the
  parent's folder first.

Recommendation for children: unchanged. `create_task` stays an MCP tool, exactly as today. The new
pattern does not change that recommendation; it adds a turn, a draft state and a blocking choice.
Least-change path: none needed. If the user still wants the brief as a file, the cheapest step is
`create_task {briefPath}` as an optional alternative to `brief` (one call, file read at approval
time, pinned by hash), not a create plus submit pair. Not recommended now: no flow needs it.

## 4. Consistency benefit versus cost

Tool count (conditional tools excluded):

| Role | Plan | Full adoption (`create` replaces `proof` and `create_task`) | Delta |
|---|---|---|---|
| Run | 7 | 7 | 0 (`proof` becomes `create`) |
| Session (manual, review) | 6 | 6 | 0 |
| Orchestrator run | 8 | 7 | -1 |
| Orchestrator session | 7 | 6 | -1 |

A listed name costs about 10 tokens at turn 1 (spike 1b probes): adoption saves about 10 tokens per
orchestrator turn 1, 0 elsewhere. Schema text is paid on load: `create` must carry both kinds'
arguments (proof steps and child title, workflow, repos) or a skeleton-only shape. Spike 1b: merged
tools load the text of every tool they replace (F6 search context FF 2,833 against T 926), so a
merged `create` costs more on load than `proof` alone for non-orchestrator runs unless the kind enum
is per grant.

Agent-facing text:

- README: plan line 3 `Proofs: use the proof tool, or edit proofs/<slug>/proof.md and submit.`
  becomes `New proof or child: call create, edit the files it names, then submit.` Same line count
  (cap 25 unchanged).
- Tool descriptions: `proof` about 430 chars (plan 7.5), `create_task` 390 chars (registry). One
  `create` text needs both kinds plus skeleton rules: about 600 to 700 chars. About 100 to 200 chars
  saved, paid only on load.
- Error handling: already uniform. The `proof` tool runs the submit pipeline (`taskfiles.Submit`
  with `Request.Tool = "proof"`) and answers in submit's per-file shape; `create_task` has its own
  outcomes (created, denied, timeout, failed) because it is state.
- Mental model: plan = "edit files, then submit; `proof` and `create_task` do it in one call".
  Spike (sonnet, Parts 1b and 1c): T agents made 0 doc or file reads before acting in 9 of 9 F2,
  F3, F6 runs; files agents read README, AGENTS or the files (Read, `cat`, Glob) in 21 of 21 F2, F3,
  F6 runs. A create reply can replace the format read, not the turn. One-call tools are the easier
  model for the agent today, by measurement.

Costs: 1 to 3 more turns per new proof or child (0.007 to 0.009 USD and about 42k cache-read tokens
each), about 0.5 to 1k more final context per created resource (skeleton in reply plus read echo),
one more schema in the ToolSearch select (`create` plus `submit` against `proof` alone).

Net: consistency saves about 10 tokens (orchestrator only) and about 100 to 200 description chars on
load; it costs 0.014 to 0.03 USD and 2 to 3 context re-sends per created resource. Cost wins.

## 5. Failure modes

- "Valid skeleton" is a contradiction. If the skeleton passes validation, a forgotten edit ships a
  placeholder proof: `finish_step` runs an implicit submit, accepts it and fires the proof-ready
  notification to the user. If it does not pass, it is a template the header comment and the `proof`
  schema already give. Must be invalid by design (zero steps), so every forgotten edit becomes an
  `invalid` line and a disk banner entry.
- Orphans: create then crash or forget = skeleton on disk with no accepted row. Stat on task open
  shows it in the "changed on disk, not submitted" banner as noise. Needs a new `task_files.review`
  value `draft` with owner session, banner suppression, and cleanup at session end, stage move and
  start-up. Precedent the plan removes: `proof.Service.PurgeStale` with `draftTTL` for abandoned
  `proof_save` drafts. History: v2.8 P560 replaced the five-call `proof_start_session`,
  `proof_add_step`, `proof_close_session` flow (create, fill, close) with one-call `proof_save`.
  Create plus submit brings that shape back.
- Slug collisions and concurrent creates: create would allocate the slug under `taskMu` (`-2`
  suffix or refusal). The `proof` tool already allocates the slug from the name (case folded,
  `NormalizeName`); no gain.
- Idempotency: a create retried after a timeout must return the same draft (key session, kind,
  name), else two skeletons. `proof` is idempotent by name today (same name = edit).
- Approval timing: approvals belong at submit (content known). Create must stay silent. For children
  this splits validation (create) from approval (submit) with the brief editable in between: the
  approval item must pin the brief hash, and an edit after submit re-raises.
- Unsandboxed agent: can write skeletons for any kind or another task's folder, same as files today;
  create adds no new guard and no new hole.
- Migration: no data impact (create only affects new resources). Code impact: Part 2 code in
  progress (`taskfiles/classify.go` kinds, `decide.go` notify, `submit.go` `Request.Tool`) gains a
  draft state and a child kind.
- Q10 (proofs only in a task): consistent. Q16: unaffected. Proof window (one proof, one file):
  consistent, but the window and Details must hide unsubmitted skeletons (no rows until submit).
- Mobile approvals: `task.create` reaches the phone today with title, workflow, repos, brief. Under
  create plus submit the payload is read from the draft at submit time; same view, new hash pin.
- Notify rules: new proof keeps "proof ready" at submit acceptance, never at create; silent accept
  elsewhere unchanged. A skeleton accepted unedited would notify wrongly (first bullet).

## 6. Recommendation

Reject full and partial adoption. Keep the plan: files plus `submit` for edits and fixed-path
content, `proof` for proofs (one call new, patch by id for edits, hand edit plus `submit` also
valid), `create_task` unchanged for children.

Reasons and numbers:

1. Scope: the pattern fits 2 of 16 agent-created or changed resources (new proof, child brief);
   every other row is an edit, a fixed path or state.
2. Cost: create-first needs 3 to 4 calls per new resource against 1 for `proof` or `create_task`;
   estimates 1.22 to 1.42 (F2) and 1.21 to 1.54 (F6) against T, gate 1.2 (F6 1.1). Best case equals
   the measured files arms that already failed (F2 FP 1.31, F6 FL 1.36).
3. Context: skeleton in the create reply plus its read echo, about 0.5 to 1k tokens per resource;
   final context about 1.01 to 1.09 against the 1.0 gate.
4. Consistency gain is about 10 tokens (orchestrator listing) and a few hundred description chars;
   error replies are already uniform through the submit pipeline.
5. It reopens closed decisions: child drafts (D-Q2 variant F dropped), a persisted child approval or
   a blocking `submit` (Q6), draft cleanup the plan deletes (`PurgeStale`), and the v2.8 move from
   multi-call proof drafts to one-call `proof_save`.

Partial kinds considered and declined: proof only (`create` = `proof` without content: strictly
more calls; `create` with content = `proof` renamed, no gain); child only (section 3); handover draft
pre-create (turns a 1-call Write into Read plus Write).

Experiment: not needed; the data already decides (best case equals measured failing arms). If the
user wants it confirmed, one run settles it:

- Harness: Part 1b test, new arm `FK` in `details_spike1b_test.go` (opt-in `KIRA_SPIKE=run1b`,
  `KIRA_SPIKE1B_ARMS=T,FK`, `KIRA_SPIKE1B_FLOWS=F2,F6`). `FK` = files stub server
  (`details_spike_stubs_test.go` around line 403) plus a stub `create {kind: proof|child, name or
  title, workflow?, repos?}` that writes the skeleton (proof: header plus `kira: proof/1`, name,
  empty `steps`; child: `children/<slug>/brief.md` header only, args kept in stub state) and replies
  with path, skeleton text and `Edit it, then call submit.`; `submit` stub validates as Part 1c and,
  for children, creates them (auto-approve, as T's stub).
- Runs: sonnet; F2 FK x3, F6 FK x3; T x1 per flow as a drift check against stored T medians (F2
  0.0638, F6 0.0669; drift over 15 percent voids the run and the result goes to the user). 8 runs,
  estimate 0.64 USD (FK about 0.08 to 0.09 per run, T about 0.065), cap 0.7 USD total, 0.15 USD per
  run.
- F3 not run: create does not apply to an edit; the uniform-pattern edit path is FA and FP data.
- Deciding metrics, per kind (F2 = proof, F6 = child), FK median over T: cost at most 1.2; final
  context at most 1.0; pass count not lower; failed edit-path calls plus retries not higher. Turns
  informational. Diagnostics, reported: "has not been read" errors, Read calls before Edit, Bash
  overwrites of the skeleton, skeletons submitted unedited, forgotten submits.
- Rule: adopt `create` for a kind only if all four gates pass for that kind; otherwise keep the plan.
  Expected from section 2: both fail cost.

## 7. Plan diffs adoption would need (not applied)

If the user adopts anyway (full: proof and child), `P643-details-on-disk.md` changes:

1. Decisions: Design bullet (proofs and `create_task` keep a tool entry) rewritten to `create` plus
   `submit`; Proofs bullet (`proof` tool) replaced; child draft folders bullet reversed (D-Q2 F back,
   brief only, args in `create`); Q6 bullet: either `submit` blocks for child items or child
   approvals persist; Silent accept bullet: proof-ready fires at submit, never at create.
2. Section 1: conflict lines for "orchestrator creates a child by writing a folder" and "agent
   writes the proof file" flip; tool count line 7 and 8 becomes 7 and 7.
3. Section 3: `task_files.review` gains `draft` (owner session); new table `task_child_drafts`
   (parent, slug, title, workflow, repos JSON, session, created) or equivalent; migration `0101`
   changes.
4. Section 4 Layout: `children/<slug>/brief.md` (draft, parent folder); proof skeleton note.
5. Section 5 Formats: skeleton formats per kind, invalid by design (zero steps; empty brief).
6. Section 6 Writer matrix: rows for `children/<slug>/brief.md` and skeletons (create via tool);
   collision rule for drafts.
7. Section 7.1 submit: child items, blocking choice, reply outcome for children (taskId, next).
   7.2 algorithm: draft state, banner suppression for own drafts, orphan cleanup at session end,
   stage move and start-up, hash pin for child approvals. 7.3 matrix: rows "skeleton submitted
   unedited: invalid", "child draft submitted: `task.create` approval"; blocking table row for child
   changes. 7.4 unchanged. 7.5 `proof` tool replaced by `create` (schema, kinds, idempotency key,
   reply with path and skeleton); proof edit patch path removed (edits become Read plus Edit plus
   `submit` only, FA 1.18 and FP 1.24 data). 7.6 README line 3, prompt tool line
   `finish_step, submit, create`, ToolSearch select names, `HeadlessSpaceInstructions`.
8. Section 8: `create_task` reply `next` text gone; `chain.children` lists drafts or not.
9. Section 9.1 rows New proof, Proof edit, Child task rewritten with the new numbers. 9.3 tool
   table: `proof` and `create_task` rows become one `create` row; counts 7, 6, 7, 6; removed names add
   `create_task`, new names `create` instead of `proof`.
10. Section 10 Proofs: new proof through `create` plus `submit`; window hides drafts.
11. Section 12 UI: disk banner excludes drafts; approvals `task.create` body from the draft;
    Details hides unsubmitted proof skeletons.
12. Section 14.1: R arm uses `create`; F2 and F6 estimates rise to about 0.08 to 0.09 per run; gate
    expected to fail on F2 and F6.
13. Section 15: Part 4 `TestProofFiles` cases for skeleton invalid when unedited, idempotent create,
    orphan cleanup, notify at submit only; Part 5 child create flow (create, brief, submit, approve,
    deny, timeout, draft cleanup, hash pin, phone); real `claude` `TestProofMcpThroughClaude`,
    `TestProofChartThroughClaude`, `TestAdeOrchestrator` rewritten.
14. Section 16 Parts: Part 2 adds draft state to `taskfiles/classify.go`, `decide.go`, migration
    `0101`; Part 4 owns new `agentmcp/create.go` (proof kind) instead of `agentmcp/proof.go`; Part 5
    rewrites `ade/taskcreate.go` (`SpawnTask` to draft submit), `agentmcp/orchestrator.go`; shared
    file table: `agentmcp/create.go` becomes shared between Streams A and B (both kinds), so the
    no-overlap claim between Part 4 and Part 5 weakens (semantic overlap in one file: one stream must
    own it, the other waits). Part 2 and Part 3 task lists for spike gaps 1, 5, 6 updated.
15. Section 17 Risks: orphan drafts, placeholder proof accepted, blocking or persisted child
    approvals, merged schema load size.
16. Section 18: Q2 reopened (child drafts); new question on submit blocking for children.

## Codegraph queries run

1. `create_task tool handler createChild task.create approval wait 30 minutes orchestrator`
2. `taskfiles Store WriteAtomic intents submit classify kind patterns`
3. `Classify Class KindProof ProofFile KindReadme KindLinks KiraOwned Write Adopt Recover`
4. `SpaceToolIDs OrchestratorToolIDs handoverGranted buildMCPServer tool grant per role`
5. `spikeSpace submit stub children task.yaml brief.md F6 files arm spike1b fixtures`
6. `proof Service Save StartSession name_key unique name ProofReady notification proof.review RaiseUnreviewed`
