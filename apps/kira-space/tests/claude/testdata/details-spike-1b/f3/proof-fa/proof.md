---
kira: proof/1
# Edit this file, then call submit.
name: Orders match ledger
summary: All 20 orders of the day match their ledger lines on id and total.
steps:
  - id: totals
    description: Orders and ledger agree on all 20 rows
    view: table
    data: totals.json
  - id: flow
    description: The matching path the reconcile command follows
    view: diagram
    data: flow.mmd
  - id: why
    description: Why the match holds and what a late edit does
    view: document
    data: explain.json
---

The reconcile command ran on the seeded day. See each step for the data it matched.
