# v0.3.15

Task-list ergonomics, prompt/dialog fixes, status bar, YAML wording cleanup. Phases `P646` to `P655`.

User gates: P646 ships a design mock only; implementation of the task-list actions waits for user approval of it. User chose task list first: P647 then P648 run sequentially in one stream (same component), before the rest.

| # | Phase | Status |
|---|-------|--------|
| P646 | Task-list actions design mock | Done |
| P647 | Task-list polish | Done |
| P648 | Task-list actions per approved design | Done |
| P649 | Prompt dialogs: empty body, 10 min limits, typing-safe, proof-update notification | Done |
| P650 | Status bar CPU/RAM includes agents | Done |
| P651 | Drop YAML path and edit-config wording app-wide | Done |
| P652 | Task-list finish: fixes, stale UI specs, real-data check | Done |
| P653 | Minor-stepped updates and release retention | Todo |
| P654 | Fix real-claude ade_smart_step failure | Done |
| P655 | Close chapter | Todo |

## P646 Task-list actions design mock

Task rows handle more of what the main task tab does, without opening it (example: edit branches). List stays narrow, fits a MacBook Air screen. Survey every capability in the main task tab, propose which move to the list, deliver a static HTML mock the user opens. No app code changed. Gate: user approval.

Result: plan `plans/P646.md`, mock `design/P646-task-list-mock.html`. User asked to judge it in the real app, so P648 implements it there.

## P647 Task-list polish

- Drop the Today group background (contrast between tasks).
- Drag and drop: less finicky, snap placeholder shown while moving.
- Drop repos header; keep only refresh-all, placed efficiently.
- Task title never underlined.
- Task detail: drop second line under title (today, branch count, repo).

Result: plan `plans/P647.md`, UI only, typecheck, lint and build green, no tests run (user). Today tint removed; plan drag resolves slot by card geometry with a dashed snap slot; repos header replaced by a refresh-all icon beside History (repo filter removed); card title plain span; task-panel facts line removed. Existing UI specs `ade-v2-plan`, `ade-v2-repo-updating`, `ade-v2-panel` and the day-bars visual baseline are stale and stay unedited; PR CI will fail on them until fixed.

## P648 Task-list actions

Implement approved P646 design.

Result: plans `plans/P648.md`, `plans/P648-iter2.md`, UI only, typecheck, lint, lint:dead and build green, no tests run (user). Row actions per P646 R-list; list 400px default, 360 floor; branch rows restored base, context, progress, merged/deployed on a second line; branch form aligns under the pencil; cluster shows on hover or keyboard focus only; compact offset drag ghost. Known: `fix/cart-flaky` line 2 clips about 8px at 400px; drag ghost still covers the last row of the target card. Stale UI specs from P647 plus plan specs reading old branch-row elements and plan visual baselines stay unedited; PR CI will fail on them until fixed.

## P649 Prompt dialogs

- Approve dialogs show nothing for explain runs, create task and other prompts (body and approve buttons empty). Likely explain-of-explain. Find root cause, fix.
- Every wait limit is 10 min. Create-task limit is still 30 min.
- Prompt-dialog keyboard shortcuts need 2 keys, so typing never approves or denies.
- User typing: do not open the dialog, show only the toast. Open after a quiet period of a few seconds (pick a value that lets a sentence finish).
- Proof-update "forward to agent" notification has no clear purpose. Show a prompt like other prompts, or nothing.

Result: plan `plans/P649.md`. Blank approve dialogs fixed at the source (create-task `base` omitempty, explain `Mode` empty) plus an unreadable-request guard; one `prompts.AnswerWait` 10 min wait for agent prompts; two-key chords (Mod+Enter approve, Mod+Backspace deny, Shift+Escape hide) with prompt focus mode; typing gate (4 s) defers dialogs to a toast; proof forward notification removed. Typecheck, lint, lint:dead, build, Go flow tests and 209 UI specs touching prompt dialogs green; full UI suite not run.

## P650 Status bar CPU/RAM

Widget counts agents and their subprocesses. Tooltip splits App and Agents.

Result: plan `plans/P650.md`. `internal/agentproc` tracks agent roots at the four spawn sites; metrics split App and Agents by process tree (gopsutil), tooltip table App/Agents/Total. Go, typecheck, lint, build green; UI specs updated, run in PR CI. darwin process code unverified on Linux. Real-claude `ade_smart_step` fails on base too (P654).

## P651 YAML wording

Remove file paths and "open file" affordances pointing to YAML files. Remove "edit the config file" copy (editing a connection, etc.). App-wide, not only the examples.

Result: plan `plans/P651.md` plus amendment. `ConfigFileBar` removed from 9 screens; invalid file shows an alert "Can't be read, fix it here" with a Show in YAML link, project-tree and script row markers; `configsvc` and task folder open/reveal removed; about 75 texts reworded; en.json sweep clean. Checks green; adeflow timed out once under load and passed on rerun.

## P652 Task-list finish

Owed after P647 and P648 (user: do it after the parallel streams, do not forget):

