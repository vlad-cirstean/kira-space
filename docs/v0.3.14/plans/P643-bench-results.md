# P643 Part 3 benchmark: F1

Command: `KIRA_BENCH_OUT=<file> KIRA_BENCH_FLOWS=F1 KIRA_BENCH_BUDGET=0.6 bun run test:claude:details`. Sonnet, 3 reps per
arm, 0.5 USD per run, 0.6 USD per round. Arm R is the real Part 3 build: real `agentmcp.Server` with the Space grant, real
README generator, seeded history, the files prompt fragment as Kira composes it. Arm T is the frozen tools baseline.
Gate: cost R/T <= 1.2, final context R/T <= 1.0, pass rate not lower, edit failures not higher.

Verdict: **gate not met.** Part 3 stays not Done. Both permitted rounds are spent (0.4282 and 0.3877 USD); the gate was not relaxed.

## Round 1 (files fragment: "Read README.md there first", load hint names all 11 Kira tools)

| Metric | R | T | R/T |
|---|---|---|---|
| Cost USD, median | 0.0838 | 0.0590 | 1.42 |
| Final context, median | 42108 | 40193 | 1.05 |
| Pass rate | 3 of 3 | 3 of 3 | equal |
| Edit failures plus retries, mean | 0.00 | 0.00 | equal |
| Turns, median (informational) | 7 | 4 | |

Verdict: fail on cost and on context.

## Root cause and fix

Two causes, both in the prompt fragment, not in the tools:

1. The fragment forced a `README.md` read, a turn with no value on a stage whose latest handover and notes are already
   inlined (the plan: "the common input needs no read").
2. The load hint named every Space tool (11 schemas), not the tools the plan names (run: `finish_step`, `submit`; session:
   `submit`; orchestrator adds `create_task`). Context grew by the unused schemas.

Fix: the fragment points at `README.md` without making the agent read it; `sessionTools` loads only the always-needed
tools and the suffix names the rest.

## Round 2 (after the fix)

| Metric | R | T | R/T |
|---|---|---|---|
| Cost USD, median | 0.0694 | 0.0526 | 1.32 |
| Final context, median | 39411 | 40202 | 0.98 |
| Pass rate | 3 of 3 | 3 of 3 | equal |
| Edit failures plus retries, mean | 0.67 | 0.00 | R higher |
| Turns, median (informational) | 7 | 4 | |

Per run: R 0.0832 (9 turns), 0.0694 (7), 0.0654 (7); T 0.0651, 0.0526, 0.0518 (4 turns each).

Verdict: fail. Context now passes (0.98). Cost 1.32 is above 1.2 and edit failures are higher (0.67 against 0.00).

## What remains (for the next owner)

The remaining gap is call structure, as the spike found: R needs `ToolSearch` (deferred MCP schemas), a draft write, and
`submit` or `finish_step`, where T has the schemas loaded at turn 1 and one `handover` call. Turns stay at 7 against 4.
Candidates not tried, because the round budget is spent:

- Make the draft write and the submit one step: let `finish_step` with `status` done take the draft body, which the plan
  rules out (no `summary` argument), so this needs a plan decision.
- Cut the `ToolSearch` turn: a provider setting that preloads two named MCP tools, if the CLI offers one.
- Edit failures: R run 1 (9 turns) hit a failed edit-path call on the draft or `links.yaml`; read that run's log before
  changing any prompt text. The failing edit has not been root-caused.

Benchmark spend of Part 3: 0.0843 (smoke) + 0.4282 + 0.3877 = 0.90 USD.

## Edit failure root cause

Not proven: the run transcripts are gone. `execute` writes the stream-json only when `KIRA_SPIKE_DUMP` is set; the bench
rounds ran without it, and the bench `t.TempDir` homes were removed with the test. The results file keeps only counts.

