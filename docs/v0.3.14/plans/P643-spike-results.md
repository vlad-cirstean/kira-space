# P643 Part 1 spike results

Test: `apps/kira-space/tests/claude/details_spike_test.go`. Arm T = today's tools (real `agentmcp.Server`, stub backends, kira-studio `proof_save` stub with today's schema). Arm F = planned files plus `submit`, `finish_step` without summary, merged `branches`, `lease`, `cancel_operation`, README in the task folder. Both arms get the same prompt core; arm F adds one line naming the task folder.

Runs: 48 of 48 planned. Spend: 0.9671 USD (cap 6.00 total, 0.25 per run). Single consolidated-file variant (one file instead of many small ones) was not run: the flows read at most two handovers, so the arm would add no signal within budget; read effort of the per-file layout is in the edit table. Cost is `total_cost_usd`; tokens are the result line's `usage` (input excludes cache; cache read and write listed apart).

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
