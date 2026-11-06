---
title: MCP tools
description: Every tool each Kira MCP server offers, with its arguments.
sources:
  - internal/llmprompts/registry.go
  - internal/memory/mcpserver
  - apps/kira-studio/internal/dbmcp/server.go
  - internal/agentmcp
  - internal/harness/claude/mcpinstall
sidebar:
  order: 3
---

Tool descriptions are editable defaults. See [Prompts](../prompts/).

## kira-memory (Kira Space)

Registered in your Claude Code user scope from `Settings > Memory`. Claude Code starts it per session over stdio. See [Memory](../../space/memory/).

### store_memory

Store facts and standing rules. A rule is an imperative that holds beyond one task, with its reason. The store splits independent claims into separate facts. A list about one subject stays one fact. It rewords each fact and updates a matching fact in the same scope, unless the stored one has a higher origin. An unclear item, a weak reason, a question or a one-off task is challenged: nothing is stored and questions come back. A rule and a fact stay separate memories. The result and every search result show the kind, `fact` or `rule`, after the version.

- `items`: 1 to 20 items. Each has `fact`, `reason`, `origin` (user, agent or external), `scope` (company, project or personal), plus `sourceTag` for external and `project` for project scope.
- `clarifications`: question and answer pairs for an earlier challenge.
- `session`: a Claude Code session id. Defaults to the current session.
- `groups`: up to 5 groups of kind task, document or thread, each with `key` and optional `title`.

To cite a memory in a code comment, write its fact id.

### search_memories

Search by words and meaning, best first. The meta line of a result shows its kind, `fact` or `rule`.

- `query`: free text.
- `includeHistory`: also return older versions.
- `limit`: default 40, maximum 200.
- `scopes`, `origins`: filters.
- `project`: a project key whose facts rank first.

### memory_history

Every version of one memory, oldest first. Argument: `id`.

### memory_context

The facts stored with one fact, per group, newest first. Arguments: `id`, `limit` (default 20, maximum 100).

### ingest_files

Store the facts and rules of files and folders by path, in the background. Send absolute paths, never content.

- `paths`: 1 to 100 paths.
- `origin`, `sourceTag`, `scope`, `project`: as in `store_memory`. Storing needs `paths`, `origin` and `scope`.
- `groups`: up to 4 groups of kind task or thread.
- `writeDoc`: also write `<name>.memory.md` beside each file. Default false.
- `session`: a Claude Code session id.
- `text`: text to distill, up to 120,000 characters. Only with `distill`.
- `distill`: `true` stores nothing and returns the merged lines instead. Default false.
- `mode`: with `distill`, `facts` (default) returns merged lines; `rewrite` returns one short text rewritten in its own form, or unchanged when already clean. Text only, up to 2,000 characters.
- `context`: code around the text, read only, up to 6,000 characters. Only with `mode` rewrite.

A large job waits for you to start it in Kira Space. So does a job holding a kira-memory doc whose header claims a higher origin, or another scope or project, than the job. A doc's header provenance wins on import, so you confirm it first.

#### Distill

With `distill` true, the call condenses text or one file into short lines and stores nothing. No gate runs and no import job is created. Give `text` or one absolute file path in `paths`, never both. Any other argument is refused: `origin`, `sourceTag`, `scope`, `project`, `groups`, `writeDoc` and `session`. A folder is refused. Do not read the file first.

It runs the import pipeline up to the point where import would store. The input is split into parts, and Claude extracts the facts and rules of each part. Doubtful facts are checked against the source. One merge call then joins duplicates across the parts, resolves unclear references, drops what is not a memory and returns the lines in source order. The merge covers only the input and never reads your memories. Examples are never lines by themselves, and a table is kept only as the headline figure of a stated conclusion.

The call waits for the result. The text holds the lines, one per line with no prefix. A blank line and one `unresolved: <fact> | <questions>` line per fact the input leaves open follow. The structured result has `lines` (each with `text` and `kind`), `unresolved` and `costUsd`. A call costs Claude tokens: large input is split into at most 16 parts, and the call stops at 2 USD. It is not available to the agent an import runs.

### ingest_status

State of an ingest job. Argument: `id`, the job id from `ingest_files`.

### remember (prompt)

Run `/mcp__kira-memory__remember` to store durable facts from given text or the recent conversation. Argument: `text`.

## Database MCP server (Kira Studio)

Server name `kira-db`. See [Database MCP server](../../studio/database-mcp/).

