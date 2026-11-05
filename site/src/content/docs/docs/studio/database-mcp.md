---
title: Database MCP server
description: Let an AI client read your databases under rules you set.
sources:
  - README.md
  - apps/kira-studio/internal/dbmcp/server.go
  - apps/kira-studio/internal/dbmcp/tools.go
  - apps/kira-studio/frontend/src/state/settingsDomain.ts
  - internal/llmprompts/registry.go
  - docs/ARCHITECTURE.md
  - packages/theme/src/i18n/en.json
sidebar:
  order: 6
---

An optional local [MCP](https://modelcontextprotocol.io) server. An AI client lists, browses and queries your connections through the same path the SQL console uses.

The server is off by default. Turn it on with **Enable the database MCP server** in `Settings > Database MCP`.

## Access

- Loopback only, on port 8766.
- Its own bearer token. **Regenerate token** issues a new one.
- A port conflict stops the server. It never falls back to another port.
- Tool list: see [MCP tools](../../reference/mcp-tools/).

## Exposed connections

Deny by default. A connection stays invisible until you check it under **Exposed connections**.

## Permissions

Each exposed connection has three modes: read, write and DDL. Each mode is allow, deny or prompt.

| Mode | Default | Meaning of prompt | Applies to |
|---|---|---|---|
| Read | allow | you approve each statement | queries that read |
| Write | prompt | you approve each statement | inserts, updates, deletes |
| DDL | deny | you approve each statement | schema changes |

The AI client declares one mode per `run_query` call: `read`, `write` or `ddl`. The declared mode picks the permission that applies. The app's statement check can only refuse: a statement more powerful than the declared mode is rejected, and an unclassifiable statement runs only under `ddl`.

A `read` call runs on a separate read-only connection with the same credentials, so the database refuses writes itself. This holds for PostgreSQL, MySQL, MariaDB, SQLite and ClickHouse. MongoDB and Redis have no read-only session, so the app checks `read` itself; use a database user with read-only rights for a hard guarantee. Some engines still allow side-effecting functions in a read-only session, such as `pg_terminate_backend`. The database cannot tell write from DDL on one connection, so a misclassified DDL statement would run under the write permission. Write and DDL are not available for read-only connections or for Kibana, Kafka, S3 and SQS.

The connection dialog's MCP tab shows which modes the client may use and what backs `read`.

## Statement timeout

Each connection has a statement timeout for AI client statements, set in the MCP tab. The default is 10 seconds. 0 turns it off. The range is 1 to 3600.

The database enforces it where it can: PostgreSQL, MySQL (reads only), MariaDB and ClickHouse. Where it cannot, the app stops the statement itself, with the same cancel as Stop. SQLite and MongoDB are stopped this way. Redis is stopped on the app side, but the server may keep running the command. Time spent queued behind other operations does not count. The app adds a 2 second grace so the database's own error wins when it can.

A timed-out statement is refused, and the AI client can ask for an exemption. You always approve it in a dialog. An exempted statement runs with no time limit, so you can stop it from the Operations panel.

## Approvals

A prompt waits for you to approve the exact statement. The dialog names the connection and the statement class.

When auto-explain is on, a query estimated above your expensive-query threshold pauses for approval. The dialog shows the estimate and the threshold.

When a limit rejects a statement, the AI client can ask you to lift that limit for that one statement. You always get a dialog, even for modes that need no approval. It shows the limit, the measured value, the limit value, the client's reason and the statement. Approving covers that one run; the limit stays for every other call. With several requests waiting, "Approve all" skips these. Approve each one on its own.

You can also review each query result before the AI client gets it. The app-wide switch is in Settings, under Database MCP. A connection's MCP tab can override it with Always or Never. A write has already run when its result shows, so withholding hides the result and does not undo the change.

## Masking

PII masking rules apply to results before the AI client sees them.
