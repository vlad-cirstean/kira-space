# P643 Part 1 spike results

Test: `apps/kira-space/tests/claude/details_spike_test.go`. Arm T = today's tools (real `agentmcp.Server`, stub backends, kira-studio `proof_save` stub with today's schema). Arm F = planned files plus `submit`, `finish_step` without summary, merged `branches`, `lease`, `cancel_operation`, README in the task folder. Both arms get the same prompt core; arm F adds one line naming the task folder.

Runs: 48 of 48 planned. Spend: 0.9671 USD (cap 6.00 total, 0.25 per run). Single consolidated-file variant (one file instead of many small ones) was not run: the flows read at most two handovers, so the arm would add no signal within budget; read effort of the per-file layout is in the edit table. Cost is `total_cost_usd`; tokens are the result line's `usage` (input excludes cache; cache read and write listed apart).

Part 1b (sonnet, realistic fixtures, README and layout ablations, context breakdown) is appended at the end of this file.

## haiku

| Flow | Arm | Runs | Pass | Mean cost USD | Median cost USD | Mean turns | Median turns | Mean in | Mean cache read | Mean cache write | Mean out | MCP tools listed | Turn-1 context | Final context | Growth per turn |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| F1 handover | T | 3 | 3/3 | 0.0050 | 0.0049 | 6.0 | 6.0 | 10 | 188005 | 10655 | 1665 | 12 | 38246 | 40954 | 677 |
| F1 handover | F | 3 | 3/3 | 0.0052 | 0.0052 | 6.7 | 6.0 | 10 | 190782 | 12123 | 1476 | 5 | 38214 | 41890 | 919 |
| F2 new proof | T | 3 | 3/3 | 0.0044 | 0.0043 | 4.7 | 5.0 | 8 | 149367 | 11688 | 786 | 14 | 38371 | 41455 | 1028 |
| F2 new proof | F | 3 | 3/3 | 0.0059 | 0.0060 | 7.0 | 7.0 | 11 | 208433 | 13795 | 1748 | 5 | 38313 | 43562 | 1228 |
| F3 proof edit | T | 3 | 3/3 | 0.0044 | 0.0043 | 4.0 | 4.0 | 8 | 150036 | 11394 | 933 | 14 | 38307 | 41537 | 1077 |
| F3 proof edit | F | 3 | 2/3 | 0.0058 | 0.0056 | 7.0 | 7.0 | 11 | 218973 | 12898 | 1852 | 5 | 38246 | 42665 | 968 |
| F4 orchestrator summary | T | 3 | 3/3 | 0.0036 | 0.0035 | 4.0 | 4.0 | 7 | 121031 | 9733 | 618 | 15 | 38268 | 40010 | 751 |
| F4 orchestrator summary | F | 3 | 3/3 | 0.0041 | 0.0040 | 3.3 | 3.0 | 7 | 122270 | 12074 | 692 | 6 | 38268 | 41841 | 1583 |
| F6 create 2 child tasks | T | 3 | 3/3 | 0.0052 | 0.0052 | 5.0 | 5.0 | 8 | 151617 | 11769 | 1676 | 15 | 39245 | 42527 | 1094 |
| F6 create 2 child tasks | F | 3 | 3/3 | 0.0069 | 0.0065 | 7.7 | 8.0 | 10 | 200350 | 15969 | 2408 | 5 | 39168 | 45736 | 1865 |
| F5 branch wait | T | 3 | 3/3 | 0.0034 | 0.0033 | 3.3 | 3.0 | 7 | 120919 | 9405 | 324 | 12 | 38225 | 39682 | 637 |
| F5 branch wait | F | 3 | 3/3 | 0.0043 | 0.0043 | 4.7 | 5.0 | 8 | 149486 | 11848 | 534 | 5 | 38192 | 41615 | 1141 |

## sonnet

| Flow | Arm | Runs | Pass | Mean cost USD | Median cost USD | Mean turns | Median turns | Mean in | Mean cache read | Mean cache write | Mean out | MCP tools listed | Turn-1 context | Final context | Growth per turn |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| F1 handover | T | 1 | 1/1 | 0.0643 | 0.0643 | 5.0 | 5.0 | 8 | 142157 | 10929 | 621 | 12 | 37419 | 38795 | 459 |
| F1 handover | F | 1 | 1/1 | 0.0735 | 0.0735 | 7.0 | 7.0 | 8 | 146362 | 11794 | 1149 | 5 | 37393 | 40758 | 1122 |
| F2 new proof | T | 1 | 1/1 | 0.0580 | 0.0580 | 3.0 | 3.0 | 6 | 106026 | 10902 | 365 | 14 | 37551 | 39866 | 1158 |
| F2 new proof | F | 1 | 1/1 | 0.0624 | 0.0624 | 5.0 | 5.0 | 6 | 106214 | 11394 | 609 | 5 | 37484 | 40358 | 1437 |
| F3 proof edit | T | 1 | 1/1 | 0.0642 | 0.0642 | 4.0 | 4.0 | 8 | 145878 | 11208 | 465 | 14 | 37484 | 40172 | 896 |
| F3 proof edit | F | 1 | 1/1 | 0.0774 | 0.0774 | 7.0 | 7.0 | 10 | 187619 | 12537 | 829 | 5 | 37427 | 41501 | 1018 |
| F4 orchestrator summary | T | 1 | 1/1 | 0.0524 | 0.0524 | 3.0 | 3.0 | 6 | 104702 | 9640 | 318 | 15 | 37445 | 38604 | 580 |
| F4 orchestrator summary | F | 1 | 1/1 | 0.0581 | 0.0581 | 3.0 | 3.0 | 6 | 105500 | 10968 | 350 | 6 | 37446 | 39932 | 1243 |
| F6 create 2 child tasks | T | 1 | 1/1 | 0.0782 | 0.0782 | 5.0 | 5.0 | 8 | 146348 | 12096 | 1474 | 15 | 38422 | 41060 | 879 |
| F6 create 2 child tasks | F | 1 | 1/1 | 0.0867 | 0.0867 | 5.0 | 5.0 | 6 | 109559 | 14962 | 1547 | 5 | 38343 | 43926 | 2792 |
| F5 branch wait | T | 1 | 1/1 | 0.0511 | 0.0511 | 3.0 | 3.0 | 6 | 104456 | 9489 | 251 | 12 | 37399 | 38453 | 527 |
| F5 branch wait | F | 1 | 1/1 | 0.0667 | 0.0667 | 5.0 | 5.0 | 8 | 145917 | 11947 | 421 | 5 | 37368 | 40911 | 1181 |

## Edit ease and read effort (mean per run)

Edit and Write calls, shell writes (Bash redirect, tee, cp, mv, sed -i), failed edit-path calls (failed edit, write or shell write; any Kira tool error; a submit or finish_step result invalid, rejected or restored), retries (a call on the same file or tool right after a failure), whole-file rewrites (Write over an existing file), wrong-file writes (README.md, task.yaml or state.yaml of the task folder), and read effort (Read, Glob, Grep and read-only Bash calls, with result bytes; per-file layout cost for arm F).

### haiku

