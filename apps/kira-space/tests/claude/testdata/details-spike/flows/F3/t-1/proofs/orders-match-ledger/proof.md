---
kira: proof/1
name: Orders match ledger
summary: Orders and ledger totals agree.
steps:
  - id: totals
    description: "Orders and ledger agree: 3 rows, total 1250"
    view: table
    data: totals.json
  - id: shot
    description: Ledger screenshot
    view: image
    data: shot.jpg
---
