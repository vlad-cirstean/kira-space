# v0.2.26

Branch `chapter/v0.2.26`. First `P` is 447 (highest in v0.2.25 is P446). Draft: opens after v0.2.25 merges.

Source: user request. Phases run top to bottom, two phases in parallel where file ownership allows (worktree per stream). Scope is closed: any further phases go to v0.2.27. P450 is the shared foundation; v0.2.27 modules (Runs, API, git repos, connections) build on it. Ends with two code review rounds, then close, then PR.

| Phase | Title | Status |
|---|---|---|
| P447 | Shared keychain secret layer; Runs secret params stored encrypted | Done |
| P448 | Agent harness seams for a second real provider | Todo |
| P449 | OpenCode provider; pick the agent per interactive session | Todo |
| P450 | Config-as-files foundation: store, layout, validation, DB sync, interpolation, AGENTS.md | Todo |
| P451 | Workflows folder AGENTS.md | Todo |
| P452 | One shared agent context, rendered per provider | Todo |
| P453 | Agent settings: common plus per provider, MCP credentials from the keychain | Todo |
| P454 | Import and Export of agent config per provider | Todo |
| P455 | Hermetic agent launches per provider | Todo |
| P456 | Docs, site and README for agents and secrets | Todo |
| P457 | Code review (2 rounds) | Todo |
| P458 | Close chapter v0.2.26 | Todo |

## Rows

### P447 Shared keychain secret layer; Runs secret params stored encrypted

- Today the AES-GCM `Cipher` (keychain-held key, `Scope` as AAD, `KIRA_INSECURE_SECRETS` Linux fallback) lives in `apps/kira-studio/internal/secrets`, Studio only. Kira Space has no secret store.
- Today a Runs secret param (`scripts.Param.Secret`, `internal/scripts/smart.go`) is never stored: asked per run, env-only (`KIRA_PARAM_*`), recorded as `scripts.SecretMask`, refused in prompts. It blocks resume (`internal/scriptruns/resume.go`) and recurring schedules without confirm (`internal/scripts/schedule.go`).
- Hoist the cipher to repo-root `internal/secrets` (cipher, scope, status, `keyring_*`). Studio imports it; Kira Space builds one in its `appwire`. Scope strings and envelope prefix stay byte-identical: every stored ciphertext must still decrypt.
- A secret param can keep a saved value, sealed under a new scope. Plaintext never reaches the DB, logs, op log, run records, previews, contracts or any bridge read. The UI field is write-only; a reveal only if the plan reuses Studio's gated reveal (`apps/kira-studio/internal/apivars/reveal.go`, `localauth`).
- A run uses the saved value when none is typed, still env-only. Run output the app stores or shows in logs masks every secret value, across chunk boundaries. The live stream masks where the app already filters output; the plan states plainly any path not masked.
- A saved value lifts the resume and schedule refusals; the plan updates both rules and their copy.
- Deleting a script or param deletes its secret. Duplicate and export never copy plaintext.
- Keychain unavailable: clear error, fall back to asking per run.
- This layer is the one P453 reuses for agent credentials: shared keychain, one namespace per owner (script, agent config). Reveal only through the existing gated pattern, nothing new. The plan shapes the layer for that second consumer, without adding unused API.
- Test: backend flow in both apps (save, run, assert the env got the value, record, log, op log and preview hold only the mask, DB column holds ciphertext). UI spec on the same scenario (write-only field, run dialog, masked output). `secrets.spec.ts` stays green. Real `claude` "Smart scripts" row.

