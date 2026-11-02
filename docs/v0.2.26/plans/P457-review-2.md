# P457 review round 2 findings

Base commit: `e415f1e6e`. Scope: `git diff e415f1e6e..HEAD` at `079bf0467` (P447 to P456 plus round-1 fixer commits `1f6409c67..079bf0467`, reviewed as new code). One reviewer, three dimensions: architecture/security, correctness, performance. Findings only; no fixes here.

Severity: High (data loss or secret leak in normal use), Medium (real defect in a reachable path), Low (edge case, doc drift, cleanup).

## Findings

### R2-1 Medium: ADE step inspector still writes secret param values into the workflow file, which runs now refuse

- Where: `apps/kira-space/frontend/src/ade/v2/workflows/inspector/AdeStepAgentFields.vue:74` (`ParamsForm` gets every `script.params`, no `saved`); `packages/workbench/src/automations/run/ParamsForm.vue:58` (clearing a field sets `[]`); `apps/kira-space/internal/adeflow/writer.go:713` (`setParams` writes `name: []` for an empty list); `apps/kira-space/internal/ade/smartstep.go:111` (`secretParams` refuses any present key).
- What: the R1-6 fix refuses a `params` entry for a secret param at run time, but the step editor still renders secret params as password fields, writes typed values into the plaintext workflow YAML, and shows a required asterisk even when a saved value exists. A cleared field leaves `token: []`, which is still a present key.
- Failing scenario: smart script `deploy` has secret param `token` with a saved value. User opens the step in the workflow editor, types the token (the field invites it), or types and clears it. The workflow file now holds the plaintext (first case) or `token: []` (second). Every run of the step fails "token is a secret param: save its value in the script".
- Fix direction: in `AdeStepAgentFields.vue` pass only non-secret params to `ParamsForm` and show a line that secret params use the script's saved value. In `secretParams`, treat an empty list as absent. Reject a secret param key at workflow validation (`adeflow` parse or Save) so the error shows in the editor, not at run time.

### R2-2 Medium: headless OpenCode merge file is unreadable under the macOS sandbox

