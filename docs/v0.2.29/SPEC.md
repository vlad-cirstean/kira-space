# v0.2.29

Branch `chapter/v0.2.29`. First `P` is 484 (highest in v0.2.28 is P483).

Source: user request. Phases run top to bottom, one at a time. Scope is closed: further phases go to v0.2.30. Ends with one code review round, then close, then PR.

| Phase | Title | Status |
|---|---|---|
| P484 | Kubernetes: ops panel tracks every action | Done |
| P485 | Kubernetes: reorder clusters | Done |
| P486 | Kubernetes: resource browser redesign, tabbed pods | Todo |
| P487 | Kubernetes: drive the kubectl CLI instead of the native client | Todo |
| P488 | Install instructions: trust the Jira tap | Done |
| P489 | Terminals: Cmd-click opens file paths in Kira Space | Done |
| P490 | Kira Space: file open window shows only ad hoc file tabs | Todo |
| P491 | ADE: hide archived tasks unless History is open | Done |
| P492 | ADE: agents widget shows active-agents indicator | Done |
| P493 | Code review (1 round) | Todo |
| P494 | Close chapter v0.2.29 | Todo |

## Rows

### P484 Kubernetes: ops panel tracks every action

- Kubernetes module records all its actions in the ops panel, as other modules do. Plan lists every mutating and long-running action (apply, delete, scale, restart, exec, port-forward, logs) and the op log shape.
- Test: backend flow and UI spec on the same scenario.
- Result: `kubernetes.Service` records every cluster call (`Status`, `Namespaces`, `Kinds`, `List`, `Describe`, `ReadLogs`, `LogsOpen`) through one `Recorder` seam, op kind `kubernetes`. `bridge.KubeRecorder` runs it over `Host.RunOp`. Command text moved to `kubernetes/commands.go`, shared with the MCP approval dialog. dbmcp dropped `Config.Ops`/`OpRunner`; MCP rows stay identical. A log stream's row spans the stream; panel Cancel ends it. Refetches of loaded data pass `background: true` and skip the log. apply/delete/scale/restart/exec/port-forward do not exist (module is read-only), so nothing to record. The running row shows no command (op:start carries none); the plan expected one. Tests: `kubeflow` `TestOpsRecordEveryAction` with contract `kubernetes.json`, `dbmcpflow` single-row guard, UI spec `kubernetes-ops.spec.ts` (3 tests).

### P485 Kubernetes: reorder clusters

- Clusters can be ordered by the user (drag or move up and down; plan picks). Order persists.
- Test: UI spec; backend flow for persistence.
- Result: pointer drag on cluster rows (`useClusterReorder` over shared `useTreePointerDrag`; `orderAfterDrop`/`EdgeDrop` moved there from `connectionReorder.ts`), with Alt+Up/Down and menu Move up/down as the keyboard path; disabled while searching. Order in `kube_clusters.sort_order` (migration 0057, NULL last). `Service.Reorder` through `sqlitex.ReorderSortOrder`, no op row. `kira:kubernetes:clusters` makes other windows refetch after `Reorder` and `SetMcp`. Tests: `kubeflow` `TestClusterOrder` (contract `kubernetes.json`: `args:KubernetesService.Reorder`, `KubernetesService.Reorder`), UI spec `kubernetes-order.spec.ts` (6 tests).

### P486 Kubernetes: resource browser redesign, tabbed pods

- Today: long lists and every resource type expanded. Usual use is Pods. Redesign: pods first, other types on demand, compact navigation.
- Open a pod in a tab. Multiple pod tabs.
- Overall presentation is poor. Plan proposes a new layout and may split into Parts.
- Test: UI spec.

### P487 Kubernetes: drive the kubectl CLI instead of the native client

