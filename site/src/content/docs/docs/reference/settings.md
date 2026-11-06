---
title: Settings
description: Every Settings section in both apps, with defaults and where data lives.
sources:
  - apps/kira-space/frontend/src/studio/state/settings.ts
  - apps/kira-space/frontend/src/studio/state/settingsDomain.ts
  - apps/kira-space/frontend/src/state/settings.ts
  - apps/kira-space/frontend/src/state/settingsDomain.ts
  - packages/shared/domain/settings.ts
  - apps/kira-space/frontend/src/studio/workbench/settings
  - apps/kira-space/frontend/src/workbench/settings
  - apps/kira-space/internal/config/paths.go
  - apps/kira-space/internal/config/paths.go
  - internal/kirapaths/paths.go
  - internal/appsettings/agent.go
  - apps/kira-space/frontend/src/workbench/SettingsDialog.vue
  - packages/theme/src/i18n/en.json
sidebar:
  order: 1
---

## Kira Studio

Sections: Appearance, Data, Cache, Api, Database MCP, Prompts, Advanced.

Kira Studio has no agent section. It runs Claude Code with hermetic launches on.

### Appearance

- Font family. Default `Menlo, monospace`.
- Font size: 9 to 24. Default 12.
- Row density: compact or comfortable. Default comfortable.
- Word wrap. On by default.
- Row colouring by column type. On by default.

### Data

Default page size: 10, 100, 1000 or 10000. The default is 100.

### Cache

Level 2 cache budget: 8 to 1024 MB, default 64.

### Api

See [API client](../../studio/api-client/). Defaults: HTTP version 2, no timeout, 5 MB maximum response, SSL verification on, redirects followed up to 10, cookie jar disabled.

### Database MCP

The server switch, exposed connections and token. Off by default. See [Database MCP server](../../studio/database-mcp/).

### Prompts

See [Prompts](../prompts/).

### Advanced

- Operation log retention: 1 to 365 days, default 30.
- Expensive query threshold: 1000 to 1000000000 estimated rows, default 100000.
- Log level: off, error, warn, info or debug. Default info.
- A system notification when a popup waits in a window that is not in front. On by default.

## Kira Space

Sections: Appearance, Git, CLI tools, File types, Capture, Mobile access, the agent (Claude Code or OpenCode), Memory, Prompts, Advanced.

### Appearance

The same appearance settings as Kira Studio, plus inline blame (on) and date format (relative or absolute, default relative).

### Git

- Protected branches. Default `main`, `master` and `release/*`.
- Auto-fetch interval in minutes: 0 to 1440. Default 0, which is off.
- Git path. Empty by default, which finds git on its own.
- Graph font size. Default 0, which follows the font size.

### CLI tools

Shows the status of the command line tools Kira runs for pull requests and issues, with install and sign-in commands. The tools keep your credentials. Kira never sees them.

### File types

Makes Kira Space open code and data files from Finder, read-only. Switching a type off gives it back to the app that had it. Installed macOS app only.

### Capture

The clipboard capture shortcut opens a quick-capture popup. On by default. The default shortcut is `CmdOrCtrl+Shift+K`.

### Mobile access

Off by default. Port 1024 to 65535, default 7790. See [Phone view](../../space/phone/).

### Agent

This section is named after the current agent: **Claude Code** or **OpenCode**. See [Agent providers](../../space/agents/#agent-providers).

- **Agent**: the provider for new sessions and background steps. Default `claude`.
- **Hermetic agent launches**: on by default. See [Hermetic launches](../../space/agents/#hermetic-launches).
- Keep-awake. Off by default.
- Notifications per event, on by default. The finished and needs-input switches need an agent with hooks.
- **Show the start of the reply** in the notification. On by default.
- The usage display. On by default, and only for Claude Code.

### Memory

Pick the Claude model memory uses (`haiku`, `sonnet` or `opus`; default `sonnet`), connect Claude Code, download the semantic model, and install the memory check hook. See [Memory](../../space/memory/).

### Advanced

Git log level (default info) and the popup notification (on).

## Where data lives

| App | Home | Override | Database |
|---|---|---|---|
| Kira Studio | `~/.kira-studio` | `KIRA_HOME` | `kira.db` |
| Kira Space | `~/.kira-space` | `KIRA_SPACE_HOME` | `kira.db` |
| Memory | `~/.kira-memory` | `KIRA_MEMORY_HOME` | `memory.db` |

Each app also keeps `logs` and `llm-prompts.json` in its home. Kira Space also keeps `workflows/`, `agent/` (context and agent settings) and `agent-secrets.db` in its home. Kira Studio reads the same agent folder. The two apps use separate homes, so they can run side by side.
