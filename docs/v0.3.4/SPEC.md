# v0.3.4

Proof images and agent branch bases. Branch `chapter/v0.3.4`, tag `v0.3.4`. First `P` is P569 (highest prior: P568).

| Phase | Title | Status |
|---|---|---|
| P569 | Proof: JPEG screenshots, agent image steps, menu bar entry | Planned |
| P570 | Agents choose the base of a requested branch | Planned |

P569 and P570 run as parallel streams (own worktree each): P569 owns proof, `dbmcp` proof tools and the proof window menu entry; P570 owns `request_branches` and the ADE base path. Grouping in `plans/batch-P569-P570.md`.

Chapter stays open after both phases: no close phase yet (user decision).

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
