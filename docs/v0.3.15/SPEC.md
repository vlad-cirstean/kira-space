# v0.3.15

Task-list ergonomics, prompt/dialog fixes, status bar, YAML wording cleanup. Phases `P646` to `P653`.

User gates: P646 ships a design mock only; implementation of the task-list actions waits for user approval of it. After approval, P647 to P649 run as 3 parallel streams.

| # | Phase | Status |
|---|-------|--------|
| P646 | Task-list actions design mock | Todo |
| P647 | Prompt dialogs: empty body, 10 min limits, typing-safe | Todo |
| P648 | Status bar CPU/RAM includes agents | Todo |
| P649 | Drop YAML path and edit-config wording app-wide | Todo |
| P650 | Proof-update notification | Todo |
| P651 | Task-list polish | Todo |
| P652 | Task-list actions per approved design | Todo |
| P653 | Close chapter | Todo |

## P646 Task-list actions design mock

Task rows handle more of what the main task tab does, without opening it (example: edit branches). List stays narrow, fits a MacBook Air screen. Survey every capability in the main task tab, propose which move to the list, deliver a static HTML mock the user opens. No app code changed. Gate: user approval.

## P647 Prompt dialogs

- Approve dialogs show nothing for explain runs, create task and other prompts (body and approve buttons empty). Likely explain-of-explain. Find root cause, fix.
- Every wait limit is 10 min. Create-task limit is still 30 min.
- Prompt-dialog keyboard shortcuts need 2 keys, so typing never approves or denies.
- User typing: do not open the dialog, show only the toast. Open after a quiet period of a few seconds (pick a value that lets a sentence finish).

## P648 Status bar CPU/RAM

Widget counts agents and their subprocesses. Tooltip splits App and Agents.

## P649 YAML wording

Remove file paths and "open file" affordances pointing to YAML files. Remove "edit the config file" copy (editing a connection, etc.). App-wide, not only the examples.

## P650 Proof-update notification

"Forward to agent" notification has no clear purpose. Show a prompt like other prompts, or nothing.

## P651 Task-list polish

- Drop the Today group background (contrast between tasks).
- Drag and drop: less finicky, snap placeholder shown while moving.
- Drop repos header; keep only refresh-all, placed efficiently.
- Task title never underlined.
- Task detail: drop second line under title (today, branch count, repo).

## P652 Task-list actions

Implement approved P646 design.

## P653 Close chapter

Per `CLAUDE.md` "Closing a chapter".
