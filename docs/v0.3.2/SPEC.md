# v0.3.2

Branch `chapter/v0.3.2`, based on `main` (v0.3.1 merged). First `P` is 555 (highest in v0.3.1 is P554).

Source: user request. The table is open: the user adds more phases to this chapter. Last `P` is undecided until the user closes the table. Close duty after the last phase lands: one full suite run, 1 code review round, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

| Phase | Title | Status |
|---|---|---|
| P555 | Install images and Pages build | Pending |
| P556 | Experimental settings menu, Memory behind a toggle | Pending |

More rows follow as the user adds them. Table order is execution order.

## User to-dos (note, not phases)

- Re-run the Release workflow for v0.3.0.
- Verify the folder-access docs text on a real Mac.

## Rows

### P555 Install images and Pages build

- Ask: user supplies `site/src/assets/install/keychain-access.webp` and `site/src/assets/install/folder-access.webp` (WebP, cropped to the dialog). Add them. Re-record the site visual baselines so they show the real images, not placeholders. `CI=true bun run build` in `site` must pass. The Pages workflow run for `main` must be green after merge.
- Baseline method: site Playwright WebKit on Linux, as CI does. Recording command in `docs/DEV_ENVIRONMENT.md` (site visual baselines): `bunx playwright test --config=site/playwright.config.ts visual --update-snapshots` after `bun run site:build`. Check the Inter font setup there first.
- Why Pages fails: last run 42348265274 failed solely with "Missing install image site/src/assets/install/keychain-access.webp" at `/`. `site/src/lib/installShots.ts` fails under `CI=true` on a missing image, by design.
- Input: the two WebP files come from the user. Phase does not start until both exist.
- Plan: `plans/P555.md`.
- Result: pending.

### P556 Experimental settings menu, Memory behind a toggle

- Ask (verbatim): "Add an experimental menu in settings and hide memory module under a toggle."
- Scope: new Experimental pane in Kira Space Settings, built as a list of flag rows. First row: `experimental.memory`, default off. Off hides every Memory UI surface: the Memory mode tab, the Memory settings pane, the editor "Distill selection" action. Memory data, memory.db, the `memory-mcp` server and its agent registration stay untouched.
- Upgrade: an install whose `memory.db` already exists gets `experimental.memory = true` once, at first boot after the update. Reason and mechanics in the plan.
- Tests: backend flow test plus UI spec, same scenario, one contract. Existing Memory specs enable the flag first. Settings and title-bar visual baselines re-recorded on Linux.
- Plan: `plans/P556.md`.
- Result: pending.

## Close

1 code review round unless the user says otherwise. After the last phase: one full suite run with all failures fixed, review round, hand-written close per CLAUDE.md, PR. The table is open: add checklist items for each new phase.

### Site and images check (mandatory)

Runs after the last phase and the full suite run, before the code review. Close does not start until every item passes.

- [ ] Both install WebP images present under `site/src/assets/install/`, cropped to the dialog, with alt text. Orchestrator views both.
- [ ] `CI=true bun run build` in `site` passes with no placeholder.
- [ ] Site visual baselines re-recorded on Linux WebKit, showing the real images, no placeholder boxes.
- [ ] Pages workflow run for `main` green after merge.
- [ ] Docs match final behaviour: README, ARCHITECTURE, PACKAGING, DEV_ENVIRONMENT, site docs.

### P556 Experimental gate check (mandatory)

- [ ] Fresh home: no Memory mode tab, no Memory settings section, no "Distill selection"; Settings has an Experimental section with the Memory switch off.
- [ ] Turning the switch on and saving shows all three; off hides them again, and a window on Memory falls back to Git.
- [ ] Existing `memory.db` before first boot reads the flag on; a later explicit off survives restart.
- [ ] Opening Settings with the flag off never opens memory.db (no Memory pane mounted).
- [ ] Visual baselines re-recorded on Linux with Inter; ARCHITECTURE notes the gate.
