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
