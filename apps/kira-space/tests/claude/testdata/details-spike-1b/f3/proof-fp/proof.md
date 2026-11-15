---
kira: proof/1
# Edit this file, then call submit.
name: Orders match ledger
summary: All 20 orders of the day match their ledger lines on id and total.
steps:
  - id: totals
    description: Orders and ledger agree on all 20 rows
    view: table
    inline:
      columns: [{"name": "order_id"}, {"name": "order_total"}, {"name": "ledger_total"}]
      rows:
        - [1, 1900, 1900]
        - [2, 1250, 1250]
        - [3, 2500, 2500]
        - [4, 2075, 2075]
        - [5, 2525, 2525]
        - [6, 1500, 1500]
        - [7, 850, 850]
        - [8, 1600, 1600]
        - [9, 425, 425]
        - [10, 1575, 1575]
        - [11, 1925, 1925]
        - [12, 1150, 1150]
        - [13, 1275, 1275]
        - [14, 2450, 2450]
        - [15, 1850, 1850]
        - [16, 2300, 2300]
        - [17, 1125, 1125]
        - [18, 2175, 2175]
        - [19, 400, 400]
        - [20, 2500, 2500]
  - id: flow
    description: The matching path the reconcile command follows
    view: diagram
    inline: |
      flowchart LR
        A["Orders table (day D)"] --> B["Export in id order"]
        B --> C["Match on order id and total in cents"]
        L["Ledger lines (day D)"] --> C
        C --> D{"Totals equal?"}
        D -- "yes" --> E["Matched"]
        D -- "no" --> F["Mismatch grouped by cause"]
        F --> G["Admin report"]
        E --> G
  - id: why
    description: Why the match holds and what a late edit does
    view: document
    inline: {"documents": [{"id": "why", "body": {"text": "Every order of the day is matched to its ledger line on order id and total in cents. All 20 orders matched, so the report shows no mismatches. The diagram is the matching path the reconcile command follows. Orders edited after the nightly export are matched against their old total until the next run, so a late edit appears as a mismatch on the following day and not on this one."}}]}
---

The reconcile command ran on the seeded day. See each step for the data it matched.
