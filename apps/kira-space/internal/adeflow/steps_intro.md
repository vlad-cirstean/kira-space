The step library holds reusable steps. A step here is a standalone definition: one file per step, edited on its own. A workflow adds a step by reference and may change it on top (see the Workflows folder's AGENTS.md).

A step file takes the keys of a workflow step, except the keys that only make sense inside a workflow: `skip`, `review.implementer`, and a result `next` that names a step id. A result `next` here is `next`, `end` or `stop`.

To add or change a step, write `<id>.yaml` in this folder with any editor, or use the Steps tab of the Workflows page. The `id` key must equal the file name without `.yaml`. An edit changes every workflow that references the step, for new work only. A running task keeps the workflow it started with. A step that a workflow references cannot be deleted from the Workflows page.

Example:

```yaml
id: implement
name: Implement
kind: agent
runs_on: each repo
timeout: 30m
prompt: |
  Implement the task. Run the tests before you finish.
allowed_tools:
  - Bash(git *)
```
