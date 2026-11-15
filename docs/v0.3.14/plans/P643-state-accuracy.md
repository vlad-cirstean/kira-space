# P643 state accuracy

Runs: 18 of 18 planned, sonnet. Spend: 1.1752 USD. 

Schema size (`tools/list` JSON of the `state` tool):

| Arm | Bytes | Tokens (bytes/4) |
|---|---|---|
| N | 1559 | 389 |
| F | 2028 | 507 |

### Per flow and arm

| Flow | Arm | Runs | Pass | Calls (mean) | Client calls (mean) | Attempts | FTC | FTC rate | Invalid by class | Retries | Cost USD (median) | Final context (median) | Turns (median) | ToolSearch ctx (median) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| A1 | N | 3 | 3 | 1.0 | 1.0 | 3 | 3 | 100.0% | none | 0 | 0.0586 | 38927 | 5 | 926 |
| A1 | F | 3 | 3 | 1.0 | 1.0 | 3 | 3 | 100.0% | none | 0 | 0.0519 | 38549 | 4 | 1162 |
| A2 | N | 3 | 3 | 6.0 | 6.0 | 18 | 18 | 100.0% | none | 0 | 0.0723 | 39470 | 9 | 926 |
| A2 | F | 3 | 3 | 6.3 | 6.3 | 19 | 19 | 100.0% | none | 0 | 0.0781 | 39749 | 9 | 1162 |
| A3 | N | 3 | 3 | 3.0 | 3.0 | 9 | 9 | 100.0% | none | 0 | 0.0620 | 38733 | 6 | 926 |
| A3 | F | 3 | 3 | 3.0 | 3.0 | 9 | 9 | 100.0% | none | 0 | 0.0636 | 39004 | 6 | 1162 |

### Overall per arm

| Arm | Runs | Pass | Failed runs | Attempts | FTC | FTC rate | Invalid by class | Retries | Cost USD (sum) |
|---|---|---|---|---|---|---|---|---|---|
| N | 9 | 9 | 0 | 30 | 30 | 100.0% | none | 0 | 0.5963 |
| F | 9 | 9 | 0 | 31 | 31 | 100.0% | none | 0 | 0.5789 |

### Verdict

WINNER: N (nested). First-try-correct N 100.0 percent, F 100.0 percent; failed runs N 0, F 0.

### Runs

| Flow | Arm | Rep | Pass | Calls | Client | Attempts | FTC | Invalid | Retries | Cost | Final ctx | Turns | Why |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| A1 | N | 1 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0766 | 39358 | 7 |  |
| A1 | F | 1 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0519 | 38547 | 4 |  |
| A2 | N | 1 | yes | 6 | 6 | 6 | 6 | none | 0 | 0.0723 | 39470 | 9 |  |
| A2 | F | 1 | yes | 6 | 6 | 6 | 6 | none | 0 | 0.0781 | 39749 | 9 |  |
| A3 | N | 1 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0620 | 38733 | 6 |  |
| A3 | F | 1 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0636 | 39017 | 6 |  |
| A1 | N | 2 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0586 | 38927 | 5 |  |
| A1 | F | 2 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0519 | 38549 | 4 |  |
| A2 | N | 2 | yes | 6 | 6 | 6 | 6 | none | 0 | 0.0721 | 39465 | 9 |  |
| A2 | F | 2 | yes | 7 | 7 | 7 | 7 | none | 0 | 0.0794 | 39949 | 10 |  |
| A3 | N | 2 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0620 | 38734 | 6 |  |
| A3 | F | 2 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0638 | 39003 | 6 |  |
| A1 | N | 3 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0582 | 38881 | 5 |  |
| A1 | F | 3 | yes | 1 | 1 | 1 | 1 | none | 0 | 0.0527 | 38608 | 4 |  |
| A2 | N | 3 | yes | 6 | 6 | 6 | 6 | none | 0 | 0.0724 | 39471 | 9 |  |
| A2 | F | 3 | yes | 6 | 6 | 6 | 6 | none | 0 | 0.0739 | 39742 | 9 |  |
| A3 | N | 3 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0620 | 38733 | 6 |  |
| A3 | F | 3 | yes | 3 | 3 | 3 | 3 | none | 0 | 0.0636 | 39004 | 6 |  |
