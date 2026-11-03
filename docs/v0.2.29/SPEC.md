# v0.2.29

Branch `chapter/v0.2.29`. First `P` is 484 (highest in v0.2.28 is P483). Draft: opens after v0.2.28 merges.

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.30. Ends with one code review round, then close, then PR.

| Phase | Title | Status |
|---|---|---|
| P484 | Kubernetes: ops panel tracks every action | Todo |
| P485 | Kubernetes: reorder clusters | Todo |
| P486 | Kubernetes: resource browser redesign, tabbed pods | Todo |
| P487 | Install instructions: trust the Jira tap | Todo |
| P488 | Terminals: Cmd-click opens file paths in Kira Space | Todo |
| P489 | Kira Space: file open window shows only ad hoc file tabs | Todo |
| P490 | ADE: hide archived tasks unless History is open | Todo |
| P491 | ADE: agents widget shows active-agents indicator | Todo |
| P492 | Code review (1 round) | Todo |
| P493 | Close chapter v0.2.29 | Todo |

## Rows

### P484 Kubernetes: ops panel tracks every action

- Kubernetes module records all its actions in the ops panel, as other modules do. Plan lists every mutating and long-running action (apply, delete, scale, restart, exec, port-forward, logs) and the op log shape.
- Test: backend flow and UI spec on the same scenario.

### P485 Kubernetes: reorder clusters

- Clusters can be ordered by the user (drag or move up and down; plan picks). Order persists.
- Test: UI spec; backend flow for persistence.

### P486 Kubernetes: resource browser redesign, tabbed pods

- Today: long lists and every resource type expanded. Usual use is Pods. Redesign: pods first, other types on demand, compact navigation.
- Open a pod in a tab. Multiple pod tabs.
- Overall presentation is poor. Plan proposes a new layout and may split into Parts.
- Test: UI spec.

### P487 Install instructions: trust the Jira tap

- Jira brew install requires trusting the tap. Update the instructions. Check the other CLI install instructions for the same requirement and update them.
- Test: none (docs and in-app text); a UI spec only if in-app text changes.

### P488 Terminals: Cmd-click opens file paths in Kira Space

- Cmd-click already opens links. Make file paths clickable too and open them in Kira Space. Plan covers `path:line:col`, relative paths against the terminal cwd, nonexistent paths.
- Test: UI spec.

### P489 Kira Space: file open window shows only ad hoc file tabs

- Opening a file in Kira Space opens a new window. That window shows no repos or sidebar: a clean window with tabs for the ad hoc opened files only.
- Test: UI spec.

### P490 ADE: hide archived tasks unless History is open

- Archived tasks stay hidden, including ones archived today, until the user opens History.
- Test: UI spec.

### P491 ADE: agents widget shows active-agents indicator

- The agents widget shows a distinct indicator when agents are running (plan picks: pulse dot, count badge).
- Test: UI spec.

### P492 Code review (1 round)

One Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR.

### P493 Close chapter v0.2.29

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
