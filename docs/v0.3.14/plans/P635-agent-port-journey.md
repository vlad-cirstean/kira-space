# P635 Agent config import/export journey (J-HAR-002)

Stream B of batch 1 (`batch-1.md`). Worktree `/home/user/kira-space-stream-b`, branch
`stream-b-v0.3.14`, base `fa00f7bf2`.

## Journey (from `git show 440706b3e^:docs/v0.3.13/plans/journeys-ade.md`)

- Steps: import from user Claude config with hooks, MCP servers with secrets, statusline; preview
  masked diff; apply; export back to provider after edits; backups listed; undo via backup.
- Oracle: backup files exist with original bytes; exported config parses and equals settings;
  secrets masked in preview and never written to `settings.yaml` in clear.
- Risk: secret in diff or YAML.

## What exists (read, not assumed)

- `internal/agentport`: `List`, `ImportPreview`, `ImportApply`, `ExportPreview`, `ExportApply`,
  `Cancel`. Bridge `apps/kira-space/internal/bridge/agentport.go`. ARCHITECTURE.md "Agent config
  import and export (P454)".
- `backup()` (`backup.go:31`) writes `<real path>.kira-backup-<UTC stamp>[-N]` beside the resolved
  file. `ExportApply` backs up each written file and `~/.claude.json` before the CLI edits it.
  `PortResult.Backups` returns their paths; `AdePortResult.vue` shows base names.
- Gaps against the journey:
  1. No way to list backups after the apply result closes. Only the one-shot result lists them.
  2. No undo. `ExportApply` doc says "the backups are the undo"; nothing restores one.
  3. Flow tests never assert: statusline export, export after editing imported settings, exported
     file parse plus structural equality with settings, settings.yaml raw text free of literals
     after import, backups after a second export, any restore.
  4. Test literals all start `sk-`, so value-shape masking hides them. Location and name
     classification (`classify.go`) are never the only thing masking a literal in a flow test.
  5. UI spec never sees statusline, backups list or restore.

## Decision: build list and restore

Journey says "backups listed; undo via backup" as user steps. Today neither exists in the app, so
the oracle cannot be met by tests alone. Build the smallest surface that meets it, inside
`agentport`, no new bound service (count stays 37).

- `Backups(ctx, BackupsArgs{Provider})` returns `[]Backup`, newest first.
- `Restore(ctx, RestoreArgs{Provider, File})` returns `PortResult`.
- Restore never writes `~/.claude.json`. Kira never writes that file (the CLI does, ARCHITECTURE
  P454), and Claude rewrites it constantly (sessions, project trust): a byte restore drops that
  state. Its backups list with `restorable: false` and a reason (restore by hand). Restore of the
  other roles (Claude `settings`, `context`; OpenCode `config`, `context`) is a file write the
  package already does on export.

Declined alternative: re-register servers from a `.claude.json` backup through `claude mcp
add-json`. Backup holds literal credentials; passing them through argv breaks the "no literal in
argv" rule export keeps (env refs only). Not worth it for an undo path.

### Wire types (`internal/agentport/backups.go`, new)

```go
type BackupsArgs struct {
	Provider string `json:"provider"`
}

type Backup struct {
	Role       string `json:"role"`       // settings, context, mcp, config
	Target     string `json:"target"`     // display path of the file it restores (~/...)
	File       string `json:"file"`       // backup base name, the Restore key
	Taken      string `json:"taken"`      // RFC 3339 UTC, parsed from the stamp
	Size       int64  `json:"size"`
	Restorable bool   `json:"restorable"`
	Reason     string `json:"reason"`     // i18n text when not restorable
}

type RestoreArgs struct {
	Provider string `json:"provider"`
	File     string `json:"file"`
}
```

### Rules