What the numbers allow: 0.67 mean over 3 runs is 2 events, one failed edit-path call plus one retry, both in run 1 (9 turns
against 7). The metric counts any `Edit`/`Write` error and any Kira tool error (`is_error`), not only edits. In arm R the
Kira calls that can error are `finish_step` refused by the bench stub ("write handovers/draft-implement.md, then call
finish_step", no draft yet) and `submit`. Two Part 3 prompt candidates fit:

1. Relative draft path. The fragment gives `Task folder: <abs>` and `Write your handover to handovers/draft-implement.md`.
   The `Write` tool takes an absolute path and the run's cwd is the repo, not the task folder. A joined-wrong write lands in
   the repo, `finish_step` is then refused, and the model rewrites: one failure, one retry, two extra turns.
2. Order. The fragment says "then call submit" while the finish suffix says `finish_step`; calling `finish_step` first gets the
   same refusal.

Neither is confirmed, so no code or prompt change was made (a fix without the transcript would be a guess that burns a
benchmark round). Next owner: rerun R with `KIRA_SPIKE_DUMP=<dir>`, read run 1, then decide. If candidate 1 holds, render
`Draft` as `<Folder>/<draft>` in `filesVars`.

# P643 Part 4 benchmark: F2 and F3

Command: `KIRA_SPIKE_DUMP=<dir> KIRA_BENCH_OUT=<file> KIRA_BENCH_FLOWS=F2,F3 KIRA_BENCH_BUDGET=<usd> bun run test:claude:details`.
Sonnet, 3 reps planned per flow and arm, 0.5 USD per run. Arm T is the frozen baseline (kira-studio `proof_save` and
`proof_list`). Arm R is the real Part 4 build on a real app: a real task folder, the board as the grant's tools, the real
`submit` with the proof unit; F3 starts from an accepted proof in the real `proof.md` format. The oracle reads the proof Kira
stored (the pending revision when an edit waits).

Verdict: **gate not met on F2 and F3** (cost, and context on F2). Recorded, not relaxed, benchmark stopped. F2 and F3 are
reported, not blocking.

## Spend

Round budget was 0.89 USD; spent 1.04 USD, 0.15 over. Attempt 1 (2 runs, 0.2375 USD) died on two harness faults, not product
faults: the arm R oracle parsed the spike's `kira: proof/1` fixture format instead of the stored proof, and the F3 seed used
the spike's `flow.mmd`, which is not a proof file. Both were fixed in the harness (oracle reads the stored proof, F3 seed
written in the real format) and attempt 2 ran with the remaining budget as its cap (0.65 USD); its guard stopped it after 5
runs (0.8034 USD).

## Result (attempt 2, 5 of 12 planned runs)

| Flow | Arm | Pass | Cost USD, median | Final context, median | Turns, median | Failed edit-path plus retries, mean |
|---|---|---|---|---|---|---|
| F2 | R | 1 of 2 | 0.2788 | 55440 | 37 | 2.00 |
| F2 | T | 1 of 1 | 0.0631 | 42760 | 3 | 0.00 |
| F3 | R | 1 of 1 | 0.1064 | 42413 | 13 | 1.00 |
| F3 | T | 1 of 1 | 0.0763 | 42510 | 4 | 0.00 |

| Flow | Cost R/T | Context R/T | Pass R vs T |
|---|---|---|---|
| F2 | 4.42 | 1.30 | 1 of 2 vs 1 of 1 |
| F3 | 1.40 | 1.00 | 1 of 1 vs 1 of 1 |

Per run: F2 R 0.1964 (30 turns, fail), 0.3612 (44 turns, pass); F2 T 0.0631 (3). F3 R 0.1064 (13); F3 T 0.0763 (4).
One or two reps per cell: the ratios are indicative, not a measurement.

## Root cause

Not the file mechanism. Both F2 R runs spent most turns guessing the document step's `inline` field names (`text`,
`explanation`, `markdown`, `body`, `content`, `title`, `sections`, `blocks`, `paragraphs`, `items`); the validator answered
"unknown field" without the format, and the README example shows only table and image. The first F2 R run gave up and
reported `needs_input`; the second found `documents: [{body: ...}]` by grepping the Go source. The chart view had the same
gap (the chart real-`claude` test failed on it). T has the schema in the tool description, so it needs 3 turns.

Fix, after the run, not verified by a benchmark (budget spent): `proof.ValidateStep` appends the view's format to every
shape rejection (`viewShapes`: table, document, key-value, chart, diagram), with an example for document and chart.
`TestProofMcpThroughClaude` and `TestProofChartThroughClaude` pass on the chart fix; the document fix came after them.

## What remains

