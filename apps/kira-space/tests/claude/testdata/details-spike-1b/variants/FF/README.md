# Kira task files (task t-1)

These files are this task's Details. Edit them, then call the `submit` tool. Kira accepts a file,
holds it for the user's approval, or rejects it, and tells you which. Formats, rules and limits for
every file kind are in ../AGENTS.md.

| File | Written by | Rules |
|---|---|---|
| README.md, task.yaml, state.yaml | Kira, read-only | state.yaml: step, repos with branch setup, leases, operations, runs, children, pending submits. Edits are undone. |
| notes/*.md | user | Read freely. Your edit needs the user's approval. |
| handovers/draft-<step>.md | you | Hand-over notes to the next stage, Markdown, at most 16 KiB. Name the step you are in. Rewrite to replace. |
| handovers/NNNN-<from>-to-<to>.md | Kira | Handed handovers. Read only. |
| links.yaml | you and the user | `jira`, `pr`, `links: [{kind, title, url or ref}]`. |
| proofs/<slug>/proof.md | you | A validation proof; format in ../AGENTS.md. |
[[CH:| children/<slug>/ | you | Draft child tasks (orchestrator); format in ../AGENTS.md. |
]]
Kira tools: `submit`, `finish_step`, `branches` (declare, request, rename, change, or wait for a branch's setup), `lease`, `cancel_operation`.

Finish a step: write handovers/draft-<step>.md, then call `finish_step` (it submits first). A session
without a run calls `submit` with finished: true instead.

Rules: never edit README.md, task.yaml or state.yaml; edits are undone. A change to an accepted proof
needs a reason and the user's approval. Keep unchanged proof steps' ids and files as they are.
