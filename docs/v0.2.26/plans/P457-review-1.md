# P457 review round 1 findings

Base commit: `e415f1e6e` (origin/main, v0.2.25 merged). Scope: `git diff e415f1e6e...HEAD` at `9a1582a0f` (414 files, P447 to P456). One reviewer, three dimensions: architecture/security, correctness, performance. Findings only; no fixes here.

Severity: High (data loss or secret leak in normal use), Medium (real defect in a reachable path), Low (edge case, doc drift, cleanup).

## Findings

### R1-1 Medium: smart scripts and smart steps can never run on OpenCode (S2)

- Where: `internal/scripts/smart.go:45`, `:258` (`MinBudgetUSD = 0.05`, 0 becomes `DefaultBudgetUSD`); `internal/scriptruns/preview.go:248`; `internal/harness/opencode/headless.go:243`; copy `packages/theme/src/i18n/en.json:563`.
- What: every saved smart script has a budget of 0.05 to 25 USD. `agentBlocker` refuses any budget on a provider without `CostCapper`, and the copy says "Remove the budget from the script". Validation forbids removing it. ADE smart steps pass the same budget into `RunHeadless`, which refuses too.
- Failing scenario: user sets `agent.provider` to OpenCode, opens any smart script, presses Run. The preview blocks with a remedy the editor cannot perform. Same for a workflow smart step on an OpenCode step.
- Fix direction: code fix, not copy only. Add an explicit "no cost cap" choice to `Smart` (a bool, not a 0 sentinel: stored 0 already means default). `validSmart` accepts it; the editor shows it as a switch beside the budget field. `agentBlocker` blocks only when a cap is set; copy becomes "{agent} can't cap a run's cost. Turn off the cost cap in the script to run it with {agent}." Claude keeps enforcing a set cap. Delete the Known open item "OpenCode has no cost cap" once it lands. A copy-only fix ("run it with Claude") would leave the feature dead on OpenCode.

### R1-2 Medium: export to OpenCode can widen the user's own permission denies

- Where: `internal/harness/opencode/port.go:102` (`MergePermission`), `internal/harness/opencode/tools.go:21` (`ruleSet.add` moves a pattern to the end).
- What: OpenCode applies the last matching rule. Export appends Kira's rules after the user's. The guard only keeps a user `deny` when the incoming pattern is identical. A broader incoming `allow` after a narrower user `deny` wins.
- Failing scenario: user `opencode.json` has `"permission":{"bash":{"*":"ask","git push *":"deny"}}`; Kira settings allow `Bash`. Export yields `{"git push *":"deny","*":"allow"}`: `*` is last, so `git push --force` is allowed. Same with `Bash(git:*)` after a user `git push *` deny.
- Fix direction: when merging, place Kira allow entries before every existing user entry and Kira deny entries after them, or skip (with a Skipped row) any allow whose glob covers an existing user deny. Never move the user's `*` entry.

### R1-3 Medium: OpenCode app settings put every MCP credential in the agent's environment

- Where: `internal/harness/opencode/settings.go:45` (comment "a resolved secret stays in the environment"), `interactive.go:43`, `headless.go:184`.
- What: resolved `mcp_servers` headers and env ride `OPENCODE_CONFIG_CONTENT`. Every child of opencode inherits it: each local MCP server sees every other server's credentials, and a Bash tool call (`env`, `printenv`) prints them into the transcript and the model request. Claude gets the same values in a 0600 `--mcp-config` file whose path only the CLI reads.
- Failing scenario: settings define `github` (header `Authorization: Bearer ${secret:gh}`) and `jira`. An interactive OpenCode session runs `env`; both tokens reach the model provider. The `jira` stdio server process holds the GitHub token in its env.
- Fix direction: keep only non-secret keys (`model`, `permission`, `plugin`, `instructions`) in `OPENCODE_CONFIG_CONTENT`; write `mcp` to a 0600 run file under the launch dir and pass it through `OPENCODE_CONFIG` (merged with the grant file, as `mergeMCP` already does). Add `OPENCODE_CONFIG_CONTENT` to the env the provider scrubs from tool children if OpenCode allows it. Update ARCHITECTURE (R1-11) either way.

