A workflow is a list of steps. A task runs the steps top to bottom. Each step is one of four kinds:

- `agent`: an agent runs a `prompt`, or a smart script.
- `script`: a shell `command` runs.
- `review`: an agent reviews the work so far.
- `manual`: a person does the step, optionally in an agent session.

To add or change a workflow, write `<id>.yaml` in this folder with any editor, or use the Workflows page. The `id` key must equal the file name without `.yaml`; renaming the file changes the id. An edit affects new tasks only. A running task keeps the workflow it started with.

Task statuses come from the Statuses list on the Workflows page. A step names one with `status`.

Example:

```yaml
id: build-and-review
name: Build and review
steps:
  - id: implement
    name: Implement
    kind: agent
    runs_on: each repo
    timeout: 30m
    prompt: |
      Implement the task. Run the tests before you finish.
    allowed_tools:
      - Bash(git *)
  - id: check
    name: Run checks
    kind: script
    runs_on: each repo
    timeout: 10m
    command: make check
  - id: review
    name: Review
    kind: review
    runs_on: each repo
    timeout: 20m
    results:
      - id: approved
        ok: true
      - id: changes
        ok: false
        description: The work needs changes.
        next: implement
  - id: signoff
    name: Sign off
    kind: manual
    session: true
```