Result: Done. Cipher hoisted to repo-root `internal/secrets`; i18n keys moved to `backend.core.secrets.*`; scope strings and envelope unchanged. Both apps share one keychain item. New `secrets.Store` (sealed `secret_values`, migrations 0052 Studio, 0062 Space) and `secrets.Masker`; scope `script-param`. Saved value is write-only (`secretValues` on Create/Update: string saves, null clears, absent keeps); record holds names only. Prune on param remove, rename, un-secret, script and collection delete. Typed value wins over saved; env-only; resume and restore use it. Schedule without confirm refused only when a required secret has no saved value. Storage off: E_SECRET_STORE on save, runs ask. Masking in log sink, outcome fields and terminal tail. Not masked, in ARCHITECTURE Known open items: live terminal tab, Space phone terminal ring. ADE workflow smart steps out of scope. No reveal added. Tests: flow `TestSecretParam*` both apps, UI `automations-secrets` both apps; full Go flows, UI automations specs both apps, `secrets.spec.ts` and real `claude` Smart scripts rows green. Open: `.github/workflows/{pr,cache-warm}.yml` still use the old secrets path; apply `docs/pending-changes/.github__workflows__secrets-path.patch`. finish_step summary masking not covered end to end (fake agent summary is fixed text).

### P448 Agent harness seams for a second real provider

- `internal/harness` already has `Provider` with roles, `providers.New`, `Selector`, and `PrepareArgs.Provider` in `apps/kira-space/internal/ade/tracker.go`. Only Claude is registered. `internal/flowtest/fakeprovider` embeds the Claude provider for `MCPHost`, `Sandboxed` and env scrub.
- Gaps against a non-Claude CLI (OpenCode as the reference; the plan confirms each in code):
  - `Interactive.InteractiveCommand` returns a command string only. A provider that takes config through env (`OPENCODE_CONFIG`, `OPENCODE_CONFIG_CONTENT`) needs env too.
  - The session id is chosen up front (`--session-id`). OpenCode documents no way to preassign one. `Tracker` needs a path where the agent session id is learned after launch, with resume gated until it is known.
  - `InteractiveSpec.MCPConfig` is a Claude `--mcp-config` path; `MCPHost.WriteUserMCPConfig` writes Claude's file shape.
  - Every role is mandatory. A provider lacking `Structured` (JSON-schema call) or `Headless` must still register. Make such roles optional by type assertion, as `HookHost` is; each caller refuses clearly or falls back to the default provider (plan decides per caller: memory, smart scripts, ADE steps).
  - Hook events carry Claude names (`Stop`, `Notification`, `PreToolUse`) into `internal/agentnotify` and `ade.Tracker`. Map them to neutral kinds at the provider.
  - Tool vocabulary is Claude's (`ToolPolicy`, `--allowedTools`); confirm `MCPHost.ToolArgs` is the only mapping point.
- Fix each gap in `internal/harness` and its callers (`ade`, `internal/scriptruns`, `internal/terminal/bound.go`, `internal/agentnotify`, `apps/kira-space/internal/agentusage`). Claude behaviour unchanged.
- No transcript parsing exists outside `internal/harness/claude` (`stream.go` for headless stream-json); the plan confirms and keeps it there.
- Test: `flows/harnessflow` `TestSecondProviderRegistered` grows one case per seam (env-carried config, late session id, a missing optional role); `fakeprovider` exercises them. Existing flows unchanged. Real `claude`: `bun run test:claude`.

### P449 OpenCode provider; pick the agent per interactive session