- `list_connections`: every exposed connection with its kind, read-only flag, status, `access` per mode, `enforcement`, `tables` and masked columns. `statementTimeout` gives the seconds (0 for off) and who enforces it per mode. `tables` is all, or the only tables MCP may touch. `columnReview` shows whether column review is on and `withheld` lists the columns results drop until you review masking. `expensiveQuery` gives `rowsThreshold` and `check`: `plan` (`run_query` refuses a SELECT or WITH at or above it), `off` or `none` (the engine gives no row estimate). `reviewResults` lists the modes whose results the user reviews first, `masked` included. `access.masked` and `permissions.masked` are `browse_table`'s permission; `browse` says whether the connection's kind supports it.
- `list_children`: the children of one container; tables outside the connection's allow list are hidden. Arguments: `connectionId`, `path`, `refresh`.
- `describe_table`: columns, primary key, foreign keys, indexes and row estimate. Arguments: `connectionId`, `path`, `refresh`.
- `describe_schema`: every table and view in one database or schema. Arguments: `connectionId`, `path`, `refresh`.
- `browse_table`: read rows of one table or view without writing SQL. Try it before `run_query`. Arguments: `connectionId`, `path`, `columns`, `filters` (`eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `like`, `in`, `isNull`, `notNull`), `orderBy`, `limit`, `offset`, `exemption`. Masked columns take only `isNull` and `notNull` filters and no sort. Withheld columns come back in `withheldColumns`. A browse runs the same expensive-query check as `run_query`. A plan estimating at least `rowsThreshold` rows is refused. The refusal advises filtering or ordering on an indexed column, or reducing scope. Engines with no row estimate (SQLite) skip the check. Not available on MongoDB, Redis, Kibana, Kafka, S3 or SQS.
- `run_query`: one statement per call. Arguments: `connectionId`, `sql`, `path`, `mode` (`read`, `write` or `ddl`, required), `maxRows` (default 200, maximum 2000), `exemption`. With column review on, the result can carry `withheldColumns` and `withheldNote` for columns dropped until you review them.
- `exemption` on `run_query` and `browse_table`: after a limit refusal, ask the user to lift that limit for this exact call. Fields `id` and `reason`. The id works once and expires in 10 minutes. A statement that runs past the connection's statement timeout is stopped and refused the same way. So is a SELECT or WITH whose query plan estimates at least `rowsThreshold` rows read; `explain_query` shows which scan.
- `explain_query`: a plan for one SELECT or WITH statement, without running it. Arguments: `connectionId`, `sql`, `path`, `includeRaw`. Not available on MongoDB, Redis, Kafka, S3 or SQS.

## Agent tools (Kira Space)

Server name `kira-ade`. `finish_step` is available to every agent run. `fetch_remote` and `push_branch` come with a sandboxed run. The other tools are always on. See [Agents](../../space/agents/).

- `finish_step`: report how a step ended. Call it once, last. The reply lists the task's artifacts. Arguments: `status`, `summary`, `reason`, `conflictedFiles`, `lastGitError`, `tried`.
- `reply_review_comment`: answer one review comment. Only in a workflow review round. Call once per comment. Arguments: `id`, `reply`, `resolved`.
- `report_artifacts`: record what the task produced. Argument: `artifacts`, 1 to 20 items of `kind` (`change_request`, `issue`, `deployment`, `doc`, `link`), `title`, `url`, `ref`. Branches, commits, change requests and deployments of the task repositories are found without a report.
- `task_info`: the task, its workflow, step, repositories and branches.
- `declare_repos`: add registered repositories to the task. Argument: `repos`, 1 to 20.
- `request_branches`: ask the user to approve branches and worktrees, one call for all. You name each branch. A repository may get several branches. The call waits for the answer, up to 30 minutes. Per branch the result is `created`, `existing`, `denied`, `timeout` or `failed`. Argument: `branches`, 1 to 20 items of `repo` and `branch`.
- `branch_status`: the worktree and setup state of a branch. Arguments: `repo`, `branch`, `waitSeconds` (0 to 600).
- `rename_branch`: ask the user to approve renaming one of the task's branches. The worktree folder stays. The remote branch is renamed only if the user agrees. The result is `renamed`, `denied`, `timeout` or `failed`, with the remote state `renamed`, `kept`, `untracked` or `failed`. Refused for main, integration and protected branches, open change requests and sandboxed branches. Arguments: `repo`, `from`, `to`, `renameRemote`.
- `list_operations`: the task's running git operations that can be stopped, such as a slow fetch or push. Returns `opId`, `kind`, `subject` and `elapsedMs` per operation.
- `cancel_operation`: stop one of them by `opId` from `list_operations`. Another task's operation reads as unknown. Argument: `opId`.
- `run_outcome`: how the latest runs of the task ended. Arguments: `runId`, `branch`, `kind`, `limit` (default 3, maximum 10).
- `fetch_remote`: fetch one remote of the repository, in the app. Sandboxed runs only. Argument: `remote` (default `origin`).
- `push_branch`: push the task branch to the same-named remote branch, in the app. Sandboxed write runs only. Never a protected branch. Argument: `force` (lease-checked force push).
- `git`: run restricted git in a named worktree. Used by rebase runs. Arguments: `worktree`, `args`.