- Where: `internal/harness/opencode/headless.go:117` (`mergedMCPPath` names `<stem>-app.mcp.merged.json` beside the last config); `internal/agentsandbox/profile.go:71` (read allow regex `-([0-9]+\.mcp|app\.[a-z]+)\.json$`; `/Users` is denied above it).
- What: R1-11's fix moved the merged `OPENCODE_CONFIG` file from `$TMPDIR` into the Kira config dir (`ade/runs`, `automations-mcp`). That dir is under `/Users`, and the profile allows only `<run>-<n>.mcp.json` and `<run>-app.<role>.json` there. `app.mcp.merged` does not match `app\.[a-z]+`. Flow tests use `sandboxtest`, which skips `sandbox-exec`, so nothing catches it.
- Failing scenario: macOS, sandbox on, provider OpenCode. (a) An ADE agent step with app `mcp_servers` set: configs are the grant plus the app mcp file, so they merge. (b) A hermetic smart step or Studio smart run with a ticked server: grant plus `Wire` file. OpenCode cannot read `OPENCODE_CONFIG`, so it loses every MCP server, including Kira's grant that reports the step result.
- Fix direction: name the merge file so the existing pattern covers it, for example `<stem>-app.merged.json` (still matched by `Sweep`'s `*-app.*.json`), or widen the regex and refresh the `.sb` goldens. Add a profile golden or unit case for the merge path.

### R2-3 Low: export lets a Kira allow override a user ask, contrary to MergePermission's own contract

- Where: `internal/harness/opencode/port.go:100`-`137` (`MergePermission`: Kira allows go just before the user's first deny).
- What: the comment says the merge never widens a user rule, then states a user `ask` before the first deny loses to Kira's allow. OpenCode applies the last match, so a broader Kira allow placed after a narrower user `ask` removes the prompt. Claude semantics (Kira's model) let `ask` beat `allow`. Separately, a Kira allow placed before a broader user deny (`"*":"deny"` first) is dead, and export reports it as applied with no note.
- Failing scenario: user `opencode.json` has `"bash":{"*":"allow","git push *":"ask"}`; Kira settings allow `Bash(git:*)`. Export yields `{"*":"allow","git push *":"ask","git *":"allow"}`: `git push --force` no longer asks.
- Fix direction: insert Kira allows right after the user's `*` entry (index 0 when none), before every other user entry, so user asks and denies keep winning and the catch-all still yields. Add a Skipped or note row for a Kira allow a later user deny fully shadows. Fix the comment either way.

### R2-4 Low: OpenCode launches clobber or lose a user `OPENCODE_CONFIG` (pre-existing, widened by R1-3)

- Where: `internal/harness/opencode/interactive.go:30` and `headless.go:186`-`196` (Kira sets `OPENCODE_CONFIG` in the process env); both commands run under the login shell after that env is set.
- What: `OPENCODE_CONFIG` names one file. A user who sets it in the app's inherited env loses their custom config on every non-hermetic Kira launch, since Kira's value replaces it. A user who exports it in the shell profile overrides Kira's value instead: the grant and, since R1-3, all app `mcp_servers` silently drop. The hermetic env words set only `XDG_CONFIG_HOME` and the hermetic env, so they do not protect it either.
- Failing scenario: `~/.zprofile` exports `OPENCODE_CONFIG=~/work/opencode.json`. A Space terminal OpenCode session starts with none of the app's MCP servers and no Kira grant tools.
- Fix direction: when not hermetic, read the user's `OPENCODE_CONFIG` (process env or login-shell probe) and merge its `mcp` into Kira's file, or append it to the merge list. Set `OPENCODE_CONFIG` through the `/usr/bin/env` words (as `XDG_CONFIG_HOME` is) so a profile export cannot undo it. If kept, add a Known open item.

### R2-5 Low (performance): XDG probe holds a global lock across a login shell and caches a failed probe forever

- Where: `internal/harness/opencode/hermetic.go:194`-`204` (`profileConfigHome`).
- What: the mutex is held for the whole `loginshell.ProbeEnv` call (up to its timeout), so concurrent hermetic launches queue behind one slow shell. An error or timeout is cached as "" for the process life, so the redirect links `~/.config` until restart, which is the R1-10 failure the probe exists to fix. `agentsandbox.DiscoverRuntime` already shows the house pattern: no lock across the probe, no cache of an incomplete result.
- Failing scenario: first hermetic launch after app start hits a slow profile (nvm, conda) and times out. Every later hermetic OpenCode launch in that process ignores the profile `XDG_CONFIG_HOME`.
- Fix direction: release the lock before probing (or use `singleflight`), cache only a successful probe.

## R1 fixes verified with no separate finding

- R1-1 noCostCap: `Smart.NoCostCap` validates to budget 0; `agentBlocker` blocks only `budget > 0`; Claude omits `--max-budget-usd` at 0; OpenCode `RunHeadless` refuses only `> 0`; editor switch disables the budget and restores 1 on off; `RunPreview` shows "no cost cap"; old rows decode as false. Known open item removed.
- R1-3 file mode: app `mcp` goes to `<stem>-app.mcp.json` via `writeFiles` (0600, dir 0700), removed on `Release`, `Store.Close` and `Sweep`. The interactive merge writes the grant into that file in place, also 0600. The settings view still renders references verbatim. Smart steps take no app settings, so their `Wire` file never collides with the app file on the same stem.
- R1-5 classifier: env and arg values holding `://` now go through `urlParts`; userinfo, credential-looking path segments and `key=` or credential-looking query values are sealed.
- R1-9 `SessionQuery.Exclude`: ambiguity returns "", ids on other `ade_sessions` rows are excluded; an overlapping earlier session gets its id on its next resume once the later one is learned. Headless `finishRun` passes no `Exclude`, but returns "" rather than guessing.
- R1-12: `putSecrets` reads the store once, refuses a changed name as stale, skips an identical one, and rolls back only names it sealed, including on a `Settings.Save` failure. Mode check matches the dialog, which defaults each conflict item to its first mode.
- R1-13: backup uses `O_EXCL` with `-N` suffixes, cleans a partial write.
- R1-14: `SecretStore.Snapshot` decrypts once per resolver; `resolver` is built per `Prepare` and per `Wire`, so a value set in the other app reaches the next launch.
- R1-6 masking: saved secret values join the step log mask in `superviseAgent`.
- R1-4, R1-7, R1-8, R1-10 (Known open items), R1-11 (doc and `checkMarks` on `MCPConfig`): match the code.