- New `internal/harness/opencode`, registered in `providers.New` after Claude. Every CLI fact comes from the installed `opencode` (version recorded in ARCHITECTURE), not docs alone.
- OpenCode dev install (verified, v1.19.5): in a scratch dir outside the repo run `npm init -y && npm i --ignore-scripts opencode-ai`. It installs the launcher plus platform package `opencode-linux-x64`. Run `node_modules/opencode-linux-x64/bin/opencode` directly; the launcher refuses without postinstall under `--ignore-scripts`. Always set `HOME=$(mktemp -d)` so user config is never read or written. `--help`, `--version`, `debug config`, `debug paths` need no auth. The Claude Code auto-mode classifier blocks a subagent told to install an external package, and blocks a probe that runs a model prompt: run the install from the orchestrating session or have the user approve.
- Verified on 1.19.5 (throwaway HOME):
  - TUI and `run` take `-s/--session <id>` (continue an existing id), `-c/--continue`, `--fork`, `-m provider/model`, `--agent`, `--pure` (no external plugins), `--auto` (auto-approve permissions); `run` also `--format default|json`, `--title`, `--dir`, `--attach <url>`, `--port`, `--variant`. TUI also `--prompt`, `--mini`. Subcommands: `serve`, `attach`, `acp`, `mcp {add,list,auth,logout,debug}`, `session list --format json`, `export [sessionID] [--sanitize]`, `import <file>`, `models`, `agent`, `debug {config,paths,agent,skill,info}`.
  - Paths: config `$XDG_CONFIG_HOME/opencode` (default `~/.config/opencode`), data `~/.local/share/opencode`, state `~/.local/state/opencode`, cache `~/.cache/opencode`, tmp `/tmp/opencode`. `debug paths` prints them.
  - Config: `opencode.json` in the config dir and in the project both load and merge; `mcp` entries are `{type:"local",command:[...]}` or `{type:"remote",url}`. `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` and `OPENCODE_CONFIG_CONTENT` layer on top of user config, they do not replace it. `OPENCODE_DISABLE_PROJECT_CONFIG=1` drops the project `opencode.json`. Also present in the binary: `OPENCODE_PERMISSION`, `OPENCODE_AUTH_CONTENT`, `OPENCODE_DISABLE_CLAUDE_CODE`, `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT`, `OPENCODE_DISABLE_CLAUDE_CODE_SKILLS`, `OPENCODE_DISABLE_EXTERNAL_SKILLS`, `OPENCODE_DISABLE_DEFAULT_PLUGINS`, `OPENCODE_PURE`.
  - No flag assigns a new session id up front: `--session` only continues an existing one. No model key is available, so these stay unverified at spec time. The plan handles each defensively (tolerant parsing, discovery fallbacks), not behind a real-test gate: whether `--session` with an unknown id creates it; the `run --format json` event stream and its completion signal; session id discovered after launch via `session list --format json` (fallback when events lack it); global `AGENTS.md` location (docs: `~/.config/opencode/AGENTS.md`) and which files reach the prompt.
- Facts to verify: TUI flags (`--session`, `--continue`, `--prompt`, `--model provider/model`, `--agent`), `opencode run --format json` events, `opencode session list --format json`, `opencode export`, config (`opencode.json` `mcp` local and remote, `instructions`, `permission`), plugins for session events, model naming.
- Roles: `Locator`, `Interactive`, `MCPHost` (opencode `mcp` shape, tool names, user servers from `~/.config/opencode/opencode.json`), `Sandboxed` (config and data dirs). `Headless` only if `run --format json` can carry an ADE step to its `finish_step` report; else the P448 optional-role refusal. `HookHost` through a Kira plugin posting to the existing hook listener, if feasible. No `UsageReporter` unless found.
- Model catalog of `provider/model` ids; the plan decides static aliases or `opencode models`.
- Agent choice: global default `agent.provider` setting, overridable per workflow step. Pressing "+" for a manual interactive session shows both agents; the user picks. Other interactive launches (ADE Start, Take over into a new session, interactive review agent, Runs Continue in terminal) use the step override, else the default. Picker lists registered providers whose CLI is found.
- Resume and Take over of an existing conversation keep its stored provider. No cross-provider resume.
- The Settings Agent pane gets the default-provider picker (today none exists). The workflow step editor gets the per-step override.
- Test: flow test in `adeflow`/`harnessflow` with a fake `opencode` persona (extend `internal/flowtest/fakeagent`): launch with each provider, resume keeps the provider. UI spec: extend `agent-provider.spec.ts` with the "+" picker, default setting and step override. No real OpenCode model tests (no key available). OpenCode is covered by fake-CLI flow tests plus no-model probes against the installed CLI (`--help`, `debug config`, `debug paths`, `debug agent`), documented in `docs/DEV_ENVIRONMENT.md`.

### P450 Config-as-files foundation: store, layout, validation, DB sync, interpolation, AGENTS.md

