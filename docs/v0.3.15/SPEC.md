# v0.3.15

Task-list ergonomics, prompt/dialog fixes, status bar, YAML wording cleanup. Phases `P646` to `P653`.

User gates: P646 ships a design mock only; implementation of the task-list actions waits for user approval of it. After approval, P647 to P649 run as 3 parallel streams.

| # | Phase | Status |
|---|-------|--------|
| P646 | Task-list actions design mock | Todo |
| P647 | Prompt dialogs: empty body, 10 min limits, typing-safe, proof-update notification | Todo |
| P648 | Status bar CPU/RAM includes agents | Todo |
| P649 | Drop YAML path and edit-config wording app-wide | Todo |
| P650 | Task-list polish | Todo |
| P651 | Task-list actions per approved design | Todo |
| P652 | Minor-stepped updates and release retention | Todo |
| P653 | Close chapter | Todo |

## P646 Task-list actions design mock

Task rows handle more of what the main task tab does, without opening it (example: edit branches). List stays narrow, fits a MacBook Air screen. Survey every capability in the main task tab, propose which move to the list, deliver a static HTML mock the user opens. No app code changed. Gate: user approval.

## P647 Prompt dialogs

- Approve dialogs show nothing for explain runs, create task and other prompts (body and approve buttons empty). Likely explain-of-explain. Find root cause, fix.
- Every wait limit is 10 min. Create-task limit is still 30 min.
- Prompt-dialog keyboard shortcuts need 2 keys, so typing never approves or denies.
- User typing: do not open the dialog, show only the toast. Open after a quiet period of a few seconds (pick a value that lets a sentence finish).
- Proof-update "forward to agent" notification has no clear purpose. Show a prompt like other prompts, or nothing.

## P648 Status bar CPU/RAM

Widget counts agents and their subprocesses. Tooltip splits App and Agents.

## P649 YAML wording

Remove file paths and "open file" affordances pointing to YAML files. Remove "edit the config file" copy (editing a connection, etc.). App-wide, not only the examples.

## P650 Task-list polish

- Drop the Today group background (contrast between tasks).
- Drag and drop: less finicky, snap placeholder shown while moving.
- Drop repos header; keep only refresh-all, placed efficiently.
- Task title never underlined.
- Task detail: drop second line under title (today, branch count, repo).

## P651 Task-list actions

Implement approved P646 design.

## P652 Minor-stepped updates and release retention

Goal: apply minor versions one at a time, so legacy migration and backward-compat code can be pruned per minor. The updater is the main part; retention supports it. Add the "Minor bumps drop legacy code" rule to `CLAUDE.md` (done at chapter open).

- Updater offers the next step, never a jump: an install on `X.Y.z` is offered the last patch of `X.Y` first, then `X.(Y+1).0`, then the last patch of `X.(Y+1)`, and so on. The last patch holds all of a minor's migrations. Clean install gets the latest release. Today it reads `releases/latest` (`docs/PACKAGING.md` §7), so it needs a release listing and a stepping rule.
- Retention in a GitHub Action (separate workflow, runs after `release.yml` succeeds and on manual dispatch, `contents: write` on that job only): keep assets of every `vX.Y.0` release, the last patch of every minor (the update path needs it), and the last 5 patches of the current minor. Delete only release assets, never tags or release pages. Dry-run mode first. Never touch `releases/latest`. Updater skips a release with no assets.
- Update `docs/PACKAGING.md` §7 and the architecture doc.

## P653 Close chapter

Per `CLAUDE.md` "Closing a chapter".
