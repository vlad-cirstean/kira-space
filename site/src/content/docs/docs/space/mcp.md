---
title: MCP and agents
description: Which MCP servers Kira offers, and what an agent session gets.
sources:
  - README.md
  - apps/kira-space/README.md
  - internal/agentmcp
  - apps/kira-space/internal/dbmcp/server.go
  - internal/memory/mcpserver
  - internal/harness/claude/mcpinstall
sidebar:
  order: 6
---

Kira Space runs several local [MCP](https://modelcontextprotocol.io) servers. Each has its own switch and scope.

| Server | For | Switch |
|---|---|---|
| `kira-space` | A task agent: task info, branches, artifacts and proof. | Always on in a task run. |
| `kira-memory` | Claude Code sessions: store and recall facts. | `Settings > Memory`. |
| `kira-studio` | An external AI client reading your databases. | Off by default. `Settings > Database MCP`. |
| `kira-studio-session` | A workflow step with **Kira database tools**. | Per step. |

## What an agent session gets

A task run gets `kira-space`. It reports how a step ended, replies to review comments, records artifacts and attaches proof. A sandboxed run also gets `fetch_remote` and `push_branch`.

A step that ticks **Kira database tools** also gets `kira-studio-session`. It shares your exposed connections, limits and approvals. See [Database MCP server](../../studio/database-mcp/).

## Reference

Every tool and its arguments: [MCP tools](../../reference/mcp-tools/).
