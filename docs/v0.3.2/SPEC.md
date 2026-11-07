# v0.3.2

Branch `chapter/v0.3.2`, based on `main` (v0.3.1 merged). First `P` is 555 (highest in v0.3.1 is P554).

Source: user request. The table is open: the user adds more phases to this chapter. Last `P` is undecided until the user closes the table. Close duty after the last phase lands: one full suite run, 1 code review round, hand-written close. After the close and push, open the PR to main (CLAUDE.md close rules), subscribe to it, and fix failing checks until it merges. Each phase: Opus plan, Sonnet implementation.

| Phase | Title | Status |
|---|---|---|
| P555 | Keychain install image and Pages build | Pending |
| P556 | Experimental settings menu, Memory behind a toggle | Pending |
| P557 | Drop backward compatibility, single baseline migration | Pending |

More rows follow as the user adds them. Table order is execution order.

## User to-dos (note, not phases)

- Re-run the Release workflow for v0.3.0.
- Verify the folder prompt text (site Install block, `docs/start/install.md`) on a real Mac.

## Rows

### P555 Keychain install image and Pages build

- Ask: add the user-supplied Keychain prompt image as `site/src/assets/install/keychain-access.webp`. Source: ready WebP, cropped to the dialog, transparent rounded corners, 868x372. No folder prompt image exists or will exist: the folder permission prompt is prose only. Remove the folder image requirement everywhere (content, Install component, placeholder fallback, site spec, DEV_ENVIRONMENT). Keep the build guard for the required image (now only `keychain-access.webp`); never weaken it.
- Baselines: re-record the site visual baselines so they show the real Keychain image and no folder placeholder. Method: site Playwright WebKit on Linux, `docs/DEV_ENVIRONMENT.md` (site visual baselines).
- Why Pages fails: last run 42348265274 failed solely with "Missing install image site/src/assets/install/keychain-access.webp" at `/`. `site/src/lib/installShots.ts` fails under `CI=true` on a missing image, by design.
- Done: `CI=true bun run build` in `site` passes; affected site specs pass; Pages workflow run for `main` green after merge (confirmable only after merge).
- Plan: `plans/P555.md`.
- Result: pending.

### P556 Experimental settings menu, Memory behind a toggle

- Ask (verbatim): "Add an experimental menu in settings and hide memory module under a toggle."
- Scope: new Experimental pane in Kira Space Settings, built as a list of flag rows. First row: `experimental.memory`, default off. Off hides every Memory UI surface: the Memory mode tab, the Memory settings pane, the editor "Distill selection" action. Memory data, memory.db, the `memory-mcp` server and its agent registration stay untouched.
- Upgrade: an install whose `memory.db` already exists gets `experimental.memory = true` once, at first boot after the update. Reason and mechanics in the plan.
- Tests: backend flow test plus UI spec, same scenario, one contract. Existing Memory specs enable the flag first. Settings and title-bar visual baselines re-recorded on Linux.
- Plan: `plans/P556.md`.
- Result: pending.

### P557 Drop backward compatibility, single baseline migration

- Ask (verbatim): "I'm the only user of this app, I updated to this latest published version 0.3.1, any migration should be dropped. I don't only mean reconciling all sqlite migrations into 1, but dropping any scripts, paths etc dedicated for backwards compat."
- Scope: every install is exactly v0.3.1 (tag `v0.3.1`, merge `e7356a352`). Squash each SQLite chain into one baseline at the version v0.3.1 records: kira.db 90, review.db 8, memory.db 8. An older non-empty DB refuses to open with a pointed error. Delete the home, config and DB folds, the old-Studio checks, one-time DB-to-file exports, legacy cleanups, the frontend key migration, `install.sh --app=studio` and its release job. Keep the migration runner, `schema_version`, `SchemaTooNewError` and the foreign-keys-off step type for future migrations. Keep the v0.3.1 updater contract (`--app=space`, `--wait-pid`, `--notify-fd`).
- Added by the user: fold `agent-secrets.db` into kira.db `secret_values` (scope `agent-credential`, owner `agent`, same cipher, only through `secrets.Store`, values never in logs or the op log); rename the macOS Keychain item from "Kira Studio" to "Kira Space" keeping the existing master key. Both are one-time boot data moves of live v0.3.1 data, idempotent, source kept until verified. Out of scope: `~/.kira-memory` move and the add-repos dialog move (separate phases).
- Zero data loss on the user's v90 kira.db: baseline proven byte-for-byte against a chain-built DB, and a chain-upgraded DB with data opens unchanged.
- Order: runs after P556 lands; deletes P556's one-time Memory upgrade seed (user decision).
- Single implementer, no streams. Plan lists 6 NEEDS USER DECISION items; the implementer starts only after the user answers them.
- Plan: `plans/P557.md`.
- Result: pending.

## Close

1 code review round unless the user says otherwise. After the last phase: one full suite run with all failures fixed, review round, hand-written close per CLAUDE.md, PR. The table is open: add checklist items for each new phase.

### Site and images check (mandatory)

Runs after the last phase and the full suite run, before the code review. Close does not start until every item passes.

- [ ] `site/src/assets/install/keychain-access.webp` present, cropped to the dialog, with alt text; orchestrator views it. Folder prompt is prose only, no image or placeholder.
- [ ] `CI=true bun run build` in `site` passes with no placeholder.
- [ ] Site visual baselines re-recorded on Linux WebKit, showing the real Keychain image, no placeholder box.
- [ ] Pages workflow run for `main` green after merge.
- [ ] Docs match final behaviour: README, ARCHITECTURE, PACKAGING, DEV_ENVIRONMENT, site docs.

### P556 Experimental gate check (mandatory)

- [ ] Fresh home: no Memory mode tab, no Memory settings section, no "Distill selection"; Settings has an Experimental section with the Memory switch off.
- [ ] Turning the switch on and saving shows all three; off hides them again, and a window on Memory falls back to Git.
- [ ] Existing `memory.db` before first boot reads the flag on; a later explicit off survives restart.
- [ ] Opening Settings with the flag off never opens memory.db (no Memory pane mounted).
- [ ] Visual baselines re-recorded on Linux with Inter; ARCHITECTURE notes the gate.

### P557 Baseline check (mandatory)

- [ ] Each DB package has exactly one migration file: kira `0090`, review `0008`, memory `0008`. Baseline proof test passed at the base commit (result in plan).
- [ ] Copy of a chain-upgraded v90 DB with data opens: version 90, row counts and content hash unchanged, `integrity_check` ok, `foreign_key_check` empty.
- [ ] v82 DB refuses with the schema-too-old startup alert; v91 refuses with schema-too-new.
- [ ] `grep -rn "homefold\|configfold\|dbfold\|MigrateOnce\|app=studio\|\.kira-studio" apps internal scripts` returns nothing outside keychain names.
- [ ] Pending release.yml patch committed under `docs/pending-changes/`; README, ARCHITECTURE, PACKAGING, DEV_ENVIRONMENT carry no fold or `--app=studio` text.
- [ ] Agent secrets: flow test moves rows from a seeded `agent-secrets.db` into kira.db, file lands under `backups/p557-*`, second boot no-op, conflicting value refuses; no plaintext anywhere in kira.db or logs.
- [ ] Keychain: user runs `go test ./internal/secrets -run Keychain` on the Mac (old only moves, both equal deletes old, both differ refuses, neither no-op); first real launch keeps connection passwords and agent credentials readable.
