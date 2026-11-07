# v0.3.3

Cleanup chapter. Branch `chapter/v0.3.3`, tag `v0.3.3`. First `P` is P564 (highest prior: P563).

| Phase | Title | Status |
|---|---|---|
| P564 | Drop v0.3.2 one-time migrations | Pending |
| P565 | Repo rename: point URLs at kira-space | Pending |
| P566 | Site: mobile image zoom | Pending |
| P567 | Remove memory module, keep presentation docs | Pending |
| P568 | Close chapter | Pending |

Chapter end: non-UI full run, 1 code review round, close phase, PR.

## P564 Drop v0.3.2 one-time migrations

- Ask: delete every v0.3.2 one-time move, cleanup, tests, flags, env overrides, doc text.
- Source: `docs/NEXT_CHAPTER.md` (inventory + scope + test list). Last commit `git rm docs/NEXT_CHAPTER.md`.
- Precondition: user confirmed all 7 Mac checks in `NEXT_CHAPTER.md` (confirmed in session, 2026-11-07).
- Single implementer, no streams.

## P565 Repo rename: point URLs at kira-space

- Ask: repo renamed `vlad-cirstean/kira-studio` to `vlad-cirstean/kira-space`. Update only strings that must point at the real repo: install script URLs, release/API URLs, badges, clone URL, `REPO_URL`, `InstallScriptURL`, `verify-packaging.sh` pin, docs.
- Out of scope (user decision): Go module path `github.com/kirathecat/kira-studio`, code identifiers, "Kira Studio" prose, `docs/HISTORY.md`.
- `.github/workflows/release.yml` cannot be pushed from a session: use `docs/pending-changes/` per `docs/DEV_ENVIRONMENT.md`.

## P566 Site: mobile image zoom

- Ask: presentation site images zoom in too far on mobile. Fix so images fit the viewport.
- Verify with Playwright at phone width.

## P567 Remove memory module, keep presentation docs

- Ask: remove the memory module from the app for now (backend, frontend, MCP, settings flag, tests, app docs). Keep it mentioned only in the presentation site docs.
- Planner defines exact boundary (what counts as module vs shared code) and the git-revival note.

## P568 Close chapter

- Fold chapter into `docs/HISTORY.md` by hand per `CLAUDE.md`.
