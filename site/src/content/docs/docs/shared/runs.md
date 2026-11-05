---
title: Runs and smart scripts
description: Run scripts and Claude Code prompts from collections, on demand or on a schedule.
sources:
  - README.md
  - apps/kira-space/README.md
  - packages/workbench/src/automations
  - internal/scriptruns
  - internal/scripts
  - internal/secrets
  - internal/agentcfg/servers.go
  - packages/theme/src/i18n/en.json
sidebar:
  order: 1
---

Runs exist in both apps. A run is a YAML file. Scripts group in collections. Each script runs in its own folder and keeps a run history with status, exit code and result.

## Run files

A run lives in `<id>.yaml` in the `runs` folder of the app home: `~/.kira-studio/runs` in Kira Studio, `~/.kira-space/runs` in Kira Space. The file name without `.yaml` is the run id. Use lowercase letters, digits, `-` and `_`.

- **New script** and **New smart script** write a small valid file and open it.
- Double-click a row, or choose **Edit file**, to edit it as a form or as YAML. Both tabs edit the same text; **Save** writes the file. A form edit keeps your comments and key order. **Open** opens the file in your editor. **Reveal** and **Copy path** work as in other config files.
- A change in your editor shows in the panel after a moment. Kira checks the file on every change.
- A file with an error shows the line and the message. A run that was valid before keeps running its last good version, and its row shows a warning mark.
- A file that was never valid is listed as a broken row. Open it to fix it.
- A file with a name that is not a valid id is listed and never used.
- **About these files** opens `AGENTS.md` beside the files. It lists every key and rule.
- Deleting a run through the row menu removes its file and its saved secret values. Deleting the file in an editor drops the run but leaves its folder under `automations`.

Keys include `name`, `kind`, `command` (or `prompt` for a smart run), `params`, `schedule`, `dir`, `path`, `color` and `collection`. A `collection` is a name. A run with no `collection` is ungrouped. The run folder is the Kira scripts folder under the app home (`dir: kira`) or a folder you name (`dir: fixed` with `path`).

## Scripts

- Add parameters. A parameter reaches the command as a variable.
- Open terminals beside the run history.
- Start a script with its play button. Click a row to select it; double-click to edit its file.
- Running a script again reuses its tab. While it runs, the play button shows the running tab.
- Drag scripts to reorder them inside their collection. Drag a collection header to reorder collections. **Move up** and **Move down** in the row menu, or Alt+Up and Alt+Down, do the same. A search pauses reordering.
- Drag a run into another collection, or use **Move to collection** and **Move to new collection** in the row menu. Both set `collection` in its file.
- A collection header menu has **Rename** and **Delete**. Rename edits every run file in it and refuses a name another collection uses. Delete removes every run in it, files and saved secret values included.

## Secret parameters

A secret parameter can keep a saved value. The file lists only the parameter name. Set the value in the **Secret values** section under the file text. The saved value is write-only: its field reads "Saved, type to replace" and shows **Value saved**.

- The run dialog reads "Saved, type to use another value". A value you type wins.
- A secret reaches the run as `KIRA_PARAM_<NAME>` only. It never enters the prompt.
- The stored log, the run outcome and the terminal tail show a mask. The live terminal tab and the phone view do not.
- Resume uses the saved value.
- Deleting a script or parameter deletes the value. So does renaming it or turning off its secret flag. Duplicate and export never copy it.
- Kira Space and Kira Studio seal values with one key held in the keychain.
- When the keychain is unavailable, a secret can't be saved and each run asks for it.

## Smart scripts

A smart script runs a prompt in headless Claude Code instead of a shell command. Create one with **New smart script**. The agent in Settings runs it. OpenCode can't cap cost, so it refuses smart scripts.

A smart script sets:

- Parameters: text, select or multi-select, each optionally secret. Use a token in the prompt. A secret reaches the run as a variable only, never in the prompt.
- A model, a budget in dollars and a timeout. The budget must be above 0.
- A list of tools and MCP servers the script may use. Bash commands take patterns such as `git status:*`.

Name the servers a smart script may use under `smart.mcp` in its file: a server with all its tools, or a list of tool names. What Kira allows depends on **Hermetic agent launches**, on by default.

- Hermetic on: only the MCP servers in Kira's agent settings are allowed. Secrets in them resolve at launch and are masked in the log. A named server missing from the settings blocks the run.
- Hermetic off: the servers `claude mcp list` reports for your user config are allowed. A run that uses one of your servers loads your Claude settings file, so your permission rules, environment and plugins apply. Hooks stay off, and only the servers you named load.

See [Hermetic launches](../../space/agents/#hermetic-launches).

The run history lists denied tools.

Kira adds a closing instruction to the prompt so the run reports through `finish_step`. See [Prompts](../../reference/prompts/).

Kira Studio smart scripts use the same agent settings and context files, from `~/.kira-space/agent/`. They always launch hermetic.

## Schedules

A script can run on a cron schedule. Add a `schedule` to its file with a cron expression (minute, hour, day, month, weekday) and a timezone.

- Set `confirm: true` to get a popup before each due run.
- Schedule parameters never hold a secret.
- Runs happen only while the app is open. A missed run is skipped, never replayed.
- A run that is still going, or still waiting for your answer, makes the next run skip.

## In Kira Space

Kira Space adds two more triggers:

- Run a script on a task or branch from Agents. `use_ade_dir: true` in the file picks the task's worktree as run folder.
- A scheduled run in Kira Space can target a task and branch. Choose **Set target…** in the row menu.
- Use a script as a workflow step.

Cmd-click (macOS) or Ctrl-click a link in a terminal to open it in your browser.
