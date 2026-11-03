# P488 Install instructions: trust the Jira tap

Plan for SPEC row P488. Planned against `1b92f567` (P484 plan). Stream C, first of P488, P491, P492.
One sequential implementer. Discovery used `codegraph_explore` over the CLI tools pane
(`CliToolsPane`, `useCliTools`, `cliToolsDomain`), the `CLISetup` role and every provider `Setup`
(`jira.Provider.Setup`, `github.Provider.Setup`, `azure.Provider.Setup`, `fakehost.Forge.Setup`),
then a repo-wide grep for install commands in code, i18n, docs and site.

## 1. Requirement

- Homebrew 6.0.0 requires explicit trust for a non-official tap (docs.brew.sh/Tap-Trust). Official
  taps (homebrew/core, homebrew/cask) stay trusted.
- An untrusted item fails to install. Trust per item: `brew trust --formula user/repo/formula`.
  Whole tap: `brew trust user/repo`. Homebrew recommends trusting only the item needed.
- acli ships from tap `atlassian/homebrew-acli` (tap name `atlassian/acli`), one formula
  `Formula/acli.rb`. Full name: `atlassian/acli/acli`.

## 2. Inventory: every CLI install instruction

| Where | Text | Tap | P488 |
|---|---|---|---|
| `apps/kira-space/internal/issuetracker/jira/jira.go:69` `Setup` install step (Settings, CLI tools, Jira card) | `brew tap atlassian/homebrew-acli && brew install acli` | third-party | change (3) |
| `apps/kira-space/internal/githost/github/provider.go:200` | `brew install gh` | homebrew/core | none |
| `apps/kira-space/internal/githost/azure/provider.go:91` | `brew install azure-cli` | homebrew/core | none |
| `apps/kira-space/internal/flowharness/fakehost/fakehost.go:495` | `brew install forge` | test fake, never shown to users | none |
| `packages/git-ui/src/lib/gitBlockedCopy.ts:28` (Git too old) | `brew upgrade git` | homebrew/core, upgrade | none |
| `packages/theme/src/i18n/en.json` `backend.core.agent.missing`, Docker `notInstalled`, Git `blocked.*` | prose, no brew command | — | none |
| `scripts/check-theme-classes.sh`, `scripts/check-ui-primitives.sh` | `brew install grep` | homebrew/core | none |
| `docs/DEV_ENVIRONMENT.md:483` az capture install | pip, not brew | — | none |
| `site/src/content/docs/docs/reference/settings.md` "CLI tools" | prose, no command | — | none |
| `README.md`, `apps/kira-space/README.md`, `site/.../start/install.md` | Kira's own `install.sh` | — | none |

Result: acli is the only instruction from a third-party tap. No other text changes.

Kubernetes: Studio shows no `kubectl` install text today. P487 (Stream A) adds a missing-`kubectl`
error. If it names a brew command, `brew install kubectl` is homebrew/core and needs no trust.
Orchestrator: pass that to P487's planner.

## 3. Change

`jira.go` `Setup` install step becomes:

```
brew tap atlassian/homebrew-acli && brew trust --formula atlassian/acli/acli && brew install acli
```

- One step, one copy button, same as today. Order: tap, trust, install.
- Tap spelling stays Atlassian's documented one (`atlassian/homebrew-acli`). Trust takes the
  canonical full name; `homebrew-` is never part of a tap name.
- Per-formula trust, not whole-tap: least privilege, as Homebrew recommends.
- No new step purpose. A separate "trust" step needs a new i18n label and splits one copy-paste line
  into three; declined, no requirement it meets.
- Homebrew before 6.0 lacks `brew trust` and the chain stops there. Not handled: Homebrew
  auto-updates, and the docs link stays on the card. Declined `brew update &&` prefix: slow, and
  needless on current Homebrew.
- No change to `CliToolsPane.vue`, `cliToolsDomain.ts`, i18n or bindings. The command is data on the
  wire (`setup.steps[].command`).

## 4. Library rules

No new UI, component, store, query or dependency. Existing shadcn-vue `Card`/`Badge` and TanStack
Query `useCliTools` render the new text unchanged.

## 5. Tests

In-app text changes, so the UI spec and its contract update. Same scenario both halves.

- Backend: `apps/kira-space/internal/flows/appflow/clitools_test.go` `TestCliTools` asserts step
  purposes only; no edit. Regenerate its contract from `apps/kira-space`:
  `KIRA_CONTRACT=write go test ./internal/flows/appflow -run TestCliTools -count=1`, then rerun
  without `KIRA_CONTRACT`. `tests/contract/space-cli-tools.json`: the six Jira install `command`
  values change; nothing else. Check with `git diff --stat` and `git diff` on that file.
- UI: `apps/kira-space/tests/ui/settings-cli-tools.spec.ts:36` asserts the old string. Change it to
  the new command. Add one assertion in the same test that the Jira steps contain
  `brew trust --formula atlassian/acli/acli`, so a later edit that drops trust fails. Run:
  `npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui settings-cli-tools.spec.ts`.
- No unit test (CLAUDE.md bar: a constant string).
- Fast checks per commit: Go build, vet, lint, typecheck (pre-commit hook).

## 6. Docs

- `docs/ARCHITECTURE.md` CLISetup bullet (line ~2014, code host section): append one sentence. An
  install step from a non-official Homebrew tap trusts the formula before install
  (`brew trust --formula`, Homebrew 6 tap trust); homebrew/core steps need none.
- `docs/ARCHITECTURE.md` Jira provider bullet (line ~2261): no change.
- No `docs/DEV_ENVIRONMENT.md`, site or README change (2).

## 7. Commits

1. `fix: Jira install step trusts the acli tap` (`jira.go`, regenerated `space-cli-tools.json`,
   `settings-cli-tools.spec.ts`).
2. `docs: CLI install steps trust non-official taps` (ARCHITECTURE).
3. `docs: P488 result` (SPEC row Done plus Result).

## 8. File ownership (Stream C, P488)

Owned by P488 only; neither Stream A (P484-P487, Studio Kubernetes) nor Stream B (P489-P490, Space
terminal and Kira Space file open) touches them:

- `apps/kira-space/internal/issuetracker/jira/jira.go`
- `apps/kira-space/tests/contract/space-cli-tools.json`
- `apps/kira-space/tests/ui/settings-cli-tools.spec.ts`
- `docs/v0.2.29/plans/P488-trust-jira-tap.md`

Shared, with resolution:

- `docs/v0.2.29/SPEC.md`: each stream edits only its own rows (status cell plus Result under its
  own `### P<n>` heading). On rebase onto the chapter branch, keep both sides' hunks; never rewrite
  another stream's row.
- `docs/ARCHITECTURE.md`: P488 edits only the CLISetup bullet in the code host section. Stream A
  edits the Kubernetes sections (P484, P487); Stream B may edit terminal or Kira Space sections. No
  shared lines; a textual conflict means adjacent hunks: keep both.
- Not touched by P488: contracts other than `space-cli-tools.json`, Wails bindings, IPC/domain
  schemas, `packages/theme/src/i18n/en.json`, `CliToolsPane.vue`. None conflict.

P491 and P492 (also Stream C, later) get their own plans and ownership sections; they run after
P488 lands, in table order.

## 9. Ordering consequences

None for Streams A and B. P487's possible `kubectl` install hint is noted in 2.
