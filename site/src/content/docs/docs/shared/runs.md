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

Runs exist in both apps. Scripts group in collections. Each script runs in its own folder and keeps a run history with status, exit code and result.

## Scripts

- Create a script with **New script**.
- Choose the run folder: the Kira scripts folder under the app home (**Use Kira scripts folder**), or **Choose folder…**.
- Add parameters. A parameter reaches the command as a variable.
- Open terminals beside the run history.
- Start a script with its play button. Click a row to select it; double-click to edit.
- Running a script again reuses its tab. While it runs, the play button shows the running tab.
- Drag scripts to reorder them, or drop one into another collection or out of one. Drag a collection header to reorder collections. **Move up** and **Move down** in the row menu, or Alt+Up and Alt+Down, do the same. A search pauses reordering.

## Secret parameters

A secret parameter can keep a saved value. The saved value is write-only: its field reads "Saved, type to replace" and shows **Value saved**.

- The run dialog reads "Saved, type to use another value". A value you type wins.
- A secret reaches the run as `KIRA_PARAM_<NAME>` only. It never enters the prompt.
- The stored log, the run outcome and the terminal tail show a mask. The live terminal tab and the phone view do not.
- Resume uses the saved value.
- Deleting a script, collection or parameter deletes the value. So does renaming it or turning off its secret flag. Duplicate and export never copy it.
- Kira Space and Kira Studio seal values with one key held in the keychain.
- When the keychain is unavailable, a secret can't be saved and each run asks for it.

## Smart scripts

A smart script runs a prompt in headless Claude Code instead of a shell command. Create one with **New smart script**. The agent in Settings runs it. OpenCode can't cap cost, so it refuses smart scripts.

A smart script sets:

- Parameters: text, select or multi-select, each optionally secret. Use a token in the prompt. A secret reaches the run as a variable only, never in the prompt.
- A model, a budget in dollars and a timeout. The budget must be above 0.
- A list of tools and MCP servers the script may use. Bash commands take patterns such as `git status:*`.

Tick a server in the MCP list to allow all its tools, or untick **All tools** and type tool names. What the list shows depends on **Hermetic agent launches**, on by default.

- Hermetic on: the list shows the MCP servers in Kira's agent settings, as "Servers from Kira's own agent settings, not your CLI config." Secrets in them resolve at launch and are masked in the log. A ticked server missing from the settings blocks the run.
- Hermetic off: the list shows the servers `claude mcp list` reports for your user config, with Claude's own health status. Opening the list starts each server briefly to check it. A run that uses one of your servers loads your Claude settings file, so your permission rules, environment and plugins apply. Hooks stay off, and only the servers you ticked load.

See [Hermetic launches](../../space/agents/#hermetic-launches).

The run history lists denied tools.

Kira adds a closing instruction to the prompt so the run reports through `finish_step`. See [Prompts](../../reference/prompts/).

Kira Studio smart scripts use the same agent settings and context files, from `~/.kira-space/agent/`. They always launch hermetic.

## Schedules

A script can run on a cron schedule. Choose **Run on a schedule**, then set a repeat or a cron expression (minute, hour, day, month, weekday) and a timezone.

- A due run asks for confirmation by default. Choose **Run without asking** to skip the popup.
- **Run without asking** needs a saved value for every required secret parameter. Without one, each run asks.
- Schedule parameters never hold a secret.
- Runs happen only while the app is open. A missed run is skipped, never replayed.
- A run that is still going, or still waiting for your answer, makes the next run skip.

## In Kira Space

Kira Space adds two more triggers:

- Run a script on a task or branch from Agents. **Run in the task's worktree when started from ADE** picks the worktree as run folder.
- Use a script as a workflow step.

Cmd-click (macOS) or Ctrl-click a link in a terminal to open it in your browser.
