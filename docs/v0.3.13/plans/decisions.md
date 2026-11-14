# v0.3.13 product decisions (user: all recommended)

Source: synthesis.md "questions for user". Streams implement these; tests assert them; fixes follow.

1. OQ-GRID-1: empty pasted field into nullable text column becomes NULL. Non-text columns already NULL.
2. OQ-GRID-5: after a lost commit acknowledgement, keep staged edits for rows that still differ from server; clear matching ones.
3. Concurrent edit policy, five places (grid row changed on server OQ-GRID-6, Mongo whole-doc replace OQ-DOC-1, saved query two windows OQ-CONS-4, API request two windows OQ-API-6, workflow two windows OQ-ADE-5): detect and warn with confirm; last write wins only after confirm.
4. OQ-DOC-2: editing `_id` refuses the save with an error.
5. OQ-ADE-4: switching a task's workflow shows a confirm naming run and log counts before deleting.
6. OQ-KV-2: warn before S3 save when ACL unreadable and may reset.
7. OQ-STR-2: duplicate SQS attribute name refuses the compose.
8. Closing a tab with staged grid edits or an open console transaction prompts; confirm rolls back.
9. Moving a stage while a headless run is live is refused until the run is stopped.
10. OQ-ADE-3: lease TTLs and run timeouts count only time the app ran (monotonic clock), not laptop sleep.
11. OQ-DKR-1 / OQ-K8S-1: Docker remove/pull/prune/compose and Kubernetes exec/edit/delete stay out of product; absence journeys dropped, coverage guard stays.
