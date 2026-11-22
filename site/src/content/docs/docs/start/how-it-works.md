---
title: How it works
description: One app, its modules, where your data lives and what runs on your machine.
sources:
  - README.md
  - apps/kira-space/README.md
  - docs/ARCHITECTURE.md
  - internal/kirapaths
  - apps/kira-space/internal/dbmcp/server.go
sidebar:
  order: 3
---

Kira Space is one native macOS app. A mode switcher moves between its modules: Agents, Automations, Studio, Api, Docker, Kubernetes and Git.

## Local data

Kira Space keeps its data in `~/.kira-space`. Config files such as workflows, steps, connections and Api collections live in `~/.kira-space/config`. Config sync backups live in `~/.kira-space/backups`.

Saved credentials, secret script parameters and agent secrets are encrypted at rest. The key sits in the macOS Keychain. See [Security and privacy](../../reference/security/).

## Agents run your own CLI

Kira starts the `claude` or `opencode` command that you installed and signed in to. It does not ship an agent. It never writes your agent config, except when you click **Export**. See [Agents](../../space/agents/).

## MCP servers

Kira offers MCP servers to agents:

- `kira-studio` and `kira-studio-session` give a client your databases, API environments, Kubernetes clusters and scripts, under rules you set.
- `kira-space` gives a task agent its task, branches and proof.

See [MCP and agents](../../space/mcp/).

## Phone view

Kira Space can serve the agents board to a phone on your home network. See [Phone view](../../space/phone/).

## Tabs and windows

Drag a tab out of the window to open it in a new window. Drop it on another window to move it there.

Cmd-click toggles a tab in the selection. Shift-click selects a range. The tab menu closes the selected tabs or moves them to a new window or to another window.

Pinned tabs never join a move. Runs terminals keep running when their tab moves.

On Wayland, dragging a tab out is off. Use **Move to new window** or **Move to window** in the tab menu instead.