- Fix `fix/cart-flaky` branch row line 2 clipping about 8px at 400px.
- Update every UI spec and visual baseline made stale by P647 and P648 (`tests/ui/ade-v2-plan.spec.ts`, `ade-v2-repo-updating.spec.ts`, `ade-v2-panel.spec.ts`, specs reading `ade-branch-context`, `ade-base-marker`, `ade-branch-prog`, `ade-branch-merged`, plan screenshots). Run them.
- Run the real app on real data and click through each row action (branch edit, add repo or branch, estimate, step, status, workflow, rename, updates, branch request, drag and drop). Fix what fails.

Result: plan `plans/P652.md`. Fixes: `fix/cart-flaky` line 2 keeps every target chip at 400px (`AdeBranchProg` yields, chips never shrink); drag ghost sits below-right of the pointer; Refresh all shows repos that are updating (P622 state lost with the P647 chips); review menu anchor keeps its hover cluster while the menu is open. Card meta truncation accepted (iter2 design). UI specs updated to the P647 to P651 design (hover reveal helpers, scoped branch menu, repo header and today tint removed, geometric drop targets, one-line titles, P649 chord wording); new `ade-v2-row-actions.spec.ts` holds the frontend halves of the row actions, each naming its flow test. Runs at the last spec commit: `ui` plus `ui-clipboard` 2257 passed, 1 skipped (`native-copy` non-Chromium guard, unchanged); `screenshots` 5 passed; `visual` 12 baselines re-recorded after opening every diff (causes in the commit body: P647/P648 rows and bands, wider default panel, P651 wording, P649 chord hints), 2 visual specs fixed for P648 tip trigger and P649 Shift+Escape. `visual` `proof-window:52` (26 px, untouched code) failed in 2 of 3 full runs and 1 of 13 isolated runs (26 px each time it failed); not root-caused. Real-data click-through on the `e2e-real` server (real SQLite, git, worktrees, fake agent), 1280x800, screenshots in the session scratchpad `p652/`: passed A1 (list 400px, heads 44px, no clipped line 2, no visible cluster), A2, B1 (worktree `fix/a` exists on disk), B2, C1, C2, D1 and D2 (slot between cards, ghost below-right, target card's last row visible), D3, E1, E2, F1 (moved to Build), G1, H1, I1, I2, L1 (no toast, no `data-updating` since no repo lagged). Not exercised: K1 (branch request needs MCP agent wiring; covered by the UI spec only), M1 (no merged or stale seed; covered by the `ade-v2-task-row` line 2 spec), J1 (a plain remote push produced no updates button after Refresh all; watcher scope not investigated, popover and Mark seen covered only by the mocked spec), L1's updating state. Open finding: in the real run the first click on the empty estimate clock right after two drag-and-drops fired no `click` event (pointerdown and pointerup arrived on the icon); the second click opened the popover and the same click before any drag worked. Likely Chromium swallowing the click after an emulated native drag; unconfirmed and not fixed. Not verifiable here: macOS WKWebView, trackpad drag, macOS fonts, the user's own repos, real GitHub PRs and checks (fake `gh`), real `claude` runs, deployment chips.

## P653 Minor-stepped updates and release retention

Goal: apply minor versions one at a time, so legacy migration and backward-compat code can be pruned per minor. The updater is the main part; retention supports it. Add the "Minor bumps drop legacy code" rule to `CLAUDE.md` (done at chapter open).

- Updater offers the next step, never a jump: an install on `X.Y.z` is offered the last patch of `X.Y` first, then `X.(Y+1).0`, then the last patch of `X.(Y+1)`, and so on. The last patch holds all of a minor's migrations. Clean install gets the latest release. Today it reads `releases/latest` (`docs/PACKAGING.md` §7), so it needs a release listing and a stepping rule.
- Retention in a GitHub Action (separate workflow, runs after `release.yml` succeeds and on manual dispatch, `contents: write` on that job only): keep assets of every `vX.Y.0` release, the last patch of every minor (the update path needs it), and the last 5 patches of the current minor. Delete only release assets, never tags or release pages. Dry-run mode first. Never touch `releases/latest`. Updater skips a release with no assets.
- Update `docs/PACKAGING.md` §7 and the architecture doc.

## P654 Real-claude ade_smart_step

`TestRealClaudeSettingsUntouched/ade_smart_step` fails ("smart step run = failed, want done"), also on base commit 07246566, found in P650. Root-cause and fix.

Result: plan `plans/P654.md`. Root cause: P643 refused an ok `step.finish` with no draft while the route continues, but a smart step's script tools (`Read`) cannot write one, and its prompt asked for it. Fix: `smartStepOf`; a smart step run hands over by the ok `reason`, gets no draft clause, and is not refused (also on resume); sessions unchanged. New flow subtest `TestAutomationStep/hands over its reason, no draft` (fails without fix, passes with), fakeagent records `.draft-asked`. Real-claude test command now names `state` `step.finish`. Real claude: `TestRealClaudeSettingsUntouched` 5 of 5 pass (0.0018 USD), ADE runs row all pass (0.0929 USD). Go build, `ade`, `agentmcp`, whole `adeflow` package, `lint:claude` green.

## P655 Close chapter

Per `CLAUDE.md` "Closing a chapter".
