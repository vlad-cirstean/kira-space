---
title: How it works
description: One app, its modules, where your data lives and what runs on your machine.
sources:
  - README.md
  - apps/kira-space/README.md
  - docs/ARCHITECTURE.md
  - internal/kirapaths
  - internal/memory/mcpserver
  - apps/kira-space/internal/dbmcp/server.go
sidebar:
  order: 3
---

Kira Space is one native macOS app. A mode switcher moves between its modules: Agents, Automations, Memory, Git, Studio, Api, Docker and Kubernetes.

## Local data

Kira Space keeps its data in `~/.kira-space`. Config files such as workflows, steps, connections and Api collections live in `~/.kira-space/config`. Backups of one-time upgrades live in `~/.kira-space/backups`.

Saved credentials, secret script parameters and agent secrets are encrypted at rest. The key sits in the macOS Keychain. See [Security and privacy](../../reference/security/).

## Agents run your own CLI

Kira starts the `claude` or `opencode` command that you installed and signed in to. It does not ship an agent. It never writes your agent config, except when you click **Export**. See [Agents](../../space/agents/).

## MCP servers

Kira offers MCP servers to agents:

- `kira-memory` recalls stored facts in Claude Code sessions.
- `kira-db` and `kira-db-session` give a client your databases, under rules you set.
- `kira-ade` gives a task agent its task, branches and proof.

See [MCP and agents](../../space/mcp/).

## Phone view

Kira Space can serve the agents board to a phone on your home network. See [Phone view](../../space/phone/).