### R1-4 Medium: imported OpenCode `{env:X}` becomes a hard launch failure (lock-in of substitution)

- Where: `internal/harness/opencode/port.go:24` (`RefsToKira`), `internal/configfs/resolve.go` (unset env is an error), `internal/agentcfg/build.go:42` (every enabled server resolves at every launch).
- What: OpenCode substitutes an unset `{env:X}` as "" (Q-P454-OC3). Import maps it to `${X}`, which Kira refuses when unset. One such reference in any enabled server fails every agent launch that takes settings (all Space terminal sessions, Continue in terminal, headless steps). The reverse holds on export: `${X}` and `${secret:x}` become `{env:X}` / `{env:KIRA_SECRET_X}`, which OpenCode silently sends as "" when the shell lacks them (a `Bearer ` header with no token, a confusing 401).
- Failing scenario: user's `opencode.json` has `"headers":{"X-Org":"{env:ORG_ID}"}` with `ORG_ID` unset (harmless in OpenCode). Import, then start any agent session in Space: launch fails "environment variable ORG_ID is not set".
- Fix direction: import preview probes each referenced variable (it already has `Service.env`) and flags unset ones on the item with a note, so the user decides before Apply. Export preview notes that OpenCode substitutes a missing variable as empty, listing `EnvNames` not set in the login shell.

### R1-5 Medium: import seals too few credentials; plaintext lands in settings.yaml

- Where: `internal/agentport/classify.go:79` (`locEnv` checks name and value pattern only), `:127`/`:151` (`urlParts` seals userinfo and secret-named query keys only).
- What: `settings.yaml` must hold references only (P450). An env value that is a URL with userinfo, a query key named `key`, or a token in the URL path is imported verbatim.
- Failing scenario: `~/.claude.json` server env `DATABASE_URL=postgres://app:hunter2@db/x`, or url `https://maps.example/mcp?key=AIzaSy...`. Import writes both literally into `settings.yaml` (0644, the shared folder users may sync). Export then copies them on as plaintext too, since `safe` uses the same classifier.
- Fix direction: in `classify`, run `urlParts` for any `locEnv`/`locArg` value that parses as a URL with userinfo; in `urlParts`, also seal a query value when `valueIsSecret(v)` or the key is `key`; consider `valueIsSecret` on path segments.

### R1-6 Medium: ADE smart step secret params: plaintext in workflow YAML, never masked, saved value ignored

- Where: `apps/kira-space/internal/ade/smartstep.go:55` (`ParamValues(script.Params, def.Params)`), `apps/kira-space/internal/ade/runs.go:845` (mask has only agent settings and ticked server secrets).
- What: P447 scoped ADE smart steps out, but nothing records it. A step's secret param value comes only from `params:` in the workflow file (or the param default). The script's saved value is never read, and the value is not in the step log mask.
- Failing scenario: smart script `deploy` has required secret param `token` with a saved value. A workflow step `smart_script: deploy` fails "needs a value for token" unless the user writes `params: {token: <plaintext>}` into the workflow YAML; the agent printing `$KIRA_PARAM_TOKEN` then stores it in the step log unmasked.
- Fix direction: in `planSmartStep`, fill missing secret params from `CustomScripts.SecretValues` (as `fillSaved` does for runs), refuse a plaintext value for a secret param in the workflow file, and add every secret param value to the step's Masker. If kept out of scope, add a Known open item naming it.

### R1-7 Low: AGENTS.md settings intro omits OpenCode MCP rendering (S3)

