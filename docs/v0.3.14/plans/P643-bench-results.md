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
