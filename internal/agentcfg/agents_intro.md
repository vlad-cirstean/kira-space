This folder holds what every agent Kira starts is given, whatever the provider. Kira Space hosts and edits it. Kira Studio reads it for its own launches.

- `context.md`: the extra system prompt.
- `settings.yaml`: environment, permissions, MCP servers, hooks and per-provider settings.

With hermetic launches on (Settings, Agent), a launch skips the agent's own user config. Servers, hooks and permissions come from this folder and the project.

Never put a secret in either file. Settings refer to secrets by name; the values live sealed in `agent-secrets.db`, beside this folder.
