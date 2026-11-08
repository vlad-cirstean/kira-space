# v0.3.4

Proof images and agent branch bases. Branch `chapter/v0.3.4`, tag `v0.3.4`. First `P` is P569 (highest prior: P568).

| Phase | Title | Status |
|---|---|---|
| P569 | Proof: JPEG screenshots, agent image steps, menu bar entry | Planned |
| P570 | Agents choose the base of a requested branch | Planned |
| P571 Part 1 | Orchestrator tasks, backend: passes, spawn, chain reads | Planned |
| P571 Part 2 | Orchestrator tasks, frontend: kind choice, card, Run pass | Planned |
| P572 | About: GitHub link, add an About settings section if none | Planned |
| P573 | Close chapter | Planned |

P569, P570 and P571 run as parallel streams (own worktree each): P569 owns proof, `dbmcp` proof tools and the proof window menu entry; P570 owns `request_branches` and the ADE base path; P571 owns the orchestrator task kind. Grouping in `plans/batch-P569-P570.md`.

Chapter ends the standard way after the last implementation phase: landing, one non-UI full run, 1 code review round, close phase P573, PR.

## P569 Proof: JPEG screenshots, agent image steps, menu bar entry

- Ask:
  - Proof window screenshot (copy, save active tab, save all tabs) encodes JPEG, not PNG, from now on.
  - `proof_save` gains an image step: the agent supplies a JPEG, as inline data or as a file path on disk. JPEG only.
  - Proof window opens from the menu bar, outside any task.
- Planner decides: step shape (`view` value, `data` fields), path validation (sandbox, size cap, JPEG signature check), how the window renders an image step, how the existing 32 MiB session cap and 8 MiB HTTP body cap apply, quality value for screenshots, clipboard handling (clipboard holds one image; PNG may stay there if JPEG copy is unsupported, planner verifies).
- Pointers: `apps/kira-space/internal/dbmcp/proof_tools.go`, `apps/kira-space/internal/proof/`, `apps/kira-space/internal/bridge/proof.go`, `proofshots.go`, `apps/kira-space/frontend/src/studio/proof/` (`screenshot.ts`, `useProofScreenshot.ts`), `docs/ARCHITECTURE.md` "Validation proof (P420)".
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).

## P570 Agents choose the base of a requested branch

- Ask: `request_branches` lets the agent give each branch a base. Today an entry is `{repo, branch}` and the base comes only from the user-set row (`SetBranchBase`, Change base).
- Planner decides: field name and shape (ref name vs task branch id, same options as `adewire.BaseChoice`), validation through the existing `resolveChoice` path (same repo, no cycles, `check-ref-format`, base not created yet), how the approval dialog shows each branch's base, behavior for an `existing` branch with a different base, prompt text and tool schema.
- Pointers: `internal/agentmcp/space.go`, `apps/kira-space/internal/ade/agenttools.go`, `branchrequest.go`, `base.go`, `setup.go`, `docs/ARCHITECTURE.md` "Task base and rebase runs (P241)" and "Branch requests".
- Tests: flow test plus UI spec on the same scenario (branch request dialog showing the base).

## P571 Orchestrator tasks

Split by the planner (plan `plans/P571.md`): Part 1 backend (migration, engine, tools, flow tests), Part 2 frontend (starts after Part 1 is committed; its UI spec reads Part 1's contract).

- Ask: new task kind `orchestrator`. It runs a normal workflow (script, agent and interactive steps), may edit code, and coordinates work across other tasks. Manual trigger only, no cron.
- Decided (user):
  - Only orchestrator tasks can spawn tasks, so depth is always 1; children get no spawn tool.
  - No parallel limit: every spawn waits for a user prompt.
  - One orchestrator, many passes. A Run action starts the next pass, with an optional message through the stage dialog.
  - Its own stage handovers are its reference to previous work (newest through the existing handover path, older ones through a read tool). No Memory module.
  - No wake of the orchestrator when a child finishes; a child's final handover lands in the parent's chain.
- Planner decides, from code: pass mechanics (a finished task moves back to the first step via `SetTaskStep`; `ResetWorkflow` deletes runs, so not it; verify `SinceJSON`, `syncStatus` and status slots handle it), migration for the parent link (next free number, P569 takes 0092: whichever lands second renumbers), `create_task` tool shape (title, workflow, repos, brief as child's first handover with source `orchestrator`, optional base through the existing `BaseChoice` and `resolveChoice`, not P570's new field), approval prompt kind, read tools (`list_tasks`, `task_chain`), board presentation (card, children grouped, parent links both ways), grant rules (which task kinds carry which tools), wire types and bindings.
- Streams: confirm no ordering dependency on P570 (P571 must not need P570's `request_branches` field). If one exists, stop and report. Size: if the phase is too large for one implementer, propose `Part 1`/`Part 2` per the numbering rule instead of cutting scope.
- Tests: flow test plus UI spec on the same scenario, split at the IPC boundary.

## P572 About: GitHub link

- Ask: add a link to the GitHub repo in the app's About section. If the app has no About section, add one to Settings.
- Planner decides: find an existing About surface (settings pane, app menu About, window) from code; where the repo URL comes from (one source, no new hard-coded copy); how external links open (`LinkService.OpenExternal`, never `window.open`); Settings pane wiring if new (shadcn-vue, Tailwind, `<script setup lang="ts">`); i18n; ARCHITECTURE update.
- Tests: flow test plus UI spec on the same scenario where it applies at the IPC boundary (opening the link goes through the bridge).

## P573 Close chapter

- Chapter close by hand per CLAUDE.md "Closing a chapter": one Sonnet implementer, no planner, no plan file.