| Flow | Arm | Edit/Write | Shell writes | Failed edit-path | Retried | Whole rewrites | Wrong-file | Read calls | Read bytes |
|---|---|---|---|---|---|---|---|---|---|
| F1 handover | T | 0.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.0 | 44 |
| F1 handover | F | 1.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.7 | 3896 |
| F2 new proof | T | 0.0 | 0.0 | 0.33 | 0.33 | 0.00 | 0.00 | 0.3 | 29 |
| F2 new proof | F | 0.7 | 1.3 | 1.00 | 0.00 | 0.00 | 0.00 | 1.0 | 4122 |
| F3 proof edit | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F3 proof edit | F | 1.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 2.7 | 4627 |
| F4 orchestrator summary | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F4 orchestrator summary | F | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 2.3 | 6405 |
| F6 create 2 child tasks | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F6 create 2 child tasks | F | 1.3 | 0.7 | 0.00 | 0.00 | 0.00 | 0.00 | 1.3 | 4789 |
| F5 branch wait | T | 0.0 | 0.0 | 0.33 | 0.33 | 0.00 | 0.00 | 0.0 | 0 |
| F5 branch wait | F | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.7 | 4029 |

### sonnet

| Flow | Arm | Edit/Write | Shell writes | Failed edit-path | Retried | Whole rewrites | Wrong-file | Read calls | Read bytes |
|---|---|---|---|---|---|---|---|---|---|
| F1 handover | T | 0.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.0 | 17 |
| F1 handover | F | 1.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 2.0 | 3541 |
| F2 new proof | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F2 new proof | F | 0.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.0 | 3477 |
| F3 proof edit | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F3 proof edit | F | 0.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 3.0 | 6120 |
| F4 orchestrator summary | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F4 orchestrator summary | F | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 2.0 | 5332 |
| F6 create 2 child tasks | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F6 create 2 child tasks | F | 0.0 | 1.0 | 0.00 | 0.00 | 0.00 | 0.00 | 1.0 | 4263 |
| F5 branch wait | T | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.0 | 0 |
| F5 branch wait | F | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 2.0 | 3948 |

## Gate (haiku)