- Targets: `s.targets(ctx, provider)`; resolve each with `resolveTarget` (backups sit beside the
  real file, so a symlinked `settings.json` lists backups from the link target's directory).
- Listing: for each target, `os.ReadDir(filepath.Dir(real))`, keep names matching
  `^` + `regexp.QuoteMeta(filepath.Base(real))` + `\.kira-backup-(\d{8}T\d{6}Z)(?:-(\d+))?$`.
  `os.Lstat` must report a regular file (skip symlinks, dirs). Missing dir is an empty list, not an
  error. Sort by stamp desc, then N desc (no suffix is N 0). Never read backup contents to list.
- Restore key is the base name only. Server finds the target whose pattern matches `File`; no match
  (including any `/`, `\`, `..`, another provider's file, a symlink) is `E_BAD_REQUEST`
  `backend.core.agentPort.restore.unknown.body`, nothing written.
- Role `mcp`: `E_BAD_REQUEST` `backend.core.agentPort.restore.claudeJSON.body`.
- Read backup with `readIfExists(path, cap)`, cap per role (`maxSettingsFile` for settings/config,
  `maxContextFile` for context). Read current target the same way.
- Current bytes equal backup bytes: write nothing, return a note
  (`backend.core.agentPort.restore.same.body`).
- Else, when current exists, `backup(real, cur, mode, s.o.Now())` first (restore is itself
  undoable), then `writeFile(real, data, defMode)` (0600 settings/config, 0644 context, existing
  mode kept). Result: `Written: [real]`, `Backups: [new backup]`.
- Bytes never cross the bridge, never logged. Errors carry file base names only, as today.
- Bridge: add `Backups` and `Restore` to `bridge.AgentPortService`.

## UI

- `AdeSettingsResult.vue`: third link `ade-port-backups-open` ("Backups") beside Import and Export.
- New `apps/kira-space/frontend/src/ade/v2/port/AdePortBackupsDialog.vue`, `<script setup
  lang="ts">`, Tailwind classes only, shadcn-vue `Dialog`, `Button`, `Badge`; `Empty` for no
  backups. Rows `ade-port-backup-<file>`: file name (`font-data`), role, `formatDateTime(taken)`
  (`@workbench/behaviour/format`), size. Restore button `ade-port-restore-<file>`; disabled when
  not restorable, reason shown as text `ade-port-backup-reason-<file>`.
- Confirm through the existing Pinia `useConfirmDialogStore().confirm({ title, message,
  confirmLabel, tone: 'danger' })` (pattern: `ConfigSyncPane.vue:85`). Message names target and
  backup and says the current file is backed up first.
- After restore: show `AdePortResult` (written, new backup), errors via `errorText`.
- `queries.ts`: `usePortBackups(provider)` as TanStack `useQuery` key
  `['agentPort', 'backups', provider]`; `useRestore()` `useMutation`, on success invalidate
  backups key. `useExportApply` on success also invalidates `['agentPort', 'backups']`.
- `types.ts`: `Backup`, `RestoreArgs`.
- `bridge/index.ts`: `agentPortBackups`, `agentPortRestore`. Regenerate bindings with
  `wails3 task common:generate:bindings` (DEV_ENVIRONMENT; `-names` matters).
- `tests/ui/support/ipcChannels.ts`: `agentPortBackups: 'kira:agentport:backups'`,
  `agentPortRestore: 'kira:agentport:restore'`. `mockRuntime.ts` CHANNEL_TO_FQN:
  `AgentPortService.Backups`, `AgentPortService.Restore`.
- i18n `packages/theme/src/i18n/en.json`: UI keys under `space.ade.port.backupList.*` (do not
  reshape existing `space.ade.port.backups.label`); backend keys
  `backend.core.agentPort.restore.{unknown,claudeJSON,same}.body`. Follow copy checks
  (`scripts/copy/check-copy.ts`, `check-catalog.ts --unused`).

## Flow half (Go, `apps/kira-space/internal/flows/harnessflow`)

New file `port_journey_test.go`, `TestAgentPortClaudeJourney`; extend `TestAgentPortOpenCode`.
Existing `TestAgentPortClaude` and its contracts stay unchanged.

Fixtures, new, under `testdata/port/` (never under `docs/`):

- `journey-settings.json`: env `JIRA_TOKEN` (`@JIRA@`), `NODE_ENV`; permissions allow, deny,
  `defaultMode: plan`; model `opus`; hooks `PreToolUse` matcher `Bash` timeout 15 (common
  tool_start), `Notification` no matcher (common), `SessionStart` matcher `startup` (native),
  `Stop` type `prompt` (skipped); `statusLine {type: command, command: "~/bin/line.sh", padding: 2,
  refreshInterval: 5}`; `apiKeyHelper` with `@APIKEY@`; `theme`.
- `journey-claude.json`: `oauthAccount` with `@OAUTH@`; `mcpServers` `gh` (http, header
  `Authorization: Bearer @GH@`), `notes` (stdio, env `API_KEY: @NOTES@`, arg `${HOME}/notes`),
  `hdr` (http, header `X-Team: @TEAM@`).
- `journey-CLAUDE.md`.

Literals: own helper, shape-free values (`"p635-" + uuid.NewString()`, dashes kept, no `sk-`, not
pure hex), so masking must come from location (`env`, `headers`) and name. Reuse `portLiterals`
methods (`seed`, `scan`, `absent`) by building the map directly.

Setup: `flowharness.New(t)`, `defer claude.SetMCPListTTL(0)()`, default settings.yaml and empty
context. `settings.json` is a symlink from the start: real file `~/dotfiles/settings.json`.

Steps and assertions:

1. Import preview.
   - `lit.absent(t, "import preview", prev)`.
   - All mapped items `new`: `env/JIRA_TOKEN`, `env/NODE_ENV`, `claude/model`,
     `claude/permission_mode`, `claude/status_line`, allow/deny rules, two common hooks, native
     `SessionStart`, `mcp/gh`, `mcp/notes`, `mcp/hdr`, `context`. `Stop` prompt hook skipped.
   - Secret slots: `env-jira_token`, `gh-authorization`, `notes-api_key`, `hdr-x-team` (verify
     the sanitized name the code derives; assert the real one). Each item text holds `${secret:`.
   - `app.Contract(t, "agent-port-journey", "import", prev, flowtest.Mask("token"))`.
2. Apply all (context `replace`).
   - `lit.absent` on result.
   - settings.yaml raw bytes (`app.ConfigDir("agent")/agentcfg.File`): no literal; holds each
     `${secret:<slot>}`; `status_line` with `command`, `padding: 2`, `refresh_interval: 5`;
     `permission_mode: plan`; `model: opus`; hooks present.
   - Sealed store (`AgentSecrets.Store.Open`) holds each literal under its slot.
   - Native files byte-equal to seeds.
3. Edit in Kira. Edit current settings text with exact `strings.Replace` (fail when a replace
   misses): model `sonnet`, add allow `Bash(go test:*)`, status_line padding 3, `NODE_ENV:
   development`, add hook `fmt` (`tool_end`, matcher `Edit`, command `gofmt -w`), add stdio server
   `tool` (`/usr/bin/tool`, args `["--home", "${HOME}"]`). Save context `Be terse.\n`.
4. Export preview.
   - `lit.absent(t, "export preview", ep)` (covers `Before` and `After`).
   - settings `Before` holds `••••`; `After` holds `"refreshInterval": 5`, `"padding": 3`,
     `"Bash(go test:*)"`, `"model": "sonnet"`.
   - `Skipped` holds `env.JIRA_TOKEN` with non-empty reason (a `${secret:}` env ref has no Claude
     form; by design, `claude_export.go` env loop).
   - Servers: `gh`, `notes`, `hdr`, `tool` with actions as computed (assert real values).
   - `envNames` lists `HOME` and each `KIRA_SECRET_*` name.
   - `app.Contract(..., "agent-port-journey", "export", ep, flowtest.Mask("token"))`.
5. Export apply (`settings`, `context`, servers).
   - Parse: real `settings.json` through `hujson.Standardize` then `json.Unmarshal` succeeds;
     `.claude.json` too.
   - Equals settings, key by key, hand-written expected values (not re-rendered by the code under
     test): `model`, `permissions.defaultMode`, allow and deny contain every settings rule,
     `statusLine` deep-equal `{type: command, command: "~/bin/line.sh", padding: 3,
     refreshInterval: 5}`, each settings hook present once as `{matcher, hooks: [{type: command,
     command, timeout}]}` under its Claude event (timeout 60 when settings omits it), `env.NODE_ENV
     == "development"`.
   - `.claude.json` `mcpServers` (written by the fake CLI): `gh` header `Bearer
     ${KIRA_SECRET_GH_AUTHORIZATION}`, `notes` env `${KIRA_SECRET_NOTES_API_KEY}`, `hdr` header
     reference, `tool` args `--home`, `${HOME}`. `oauthAccount` kept.
   - Union rule (ARCHITECTURE P454 "lists unioned, nothing of the user's removed"): user keys
     `theme`, `apiKeyHelper` remain; each literal occurs in the exported file exactly as often as
     in the seed (export never copies a credential into a new key).
   - settings.yaml bytes unchanged by the export.
   - No fake-CLI argv recording holds a literal.
   - Round trip: `ImportPreview` again; every item settings holds is `same`. Any item not `same`
     must be a user leftover the union keeps; list each explicitly with why.
6. Backups listed.
   - `svc.Backups(claude)`: three entries (`settings`, `context`, `mcp`), files beside the real
     paths (dotfiles dir for settings). Bytes of each equal the seed (`journey-settings.json`,
     `journey-CLAUDE.md`, `journey-claude.json`). `mcp` entry `restorable: false`, reason set.
   - Second edit (model `haiku`) and export: settings has two backups, newest first, distinct names
     (same-second `-1` suffix sorts above no suffix); older one still equals the seed, newer equals
     the first export's output.
   - `app.Contract(..., "agent-port-journey", "backups", list, flowtest.Mask("file", "taken"))`
     (stamps vary per run; check `flowtest.Mask` masks nested slice fields, else mask in the test
     before the contract call).
7. Undo via backup.
   - `Restore` the oldest settings backup: real file bytes equal the seed exactly; symlink still a
     symlink; result `Backups` names a new backup whose bytes equal the pre-restore file.
   - Restore the `CLAUDE.md` backup: bytes equal the seed.
   - Restore the same backup again: nothing written, note returned, no new backup.
   - `ImportPreview` after restore: `claude/model` `conflict` (opus vs haiku), status_line
     `conflict`.
   - Refusals, each `E_BAD_REQUEST`, file bytes and backup count unchanged: `.claude.json` backup,
     unknown name, `../` name, absolute path, OpenCode-provider call with a Claude backup name, a
     symlink named like a backup.
   - `app.Contract(..., "agent-port-journey", "restore", res)` with paths placeholdered by the
     harness.
8. `lit.scan(t, app)`: no literal anywhere under app root outside Home, Work, FakeDir.

`TestAgentPortOpenCode` extension, after its export apply:

- `opencode.jsonc` parses via `hujson.Standardize`; `model` and `mcp.fresh` hold the expected
  values and `{env:KIRA_SECRET_OC_TOKEN}`.
- `Backups(opencode)` lists the config backup, `restorable: true`.
- `Restore` it: bytes equal `seedCfg`; a new backup holds the exported bytes.

No unit tests: list and restore are a regex, a sort and two file calls (WORKING_AGREEMENT Testing).

## UI half (`apps/kira-space/tests/ui/ade-agent-port.spec.ts`)

Fixtures are the flow test's contracts (`agent-port-journey.json`). Same scenario, same results:

- `journey: import masks secrets, export shows statusline and skips, backups list, restore`:
  - Import dialog from `import` contract: every `ade-port-secret` shows `••••`; dialog never holds
    `p635-`; `claude/status_line` row `new`.
  - Export dialog from `export` contract: settings diff modified side holds `refreshInterval`;
    original side holds `••••`, not `p635-`; `ade-port-skipped` holds `env.JIRA_TOKEN`.
  - Backups dialog from `backups` contract: rows in contract order; `.claude.json` row restore
    disabled, reason visible.
  - Restore the oldest settings backup: confirm dialog appears; confirm; exactly one
    `agentPortRestore` call with `{ provider: 'claude', file }`; result shows written and new
    backup from `restore` contract; `agentPortBackups` called again (list reloads).
- `restore cancelled at the confirm sends nothing`.
- `export apply reloads the backups list` (open Backups after an export apply: second
  `agentPortBackups` call).

## Fix every bug found

- Write the failing assertion first. Root-cause. Fix in product code (`internal/agentport`,
  frontend port dir), never by weakening the oracle.
- Commit each fix on its own: `fix(agentport): <what>`, with the test that caught it.
- Append each bug to "Bugs found" below in the same commit (resumability).
- Behaviour that is designed (cite the ARCHITECTURE line) gets asserted as designed, not "fixed".
- A fix needing a design decision outside this phase: stop, write it under "Bugs found" as open,
  report to orchestrator for a follow-up `SPEC.md` row. Never pin silently.

Probes to check explicitly (bug only if real):

- statusline `refresh_interval` to `refreshInterval` round trip; padding 0 handling.
- Hook without timeout: export writes 60; re-import must be `same`, not a duplicate.
- Editing an imported hook's command: union leaves the old one in the user's file. Designed;
  assert and list as leftover in step 5.
- Location-only literal (`X-Team` header) masked in import preview, export `Before` and `After`.
- Same-second backups: `-N` ordering in `Backups`.
- Export when `CLAUDE.md` is absent: no backup, no error.

## Files

New:

- `internal/agentport/backups.go`
- `apps/kira-space/internal/flows/harnessflow/port_journey_test.go`
- `apps/kira-space/internal/flows/harnessflow/testdata/port/journey-settings.json`,
  `journey-claude.json`, `journey-CLAUDE.md`
- `apps/kira-space/tests/contract/agent-port-journey.json` (generated)
- `apps/kira-space/frontend/src/ade/v2/port/AdePortBackupsDialog.vue`

Changed:

- `internal/agentport/export.go` (comment "backups are the undo" points at `Restore`; fixes only
  as found)
- `apps/kira-space/internal/bridge/agentport.go`
- `apps/kira-space/internal/flows/harnessflow/port_test.go` (OpenCode extension)
- `apps/kira-space/frontend/src/ade/v2/port/{queries.ts,types.ts}`
- `apps/kira-space/frontend/src/ade/v2/settings/AdeSettingsResult.vue`
- `apps/kira-space/frontend/src/bridge/index.ts`
- `apps/kira-space/tests/ui/{ade-agent-port.spec.ts,support/ipcChannels.ts,support/mockRuntime.ts}`
- `packages/theme/src/i18n/en.json` (shared, additive)
- `docs/ARCHITECTURE.md` section "Agent config import and export (P454)" (shared, additive):
  bound methods list adds `Backups`, `Restore`; restore rules; tests line adds
  `TestAgentPortClaudeJourney` and contract `agent-port-journey.json`.

Not touched: `docs/v0.3.14/SPEC.md` (Result at landing), `docs/DEV_ENVIRONMENT.md` (Stream A).

## Implementer steps (Sonnet, one sequential agent)

1. `backups.go` plus bridge methods. `go build ./... && go vet ./internal/agentport/`. Commit
   `feat(agentport): list and restore export backups`.
2. Fixtures and `TestAgentPortClaudeJourney` steps 1 to 8; OpenCode extension. Run, fix what fails
   per "Fix every bug found". Generate contract with `KIRA_CONTRACT=write`, rerun without it.
   Commit `test(harnessflow): agent config journey J-HAR-002` (fixes as separate commits).
3. Bindings regen, `bridge/index.ts`, types, queries, dialog, settings link, i18n. Commit
   `feat(ade): agent config backups and restore`.
4. UI spec, channels, mockRuntime. Commit `test(ui): agent port journey`.
5. ARCHITECTURE.md section. Commit `docs: agent port backups and restore`.
6. Verification below, fix and commit anything red.

## Verification

- `go build ./...`
- `go test -count=1 ./internal/agentport/ ./internal/conventiontest/`
- `go test -count=1 -race -run 'TestAgentPort' ./apps/kira-space/internal/flows/harnessflow/`
- `KIRA_CONTRACT=write go test -count=1 -run 'TestAgentPort' ./apps/kira-space/internal/flows/harnessflow/`
  then rerun without; `git status apps/kira-space/tests/contract` shows only
  `agent-port-journey.json` new (existing port contracts unchanged).
- `bun run lint`, `bun run lint:go`, `bun run lint:dead`, `bun run typecheck`
- `bun run build:test && bunx playwright test --config=apps/kira-space/playwright.config.ts
  --project=ui apps/kira-space/tests/ui/ade-agent-port.spec.ts` (targeted only; full UI in PR CI)
- Real claude (DEV_ENVIRONMENT row "Agent config import and export (P454)"; verify auth with its
  probe first): `bun run test:claude -count=1 -run 'TestAgentPortClaude|TestRealClaudeSettingsUntouched'`
- Orchestrator check: grep `Backups(`/`Restore(` callers in `AdePortBackupsDialog.vue` via
  `queries.ts`; test log shows `TestAgentPortClaudeJourney` PASS; `agent-port-journey.json` holds
  no `p635-`.

## Bugs found

None. Every probe asserted as designed:

- Statusline `refresh_interval` to `refreshInterval` and padding 0 round trip (`TestAgentPortExportProbes`).
- Hook without timeout: export writes 60, re-import is `same`, no duplicate.
- Edited imported hook: old command stays in the user's file (union rule); round trip lists it as the one leftover (`hook/notification-2`).
- A hook the file already holds (same matcher and command, e.g. `SessionStart`) is left as the user wrote it, no timeout added; `hasHook` is by design.
- `X-Team` header literal (location only) masked in import preview and export diff.
- Same-second backups: distinct names, newest first, older keeps the older bytes.
- Absent `CLAUDE.md`: export writes it, no backup, no error.
- Export `Skipped` holds `env.JIRA_TOKEN` (a `${secret:}` env ref has no Claude form); `envNames` omits its variable.

The `.claude.json` literal count in the exported file is at most the seed count, not equal: replacing a
server through the CLI drops that server's literal.