- Rerun F2 and F3 with the shape hints (3 reps per arm, about 0.9 USD) in the final benchmark, Part 5.
- F3 R used 13 turns and one failed edit-path call against T's 4: ToolSearch, a README read, a proof read, the edit, then
  `submit` with a reason. The README read could be cut by inlining the proof index (already in the prompt `Proofs:` line).
- Cost floor: arm R needs a `ToolSearch` turn that T does not (deferred MCP schemas), as Part 3 found.

## Part 5 round: F4, F5, F6 and the F1 rerun on the uniform surface

Arm R is the real `agentmcp.Server` with `submit`, `state` (and `create_task` for F4 and F6) over a task folder with Kira-shaped
`README.md`, `task.yaml`, `state.yaml`. F4 adds the two child folders with `--add-dir`. F5 answers `state branch.request` once setup ends
(4 s after start, not the 20 s of the plan; both arms share the delay). Sonnet, 3 reps. Raw transcripts were kept with `KIRA_SPIKE_DUMP`.

First round (24 runs, 1.8473 USD): arm T failed every F4 and F5 run. The frozen tools declare an output schema and the T handlers returned
text only, so the CLI refused each reply ("did not return structured content"). F6 and F1 T runs passed. This was a harness bug, not a gate
result: `textResult` now sets `StructuredContent`, and `TestDetailsBenchFrozenServes` checks each reply against its schema. Arm R rows of that
round stand. Arm T rows for F4 to F6 were rerun alone (9 runs, 0.6392 USD), so R and T come from different invocations of the same harness.

| Flow | Arm | Pass | Cost (median) | Final context (median) | Turns | Failed edit-path + retries |
|---|---|---|---|---|---|---|
| F4 | R | 3 of 3 | 0.0871 | 43548 | 7 | 0.00 |
| F4 | T | 3 of 3 | 0.0979 | 48920 | 5 | 0.00 |
| F5 | R | 3 of 3 | 0.0774 | 39992 | 8 | 0.00 |
| F5 | T | 3 of 3 | 0.0507 | 40733 | 3 | 0.33 |
| F6 | R | 3 of 3 | 0.0969 | 42368 | 8 | 0.00 |
| F6 | T | 3 of 3 | 0.0648 | 40341 | 4 | 0.00 |
| F1 | R | 3 of 3 | 0.0685 | 39461 | 7 | 0.00 |
| F1 | T | 3 of 3 | 0.0540 | 40196 | 4 | 0.00 |

Gate, R over T (cost at most 1.2; context at most 1.05 on F1 to F4 and 1.0 on F5 and F6; pass not lower; failed plus retries not higher):

| Flow | Cost R/T | Context R/T | Pass | Failed + retries | Verdict |
|---|---|---|---|---|---|
| F4 | 0.89 | 0.89 | 3 vs 3 | 0.00 vs 0.00 | pass |
| F5 | 1.53 | 0.98 | 3 vs 3 | 0.00 vs 0.33 | fail (cost) |
| F6 | 1.50 | 1.05 | 3 vs 3 | 0.00 vs 0.00 | fail (cost, context) |
| F1 | 1.27 | 0.98 | 3 vs 3 | 0.00 vs 0.00 | fail (cost) |

Verdict: the gate is not met and was not relaxed. Run-to-run spread is under 0.01 USD per cell, so the cost gaps are not noise.

Cause, from the R transcripts: R runs the step ceremony the files design requires and T does not. Per run: `ToolSearch`, `Read` of the
README, the call itself, a `Write` of `handovers/draft-implement.md`, `submit`, then `state step.finish`: 7 to 8 turns against 3 to 5. The T
flows stop at the tool answer. F5 shows it most: T is one `task_info` call with a wait; R must also write a draft and finish. F4 passes because
T needs three read tools there and R reads the files. F6 context is 1.05 because the README and the draft sit in R's context.

Open for the owner (plan decision, nothing tried): let `step.finish` take the handover body so the draft write and `submit` collapse into
one call; preload the two tools to drop the `ToolSearch` turn; or compare against a T arm that also writes a handover and calls `finish_step`.

Spend of Part 5: 1.8473 + 0.6392 = 2.4865 USD, above the 1.26 USD planned for F4 to F6 because of the T harness bug.