- Kubernetes module shells out to `kubectl` for all cluster operations instead of the in-process native client (client-go or equivalent).
- Plan first inventories current native-client usage: cluster list, resources, logs, exec, port-forward, apply/delete/scale/restart. Maps each to kubectl commands: prefer `-o json`, `--context`, streaming for logs/exec/port-forward.
- Missing `kubectl` binary: clear error.
- Never read kubeconfig credentials/tokens. CLI credential rule, ARCHITECTURE "CLI credentials: never touched". Reading non-secret config such as context names via kubectl is fine.
- Interacts with P484 (ops panel records the kubectl invocations) and P486 (redesign consumes same data). Plan states ordering consequences.
- Test: backend flow with fake `kubectl` on PATH; UI spec on same scenario.

### P488 Install instructions: trust the Jira tap

- Jira brew install requires trusting the tap. Update the instructions. Check the other CLI install instructions for the same requirement and update them.
- Test: none (docs and in-app text); a UI spec only if in-app text changes.
- Result: Jira install step now `brew tap atlassian/homebrew-acli && brew trust --formula atlassian/acli/acli && brew install acli` (Homebrew 6 tap trust, per-formula). Other CLI steps (gh, azure-cli, git, grep) come from homebrew/core; no change. Contract `space-cli-tools.json` regenerated; UI spec asserts new command and trust. ARCHITECTURE CLISetup bullet notes the rule.

### P489 Terminals: Cmd-click opens file paths in Kira Space

- Cmd-click already opens links. Make file paths clickable too and open them in Kira Space. Plan covers `path:line:col`, relative paths against the terminal cwd, nonexistent paths.
- Test: UI spec.
- Result: Kira Space desktop terminals link file paths (`path`, `path:line`, `path:line:col`); Cmd/Ctrl-click opens the file in the opened-files window with the cursor placed. Relative paths resolve against the live cwd (foreground process group, then shell, then start cwd; `fallbackCwd` once exited). Only existing, supported files link. Backend: `Registry.Cwd`, `openedfiles.TerminalPath/Openable/OpenAt/deliver`, `OpenedFilesService.TerminalResolve/TerminalOpen`, `Target.line/col`. Frontend: `filePaths.ts`, `fileLinkProvider.ts`, `terminalFileLinks.ts`, `openTarget`, `applyMountReveal`. Studio, Docker exec and phone terminals unchanged. A bare word with no slash or extension (`src`) is never a candidate. Tests: `TestTerminalFileLinks`, `terminal-file-links` UI spec, `terminal-file-paths` unit spec.

### P490 Kira Space: file open window shows only ad hoc file tabs

- Opening a file in Kira Space opens a new window. That window shows no repos or sidebar: a clean window with tabs for the ad hoc opened files only.
- Test: UI spec.

### P491 ADE: hide archived tasks unless History is open

- Archived tasks stay hidden, including ones archived today, until the user opens History.
- Test: UI spec.
- Result: `buildBand` (`timeline.ts`) shows history rows only while History is open; day 0 no longer exempt. `onFindArchived` opens History for every archived pick, today's included. Frontend only: `board.history` payload unchanged, so no backend test. UI specs: `ade-v2-plan` today-archive test rewritten (hidden, count on button, shows on open, hides on close); `ade-v2-archived` gains finder-picks-today-task test. ARCHITECTURE archived-tasks bullet updated.

### P492 ADE: agents widget shows active-agents indicator

- The agents widget shows a distinct indicator when agents are running (plan picks: pulse dot, count badge).
- Test: UI spec.
- Result: "running" read as "mid-turn (working)": `AdeAgentsStatusItem` shows a pulsing `StatusDot` (tone ok, `N working` label, testid `status-agents-working`) while `summary.byKind.working > 0`. Count badge declined: label already prints the count. Reduced motion static via global CSS. Frontend only, no backend test. UI spec `status-agents` extended (dot, TUI follow, reduced motion); status-bar visual baseline re-recorded (tooltip, popover unchanged). ARCHITECTURE status bar bullet updated.

### P493 Code review (1 round)

One Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR.

### P494 Close chapter v0.2.29

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
