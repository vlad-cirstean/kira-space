---
title: Databases and connections
description: Supported engines, how to save a connection, and the guards around it.
sources:
  - README.md
  - apps/kira-space/internal/adapters/caps.go
  - apps/kira-space/internal/adapters/clickhouse/caps.go
  - apps/kira-space/internal/adapters/redis/caps.go
  - apps/kira-space/internal/adapters/s3/caps.go
  - apps/kira-space/internal/mask/mask.go
  - packages/theme/src/i18n/en.json
sidebar:
  order: 1
---

## Engines

| Engine | Default view | Query console | Writes |
|---|---|---|---|
| PostgreSQL | Grid | SQL | yes |
| MariaDB | Grid | SQL | yes |
| MySQL (8.0.16+) | Grid | SQL | yes |
| SQLite | Grid | SQL | yes |
| ClickHouse | Grid | SQL | insert only |
| MongoDB | Documents | shell-style | yes |
| Redis | Key/value | Redis commands | string keys, delete on any type |
| Kafka | Stream | none | produce |
| SQS | Stream | none | send and delete |
| S3 | Object browser | none | yes, plus upload and download |

Stop cancels the operation itself, not only its display. Examples: `pg_cancel_backend`, `KILL QUERY`, cursor abort, consumer stop.

SQS never polls on its own. Reads happen only when you press **Poll**, because `ReceiveMessage` hides messages from real consumers.

## Save a connection

**New connection** asks for a name, an engine and a colour. It writes a file in `~/.kira-space/config/connections`, one `<name>.yaml` per connection. Use **Edit connection file…** to change anything else, as a form or as YAML. Or open the file in your editor with **Open in editor**. The dialog also has **Duplicate** and **Delete**.

The file holds host, port, database, username, URI mode, the read-only guard, the pre-connect command, MCP settings, the row colour and the tree filters. The app watches the folder. A saved edit applies at once.

- **Read-only guard**: a per-connection key. The backend enforces it, not the UI.
- **Pre-connect command**: a script that runs before the connection opens, for example a port-forward. It holds 1 to 2000 characters.
- **Import from DataGrip**: reads a JetBrains DataGrip project and writes one file per data source.
- **Paste credentials…**: paste key/value lines, `.env` lines or a `psql` or `mysql` style command. Review what the app recognised, then apply.
- **Bad edit**: the connection keeps its last valid version. The tree row shows the error line.

Passwords and URIs never sit in the file. The file holds a reference such as `${secret:local.password}`. Set the value in the **Secrets** section of the file dialog. Values are write-only: the eye on a saved field asks to confirm before it shows one.

Saved credentials are encrypted at rest in the macOS Keychain. Touch ID or the system password confirms a reveal of a saved password.

## Project panel

The panel shows a lazy, cached connection tree. It has per-connection colours, live status, search and hide or show filters. Colours and filters live in the connection file.

Open the same table in any number of tabs. Each tab keeps its own paging, sort, filter and scroll. Session restore reopens tabs without connecting.

## Column masking

Mark a column as PII with one of six masks: name, email, text, number, date or redact. The preview shows the masked view live.

Masking is display only. The data never changes.
