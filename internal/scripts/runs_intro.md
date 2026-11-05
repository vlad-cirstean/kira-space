Each file here is one run. The file name without `.yaml` is its id. Kira Runs lists the files in its panel.

- A normal script runs `command` in the login shell. A smart script runs `prompt` in headless Claude Code.
- Variables: a prompt reads `{name}`, a command reads `KIRA_PARAM_<NAME>`. Kira Space adds built-in variables from the ADE task.
- `dir: kira` runs in a folder Kira keeps under its data folder for this run.
- Secret params: a value never goes in a file. Set it in Kira (Edit file, Saved secret values). It stays in the sealed secret store.
- `collection` groups runs in the panel. Order comes from dragging in the panel and is not in the files.
- A schedule's task and branch (Kira Space) are set in Kira (Schedule target), not in the file.
- `mcp` opts a normal script into the Kira database MCP server. Agents see only scripts with `mcp.enabled: true`. `mcp.run: prompt` (default) asks the user before each agent run; `allow` runs at once. An agent never sets a secret param.
- Run history lives in Kira's database. Deleting a file removes the run and its saved secret values. Renaming a file is a delete plus a create: the new run starts with no saved values and no history.
- Schedules fire only while the app is open. A missed fire is skipped.