# F1b and F3b: read-and-edit, two passes

Why: F1 to F3 pay the files design's fixed ceremony once and show none of its savings. F1b and F3b read an existing large artifact and
revise a small part of it, twice. A run is two separate `claude -p` passes over the same task state (fresh context each, as two steps
would be). The cell is the sum of both passes; per-pass records feed the crossover table. Sonnet, `KIRA_SPIKE_DUMP` set (transcripts
kept: `F1b-R-sonnet-1-p1.jsonl` and so on in the round's dump dir), round guard 0.9 USD. Not gated: new flows, the existing bounds are
shown for information only.

Command: `KIRA_SPIKE_DUMP=<dir> KIRA_BENCH_OUT=<file> KIRA_BENCH_FLOWS=F1b,F3b KIRA_BENCH_BUDGET=0.9 bun run test:claude:details`.
Code: `details_bench_revise_test.go` (flows, oracles, `executePasses`, per-pass table, free test `TestDetailsBenchReviseServes`).

## Flows

- F1b handover revision. The task holds the 4,993-byte (about 1,250 token) `0003-review-to-implement.md` handover: summary, 14 changes,
  7 decisions, risks, verification. Pass 1: change the seed-script line, change the admin-table risk line, add a `## Follow-up`
  section. Pass 2: change decision 6. Arm T: the handover text is in the prompt (pass 2: pass 1's `finish_step` summary) and the model
  resends it whole in `finish_step`'s `summary`; the frozen tools have no separate `handover` tool, `finish_step` summary is the
  handover call. Arm R: the text is `handovers/draft-implement.md`; the model edits it, then `submit` and `state step.finish`.
- F3b proof revision. Closed proof "Orders match ledger": table of 10 rows (F3 fixture rows 1 to 10), the F3 diagram, the F3 document
  (3 steps). Pass 1: order 7's two cells to 900, add a fourth document step "Late correction note". Pass 2: one cell (order 3
  `order_total` 2550). Arm T: `proof_list` (ordinals only, no data), so the current rows come in the prompt; `proof_save` resends the
  table and uses `keepOrdinal` for untouched steps; the harness accepts pass 1's pending edit between passes. Arm R: real board, real
  `proof.md` with `totals.json`, `explain.json`, inline diagram; edit files, then `submit` with a reason.
- Oracles check the stored result per pass: every earlier change kept, nothing else lost (handover length, sections, diagram and
  document text), `finish_step done` or a submit answered "pending user, change to" in that pass.

## Free test (before spending)

`TestDetailsBenchReviseServes`, `TestDetailsBenchFrozenServes`, `TestDetailsBenchArmRServes`: all pass, no tokens. Per arm and flow: seed
is correct (T F1b prompt carries the handover; R draft equals the handover; T proof 10 rows with the current rows in the prompt, R
`proof.md` and `totals.json` a real 10-row proof the real board accepts), replies carry structured content, the oracle refuses the
untouched start and accepts a correct revision in pass 1 and in pass 2 (R proof: a second `submit` over a pending first edit is
accepted and replaces it; T: the edit is promoted between passes).

## Result (1 rep per cell: the guard stopped the round)

Planned 12 runs (3 reps x 2 flows x 2 arms, 24 passes). The guard counts the dearest run seen (0.2285) and stopped after rep 1:
spent 0.6836 USD (4 runs, 8 passes) of 0.90; free tests 0.0018. Not rerun. One rep per cell: ratios are indicative, not a
measurement; run-to-run spread on earlier flows was under 0.01 USD per cell, which is about the size of the F1b cost gap.

| Flow | Arm | Pass | Cost USD | Final context | Turns | Failed edit-path + retries |
|---|---|---|---|---|---|---|
| F1b | R | 1 of 1 | 0.1531 | 41008 | 14 | 0 |
| F1b | T | 1 of 1 | 0.1663 | 41531 | 6 | 0 |
| F3b | R | 1 of 1 | 0.2285 | 42017 | 25 | 2 |
| F3b | T | 1 of 1 | 0.1356 | 40408 | 8 | 0 |

R/T against the existing gate numbers (information only; cost max 1.2, context max 1.05; earlier F1 cost 1.27, F3 cost 1.40):

| Flow | Cost R/T | Context R/T | Pass R vs T | Failed + retries R vs T | Turns R/T |
|---|---|---|---|---|---|
| F1b | 0.92 | 0.99 | 1 vs 1 | 0 vs 0 | 2.33 |
| F3b | 1.68 | 1.04 | 1 vs 1 | 2 vs 0 | 3.12 |

## Per pass

Input tokens are fresh plus cache read plus cache write summed over the pass's model calls.

| Flow | Arm | Pass | Cost USD | Input tokens | Output tokens | Final context | Turns |
|---|---|---|---|---|---|---|---|
| F1b | R | 1 | 0.0801 | 202602 | 1020 | 41644 | 8 |
| F1b | R | 2 | 0.0730 | 200537 | 585 | 41008 | 6 |
| F1b | T | 1 | 0.0835 | 120383 | 2202 | 41555 | 3 |
| F1b | T | 2 | 0.0828 | 120268 | 2148 | 41531 | 3 |
| F1b | R/T | 1 | 0.96 | 1.68 | 0.46 | 1.00 | 2.67 |
| F1b | R/T | 2 | 0.88 | 1.67 | 0.27 | 0.99 | 2.00 |
| F3b | R | 1 | 0.1296 | 456302 | 2564 | 43871 | 15 |
| F3b | R | 2 | 0.0989 | 322790 | 1550 | 42017 | 10 |
| F3b | T | 1 | 0.0673 | 157468 | 657 | 40454 | 4 |
| F3b | T | 2 | 0.0683 | 157309 | 770 | 40408 | 4 |
| F3b | R/T | 1 | 1.93 | 2.90 | 3.90 | 1.08 | 3.75 |
| F3b | R/T | 2 | 1.45 | 2.05 | 2.01 | 1.04 | 2.50 |

## Verdict

- F1b: files break even to a slight win (0.92 cost, 0.99 context, one rep). The saving is real but small and sits in output: T
  writes the whole 1,250-token handover each pass (about 2,200 output tokens, flat across passes), R writes only the edits (1,020 then
  585). R pays that back in input: every pass is a fresh process with its own `ToolSearch`, README-less draft read and `Edit` turns,
  so it makes 2 to 3 times the turns and reads about 1.7x the input tokens (cache reads of the same 41k context per turn). Crossover:
  R/T cost goes 0.96 on pass 1 to 0.88 on pass 2, so the margin grows with each pass because R's output shrinks to the edit while T's
  stays the full text; per-pass fixed ceremony does not shrink. With one rep the 8 percent gap is about the run-to-run spread; the
  honest reading is "no loss, small gain, not proven".
- F3b: files lose (1.68 cost, 2 failed edit-path calls against 0). The 10-row proof is small: T's `proof_save` resend is about 700
  output tokens, so there is little to save, and T needs 4 turns per pass. R needed 15 and 10 turns, 2.9x and 2.1x the input tokens.
  Transcripts show behaviour that is partly the prompt fragment, not the file mechanism: in pass 1 the files fragment's "Write your
  handover to handovers/draft-implement.md, then call submit" pulled the model away from a proof-only task. It edited the proof with
  `sed`, called `submit`, then tried to `mv` a draft that did not exist (failed call), created `handovers/draft-work.md`, submitted
  it, called `state step.finish` (not asked), and submitted the proof paths again. Pass 2 repeated the pattern in a milder form (a
  failed `rm` of a missing draft, a second `submit`). It also used `Bash` (`sed`, `python3`) for edits instead of `Edit`. Pass 2 is
  cheaper (1.45) because the model had less to discover, but the detour persists.
- Overall: reading and editing a large artifact in place is where files should win, and the handover case shows the direction (R
  output 27 to 46 percent of T's), but at about 1,250 tokens the saving is within what one extra `ToolSearch` and read turn costs. The
  proof case does not show it at 10 rows. Cause for the loss in F3b is call count, as in every earlier flow, plus a handover-draft
  instruction that does not belong in a proof task. Not proven by more reps: the guard stopped the round at one rep per cell.

Spend of this round: 0.6836 USD (benchmark) plus 0.0018 (free tests).

The files fragment's unconditional handover instruction is fixed (commit c0f2960cf); the F3b and F3 numbers predate the fix and were not rerun by decision.
