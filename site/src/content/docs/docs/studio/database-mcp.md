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

## Table allow list

By default an exposed connection shows every table, including tables created later. In the MCP tab, untick **All tables** to choose which tables, views and collections the client may touch. The choice is saved in the connection file as `mcp.tables`: leave the key out for all tables, `[]` for none.

The app parses every `run_query` and `explain_query` statement before it runs. A statement that touches an unlisted table is refused, in every mode, and no approval or exemption applies. A statement the app cannot parse is refused too. So are `CALL`, `SET`, `USE`, `PRAGMA`, calls to your own functions, and engine features that read other tables. `list_children` and `describe_schema` hide unlisted tables, and `describe_table` refuses them.

Limits: a view is checked by its own name, so listing a view grants what it shows. Scripts and operations are not checked. Kafka, S3, SQS and Redis have no tables, so the list does not apply. Triggers, rules and other code that runs inside the database can still reach unlisted tables; use database permissions for a hard guarantee.

## Expensive queries

Before an AI client's SELECT or WITH runs, the app asks the database for the query plan. The query itself does not run. The app compares the plan's estimated row count with your expensive-query threshold. At or above it, the call is refused before it runs. The estimate is the rows the plan reads, not the rows the query returns.

Set the threshold in Settings, under Advanced. The default is 100,000, and the same number drives the warning in the console. Each connection's MCP tab turns the check on or off.

The AI client can improve the query or ask for an exemption. You always approve an exemption in a dialog. It shows the estimate and the plan. If the plan cannot be read, the call is refused and no exemption is offered.

Kira enforces this, not the database. PostgreSQL, MySQL, MariaDB and ClickHouse give a row estimate. SQLite, MongoDB, Redis, Kibana, Kafka, S3 and SQS give none, so queries there are not checked.

Known limits:
- Estimates come from table statistics, so stale statistics give wrong numbers.
- The estimate ignores LIMIT: a small sample of a big table can be refused.
- Only SELECT and WITH statements are checked. A plan with no table scan has no estimate and passes.
- The plan request is one more operation and counts against the connection's throttle.
- The statement timeout still applies to everything.

## Masked browse

`browse_table` reads rows of one table or view without SQL. The AI client names the table, columns, filters, order and page; Kira builds the statement. The connection dialog's MCP tab has a **Read masked data** row. Its choices are deny, allow, ask me, or the default, same as read. Use it to deny `run_query` reads and still let the AI see masked rows.

Rules:

- Masked columns accept only is-empty and is-not-empty filters and cannot be sorted on.
- A view is refused on a connection that has masking rules, because Kira cannot see which columns it exposes.
- Table allow list and column review apply as for `run_query`.
- The expensive-query check applies. A browse whose plan estimates at least the row threshold is refused. The AI client can ask for an exemption, and you always see a dialog. SQLite gives no estimate, so it skips the check.
- Not available for MongoDB, Redis, Kibana, Kafka, S3 and SQS.

Limits: masking matches by column name. A filter value containing a semicolon is refused while the plan check is on. Paging is by offset.

## Column review

Masking rules only protect columns you marked. A column added later is unmarked, so it would reach the AI client. Column review closes that gap for PostgreSQL, MySQL, MariaDB, ClickHouse and SQLite.

In the Privacy tab, turn on **Withhold new columns until reviewed**. Kira records every current column as reviewed. From then on a column that no rule masks and nobody reviewed is withheld from MCP results. `run_query` drops it and names it in `withheldColumns`. A query that renames, transforms or filters on it is refused. Kira tells the agent the column name, never a value, and shows a prompt so you can review it.

Review a column by ticking it, masking it with the **Mask** button, or choosing **Confirm all as reviewed**. The choice is saved in the connection file as `mask_review`.

Limits: matching is by column name, like masking, so a reviewed column can share its name with a withheld one. Kira checks on connect, on Refresh and after an MCP write. It also rechecks every 30 seconds. A column added in the last 30 seconds can slip through. A renamed column counts as new. Scripts are not checked. No exemption applies.

## Approvals

A prompt waits for you to approve the exact statement. The dialog names the connection and the statement class.

When a limit rejects a statement, the AI client can ask you to lift that limit for that one statement. You always get a dialog, even for modes that need no approval. It shows the limit, the measured value, the limit value, the client's reason and the statement. Approving covers that one run; the limit stays for every other call. With several requests waiting, "Approve all" skips these. Approve each one on its own.

You can review each query result before the AI client gets it. The app-wide switch is in Settings, under Database MCP. A connection's MCP tab sets Read, Write and DDL results on their own: inherit the app setting, always review, or always send. The tab explains which setting applies.

A permission prompt also has a checkbox, "Review the result before the AI gets it". Check it to review the result of that one call, even when review is off. Limit exemption prompts have it too.

A held result shows as a table with the masking already applied. Send gives the AI client that exact result. Withhold gives it an error with no data. A write has already run when its result shows, so withholding hides the result and does not undo the change. "Approve all" skips result reviews. Approve each one on its own.

Held results stay in memory only. Nothing is stored or logged. A result over 8 MB is refused instead of held, and so is any result once held results total 64 MB.

## Masking

PII masking rules apply to results before the AI client sees them.