Primary metrics, per flow, haiku runs (user re-weighting): (1) cost, F median at most 1.2 times T (F6: 1.1); (2) final context, F median at most 1.0 times T (median of the last model call's fresh input + cache read + cache write); (3) pass rate F at least T; (4) edit ease, F mean of failed edit-path calls plus retries at most T's. Turns are reported only, never a threshold. Turn-1 context (tool-schema overhead) and growth per turn are in the first tables. Each metric has its own verdict, so a flow that fails only one is visible.

| Flow | Cost F/T | Cost | Final ctx F/T | Ctx | Pass F vs T | Pass | Edit fails+retries F vs T | Edit | Turns F vs T (info) | Verdict |
|---|---|---|---|---|---|---|---|---|---|---|
| F1 handover | 1.06 (<= 1.2) | yes | 1.02 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes | 6.0 vs 6.0 | FAIL |
| F2 new proof | 1.38 (<= 1.2) | no | 1.04 (<= 1.0) | no | 3/3 vs 3/3 | yes | 1.00 vs 0.67 | no | 7.0 vs 5.0 | FAIL |
| F3 proof edit | 1.29 (<= 1.2) | no | 1.03 (<= 1.0) | no | 2/3 vs 3/3 | no | 0.00 vs 0.00 | yes | 7.0 vs 4.0 | FAIL |
| F4 orchestrator summary | 1.15 (<= 1.2) | yes | 1.04 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes | 3.0 vs 4.0 | FAIL |
| F6 create 2 child tasks | 1.24 (<= 1.1) | no | 1.08 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes | 8.0 vs 5.0 | FAIL |
| F5 branch wait | 1.30 (<= 1.2) | no | 1.04 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.67 | yes | 5.0 vs 3.0 | FAIL |

**Gate verdict: NO-GO**

## Reading the numbers

- Context here is the Claude Code context of each model call (fresh + cache read + cache write) from the stream usage. About 38k tokens of it is Claude Code's own prompt, equal in both arms. Claude Code 2.1.325 lists MCP tools by name at turn 1 and loads schemas on demand through ToolSearch, so turn-1 context does not show tool-schema size; the 12/14/15 versus 5/6 tool counts show in "MCP tools listed" and in the ToolSearch calls instead.
- Arm T has the real old tool count for each flow's role (12 run, 14 with the two proof tools in F2 and F3, 15 orchestrator in F4 and F6). Arm F has the new count (5, 6 with create_task in F4; F6 files arm has 5 because creation moves into submit). Merged-tool descriptions carry the registry text of every tool they replace; their argument descriptions are short field notes, so a final schema may be somewhat larger.
- Both arms get the same prompt core with the full data the agent needs; arm F adds one line naming the task folder. Allowed tools are the same in both arms (Read, Edit, Write, Glob, Grep, Bash, the Kira servers).
- Edit-path failures count: failed edit, write or shell write calls, any Kira tool error, and submit or finish_step results invalid, rejected or restored. Retries are the next call on the same file or tool. Wrong-file writes (read-only task files) were zero in every run.
- Wall time is not in this file (rebuilt offline from stored transcripts). Pass flags come from the run's oracle: file content or stub state, never the agent's own claim.
- Turns are reported only. The single consolidated-file variant was skipped (see above).

## Runs

| Flow | Arm | Model | Rep | Pass | Cost | Turns | In | Cache read | Cache write | Out | Wall s | Tool calls | Note |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| F1 | F | haiku | 1 | yes | 0.0053 | 8 | 10 | 191324 | 12207 | 1557 | 0 | Bash×2 Read×2 ToolSearch×1 Write×1 finish_step×1 |  |
| F1 | F | haiku | 2 | yes | 0.0052 | 6 | 10 | 190489 | 12057 | 1412 | 0 | Bash×2 ToolSearch×1 Write×1 finish_step×1 |  |
| F1 | F | haiku | 3 | yes | 0.0052 | 6 | 10 | 190533 | 12104 | 1458 | 0 | Bash×2 ToolSearch×1 Write×1 finish_step×1 |  |
| F1 | F | sonnet | 1 | yes | 0.0735 | 7 | 8 | 146362 | 11794 | 1149 | 0 | Bash×2 Read×1 ToolSearch×1 Write×1 finish_step×1 |  |
| F1 | T | haiku | 1 | yes | 0.0052 | 6 | 10 | 186894 | 10624 | 2053 | 0 | Bash×2 ToolSearch×1 finish_step×1 task_info×1 |  |
| F1 | T | haiku | 2 | yes | 0.0049 | 6 | 10 | 187844 | 10429 | 1589 | 0 | Bash×2 ToolSearch×1 finish_step×1 task_info×1 |  |
| F1 | T | haiku | 3 | yes | 0.0049 | 6 | 10 | 189276 | 10911 | 1352 | 0 | Bash×2 ToolSearch×1 finish_step×1 task_info×1 |  |
| F1 | T | sonnet | 1 | yes | 0.0643 | 5 | 8 | 142157 | 10929 | 621 | 0 | Bash×2 ToolSearch×1 finish_step×1 |  |
| F2 | F | haiku | 1 | yes | 0.0062 | 7 | 12 | 234880 | 13604 | 2031 | 0 | Bash×2 Edit×1 ToolSearch×1 submit×2 |  |
| F2 | F | haiku | 2 | yes | 0.0060 | 7 | 10 | 198407 | 14983 | 1684 | 0 | Bash×3 ToolSearch×1 submit×2 |  |
| F2 | F | haiku | 3 | yes | 0.0054 | 7 | 10 | 192013 | 12799 | 1528 | 0 | Bash×2 Edit×1 ToolSearch×1 submit×2 |  |
| F2 | F | sonnet | 1 | yes | 0.0624 | 5 | 6 | 106214 | 11394 | 609 | 0 | Bash×1 Read×1 ToolSearch×1 submit×1 |  |
| F2 | T | haiku | 1 | yes | 0.0043 | 5 | 8 | 147387 | 11236 | 912 | 0 | proof_list×1 proof_save×2 task_info×1 |  |
| F2 | T | haiku | 2 | yes | 0.0045 | 5 | 8 | 150444 | 11978 | 820 | 0 | Bash×1 ToolSearch×1 proof_save×1 task_info×1 |  |
| F2 | T | haiku | 3 | yes | 0.0043 | 4 | 8 | 150269 | 11850 | 625 | 0 | ToolSearch×1 proof_save×1 task_info×1 |  |
| F2 | T | sonnet | 1 | yes | 0.0580 | 3 | 6 | 106026 | 10902 | 365 | 0 | ToolSearch×1 proof_save×1 |  |
| F3 | F | haiku | 1 | no | 0.0054 | 6 | 10 | 190173 | 12411 | 1795 | 0 | Bash×4 Edit×1 | edit never submitted |
| F3 | F | haiku | 2 | yes | 0.0065 | 8 | 14 | 275262 | 13380 | 1828 | 0 | Bash×5 ToolSearch×1 submit×1 |  |
| F3 | F | haiku | 3 | yes | 0.0056 | 7 | 10 | 191483 | 12902 | 1934 | 0 | Bash×2 Edit×2 ToolSearch×1 submit×1 |  |
| F3 | F | sonnet | 1 | yes | 0.0774 | 7 | 10 | 187619 | 12537 | 829 | 0 | Bash×3 Read×1 ToolSearch×1 submit×1 |  |
| F3 | T | haiku | 1 | yes | 0.0046 | 4 | 8 | 149471 | 11820 | 1126 | 0 | ToolSearch×1 proof_list×1 proof_save×1 |  |
| F3 | T | haiku | 2 | yes | 0.0043 | 4 | 8 | 150060 | 11097 | 954 | 0 | ToolSearch×1 proof_list×1 proof_save×1 |  |
| F3 | T | haiku | 3 | yes | 0.0043 | 4 | 8 | 150577 | 11266 | 720 | 0 | ToolSearch×1 proof_list×1 proof_save×1 |  |
| F3 | T | sonnet | 1 | yes | 0.0642 | 4 | 8 | 145878 | 11208 | 465 | 0 | ToolSearch×1 proof_list×1 proof_save×1 |  |
| F4 | F | haiku | 1 | yes | 0.0039 | 3 | 6 | 108422 | 12032 | 646 | 0 | Bash×2 |  |
| F4 | F | haiku | 2 | yes | 0.0044 | 4 | 8 | 149966 | 12160 | 715 | 0 | Bash×3 |  |
| F4 | F | haiku | 3 | yes | 0.0040 | 3 | 6 | 108423 | 12029 | 715 | 0 | Bash×2 |  |
| F4 | F | sonnet | 1 | yes | 0.0581 | 3 | 6 | 105500 | 10968 | 350 | 0 | Bash×2 |  |
| F4 | T | haiku | 1 | yes | 0.0033 | 3 | 6 | 107751 | 9284 | 462 | 0 | ToolSearch×1 list_tasks×1 |  |
| F4 | T | haiku | 2 | yes | 0.0040 | 5 | 8 | 147288 | 10147 | 782 | 0 | ToolSearch×1 list_tasks×1 task_chain×2 |  |
| F4 | T | haiku | 3 | yes | 0.0035 | 4 | 6 | 108054 | 9769 | 610 | 0 | ListAgents×1 ToolSearch×1 list_tasks×1 |  |
| F4 | T | sonnet | 1 | yes | 0.0524 | 3 | 6 | 104702 | 9640 | 318 | 0 | ToolSearch×1 list_tasks×1 |  |
| F5 | F | haiku | 1 | yes | 0.0043 | 5 | 8 | 149466 | 11847 | 551 | 0 | Read×2 ToolSearch×1 branches×1 |  |
| F5 | F | haiku | 2 | yes | 0.0042 | 5 | 8 | 149427 | 11809 | 504 | 0 | Read×2 ToolSearch×1 branches×1 |  |
| F5 | F | haiku | 3 | yes | 0.0043 | 4 | 8 | 149566 | 11889 | 547 | 0 | Bash×1 ToolSearch×1 branches×1 |  |
| F5 | F | sonnet | 1 | yes | 0.0667 | 5 | 8 | 145917 | 11947 | 421 | 0 | Bash×1 Read×1 ToolSearch×1 branches×1 |  |
| F5 | T | haiku | 1 | yes | 0.0033 | 3 | 6 | 108051 | 9661 | 313 | 0 | ToolSearch×1 task_info×1 |  |
| F5 | T | haiku | 2 | yes | 0.0037 | 4 | 8 | 147353 | 9598 | 363 | 0 | ToolSearch×1 task_info×2 |  |
| F5 | T | haiku | 3 | yes | 0.0031 | 3 | 6 | 107353 | 8956 | 295 | 0 | ToolSearch×1 task_info×1 |  |
| F5 | T | sonnet | 1 | yes | 0.0511 | 3 | 6 | 104456 | 9489 | 251 | 0 | ToolSearch×1 task_info×1 |  |
| F6 | F | haiku | 1 | yes | 0.0077 | 10 | 14 | 284465 | 15246 | 2683 | 0 | Bash×3 ToolSearch×1 submit×5 |  |
| F6 | F | haiku | 2 | yes | 0.0065 | 8 | 8 | 158474 | 16489 | 2338 | 0 | Bash×1 ToolSearch×1 Write×4 submit×1 |  |
| F6 | F | haiku | 3 | yes | 0.0064 | 5 | 8 | 158111 | 16173 | 2202 | 0 | Bash×2 ToolSearch×1 submit×1 |  |
| F6 | F | sonnet | 1 | yes | 0.0867 | 5 | 6 | 109559 | 14962 | 1547 | 0 | Bash×1 Read×1 ToolSearch×1 submit×1 |  |
| F6 | T | haiku | 1 | yes | 0.0053 | 5 | 8 | 150488 | 12661 | 1619 | 0 | ToolSearch×1 create_task×2 task_info×1 |  |
| F6 | T | haiku | 2 | yes | 0.0050 | 5 | 8 | 151977 | 11190 | 1681 | 0 | ToolSearch×1 create_task×2 task_info×1 |  |
| F6 | T | haiku | 3 | yes | 0.0052 | 5 | 8 | 152387 | 11456 | 1727 | 0 | ToolSearch×1 create_task×2 task_info×1 |  |
| F6 | T | sonnet | 1 | yes | 0.0782 | 5 | 8 | 146348 | 12096 | 1474 | 0 | ToolSearch×1 create_task×2 task_info×1 |  |

---

# Part 1b: sonnet, realistic fixtures, files-arm ablations

Still P643 Part 1 (no new phase number). The user challenged the Part 1 NO-GO: haiku is a weak
model for this test; the files arm may have lost on README length, redundancy or file count, not on
files; fixtures were tiny; context after tool load is already high. Part 1b answers each. Part 1
tables above stay as shipped; haiku numbers are reference only, no new haiku runs.

Test: `apps/kira-space/tests/claude/details_spike1b_test.go` (fixtures `testdata/details-spike-1b/`,
helpers shared with the Part 1 test). Opt-in: `KIRA_SPIKE=run1b`. Raw transcripts and logs of these
runs live in the session scratchpad only; `TestDetailsSpike1bReport` rebuilds the tables from dumps.

## Setup

- Model: sonnet for every run. Arms: T (today's tools, real `agentmcp.Server`, unchanged), FF
  (files, full README plus `AGENTS.md` as the planned generator writes them), FL (files, lean),
  FC (FL with one `handovers.md` and one `notes.md` per task instead of one file each). All files arms
  have the same stub `submit`, `finish_step`, `branches`, `lease`, `cancel_operation`; the task folder
  and `../AGENTS.md` (tasks root) are `--add-dir`ed.
- Fixtures, same content in both arms (T gets it from stub backends): task t-1 has 3 handovers of
  4.8 to 5.0 KB (summary, what changed with file paths, decisions and why, open risks, how verified,
  next-step guidance) and 3 notes of 331 to 392 bytes. Orchestrator children have 3 handovers each of
  4.3 to 4.5 KB. F3 proof: three steps (table of 20 rows by 3 columns, a Mermaid flowchart, a
  document explanation). The image step of Part 1 is out; its validators are unchanged.
- Step prompts inline the latest handover and the notes in both arms (Q13). F4 asks for "the blocking
  open risk its latest handover reports" per child. The key sentence sits in the latest handover's
  Open risks section, past the 500-byte `list_tasks` preview, so arm T needs `task_chain` and arm F
  needs the right file; an older handover carries a decoy risk. Oracle tokens occur only in the latest
  handover of each child (asserted by the generator).
- Gate unchanged: cost F/T median at most 1.2 (F6 1.1); final context at most 1.0; pass not lower;
  failed edit-path calls plus retries (mean) not higher than T's; turns reported only. Medians with
  min-max; a gap smaller than the larger run-to-run range is flagged inconclusive.
- Token counts for README and `AGENTS.md` are bytes/4 (no tokenizer offline); the tool-size and
  context rows use the stream's real usage where stated.

### README and AGENTS.md sizes

Counted as rendered for a non-orchestrator task; the orchestrator adds a children row to the README
and a child section to `AGENTS.md`.

| Variant | README bytes | AGENTS.md bytes | Lines | Tokens (bytes/4) | With child section |
|---|---|---|---|---|---|
| FF full | 1445 | 2854 | 71 | 1075 | 1282 tokens, 5130 bytes |
| FL lean | 312 | 713 | 17 | 256 | 306 tokens, 1222 bytes (18 lines) |
| FC lean consolidated | 418 | 752 | 18 | 293 | 342 tokens, 1367 bytes (19 lines) |

FL is 76 percent smaller than FF and has 17 lines (18 with the child section), under the 25-line aim.

Redundancies removed from FF to FL:

1. The "edit, then call `submit`; Kira accepts, holds or rejects" paragraph appears in the README and
   again in `AGENTS.md`; kept once, README line 2.
2. The `Kira tools:` line (tool names and one-clause use) restates the tool descriptions; removed.
3. How to finish a step appears in the README, in `AGENTS.md` and in the `finish_step` description;
   kept as one `AGENTS.md` line.
4. Draft rules (Markdown, at most 16 KiB, name the step, rewrite to replace) appear in the file table
   and in `AGENTS.md`; one line.
5. "README.md, task.yaml, state.yaml are read-only, edits are undone" appears three times (table,
   `AGENTS.md`, Rules paragraph) and in the `submit` description (`restored`); kept once.
6. The accepted-proof rule (reason required, user approves, keep unchanged step ids and files) appears
   in the Rules paragraph, the proof section and the `submit` description; one line.
7. The five-column-style file table with a "Written by" and "Rules" column cut to three lines; the
   `links.yaml` row is dropped (no flow uses it; a shipped generator needs one line for it).
8. Proof template extras not needed by the step: the three optional keys (query, language, source),
   "steps show in order, 1 to 100", chart and pie formats, the limits paragraph. Chart format and the
   limits are dropped from FL; the validator's error text should carry a limit when it is hit.
9. The orchestrator child section from 13 lines to 1.

## Results (sonnet)

Executed matrix: 45 runs. T, FF, FL on F1, F3, F4, F6 with 3 runs each; FC on F1 and F4 with 3 runs;
F5 with 1 run for T, FF, FL. F2 not run. FC not run on F3, F5 and F6 (and F2). See Budget.

### Budget, run count and spend

Read as: 5 USD is the total for the P643 spike; 0.97 USD was spent in Part 1, so Part 1b may spend
4.03 USD. A 4-run calibration (F1 and F4, T and FF) cost 0.35 USD, 0.0875 USD per run on average.
Projection before the main run: the full matrix (6 flows, 4 arms, 3 runs = 72 runs) about 6.3 USD;
with FC dropped for F2, F4 and F5 (63 runs) about 5.5 USD; both above the 4.03 USD left. The plan
dropped further than the brief's fallback: FC only where history layout matters (F1, F4), and F2
and F5 cut (F2 is a proof create, the same shape as F3; F5 kept as one run per arm with the last
cents of budget). Spent: calibration 0.3497, main run 3.1851, F5 0.1963, context probes 0.185; Part 1b
total 3.916 USD, running total 4.886 USD, under 5.00. Per-run cap 0.5 USD held (largest run
0.1044). Consequences: F2 has no sonnet-realistic data; F5 is a single run per arm, so its verdicts are
indicative only; FC on F3, F5 and F6 is untested (it differs from FL only in the two history files,
which those flows barely read: see below). Extending to the full matrix costs about 2.5 USD more.

### Cost, final context, turns, edit ease

Cells are median [min-max] over the runs.

| Flow | Arm | Pass | Cost USD | Final context | Turns | Failed edit-path + retries |
|---|---|---|---|---|---|---|
| F1 handover | T | 3/3 | 0.0741 [0.0729-0.0907] | 41477 [41393-41672] | 6 [6-7] | 2 [2-2] |
| F1 handover | FF | 3/3 | 0.0829 [0.0759-0.0852] | 41818 [41569-42193] | 6 [5-6] | 2 [0-2] |
| F1 handover | FL | 3/3 | 0.0798 [0.0784-0.0919] | 41661 [41363-41995] | 6 [6-8] | 0 [0-2] |
| F1 handover | FC | 3/3 | 0.0822 [0.0781-0.0919] | 42007 [41711-42383] | 6 [6-7] | 2 [0-2] |
| F3 proof edit | T | 2/3 | 0.0666 [0.0643-0.0815] | 42778 [42163-42778] | 5 [4-5] | 0 [0-0] |
| F3 proof edit | FF | 3/3 | 0.0933 [0.0880-0.0941] | 43995 [43942-44240] | 8 [6-8] | 0 [0-0] |
| F3 proof edit | FL | 3/3 | 0.0854 [0.0796-0.0875] | 42531 [42319-42858] | 7 [6-7] | 0 [0-0] |
| F4 orchestrator summary | T | 3/3 | 0.0982 [0.0980-0.1015] | 49058 [49050-49298] | 5 [5-6] | 0 [0-0] |
| F4 orchestrator summary | FF | 3/3 | 0.0777 [0.0747-0.0778] | 43062 [43051-43334] | 7 [7-7] | 0 [0-0] |
| F4 orchestrator summary | FL | 3/3 | 0.0711 [0.0673-0.0915] | 42633 [42046-48054] | 4 [4-6] | 0 [0-0] |
| F4 orchestrator summary | FC | 3/3 | 0.1026 [0.0914-0.1044] | 48298 [47991-49474] | 6 [4-8] | 0 [0-0] |
| F6 create 2 child tasks | T | 3/3 | 0.0669 [0.0668-0.0672] | 40824 [40789-40834] | 4 [4-4] | 0 [0-0] |
| F6 create 2 child tasks | FF | 3/3 | 0.0939 [0.0906-0.0958] | 44527 [43351-44708] | 6 [6-6] | 0 [0-0] |
| F6 create 2 child tasks | FL | 3/3 | 0.0912 [0.0904-0.0918] | 43415 [43374-43489] | 7 [7-7] | 0 [0-0] |
| F5 branch wait | T | 1/1 | 0.0571 [0.0571-0.0571] | 40026 [40026-40026] | 4 [4-4] | 0 [0-0] |
| F5 branch wait | FF | 1/1 | 0.0670 [0.0670-0.0670] | 41868 [41868-41868] | 5 [5-5] | 0 [0-0] |
| F5 branch wait | FL | 1/1 | 0.0722 [0.0722-0.0722] | 42094 [42094-42094] | 5 [5-5] | 0 [0-0] |

### What each run read, and ToolSearch

Doc = README.md and AGENTS.md result bytes read (tokens are bytes/4). History = reads that touched handovers or notes. Reads = Read, Glob, Grep and read-only Bash calls. Search = ToolSearch calls and the context they added. Medians.

| Flow | Arm | Doc bytes | Doc tokens | Reads | Read bytes | History reads | History bytes | Search calls | Search ctx |
|---|---|---|---|---|---|---|---|---|---|
| F1 | T | 0 | 0 | 1.0 | 39 | 0.0 | 0 | 0.0 | 0 |
| F1 | FF | 1847 | 462 | 1.0 | 1847 | 0.0 | 0 | 0.0 | 0 |
| F1 | FL | 405 | 101 | 1.0 | 405 | 0.0 | 0 | 1.0 | 1045 |
| F1 | FC | 820 | 205 | 1.0 | 820 | 1.0 | 464 | 0.0 | 0 |
| F3 | T | 0 | 0 | 0.0 | 0 | 0.0 | 0 | 1.0 | 2157 |
| F3 | FF | 5091 | 1273 | 4.0 | 6450 | 0.0 | 0 | 1.0 | 691 |
| F3 | FL | 2075 | 519 | 3.0 | 3327 | 1.0 | 1099 | 0.0 | 0 |
| F4 | T | 0 | 0 | 0.0 | 0 | 0.0 | 0 | 1.0 | 839 |
| F4 | FF | 1595 | 399 | 6.0 | 12980 | 2.0 | 9315 | 0.0 | 0 |
| F4 | FL | 340 | 85 | 3.0 | 12178 | 1.0 | 10949 | 0.0 | 0 |
| F4 | FC | 448 | 112 | 5.0 | 28893 | 2.0 | 27542 | 0.0 | 0 |
| F6 | T | 0 | 0 | 0.0 | 0 | 0.0 | 0 | 1.0 | 926 |
| F6 | FF | 5373 | 1343 | 2.0 | 5373 | 0.0 | 0 | 1.0 | 2833 |
| F6 | FL | 1271 | 318 | 3.0 | 1999 | 0.0 | 0 | 1.0 | 2319 |
| F5 | T | 0 | 0 | 0.0 | 0 | 0.0 | 0 | 0.0 | 0 |
| F5 | FF | 1505 | 376 | 2.0 | 1986 | 0.0 | 0 | 1.0 | 1807 |
| F5 | FL | 322 | 80 | 2.0 | 796 | 0.0 | 0 | 1.0 | 1725 |

### Gate per files arm against T, per metric

Cost F/T median at most 1.2 (F6: 1.1); final context F/T median at most 1.0; pass count not lower; failed edit-path calls plus retries (mean) not higher than T's. `inconclusive` marks a metric where the gap between medians is smaller than the larger run-to-run range of the two arms. Turns are in the table above, never gated.

| Flow | Arm | Cost F/T | Cost | Ctx F/T | Ctx | Pass | Pass | Edit F vs T | Edit |
|---|---|---|---|---|---|---|---|---|---|
| F1 | FF | 1.12 (<= 1.2) | yes (inconclusive) | 1.01 (<= 1.0) | no (inconclusive) | 3/3 vs 3/3 | yes | 1.33 vs 2.00 | yes (inconclusive) |
| F1 | FL | 1.08 (<= 1.2) | yes (inconclusive) | 1.00 (<= 1.0) | no (inconclusive) | 3/3 vs 3/3 | yes | 0.67 vs 2.00 | yes |
| F1 | FC | 1.11 (<= 1.2) | yes (inconclusive) | 1.01 (<= 1.0) | no (inconclusive) | 3/3 vs 3/3 | yes | 1.33 vs 2.00 | yes (inconclusive) |
| F3 | FF | 1.40 (<= 1.2) | no | 1.03 (<= 1.0) | no | 3/3 vs 2/3 | yes | 0.00 vs 0.00 | yes |
| F3 | FL | 1.28 (<= 1.2) | no | 0.99 (<= 1.0) | yes (inconclusive) | 3/3 vs 2/3 | yes | 0.00 vs 0.00 | yes |
| F4 | FF | 0.79 (<= 1.2) | yes | 0.88 (<= 1.0) | yes | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes |
| F4 | FL | 0.72 (<= 1.2) | yes | 0.87 (<= 1.0) | yes | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes |
| F4 | FC | 1.04 (<= 1.2) | yes (inconclusive) | 0.98 (<= 1.0) | yes (inconclusive) | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes |
| F6 | FF | 1.40 (<= 1.1) | no | 1.09 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes |
| F6 | FL | 1.36 (<= 1.1) | no | 1.06 (<= 1.0) | no | 3/3 vs 3/3 | yes | 0.00 vs 0.00 | yes |
| F5 | FF | 1.17 (<= 1.2) | yes | 1.05 (<= 1.0) | no | 1/1 vs 1/1 | yes | 0.00 vs 0.00 | yes |
| F5 | FL | 1.26 (<= 1.2) | no | 1.05 (<= 1.0) | no | 1/1 vs 1/1 | yes | 0.00 vs 0.00 | yes |



### Attribution: which factor explains the gap to T

Median cost per run, USD, and the deltas. FF minus FL isolates README and AGENTS length plus
redundancy. FC minus FL isolates file count (history layout). FL minus T is what stays after both:
the call structure of a files flow.

| Flow | T | FF | FL | FC | FF-FL (length, redundancy) | FC-FL (file count) | FL-T (remainder) |
|---|---|---|---|---|---|---|---|
| F1 handover | 0.0741 | 0.0829 | 0.0798 | 0.0822 | +0.0031 (noise: FF range 0.0759-0.0852) | +0.0024 (noise) | +0.0057 (noise) |
| F3 proof edit | 0.0666 | 0.0933 | 0.0854 | not run | +0.0079 | - | +0.0188 |
| F4 orchestrator | 0.0982 | 0.0777 | 0.0711 | 0.1026 | +0.0066 (inconclusive) | +0.0315 | -0.0271 |
| F6 create 2 children | 0.0669 | 0.0939 | 0.0912 | not run | +0.0027 | - | +0.0243 |
| F5 branch wait (1 run) | 0.0571 | 0.0670 | 0.0722 | not run | -0.0052 | - | +0.0151 |

- README length and redundancy: small. FF reads 376 to 1343 doc tokens per run, FL 80 to 519 (table
  above); the saving is 0.003 to 0.008 USD per run, 10 to 30 percent of the FF-versus-T gap in F3
  and F6, noise in F1, inconclusive in F4. It does not close the gap: FL still fails F3 and F6 cost.
- Redundancy specifically: the cut text is mostly the same rules stated twice; its removal is inside
  the length number above. FL did not lose pass rate or edit ease on any flow (lean and strict: no
  `branches` pointer, F5 single run still passed).
- File count: not a cost driver for writes (F1 FC versus FL +0.0024, noise). For history reads, more
  and smaller files win: the consolidated `handovers.md` made F4 44 percent dearer than FL (0.1026
  versus 0.0711; history bytes read 27.5 KB versus 10.9 KB), because the agent reads the whole file
  to reach the latest section. Per-file layout stays.
- History reading: irrelevant in F1, F3, F5, F6 (0 to 1 history reads, the prompt inlines the latest
  handover and notes). In F4 it is the whole story and it favors files: T needs `list_tasks` plus
  one `task_chain` per child, and `task_chain` returns every row of the child (final context 49.1k
  versus 42.6k to 43.1k for FL and FF); the files arm opens only the latest handover. Cost 0.72 to
  0.79 of T, context 0.87 to 0.88.
- Remainder (FL-T, 70 to 90 percent of the gap in F3 and F6): call structure, not text size. A files
  flow reads two docs (README, then `AGENTS.md`), writes with Bash or Edit calls, then calls `submit`;
  T loads a tool and calls it once. Turns: F6 T 4, FF 6, FL 7; F3 T 5, FF 8, FL 7. A turn costs 0.015
  to 0.02 USD here (median run cost over turns), so two to three extra turns are the gap.

### Context after tool load

Single-turn probes on sonnet (`Reply with the single word: ok`), turn-1 context from the stream usage,
0.185 USD in total:

| Component | Tokens | How measured |
|---|---|---|
| System prompt, no tools, no MCP | 2,396 | probe `--tools ""` |
| Built-in tool schemas (Read, Edit, Write, Bash, Glob, Grep, Agent and the rest) | 33,080 | probe default tools minus probe above (35,476) |
| ToolSearch definition plus MCP name listing, T run role (12 tools) | +336 | probe minus 35,476 |
| same, T orchestrator (15 tools) | +371 | |
| same, F run role (5 tools) | +263 | |
| same, F orchestrator (6 tools) | +272 | |
| MCP schemas, all arms, turn 1 | 0 | deferred behind ToolSearch |
| Step prompt (inlined handover and notes) | 0.1k to 2.1k | F4 prompt 258 bytes: turn 1 37.4k; F1 6.4 KB prompt: 39.5k |
| README and AGENTS.md | 0 at turn 1 | read in turn 2 or later: FF 376 to 1343 tokens, FL 80 to 519 |

Turn-1 context per run: 37.4k (F4) to 39.8k (F3); T is 30 to 80 tokens above the files arms. Kira
controls: the tool count (about 10 tokens per listed tool name: 12 to 5 tools saves 73 tokens, 15 to 6
saves 99), the description and schema length (0 at turn 1; paid when loaded), the README (paid when
read) and the inlined step prompt. Kira does not control: the system prompt (2.4k), the built-in tool
schemas (33.1k, 87 percent of turn 1), the ToolSearch definition (about 210) or whether the model
loads schemas at all.

Loaded size if a client does not defer (JSON of `tools/list`, name, description, schema; bytes/4,
no run): T run role 12 tools 3,868 tokens; T with the two proof tools 5,150; T orchestrator 4,767; F
5 tools 1,396; F orchestrator 6 tools 1,719. Not run with deferral off (cold-cache probes were not
worth 0.3 USD): the estimate is 2.5k to 3.0k tokens in T's disfavor for a client that loads every
schema, 0 on Claude Code 2.1.325 where schemas are deferred.

ToolSearch, T versus files, from the runs (13 runs per arm): T ran ToolSearch in 9, FF in 7, FL in 7,
FC in 1 of 6 (F4, which needs no Kira tool). The step costs one turn (0.015 to 0.02 USD, about 20
percent of a 0.08 USD run) and adds 0.7k to 2.8k tokens of context once loaded (T: F3 2,157, F4 839,
F6 926; files: F6 2,319 to 2,833, F5 1,725 to 1,807, F1 1,045). The load per search is similar in size and varies by flow (F3 T 2,157 against FF 691; F6 T 926
against FF 2,833); the merged files tools carry the text of the tools they replace. Skipping it
fails: calling a deferred tool before loading it returns "input schema was not in your prompt"; that
is the two failed edit-path calls per run in T F1 (3 of 3 runs), FF F1 (2 of 3) and FC F1 (2 of 3).
The cost of ToolSearch is therefore a wash between arms; it is not a reason to prefer either.

### Per-metric verdicts (F1, F3, F4, F6 3 runs per arm; F5 1 run)

`ok` passes, `FAIL` fails, `~` marks a metric inconclusive (gap below run-to-run range).

| Flow | Arm | Cost | Final context | Pass | Edit ease |
|---|---|---|---|---|---|
| F1 | FF | 1.12 ok ~ | 1.01 FAIL ~ | 3/3 vs 3/3 ok | 1.33 vs 2.00 ok ~ |
| F1 | FL | 1.08 ok ~ | 1.00 FAIL ~ | ok | 0.67 vs 2.00 ok |
| F1 | FC | 1.11 ok ~ | 1.01 FAIL ~ | ok | 1.33 vs 2.00 ok ~ |
| F3 | FF | 1.40 FAIL | 1.03 FAIL | 3/3 vs 2/3 ok | ok |
| F3 | FL | 1.28 FAIL | 0.99 ok ~ | 3/3 vs 2/3 ok | ok |
| F4 | FF | 0.79 ok | 0.88 ok | ok | ok |
| F4 | FL | 0.72 ok | 0.87 ok | ok | ok |
| F4 | FC | 1.04 ok ~ | 0.98 ok ~ | ok | ok |
| F6 (cost 1.1) | FF | 1.40 FAIL | 1.09 FAIL | ok | ok |
| F6 (cost 1.1) | FL | 1.36 FAIL | 1.06 FAIL | ok | ok |
| F5 (1 run) | FF | 1.17 ok | 1.05 FAIL | ok | ok |
| F5 (1 run) | FL | 1.26 FAIL | 1.05 FAIL | ok | ok |
| F2 | all | not run | not run | not run | not run |

Against Part 1 (haiku): F1 1.06 to 1.12 (same), F3 1.28 to 1.40 (1.29), F6 1.36 to 1.40 (1.24, worse),
F5 1.17 to 1.26 (1.30), F4 0.72 to 0.79 (1.15, now a clear win because the fixtures are realistic
and T's `task_chain` returns whole chains). T's F3 pass is 2 of 3: in one run the agent could not tell
which proof step is the table (`proof_list` returns ordinals, no step views) and stopped; files expose
the proof, so this is a pass-rate edge for files that cost cannot show.

Final-context ratios of 1.00 to 1.01 in F1 sit inside the run spread (T 41,393 to 41,672 tokens; FL
41,363 to 41,995): treat as parity, not a pass.

Which arm passes: none passes the unchanged gate on every flow. Overall verdict stays NO-GO for a
files-only Details surface. FL is the best files arm (cheapest, no pass or edit regression) and wins F4;
F3 and F6 fail on cost, F6 also on context.

### Recommendation

1. Do not build the files-only surface as planned. The gap that remains after cutting README text and
   fixing file layout is call structure (extra turns to read docs, write files, submit), which a better
   README cannot remove. Choose fallback (a) from section 14, narrowed by the data: files stay the
   source of truth and the readable state (notes, handovers, `state.yaml`, links, proofs, orchestrator
   chain reads), but structured writes keep a tool entry that writes the files: proofs (F3 +28 to
   +40 percent as files) and child task creation (F6 +36 to +40 percent, context +6 to +9 percent).
   Handover through a draft file plus `finish_step` is at parity (F1) and may stay file-based.
2. D-Q2: the verdict rule gives `D-Q2: T` (F6 cost 1.36 to 1.40 against 1.1 is 24 to 27 percent over
   the threshold, not borderline; turns 6 to 7 against 4). Part 1 did not write the line; the user
   decides, this is the data.
3. If files-only is chosen anyway, adopt: the FL shape. One README of at most 25 lines per task, no
   restated rules, no tool list, formats only for kinds the task uses (children section only for
   orchestrators), per-file layout (never consolidated `handovers.md`: +44 percent on F4), the latest
   handover inlined in the step prompt as today. Untested but implied by the remainder: put the formats
   in the README itself, not a second `AGENTS.md`; every agent reads both today (two calls, one turn,
   0.015 to 0.02 USD, about a quarter of the F3 and F6 remainder). Part 2's benchmark should test it.
4. Orchestrator reads from files are a real win (F4 0.72 to 0.79 cost, 0.87 to 0.88 context). If the
   hybrid is chosen, keep that part: it removes `task_chain` and `list_tasks` previews from the agent
   path.
5. Not answered here: F2 on sonnet, FC on F3, F5 and F6, F5 beyond one run, runs without
   deferral. They need about 2.5 USD more; the user decides whether the decision turns on them (F2 is
   the same shape as F3, so the data point to the same verdict).

### Part 1b runs

| Flow | Arm | Rep | Pass | Cost | Turns | Final ctx | Doc bytes | Reads | Hist reads | Search | Failed+retr | Tool calls | Note |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| F1 | FC | 1 | yes | 0.0919 | 7 | 42383 | 1461 | 2 | 1 | 0 | 2 | Bash×4 finish_step×2 |  |
| F1 | FC | 2 | yes | 0.0781 | 6 | 41711 | 0 | 1 | 1 | 1 | 0 | Bash×3 ToolSearch×1 finish_step×1 |  |
| F1 | FC | 3 | yes | 0.0822 | 6 | 42007 | 820 | 1 | 0 | 0 | 2 | Bash×3 finish_step×2 |  |
| F1 | FF | 1 | yes | 0.0829 | 6 | 41818 | 1847 | 1 | 0 | 0 | 2 | Bash×3 finish_step×2 |  |
| F1 | FF | 2 | yes | 0.0852 | 6 | 42193 | 1538 | 1 | 0 | 0 | 2 | Bash×2 Write×1 finish_step×2 |  |
| F1 | FF | 3 | yes | 0.0759 | 5 | 41569 | 1847 | 1 | 0 | 1 | 0 | Bash×2 ToolSearch×1 finish_step×1 |  |
| F1 | FL | 1 | yes | 0.0919 | 8 | 41661 | 598 | 2 | 1 | 0 | 2 | Bash×3 Write×1 finish_step×2 submit×1 |  |
| F1 | FL | 2 | yes | 0.0784 | 6 | 41363 | 405 | 1 | 0 | 1 | 0 | Bash×3 ToolSearch×1 finish_step×1 |  |
| F1 | FL | 3 | yes | 0.0798 | 6 | 41995 | 405 | 1 | 0 | 1 | 0 | Bash×3 ToolSearch×1 finish_step×1 |  |
| F1 | T | 1 | yes | 0.0907 | 7 | 41672 | 0 | 2 | 0 | 0 | 2 | Bash×3 finish_step×2 task_info×1 |  |
| F1 | T | 2 | yes | 0.0729 | 6 | 41393 | 0 | 1 | 0 | 0 | 2 | Bash×1 Read×1 finish_step×2 task_info×1 |  |
| F1 | T | 3 | yes | 0.0741 | 6 | 41477 | 0 | 1 | 0 | 0 | 2 | Bash×2 finish_step×2 task_info×1 |  |
| F3 | FF | 1 | yes | 0.0933 | 8 | 43995 | 4525 | 4 | 0 | 0 | 0 | Bash×3 Edit×2 Read×1 submit×1 |  |
| F3 | FF | 2 | yes | 0.0880 | 6 | 43942 | 6745 | 2 | 0 | 1 | 0 | Bash×3 ToolSearch×1 submit×1 |  |
| F3 | FF | 3 | yes | 0.0941 | 8 | 44240 | 5091 | 4 | 0 | 1 | 0 | Bash×4 Read×1 ToolSearch×1 submit×1 |  |
| F3 | FL | 1 | yes | 0.0875 | 7 | 42858 | 2115 | 3 | 1 | 1 | 0 | Bash×4 ToolSearch×1 submit×1 |  |
| F3 | FL | 2 | yes | 0.0854 | 7 | 42531 | 1421 | 4 | 1 | 0 | 0 | Bash×4 Read×1 submit×1 |  |
| F3 | FL | 3 | yes | 0.0796 | 6 | 42319 | 2075 | 3 | 1 | 0 | 0 | Bash×4 submit×1 |  |
| F3 | T | 1 | yes | 0.0815 | 5 | 42778 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 proof_list×2 proof_save×1 |  |
| F3 | T | 2 | yes | 0.0666 | 5 | 42778 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 proof_list×2 proof_save×1 |  |
| F3 | T | 3 | no | 0.0643 | 4 | 42163 | 0 | 1 | 0 | 1 | 0 | Grep×1 ToolSearch×1 proof_list×1 | no pending edit |
| F4 | FC | 1 | yes | 0.1044 | 8 | 49474 | 448 | 7 | 2 | 0 | 0 | Glob×2 Read×5 |  |
| F4 | FC | 2 | yes | 0.1026 | 6 | 48298 | 448 | 5 | 2 | 0 | 0 | Bash×4 Read×1 |  |
| F4 | FC | 3 | yes | 0.0914 | 4 | 47991 | 448 | 3 | 1 | 0 | 0 | Bash×2 Read×1 |  |
| F4 | FF | 1 | yes | 0.0747 | 7 | 43334 | 1595 | 6 | 2 | 0 | 0 | Glob×1 Read×5 |  |
| F4 | FF | 2 | yes | 0.0777 | 7 | 43062 | 1595 | 6 | 2 | 0 | 0 | Glob×1 Read×5 |  |
| F4 | FF | 3 | yes | 0.0778 | 7 | 43051 | 1595 | 6 | 2 | 0 | 0 | Glob×1 Read×5 |  |
| F4 | FL | 1 | yes | 0.0673 | 4 | 42046 | 340 | 3 | 1 | 0 | 0 | Bash×2 Read×1 |  |
| F4 | FL | 2 | yes | 0.0915 | 4 | 48054 | 340 | 3 | 1 | 0 | 0 | Bash×2 Read×1 |  |
| F4 | FL | 3 | yes | 0.0711 | 6 | 42633 | 340 | 5 | 2 | 0 | 0 | Glob×1 Read×4 |  |
| F4 | T | 1 | yes | 0.1015 | 6 | 49298 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 list_tasks×1 task_chain×2 task_info×1 |  |
| F4 | T | 2 | yes | 0.0980 | 5 | 49050 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 list_tasks×1 task_chain×2 |  |
| F4 | T | 3 | yes | 0.0982 | 5 | 49058 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 list_tasks×1 task_chain×2 |  |
| F5 | FF | 1 | yes | 0.0670 | 5 | 41868 | 1505 | 2 | 0 | 1 | 0 | Read×2 ToolSearch×1 branches×1 |  |
| F5 | FL | 1 | yes | 0.0722 | 5 | 42094 | 322 | 2 | 0 | 1 | 0 | Bash×1 Read×1 ToolSearch×1 branches×1 |  |
| F5 | T | 1 | yes | 0.0571 | 4 | 40026 | 0 | 0 | 0 | 0 | 0 | Bash×1 task_info×2 |  |
| F6 | FF | 1 | yes | 0.0906 | 6 | 43351 | 1595 | 2 | 0 | 1 | 0 | Bash×1 Grep×1 Read×1 ToolSearch×1 submit×1 |  |
| F6 | FF | 2 | yes | 0.0939 | 6 | 44527 | 5373 | 2 | 0 | 1 | 0 | Bash×1 Read×2 ToolSearch×1 submit×1 |  |
| F6 | FF | 3 | yes | 0.0958 | 6 | 44708 | 5571 | 2 | 0 | 1 | 0 | Bash×2 Read×1 ToolSearch×1 submit×1 |  |
| F6 | FL | 1 | yes | 0.0918 | 7 | 43489 | 1271 | 3 | 0 | 1 | 0 | Bash×2 Read×2 ToolSearch×1 submit×1 |  |
| F6 | FL | 2 | yes | 0.0912 | 7 | 43374 | 1271 | 3 | 0 | 1 | 0 | Bash×2 Read×2 ToolSearch×1 submit×1 |  |
| F6 | FL | 3 | yes | 0.0904 | 7 | 43415 | 1271 | 3 | 0 | 1 | 0 | Bash×2 Read×2 ToolSearch×1 submit×1 |  |
| F6 | T | 1 | yes | 0.0672 | 4 | 40834 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 create_task×2 |  |
| F6 | T | 2 | yes | 0.0668 | 4 | 40789 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 create_task×2 |  |
| F6 | T | 3 | yes | 0.0669 | 4 | 40824 | 0 | 0 | 0 | 1 | 0 | ToolSearch×1 create_task×2 |  |

## Part 1c: proof flows, improved files approach

Scope: F2 (new proof) and F3 (proof edit), sonnet, 3 runs per cell. 12 new runs, spend 0.9441 USD, running total about 5.83 USD of the 6 USD cap. All 12 passed. T F3 reuses the Part 1b runs (stub unchanged, see D). FL F3 is the Part 1b baseline. Raw per-run rows: `KIRA_SPIKE_DUMP` dirs, rebuild with `TestDetailsSpike1bReport`.

Arms: `FA` = A+C, separate data files. `FP` = A+B+C, one inline proof file. Ladder: FL to FA isolates A+C, FA to FP isolates B. FA ran F3 only (budget); F2 has T and FP.

- A: the step prompt names `proofs/orders-match-ledger/proof.md` (F3 only; F2 names the proof by title, as a user would). README adds `Proofs: <slug>, "<title>", N steps: <path>` (`none yet.` in F2). No answer leaks: the index names the proof, not its content.
- B: `proof.md` holds frontmatter plus step data under `inline:` (table rows one per line, diagram as `|` block scalar, document as one JSON line). Images and large data stay `data: <file>`.
- C: README line 1 reads "Edit, then call `submit`: a change counts only after it answers per file." The proof file starts with the comment `# Edit this file, then call submit.`

### D: fair T proof_list

No stub change. Real `proof_list` with a name returns `{name, steps: StepsForAgent(name)}`, each step `proof.StepBrief{ordinal, description, view}` (`internal/proof/service.go:181`). No step data, by design: an edit replaces a step with new data from the caller and keeps the rest by ordinal. Without a name it returns `ListForAgent()`. The Part 1b stub returns the same fields. T is neither handicapped nor advantaged. The Part 1b T F3 rep 3 failure was agent behaviour (stopped after `proof_list` and Grep, never called `proof_save`), so T F3 stays 2/3.

### Results (median [min-max])

| Flow | Arm | Pass | Cost USD | Final context | Turns | Failed edits + retries |
|---|---|---|---|---|---|---|
| F2 | T | 3/3 | 0.0638 [0.0637-0.0760] | 42921 [42913-42922] | 3 [3-3] | 0 |
| F2 | FP | 3/3 | 0.0837 [0.0833-0.0847] | 42635 [42585-42688] | 6 [6-6] | 0 |
| F3 | T | 2/3 | 0.0666 [0.0643-0.0815] | 42778 [42163-42778] | 5 [4-5] | 0 |
| F3 | FL (1b) | 3/3 | 0.0854 [0.0796-0.0875] | 42531 [42319-42858] | 7 [6-7] | 0 |
| F3 | FA | 3/3 | 0.0787 [0.0782-0.0854] | 42723 [42642-42818] | 7 [6-8] | 0 |
| F3 | FP | 3/3 | 0.0826 [0.0810-0.0831] | 42489 [42461-43037] | 8 [8-8] | 0.67 (2 of 3 runs) |

Against T (gate: cost <= 1.2, final context <= 1.0, pass not lower, edit ease not worse):

| Flow | Arm | Cost F/T | Ctx F/T | Pass | Edit ease |
|---|---|---|---|---|---|
| F2 | FP | 1.31, fails | 0.99, ok | 3/3 vs 3/3 | ok |
| F3 | FL | 1.28, fails | 0.99, ok | 3/3 vs 2/3 | ok |
| F3 | FA | 1.18, ok (inconclusive) | 1.00, ok (inconclusive) | 3/3 vs 2/3 | ok |
| F3 | FP | 1.24, fails (inconclusive) | 0.99, ok (inconclusive) | 3/3 vs 2/3 | 0.67 vs 0, worse |

Inconclusive: gap between medians is below the larger run-to-run range. T F3 spans 0.064-0.082, so F3 cost ratios carry roughly +-0.1 of noise. Turns are informational: T 3 (F2) and 5 (F3) against 6 to 8 for files.

### What explains what

- Discovery (A): Bash discovery calls per F3 run fall from 4 (FL: 4, 4, 4) to 1-3 (FA) to 0 (FP). Cost FL to FA falls 0.0854 to 0.0787. A is real but small, and FA still reads README or Bash-lists before editing.
- Doc bytes read: FL 2075, FA 1918, FP 442 (median). B stops the agent reading AGENTS.md, since the proof file is self-describing. Bytes saved are about 400 tokens, below the cost of the extra turns.
- B on cost: no gain. FP F3 costs 0.0826 against FA 0.0787 because it takes 8 turns against 7. Both Edit calls (cell, then `reason`) are separate turns, and 2 of 3 runs submitted first, got "set reason in proof.md frontmatter", fixed it and submitted again. Cause: the README and header comment never mention `reason`; the format doc that does is no longer read. Inline data itself edits cleanly: Edit of one table cell `[12, 1150, 1150]` to `[12, 1175, 1175]` applied and validated in 3 of 3 runs, no failed Edit.
- Submit friction (C): forgotten submits, 0 of 12 new runs (and 0 in the Part 1b F3 files runs). Every files run submitted. The stub's clear error text was enough for the one thing agents got wrong (missing `reason`). C costs nothing and is kept.
- ToolSearch: T, 1 per run (loads `proof_save`; F3 `proof_list` rides the same search). FA and FP, 1 per run (loads `submit`) in 12 of 12 new runs. FL (1b): 1 of 3. No arm avoids the load; the tools arm and the files arm pay the same here.
- Remaining gap is turn structure: T does `ToolSearch`, `proof_save` (F3: `proof_list` x2). Files do ToolSearch, Read, Edit (x1-2), submit, plus optional Bash. Each turn costs about 0.007-0.01 USD at this context size. Closing the gap needs fewer turns, not fewer bytes.

### Verdict

No variant reaches full parity. FA (A+C) is within the cost gate on F3 (1.18) but inconclusive, and F2 was not run for it. FP fails cost on both flows (1.24, 1.31) and edit ease on F3. Pass rate and final context are at or above T for every files arm. All files arms stay at or below T on context, so the earlier NO-GO for files-only stands; proof flows narrow from 1.28 (FL) to about 1.2 with A+C, not to 1.0.

Single-file inline proof (B) is valid and edits cleanly but does not pay for itself in cost. Follow-up not run (budget left 0.156 USD): add `reason:` to the proof header comment (`# To change an accepted proof: set reason:, edit, then call submit.`) and rerun FP F3; the retry turn is the only cost B adds over FA. The plan's proof section (format of proof.md, folder index) is left unchanged because no variant passed the gate.
