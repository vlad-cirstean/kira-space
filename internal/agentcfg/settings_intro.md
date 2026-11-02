`settings.yaml` is the agent settings: one shared definition, rendered in each provider's own format. Edit it with any editor, or use the Settings page in the Agents module of Kira Space. The page also shows the exact config each provider receives. A missing or empty file means no settings.

Common keys apply to every provider: `env`, `permissions`, `mcp_servers`, `hooks`. A key named after a provider holds that provider's own settings, validated by that provider.

Hooks:

- `hooks` lists your hook commands by name, with a neutral event: `session_start`, `session_end`, `prompt`, `tool_start`, `tool_end`, `notification`, `turn_end`.
- A provider section can add hooks in its own events and can list common hook names in `skip_hooks` to leave them out.
- Kira's own hooks (session status, notifications, MCP grants) always run first. They are never listed here and cannot be skipped. Yours run in addition.
- A provider without hook commands shows common hooks as not applied in the Settings page.

MCP servers:

- Define a server once. Kira writes it in each provider's format: a `--mcp-config` file for Claude, a per-run file named by `OPENCODE_CONFIG` for OpenCode.
- `enabled: false` keeps the definition and skips the server.

Secrets and variables:

- Use `${secret:name}` for a secret and `${NAME}` for a variable from your login shell, in `env` values and in MCP `command`, `args`, `url`, `headers` and `env`.
- Set the value in Settings, Secrets. It is write-only: Kira never shows it again. Deleting a reference from the file does not delete the value.
- Hook commands and status line commands take no references: the shell expands `$NAME`. Put the secret in `env` and read `$NAME` in the command.
- A reference resolves only at launch, into a per-run file readable by you alone or into the agent's environment. Kira removes the file when the agent exits.

Which launches use these settings: every terminal session in Kira Space, Continue in terminal (both apps) and Kira Space's headless agent steps and review rounds. Rebase runs use none. Smart scripts and smart steps use only the `mcp_servers` they tick: their tools and servers are chosen explicitly.

A save applies to agents started after it. A running session keeps its settings.
