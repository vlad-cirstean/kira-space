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

- `list`: what the AI client may use. Argument `what`, any of `connections`, `clusters`, `environments`, `scripts`, `temp` (default: all it can see). A connection shows its kind, status, `access` summary and `enforcement`. It also shows `review` (modes whose results you review) and `timeoutSeconds`. `expensiveRows` appears only when the expensive-query check is on. `tables` is the only tables MCP may touch. The rest: `maskedColumns`, `withheldColumns` and what it `can` do. Clusters, API environments and exposed scripts list the same way. `temp` lists temporary variables by name, never by value. Nothing here is gated by read mode.
- `describe`: columns, primary key, foreign keys, indexes and row estimate of a table or view. On a database or schema path it returns the columns of every table and view in one call. Arguments: `connectionId`, `path`, `refresh`.
- `browse_table`: read rows of one table or view without writing SQL. Use it for every plain read: rows, counts and pages. Arguments: `connectionId`, `path`, `columns`, `filters` (`eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `like`, `in`, `isNull`, `notNull`), `order`, `limit`, `offset`, `count`, `exemption`, `confirm`. `count: true` returns the number of matching rows instead of rows and takes filters only. Pages: while `hasMore` is true, pass `nextOffset` back as `offset`, up to offset 100000. The table's key is appended to the order so pages do not overlap; `stableOrder` is false when the table has no usable key. Masked columns take only `isNull` and `notNull` filters and no sort. Withheld columns come back in `withheldColumns`. A browse runs the same expensive-query check as `run_query`. A plan estimating at least `rowsThreshold` rows is refused. The refusal advises filtering or ordering on an indexed column, or reducing scope. Engines with no row estimate (SQLite) skip the check. Not available on MongoDB, Redis, Kafka, S3 or SQS.
- `run_query`: not for plain row reads: use `browse_table`. One statement per call. Arguments: `connectionId`, `sql`, `path`, `mode` (`read`, `write` or `ddl`, required), `maxRows` (default 100, maximum 2000), `explain`, `includeRaw`, `exemption`, `confirm`. With `explain: true` it plans one SELECT or WITH statement without running it. That needs mode `read`. MongoDB, Redis, Kafka, S3 and SQS have no plans. `includeRaw` adds the raw plan. With column review on, the result can carry `withheldColumns` and `withheldNote` for columns dropped until you review them.
- `exemption` on `run_query` and `browse_table`: after a limit refusal, ask the user to lift that limit for this exact call. Fields `id` and `reason`. The id works once and expires in 10 minutes. A statement that runs past the connection's statement timeout is stopped and refused the same way. So is a SELECT or WITH whose query plan estimates at least `rowsThreshold` rows read; `run_query` with `explain` shows which scan.
- `confirm` on `run_query`, `browse_table`, `kube_read`, `run_script` and `api_request`: ask the user to approve this call even when the settings allow it, for a sensitive action. One sentence, at most 300 characters; the dialog shows it. It never lifts a refusal: a denied mode, limit or rule still refuses first.
- `operations`: running database and script work. Argument `cancel` (an `opId` from the list) stops one.
- `run_script`: run a script the user exposed. Arguments: `scriptId`, `params`, `waitSeconds`, `confirm`. Returns state, exit code and, once the run ends, the last 50 output lines. A script with `mode: long` returns at once and keeps running; stop it with `operations`. With `runId` (and `waitSeconds`, `from`, `lines`, `contains`) it reads a past run's state and stored output instead.
- `kube_read`: read one Kubernetes cluster you turned on. Argument `op` is one of four. `discover` gives namespaces and listable kinds. `list` gives one kind's objects as `kubectl get` columns. `describe` gives description, events and manifest; Secret values stay hidden. `logs` gives a pod's container log. Other arguments: `clusterId`, `kind`, `name`, `namespace`, `labelSelector`, `maxRows`, `container`, `previous`, `timestamps`, `tail`. A cluster that is off answers like an unknown id. Optional `confirm`, described above.
- `api_request`: send one HTTP request through an API environment you exposed (off by default; Settings, Studio MCP). Environments come from `list`. Large bodies are cut with a hint; writes can wait for your approval. Optional `confirm`, described above.

## Agent tools (Kira Space)

Server name `kira-space`. A run or session on a task has `state`. A rebase run has `finish_step` and `git`. See [Agents](../../space/agents/).

Files in the task folder need no tool: Kira watches the folder and applies saves, such as the handover draft, notes, links and proofs. An edit of another agent's note, link or proof waits for the user's approval. See [Proof](../../space/proof/).

- `state`: ask Kira for something, or finish the step. Arguments: `action` and `args`. Every reply lists items, each with a status. Statuses: `done`, `pending` (awaiting the user), `waiting` (Kira is working), `denied`, `failed` and `invalid` (the call is wrong; the item shows a fixed example). Read `state.yaml` in the task folder for what Kira knows: repositories, setup, leases, operations, runs and late answers.

Actions of `state`:

- `repo.declare`: add registered repositories to the task. Args: `repos`.
- `branch.request`: create a branch and its worktree. You name the branch. A new branch or a free existing one needs no approval. The call waits for the worktree setup, up to 10 minutes, then read `state.yaml`. Only adopting a branch held by another task or folder asks the user. Args: `repo`, `branch`, optional `base` and `existing`.
- `branch.rename`: ask the user to approve renaming one of the task's branches. The worktree folder stays. The remote branch is renamed only if the user agrees. Args: `repo`, `from`, `to`, `renameRemote`.
- `worktree.remove`: remove the worktree of one of the task's own branches, with no prompt. The git branch stays; check it out again with `branch.request` and `existing`. Refused when the branch is not the task's own, has no worktree or is your working folder. Also refused when it is busy, open, blocked (locked, open elsewhere, main checkout) or has uncommitted changes. There is no discard argument. Args: `repo`, `branch`.
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
