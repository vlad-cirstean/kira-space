# v0.3.10

UI polish, dialog speed and MCP dialog fixes. Three phases, run as parallel streams A, B, C (batch requested by user). Highest prior phase: P616.

| Phase | Title | Status |
|-------|-------|--------|
| P617 | Task list and detail UI | Todo |
| P618 | Dialogs: speed, colors, background ops, proof, copy | Todo |
| P619 | Settings MCP split and GitHub PR open | Todo |

## P617 Task list and detail UI (Stream A)

- Task detail shows worktree's current checked-out branch next to the assigned branch; mismatch is visible (agents sometimes switch branches). MCP gains a tool to change the branch, gated by an approval prompt.
- Link card gets a copy button beside open-external.
- "Needs input" indicator sits at the start of the task row, not mid-row. Agent status indicators bigger.
- Task row elements use fixed, identical column sizing across tasks (status first: width independent of its text) so rows scan at a glance.
- Task list "today" left bar: clearly marks today without a bright yellow clashing with the task color. Task color gets a presentation other than, or in addition to, a colored left border.
- Opening Sessions tab in task detail focuses the keyboard into the open agent terminal.

Result: _pending_

## P618 Dialogs: speed, colors, background ops, proof, copy (Stream B)

- Audit every MCP-triggered dialog (create branch, create task, handover, others): content renders instantly with the dialog, handover text included. No delayed content.
- Semantic button colors (approve, deny, destructive, confirm) consistent across the whole app via shared shadcn-vue variants.
- Delete branch and close task: dialog dismisses once confirmed; progress shows as status in relevant places (task row, branch row, toast) until done.
- Proof-ready dialog: closes after open or close. Title order: task title first, proof title second.
- Studio MCP dialogs: text selectable and copyable (API host and all other fields).

Result: _pending_

## P619 Settings MCP split and GitHub PR open (Stream C)

- Settings "database MCP" section is too long: split into several sections. Rename user-facing label to "Studio MCP".
- Opening a GitHub PR from the app does not work: reproduce, root-cause, fix.

Result: _pending_