- Config lives as plain files in the Kira Space app folder, in folders alongside the workflows folder. Config is read from files, not edited through a rich UI. Shared by Studio and Space; Studio's shared definition rule for agent config still applies.
- Build once, reused by P451 to P454 and every v0.2.27 module: folder layout, file store with atomic write, format per thing (yml, json or text; each plan picks), schema validation with errors shown to the user, load and file watch with external edits, bad-edit handling (last good state kept), `${ENV}` and keychain secret interpolation at runtime (files hold references only, never plaintext; P447 layer).
- DB sync mechanism: per field state (stays in SQLite) versus config (moves to files), sync direction, migration of existing rows to files, conflict with the Studio/Space DB split. Plan decides the mechanism and states it once for all modules.
- Config versus state rule (user-approved). Every module plan applies it:
  - Files hold config: what a user or agent deliberately sets and expects to survive a reinstall. Names, descriptions, permissions, MCP settings, hooks, context text, per-repo options (worktree prepare script etc.), filter colors, API definitions, script and step definitions. Secrets as references only, never values.
  - SQLite keeps state: anything the app produces or changes on its own. Run history and logs, caches, op-log entries, open tabs and layout, "was running at quit" markers (P441), last-used timestamps, connection health, query history; anything derived, large or high-churn.
  - Borderline, decided once here:
    - Window and layout preferences: SQLite (per machine, high churn).
    - Saved queries and history: SQLite unless user asks otherwise.
    - Per-machine values (paths, ports): files need a per-machine override if the folder is ever synced or committed.
  - Each module plan (v0.2.27 P459 to P462) lists every field as config or state against this rule and justifies each exception.
- `AGENTS.md` convention: per config folder, explains how each file is configured and updated so an AI agent can configure everything. Plan decides generation (template per module versus generated from the schema) so docs cannot drift from the validator.
- Minimal UI contract for modules: view, open the file, plan decides plain file edit or a small edit action.
- Test: flow test (edit file, DB follows, invalid edit rejected and state kept, external edit picked up, secret reference resolved at runtime and absent from DB and logs). UI spec: validation error shown.

### P451 Workflows folder AGENTS.md

- Own phase: workflows already exist as files, so this needs the P450 `AGENTS.md` convention but no new store. Not folded into P450 (separate surface, own review) or P456 (docs phase is not the product).
- `AGENTS.md` in the workflows folder lists all available options and how to update them, enough for an AI agent to author workflows.
- Generated or checked against the workflow schema (P450 convention) so it cannot drift from the validator.
- Test: flow test asserts every schema option appears in the file.

### P452 One shared agent context, rendered per provider

- Agent context is one definition shared by both apps; Kira Studio owns it, Kira Space consumes it. The Agents module (Kira Space `ade`) shows it and opens it for editing. Stored as a file in an agent folder in the app folder, per the P450 foundation (format by plan, likely markdown text); the editor is minimal: view, open and edit the file. The file is the source; the plan decides what, if anything, stays in SQLite.
- Every agent the app launches gets it, whatever the provider: interactive, Take over, resume, headless steps, smart scripts.
- New neutral field on the harness launch specs; only the provider maps it. Claude candidates: `--append-system-prompt`, or a generated `CLAUDE.md` in an app dir passed with `--add-dir` plus `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1`. OpenCode: `instructions` in generated config. The plan picks per provider and verifies with the real CLI.
- Claude's `--system-prompt-snapshot` is on by default: a resumed conversation keeps its recorded prompt until compaction. The plan states which launches see a context edit, and the editor says so.
- User-authored text stays out of `internal/llmprompts`, per that package's rule. App prompts keep their role.
- Test: flow test sets the context, launches each provider interactive and headless (fake personas record argv, env and files), asserts the rendered context arrived. UI spec: edit, save, reload, same text.

### P453 Agent settings: common plus per provider, MCP credentials from the keychain

