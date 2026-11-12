# v0.3.10

UI polish, dialog speed and MCP dialog fixes. Phases P617 to P619 run as parallel streams A, B, C (batch requested by user); P620 runs after they land. Highest prior phase: P616.

| Phase | Title | Status |
|-------|-------|--------|
| P617 | Task list and detail UI | Done |
| P618 | Dialogs: speed, colors, background ops, proof, copy | Todo |
| P619 | Settings MCP split and GitHub PR open | Todo |
| P620 | Resource leases: named queues, push wake-up | Todo |

## P617 Task list and detail UI (Stream A)

- Task detail shows worktree's current checked-out branch next to the assigned branch; mismatch is visible (agents sometimes switch branches). MCP gains a tool to change the branch, gated by an approval prompt.
- Link card gets a copy button beside open-external.
- "Needs input" indicator sits at the start of the task row, not mid-row. Agent status indicators bigger.
- Task row elements use fixed, identical column sizing across tasks (status first: width independent of its text) so rows scan at a glance.
- Task list "today" left bar: clearly marks today without a bright yellow clashing with the task color. Task color gets a presentation other than, or in addition to, a colored left border.
- Opening Sessions tab in task detail focuses the keyboard into the open agent terminal.

Result: Done. Backend: `BranchDetached.head`, worktree mismatch in `task_info` and `branch_status`, new approval-gated `change_branch` MCP tool (`TaskBoard.ChangeBranch`, switch item, approvals class "change"; adeflow `TestBranchChange`, contract `ade-branch-change`). UI: mismatch chip and panel checkout line, change request body, link card copy, card head columns, lead attention slot, 16px activity icons, today marker, task tint, terminal focus on Sessions open. Deviations: plan commits 1 and 2 merged (interface change breaks build alone); contracts hold branch rows, not boards; refusals use outcome `failed`; rename path has no `snapshotChangeRequests`, so change clears the row's stored change request directly; permanent U62 exemption for `terminalRenderer.ts`; check icon 16px and `min-h-5` forced by lint guards; Later shelf row follows the card grid (lead, status, flags at card offsets; spec checks it). Tests: Go vet and lint clean; agentmcp, llmprompts, ade, adeflow branch tests, llmpromptsflow pass; typecheck passes; 12 targeted ui specs pass after fixing plan, task-row and adopt specs for tinted cards and `head` fallback.

## P618 Dialogs: speed, colors, background ops, proof, copy (Stream B)

- Audit every MCP-triggered dialog (create branch, create task, handover, others): content renders instantly with the dialog, handover text included. No delayed content.
- Semantic button colors (approve, deny, destructive, confirm) consistent across the whole app via shared shadcn-vue variants.
- Delete branch and close task: dialog dismisses once confirmed; progress shows as status in relevant places (task row, branch row, toast) until done.
- Proof-ready dialog: closes after open or close. Title order: task title first, proof title second.
- Studio MCP dialogs: text selectable and copyable (API host and all other fields).
- MCP prompt dialogs (approval, query, request) use the full dialog space with a fixed layout: approve/deny stay at the same screen position whatever the query or request length (fixed footer, content fills the rest).
- Query/request text gets maximum room: larger dialog, content area flex-1, scroll only on real overflow.
- Exactly one prompt per dialog: others queue and the next shows once the current one is resolved. Root-cause the stacking; UI spec, plus flow test if backend-involved.

Result: _pending_

## P619 Settings MCP split and GitHub PR open (Stream C)

- Settings "database MCP" section is too long: split into several sections. Rename user-facing label to "Studio MCP".
- Opening a GitHub PR from the app does not work: reproduce, root-cause, fix.

Result: _pending_

## P620 Resource leases: named queues, push wake-up

Agents sharing an unsharable resource (port, full-suite run, push, merge) wait in order instead of colliding.

- Multiple named queues, each its own resource: one holder, FIFO-by-priority waiters. User names queues. Backend in Go, frontend state in a Pinia store.
- MCP tools: acquire, release, status. No blocking: acquire returns at once, granted or queued with position. Queue entry keyed by session id, so position survives agent re-calls.
- Wake-up: when a lease frees, app injects a short message into the next agent's session once that agent is idle (never types into a busy agent). Fallback: hook on the agent's next turn start or stop reports the grant.
- Release guarantees: all leases of a session release when the session ends (crash, kill, exit); TTL with heartbeat (any holder tool call extends); explicit release as courtesy.
- Priority: next holder is the waiter whose task sits highest in the task list; re-evaluated on each grant. Never preempts a current holder.
- UI: panel lists every queue with holder and waiters. Click holder: "Request release" (injects finish-up message) or separate "Force release" (revokes). Queue state shown on task rows.
- Verify first: how session end and idle detection behave on a hard kill; drop injection to hooks-only if idle cannot be detected safely.
- Tests: backend flow test and UI spec for same scenario (queue, grant, release, force release).

Result: _pending_
