# v0.2.35

Branch `chapter/v0.2.35`, based on `chapter/v0.2.34` (v0.2.34 not yet merged to main). First `P` is 512 (highest in v0.2.34 is P511), last is P523.

Source: user request. Twelve phases. P523 is the last implementation phase. Close duty after P523: one full suite run, 2 code review rounds (user ask), hand-written close. User decision: the v0.2.34 PR is not merged yet, so after the close and push, stop. Do NOT open the v0.2.35 PR or subscribe to one; report that the branch is ready. Each phase: Opus plan, Sonnet implementation.

Parallelism (user ask): up to 3 phases run at once as named streams, each its own worktree off the same base, zero file overlap. Waves keep table order; a wave starts only when the prior wave is landed. Waves: A = P512, P513, P514. B = P515, P516, P517. C = P518, P519. D = P520 alone (async touches every MCP tool). E = P521, P522 (planner may force sequential on overlap in the shared MCP access settings). F = P523 alone (touches every id generator). From P512 on, new Kira-generated ids follow the P523 convention where cheap.

| Phase | Title | Status |
|---|---|---|
| P512 | Tracked-item update indicators with per-concept settings | Pending |
| P513 | Repo scripts reference run-module scripts (backend, YAML) | Pending |
| P514 | YAML syntax highlighting and restored config dialogs | Pending |
| P515 | Run-script picker in the restored repo dialog | Pending |
| P516 | Reveal button on every secret field | Pending |
| P517 | Multiple worktrees per repo, agent-named branches, branch rename | Pending |
| P518 | Cancel ops via MCP and status bar running-ops widget | Pending |
| P519 | Proof window cell editor and multi-file screenshots | Pending |
| P520 | Async MCP tools with aggregated approvals and result push | Pending |
| P521 | Scripts MCP | Pending |
| P522 | API MCP | Pending |
| P523 | Prefixed nanoid for every Kira-generated id | Pending |

## Rows

### P512 Tracked-item update indicators with per-concept settings

- Ask: Task shows at a glance that a tracked item changed since last seen: PR (new comments, merged, checks failed), Jira issue, branch, and the other trackable kinds.
- Meaningful vs noise is configurable in Settings, per trackable kind and per concept, independently. Examples: PR comment added, PR merged, checks passed, checks failed (two separate toggles), assignee change. Defaults: comments, merge/close, checks failed, status change on; assignee, checks passed off.
- Plan: `plans/P512.md`.

### P513 Repo scripts reference run-module scripts (backend, YAML)

- Ask: Repo scripts (prepare worktree, check deployments, etc.) may reference a script from the Run module and run it the same way workflows do.
- Scope (user decision): normal Run scripts only; smart scripts refused with a clear message. Picker UI is P515.
- Plan: `plans/P513.md`.

### P514 YAML syntax highlighting and restored config dialogs

- Ask: Every config YAML view/editor gets syntax highlighting. Bring back the form config dialogs; YAML stays hand-editable, dialogs edit the same files, human-friendly as before.
- Scope (user decision): also restore the lifecycle actions the old dialogs had (connection delete and duplicate, API rename/delete/move, run collection editing).
- Plan: `plans/P514.md`.

### P515 Run-script picker in the restored repo dialog

- Ask: Repo dialog (restored by P514) lets the user pick a Run script for the prepare worktree and deployed-SHA hooks, using the P513 keys; params editable. Depends on P513 and P514.
- Plan: `plans/P515.md`.

### P516 Reveal button on every secret field

- Ask: Every place a secret is entered or shown has a reveal (eye) toggle, including dialogs restored by P514.
- Plan: `plans/P516.md`.

### P517 Multiple worktrees per repo, agent-named branches, branch rename

- Ask: A task may hold several worktrees for one repo (several branches in one repo). Worktree dir name is a nanoid. Branch name is a required argument of the MCP create tool, chosen by the agent. MCP tool can rename an existing branch, gated by an approval dialog.
- Plan: `plans/P517.md`.

### P518 Cancel ops via MCP and status bar running-ops widget

- Ask: Agent can stop a running query via MCP, and any request listed in the ops panel. Status bar widget lists running ops with a stop action per op.
- Plan: `plans/P518.md`.

### P519 Proof window cell editor and multi-file screenshots

- Ask: Proof window gets the cell editor (read-only is fine). Screenshots are split into several files, not one long PNG.
- Plan: `plans/P519.md`.

### P520 Async MCP tools with aggregated approvals and result push

- Ask: Every MCP tool is async: call returns at once, result is pushed to the agent when done. Pending approvals accumulate in one dialog (agent launches 3 requests: one dialog, accept or deny each). Results push only after the last pending approval is answered. Same for endpoints with result preview enabled.
- Plan: `plans/P520.md`.

### P521 Scripts MCP

- Ask: Agents run a script and fetch its logs via MCP. Log fetch capped at a max line count; simple filter (planner picks, e.g. tail/head and substring). Per-script MCP access is explicit opt-in. Per-script choice to show an approval prompt or not. Scripts get descriptions, as connections have. Tools are async per P520.
- Plan: `plans/P521.md`.

### P522 API MCP

- Ask: Agents call saved API requests via MCP. Same access model as P521: explicit opt-in per item, prompt on or off, description field. Tools are async per P520.
- Plan: `plans/P522.md`.

### P523 Prefixed nanoid for every Kira-generated id

- Ask: Every id Kira generates, of any kind, in the Kira apps is a nanoid with a short prefix. The prefix names the feature the id belongs to. External ids (GitHub, Jira, Docker, etc.) stay unaltered.
- Plan decides: inventory of all generators (uuid, ulid, autoincrement-as-id, hand-rolled), one shared id helper and prefix registry per language, handling of stored existing ids (migrate or leave, with reasoning), lint guard against new unprefixed generators. Uses an existing nanoid library.
- Plan: `plans/P523.md`.