- Agent settings are one definition shared by both apps; Kira Studio owns it, Kira Space consumes it. Keychain secrets are shared. Stored as files in the agent folder per the P450 foundation (yml, json or text per thing; plan decides). Env vars and secrets interpolate at runtime; files hold references only.
- One set of common settings supported by every provider; the plan derives it from what both CLIs support (candidates: model per provider, permission mode, allowed and denied tools, MCP servers, env vars).
- Plus one provider-specific section per provider, validated by that provider's package.
- Hooks are a managed setting like the others: user adds, edits, removes hooks in the shared definition (common) and in per provider overrides. Studio's definition is the shared one.
- Kira's own hooks (status, notifications, MCP grants; injected through its listener) stay always on. Never listed as user hooks, never overridable. User hooks run in addition.
- Providers lacking a hook mechanism (OpenCode: plugin-based, per P449 facts) map hooks to their native form or show them unsupported in the resulting-config view; plan decides.
- P444 (v0.2.25) runs claude with `disableAllHooks` for runs loading user settings; P453/P455 supersede that.
- `internal/harness/claude/hooks/statusline.go` `userStatusLine` opens whole user and project `settings.json` (can hold `env`/`apiKeyHelper`) though it keeps only `statusLine`. P453/P455 replace that read with the app-supplied settings.
- MCP servers defined once in the app (stdio command, args, env; http URL, headers), rendered per provider (Claude `--mcp-config`, OpenCode `mcp`).
- Credentials in MCP env, headers, args and settings env are references to app secrets, interpolated like Runs params and Studio API vars (`apivars` two-stage `{{name}}` substitution). The plan picks the syntax. Values are stored through the P447 layer.
- A reference resolves only at launch, into a 0600 per-run file or env, removed when the agent exits. Never plaintext in the DB, logs, op log, contracts or a file kept after the run.
- Agent folder `AGENTS.md` (P450 convention) explains every file: context, settings, hooks, MCP.
- UI in the Agents module, shadcn-vue primitives, minimal: view, open and edit the file or a small edit action (plan decides), secret fields write-only. The editor shows, per provider, the resulting config: the exact native config that provider receives (hooks in the provider's native form), secrets masked or shown as references, next to the editable common and provider-specific settings.
- Test: flow test defines a server with a secret header, launches each provider, asserts the resolved value reached the agent, DB and logs hold only the reference, the file is gone after exit, the resulting-config view matches the rendered native config with the secret masked. UI spec on the same scenario. Real `claude` "Claude config isolation" row.

### P454 Import and Export of agent config per provider

- Files are now the config, so scope shrinks to moving native provider config in and out; the plan drops any step the file store makes redundant (state sync, conflict handling beyond preview). Each supported provider gets an Import and an Export button. User-level files only; project-level files are never read or written.
- Import reads the provider's native config into the shared context and settings files (P452, P453). Claude: `~/.claude/settings.json`, `~/.claude/CLAUDE.md`, user `mcpServers` in `~/.claude.json`. OpenCode: `~/.config/opencode/opencode.json`, `~/.config/opencode/AGENTS.md`.
- A literal credential found on import becomes a keychain secret plus a reference in the file (credentials rule kept). A preview lists every item, with per-item selection and conflicts, before apply.
- Export renders app context and settings into the provider's native files. Diff preview, explicit confirm, backup of each overwritten file. Secrets export as env references only, never plaintext; no plaintext option.
- Hooks (P453) import and export too: user-level files only, hook commands exported as written, env references only for secrets. Same rule as MCP.
- Today the app never writes the user's agent config (`TestRealClaudeSettingsUntouched`, statusline read-only). Export is the one user-initiated exception; that test keeps passing for every launch path.
- Test: flow test per provider on a temp `HOME`: import fixture files, assert app state and secrets; export, assert files and backup. UI spec: preview, selection, confirm.

### P455 Hermetic agent launches per provider

- Goal: an app-launched agent loads no user-level config for three things only: settings, MCP tools, context. The app supplies them instead (P452 context, P453 settings and MCP, Kira's own hooks and MCP grants).
- Hermetic launches drop user-level hooks from the machine's own config; the app-supplied hooks list from P453 replaces them.
- Out of scope: project-level files (project `CLAUDE.md`, `.claude/`, project MCP) still load. Login, credentials and keychain are left alone: no `--bare`, nothing that skips auth or moves login. Org-managed policy still applies.
- Plan researches isolation that blocks or replaces user-level settings, user MCP servers and user context while login stays intact. It verifies against installed `claude --help` and real behavior (`claude` 2.1.316 at spec time), findings in the plan.
- Claude controls to evaluate: `--setting-sources` (Kira passes `""` for structured calls, `user` or all for headless today), `--settings`, `--strict-mcp-config`, `--mcp-config`, `CLAUDE_CONFIG_DIR` only if login and trust survive, `claudeMdExcludes` for `~/.claude/CLAUDE.md`. Check whether dropping `user` from `--setting-sources` also drops `~/.claude/CLAUDE.md`. Reject any control that breaks login.
- OpenCode controls to evaluate: `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR`, `OPENCODE_CONFIG_CONTENT`, global `~/.config/opencode/AGENTS.md`, `OPENCODE_DISABLE_CLAUDE_CODE` (else it reads `~/.claude/CLAUDE.md`). Verified on 1.19.5 (install steps in P449): user config lives under `$XDG_CONFIG_HOME/opencode` and auth under the data dir `~/.local/share/opencode`, so pointing `XDG_CONFIG_HOME` at an app dir moves user config (user `mcp` and `model` gone from `debug config`) and leaves login in place. `OPENCODE_CONFIG`, `OPENCODE_CONFIG_DIR` and `OPENCODE_CONFIG_CONTENT` only layer on top, so they cannot hide user config alone; the app's own config goes in via the redirected config dir or `OPENCODE_CONFIG_CONTENT`. `opencode auth` credentials stay untouched. Plan-time checks: that an empty redirected config dir also drops the global `AGENTS.md`; that `OPENCODE_DISABLE_CLAUDE_CODE*` stops `~/.claude/CLAUDE.md` and skills loading; that the data dir holds only auth-relevant state the app may share.
- Each provider maps hermetic mode in its own package. Hermetic mode is on by default, with a user toggle in Settings Agent. It applies to every agent launch: interactive, Take over, resume, workflow steps, headless, smart scripts.
- Cannot be isolated, stated in ARCHITECTURE "Known open items" and in the setting's description: Claude managed settings (`managed-settings.json`, MDM, server-managed) and managed `CLAUDE.md`; OpenCode managed config files and macOS managed preferences; anything else the research finds.
- Test: flow test (`claudeflow` isolation pattern) on a temp `HOME` seeded with a user `CLAUDE.md` marker, user settings and permissions, and a `~/.claude.json` MCP server, plus a project `CLAUDE.md` and project MCP; assert the argv and env the app builds per provider block the user items and keep the project items. Real `claude`: new `TestHermeticSession` in `apps/kira-space/tests/claude/` reads the stream-json `system` init event (user MCP servers absent, project MCP present) and one haiku prompt shows the user marker absent, project marker present, and login working; new row in `docs/DEV_ENVIRONMENT.md` "Real `claude` tests". OpenCode: fake-CLI flow test of the same scenario, plus a no-model probe asserting `debug config` under the app's redirected `XDG_CONFIG_HOME` omits the seeded user MCP and model. No real OpenCode model test.

### P456 Docs, site and README for agents and secrets

- Site docs: `site/src/content/docs/docs/space/agents.mdx` (providers, picker, context, settings, config files and AGENTS.md, Import and Export, hermetic mode), `shared/runs.md` (stored secret params), `reference/settings.md`. Both READMEs.
- `docs/ARCHITECTURE.md` "Agent harness" and "Known open items"; `docs/DEV_ENVIRONMENT.md` OpenCode install and no-model probes.
- Screenshots re-captured where a changed surface appears (`bun run screenshots`). `bun run site:check` passes.

### P457 Code review (2 rounds)

One row, two rounds. Each round: one Opus review over the full diff since `main`, findings file under `plans/`, then one Sonnet fixer. Then close, then PR, as in v0.2.25.

### P458 Close chapter v0.2.26

Hand-written close per CLAUDE.md "Closing a chapter". Push, open PR to `main`.
