---
title: MCP tools
description: Every tool each Kira MCP server offers, with its arguments.
sources:
  - internal/llmprompts/registry.go
  - apps/kira-space/internal/dbmcp/server.go
  - internal/agentmcp
  - internal/harness/claude/mcpinstall
sidebar:
  order: 3
---

Tool descriptions are editable defaults. See [Prompts](../prompts/).

## Studio MCP server (Studio modules)

Server name `kira-studio`. Workflow steps with `db_mcp` reach the same tools as `kira-studio-session`. See [Studio MCP server](../../studio/database-mcp/).

- `list_connections`: every exposed connection with its kind, read-only flag, status, `access` per mode, `enforcement`, `tables` and masked columns. `statementTimeout` gives the seconds (0 for off) and who enforces it per mode. `tables` is all, or the only tables MCP may touch. `columnReview` shows whether column review is on and `withheld` lists the columns results drop until you review masking. `expensiveQuery` gives `rowsThreshold` and `check`: `plan` (`run_query` refuses a SELECT or WITH at or above it), `off` or `none` (the engine gives no row estimate). `rowsThreshold` is the connection's own threshold when set, else the app default. `reviewResults` lists the modes whose results the user reviews first, `masked` included. `access.masked` and `permissions.masked` are `browse_table`'s permission; `browse` says whether the connection's kind supports it.
- `list_children`: the children of one container; tables outside the connection's allow list are hidden. Arguments: `connectionId`, `path`, `refresh`.
- `describe`: columns, primary key, foreign keys, indexes and row estimate of a table or view. On a database or schema path it returns the columns of every table and view in one call. Arguments: `connectionId`, `path`, `refresh`.
- `browse_table`: read rows of one table or view without writing SQL. Use it for every plain read: rows, counts and pages. Arguments: `connectionId`, `path`, `columns`, `filters` (`eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `like`, `in`, `isNull`, `notNull`), `order`, `limit`, `offset`, `count`, `exemption`. `count: true` returns the number of matching rows instead of rows and takes filters only. Pages: while `hasMore` is true, pass `nextOffset` back as `offset`, up to offset 100000. The table's key is appended to the order so pages do not overlap; `stableOrder` is false when the table has no usable key. Masked columns take only `isNull` and `notNull` filters and no sort. Withheld columns come back in `withheldColumns`. A browse runs the same expensive-query check as `run_query`. A plan estimating at least `rowsThreshold` rows is refused. The refusal advises filtering or ordering on an indexed column, or reducing scope. Engines with no row estimate (SQLite) skip the check. Not available on MongoDB, Redis, Kibana, Kafka, S3 or SQS.
- `run_query`: not for plain row reads: use `browse_table`. One statement per call. Arguments: `connectionId`, `sql`, `path`, `mode` (`read`, `write` or `ddl`, required), `maxRows` (default 200, maximum 2000), `explain`, `includeRaw`, `exemption`. With `explain: true` it plans one SELECT or WITH statement without running it. That needs mode `read`. MongoDB, Redis, Kafka, S3 and SQS have no plans. `includeRaw` adds the raw plan. With column review on, the result can carry `withheldColumns` and `withheldNote` for columns dropped until you review them.
- `exemption` on `run_query` and `browse_table`: after a limit refusal, ask the user to lift that limit for this exact call. Fields `id` and `reason`. The id works once and expires in 10 minutes. A statement that runs past the connection's statement timeout is stopped and refused the same way. So is a SELECT or WITH whose query plan estimates at least `rowsThreshold` rows read; `run_query` with `explain` shows which scan.
- `operations`: running database and script work. Argument `cancel` (an `opId` from the list) stops one.
- `run_script`: run a script the user exposed. Arguments: `scriptId`, `params`, `waitSeconds`. Returns state, exit code and, once the run ends, the last 50 output lines. A script with `mode: long` in its run file returns at once and keeps running. Read its output with `get_script_run`; stop it with `operations`.
- `get_script_run`: state of a run, optionally waiting. Arguments: `runId`, `waitSeconds`, `from`, `lines`, `contains`. Returns stored output once the run ended or when a filter is given. A long script returns its last 50 lines at any state.
- `kube_list_clusters`: the Kubernetes clusters you turned on, with context, server, namespace, description and `approval` (`prompt` or `allow`). A cluster that is off is not listed, and every `kube_*` tool answers it like an unknown id.
- `kube_discover`: namespaces and listable kinds of one cluster, in one call. Argument: `clusterId`.
- `kube_list_resources`: one kind's objects as `kubectl get` columns. Arguments: `clusterId`, `kind` (a key from `kube_discover`, such as `v1/pods`), `namespace` (empty for all), `labelSelector`, `maxRows` (default 200, maximum 2000).
- `kube_describe`: one object's description, events and manifest; Secret values stay hidden. Arguments: `clusterId`, `kind`, `name`, `namespace`.
- `kube_pod_logs`: a pod's container log. Arguments: `clusterId`, `namespace`, `pod`, `container`, `previous`, `timestamps`, `tail` (default 200, maximum 5000).

## Agent tools (Kira Space)

Server name `kira-space`. A run or session on a task has `submit` and `state`. A rebase run has `finish_step` and `git`. See [Agents](../../space/agents/).

- `submit`: hand the task folder's files to Kira, such as the handover draft, notes and links. A proof is a folder in the task folder and needs no report: see [Proof](../../space/proof/). Arguments: `paths` (default every changed file), `reason`. It never finishes a step.
- `state`: ask Kira for something, or finish the step. Arguments: `action` and `args`. Every reply lists items, each with a status. Statuses: `done`, `pending` (awaiting the user), `waiting` (Kira is working), `denied`, `failed` and `invalid` (the call is wrong; the item shows a fixed example). Read `state.yaml` in the task folder for what Kira knows: repositories, setup, leases, operations, runs and late answers.

Actions of `state`:

- `repo.declare`: add registered repositories to the task. Args: `repos`.
- `branch.request`: ask the user to approve a branch and its worktree. You name the branch. The call waits for the answer, then for the worktree setup, up to 10 minutes; after that read `state.yaml`, the request stays open. Args: `repo`, `branch`, optional `base` and `existing`.
- `branch.rename`: ask the user to approve renaming one of the task's branches. The worktree folder stays. The remote branch is renamed only if the user agrees. Args: `repo`, `from`, `to`, `renameRemote`.
- `branch.change`: ask the user to approve pointing one of the task's branches at another existing local branch, when the worktree folder holds a different one. Args: `repo`, `from`, `to`.
- `lease.acquire`: take a named lease on a shared resource the user set up, such as a port, the full test suite, push or merge. The answer is `done` (held) or `waiting` with your position; calling again keeps your place. A lease ends with the session, or after 10 minutes of silence. Args: `queue`.
- `lease.release`: give a lease back, or leave its line. Without `queue` it leaves every queue. Args: `queue`.
- `operation.cancel`: stop one running git operation of the task, such as a slow fetch or push. Args: `opId` from `state.yaml`.
- `remote.fetch`: fetch one remote, in the app. Sandboxed runs only. Args: `remote` (default `origin`).
- `branch.push`: push the task branch to the same-named remote branch, in the app. Sandboxed write runs only. Never a protected branch. Args: `force`.
- `review.reply`: answer one review comment. Only in a workflow review round. Args: `id`, `reply`, `resolved`.
- `step.finish`: report how the step ended. Call it once, last. In a session only `done`. The reply lists the task's artifacts. Args: `status`, `reason`, `conflictedFiles`, `lastGitError`, `tried`.
- `finish_step`: the same report, for a rebase run or a review round. Arguments: `status`, `summary`, `reason`, `conflictedFiles`, `lastGitError`, `tried`.
- `git`: run restricted git in a named worktree. Used by rebase runs. Arguments: `worktree`, `args`.

### Orchestrator tools

Only a task of kind orchestrator carries these. A child task never does.

- `create_task`: ask the user to approve a child task. The call waits for the answer, up to 30 minutes. The result is `created` with the `taskId`, `denied`, `timeout` or `failed`. The child starts on the first step of its workflow with your brief as its first handover, and waits for the user to run it. Arguments: `title`, `workflow` (id or name), `repos` (0 to 10 items of `repo` and `base`), `brief`.
- Read your children in `state.yaml` (`chain`): step, finished, archived, newest handover.
