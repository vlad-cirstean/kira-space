---
title: Prompts
description: View and edit the prompts Kira Space send to Claude.
sources:
  - internal/llmprompts/entry.go
  - internal/llmprompts/registry.go
  - packages/workbench/src/llmPrompts
  - docs/ARCHITECTURE.md
  - packages/theme/src/i18n/en.json
sidebar:
  order: 2
---

`Settings > Prompts` lists every prompt and instruction the app sends to Claude. Each app lists its own.

## Areas

- Memory (Kira Space).
- Agent runs.
- Rebase and review (Kira Space).
- Database MCP (Studio modules).

Text you write yourself stays outside this list. That covers smart script bodies and workflow step prompts.

## Kinds

| Kind | What it is | Example use |
|---|---|---|
| System | A system prompt with a fixed reply schema | Memory checks |
| Message | A message sent to an agent, with variables filled in | Rebase prompt |
| Suffix | A closing instruction added to a prompt | `finish_step` report |
| Instructions | Server instructions an MCP client reads on connect | Database MCP |
| Tool | The description an agent reads for an MCP tool | `run_query` |
| Tool result | Text a tool returns to steer the next step | Challenged store |

An MCP tool argument has its own entry under its tool. A fragment is a shared paragraph that other prompts include.

## Edit and reset

1. Select a prompt.
2. Edit the text. The **Preview** tab shows variables filled in.
3. Save the dialog.

Choose **Reset to default** to drop your edit. Saving the default text also removes the edit.

Some prompts must include shared paragraphs. Click one in the editor to insert it. A prompt with an output schema keeps that schema fixed.

## Badges

- **Edited**: your text replaces the default.
- **Default changed**: the default changed after you saved the edit, for example when an app update shortens it. Review the diff.
- **Invalid**: the saved edit fails validation, so the default is in use.

## When an edit applies

- Most prompts apply on the next call.
- Memory MCP instructions, tool descriptions and argument descriptions apply to new Claude sessions. Running sessions keep the old text.

Edits live in `llm-prompts.json` in each app's home.