- Where: `internal/agentcfg/settings_intro.md:14`.
- What: "Kira writes it in each provider's format: a `--mcp-config` file for Claude." OpenCode (inline `mcp` in `OPENCODE_CONFIG_CONTENT`, or the `OPENCODE_CONFIG` file for ticked servers) is missing. The same text says "Smart scripts ... use none" while `agents_intro.md` says smart scripts can tick `mcp_servers`.
- Failing scenario: an agent or user reading the generated `AGENTS.md` cannot tell how an OpenCode launch receives servers.
- Fix direction: add the OpenCode form (follow R1-3's outcome); reword the last paragraph: smart scripts and smart steps take only the servers they tick.

### R1-8 Low: facts.go Q-P454-OC7 comment misstates the probe, and the consequence is undocumented (S4)

- Where: `internal/harness/opencode/facts.go:120`-`122`; hook env `internal/harness/opencode/hooks/server.go:102`.
- What: "Setting OPENCODE_CONFIG_DIR appears to replace the global dir" is wrong. Probe: user `opencode.json` still loads with `OPENCODE_CONFIG_DIR` set; only the global `~/.config/opencode/AGENTS.md` does not (it did load in a plain run). Every Kira interactive OpenCode launch sets `OPENCODE_CONFIG_DIR` for the hook plugin, so with `agent.hermetic` off the user's global AGENTS.md is silently dropped. No Known open item says so.
- Failing scenario: hermetic off, user keeps rules in `~/.config/opencode/AGENTS.md`; a Kira-started OpenCode session ignores them, a plain `opencode` run in a shell uses them.
- Fix direction: reword: "OPENCODE_CONFIG_DIR adds a config layer: user opencode.json still loads, but the global AGENTS.md is read from OPENCODE_CONFIG_DIR instead of the user config dir." Then either add the user's global AGENTS.md to `instructions` when not hermetic, or add a Known open item.

### R1-9 Low: late OpenCode session id can bind another session's id

- Where: `internal/harness/opencode/session.go:42` (`parseSessionList` picks the newest since start), callers `apps/kira-space/internal/ade/tracker.go:429`, `internal/harness/opencode/headless.go` `finishRun`.
- What: the fallback (hook events lack an id, U12 unverified; or the stream lacks one) takes the newest session in the cwd created after the record started. Two sessions in one worktree overlap.
- Failing scenario: user opens OpenCode session A, then B, in the same worktree. A stops first; `learnSessionID(A)` stores B's id. Resume of A continues B's conversation.
- Fix direction: skip ids already stored on another `ade_sessions` row, and return "" (unknown) when more than one candidate remains rather than guess.

### R1-10 Low: hermetic OpenCode XDG redirect loses new config written during a session and ignores a profile-only XDG_CONFIG_HOME

- Where: `internal/harness/opencode/hermetic.go:84` (`redirectConfigHome`), callers `harness.PrepareIsolation(..., os.Environ())` in `apps/kira-space/internal/ade/launches.go:447`, `internal/scriptruns/resume.go:166`; headless `headless.go:139` (`base` env).
- What: the redirect symlinks each existing `~/.config` entry at launch. A tool the agent runs that creates a new entry (first `gh auth login` creates `~/.config/gh`), or replaces a top-level file by rename, writes into the iso dir, which is deleted on exit. `XDG_CONFIG_HOME` is read from the app process env, not the login shell; on macOS a GUI app lacks a profile-only value, so the redirect links `~/.config` instead and the profile value is overridden by the env word.
- Failing scenario: in a hermetic OpenCode session the agent runs `gh auth login` for a user without `~/.config/gh`; after exit the login is gone.
- Fix direction: probe `XDG_CONFIG_HOME` through `loginshell.ProbeEnv`; record as a Known open item that new top-level config entries created inside a hermetic OpenCode session are lost (or link the real dir's parent and only shadow `opencode`, if OpenCode honours a narrower override).

### R1-11 Low: ARCHITECTURE says no OpenCode file holds a secret; two do

- Where: `docs/ARCHITECTURE.md:3251` ("No file holds a secret"); `internal/agentcfg/servers.go:146` (`Wire` writes resolved ticked servers via `MCPConfig` to `<stem>-app.mcp.json`); `internal/harness/opencode/headless.go:92` (`mergeMCP` copies them to `$TMPDIR/kira-opencode-*/kira-agent-config.json`).
- What: hermetic smart runs and smart steps on OpenCode write resolved credentials to two 0600 files. The `$TMPDIR` copy is outside `Store.Sweep`'s dirs, so a crash leaves it. Neither file is checked for `substitutionMarks`, unlike the inline document, so a resolved value holding `{env:` or `{file:` is substituted by OpenCode.
- Failing scenario: app is killed mid smart run on OpenCode with a ticked server holding `${secret:x}`; the plaintext stays in `/tmp/kira-opencode-*/kira-agent-config.json` until the OS cleans `/tmp`.
- Fix direction: create the scratch dir under the launch's Kira-owned dir (swept at start), apply the `substitutionMarks` refusal to `MCPConfig` output on OpenCode, and fix the doc sentence.

### R1-12 Low: import Apply can overwrite a secret set after the preview, and accepts any context mode

- Where: `internal/agentport/import.go:161` (`assign` reserves names against secrets read at preview), `:473` (`putSecrets` writes without rechecking), `:491` (`contextText`: any mode but `append` replaces).
- What: the staleness check covers `settings.yaml` and `context.md` only. A secret saved under the same name in Settings, Secrets between preview and Apply is overwritten. A conflict item sent with an empty or unknown `mode` replaces the user's context without the choice being made. Secrets are also sealed before `Settings.Save`; a save conflict leaves them orphaned.
- Failing scenario: preview proposes new secret `github-token`; user sets `github-token` for another server meanwhile; Apply replaces it silently.
- Fix direction: in `putSecrets`, re-read names and fail `stale()` when a non-reuse name now exists with a different value; reject a `mode` not in `it.Modes`; save settings before sealing, or delete the sealed names on a save failure.

### R1-13 Low: export backup name has one-second resolution

- Where: `internal/agentport/backup.go` `backup` (`backupStamp = "20060102T150405Z"`, `renameio.WriteFile` replaces).
- What: two applies to the same file within one second overwrite the first backup, the only copy of the original.
- Failing scenario: an automated or double-clicked export apply twice in one second loses the pre-export `settings.json`.
- Fix direction: refuse to overwrite (`O_EXCL` semantics) and add a counter suffix, or use millisecond stamps.

### R1-14 Low (performance): secret resolution decrypts the whole store per reference

- Where: `internal/agentcfg/secrets.go:79` (`Source` calls `Store.Open` per `${secret:}`).
- What: each reference runs one query and decrypts every agent secret, so a launch with n references does n×N decrypts. Every Space terminal agent launch pays it.
- Failing scenario: 15 servers with 2 secret refs each and 30 saved secrets: 900 AES-GCM opens plus 30 queries per launch.
- Fix direction: open the owner's values once per resolver (cache inside `Store.resolver` for the launch).

## Must-check items with no separate finding

- OpenCode rewriting `opencode.json` (`$schema` insert) and creating `package.json`/`node_modules` in the config dir: export tolerates it. `stripSchema` normalizes both staleness hashes; export touches only the config file and `AGENTS.md`. A reformat by OpenCode (minified rewrite) between preview and apply is refused as stale, which is safe; a fresh preview passes.
- P455 stdio MCP rendering (`internal/harness/claude/mcp.go`): omitting empty `args`/`env` is behaviour-neutral. A probe with claude 2.1.316 `-p --strict-mcp-config` accepted both `"args":null` and `"headers":null`, so the comment "claude rejects null args and env" is not reproduced for `args`; no regression either way. (The probe made three tiny model calls; the CLI was signed in.)
- Studio hermetic: default on (`appsettings.DefaultAgent`), no UI switch; code honours a stored `agent.hermetic`, settable only over IPC. Known open item is accurate enough.
- Live terminal secrets unmasked: real, and both Known open items (P447 scripts, P453 agent settings) state it. No action unless the user wants masking in the PTY stream.
- Unit tests added: `configfs/{docs,refs,schema}_test.go`, `agentport/classify_test.go`, `secrets/mask_test.go`, `opencode/parse_test.go`, `adeflow/rules_test.go` guard parsers, classifiers or drift and meet the bar; `opencode/hermetic_probe_test.go` is an opt-in CLI fact probe. None flagged for deletion.
- Vue rules: every new or changed `.vue` file has one `<script setup lang="ts">` and no `<style>` block; server state uses TanStack Query (`port/queries.ts`, `settings/queries.ts`, `context/queries.ts`); selects use `NativeSelect`.
- ARCHITECTURE Known open items: checked against code; accurate except R1-6 (missing item), R1-8 (missing consequence) and R1-11 (wrong sentence outside the list).
