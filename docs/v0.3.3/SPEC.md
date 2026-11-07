# v0.3.3

Cleanup chapter. Branch `chapter/v0.3.3`, tag `v0.3.3`. First `P` is P564 (highest prior: P563).

| Phase | Title | Status |
|---|---|---|
| P564 | Cleanup: drop v0.3.2 one-time migrations, repo rename URLs | Done |
| P565 | Presentation site: redesign, repo URLs, mobile zoom, drop carousel | Pending |
| P566 | Close chapter | Pending |

P564 and P565 run as parallel streams (own worktree each): P564 owns everything outside `site/`, P565 owns `site/`. Grouping in `plans/batch-P564-P565.md`.

Chapter end: non-UI full run, 1 code review round, close phase, PR.

## P564 Drop v0.3.2 one-time migrations

- Ask: delete every v0.3.2 one-time move, cleanup, tests, flags, env overrides, doc text.
- Source: `docs/NEXT_CHAPTER.md` (inventory + scope + test list). Last commit `git rm docs/NEXT_CHAPTER.md`.
- Precondition: user confirmed all 7 Mac checks in `NEXT_CHAPTER.md` (confirmed in session, 2026-11-07).
- Single implementer, no streams. Second item group, after the migration commits: repo rename URLs (below).

### P564 item: repo rename URLs (outside `site/`)

- Ask: repo renamed `vlad-cirstean/kira-studio` to `vlad-cirstean/kira-space`. Update only strings that must point at the real repo outside `site/`: install script URLs, release/API URLs, badges, clone URL, `InstallScriptURL`, `verify-packaging.sh` pin, docs.
- Out of scope (user decision): Go module path `github.com/kirathecat/kira-studio`, code identifiers, "Kira Studio" prose, `docs/HISTORY.md`.
- `.github/workflows/release.yml` cannot be pushed from a session: use `docs/pending-changes/` per `docs/DEV_ENVIRONMENT.md`.

### Result

- Dropped: legacy MCP removal, leftovers cleanup, agent secrets fold, Keychain rename, worktree move (`wtmove`, `projectdir.go`), `ade.PathSegment` now unexported, dead ARCHITECTURE/DEV_ENVIRONMENT/README text, `docs/NEXT_CHAPTER.md`.
- Rename: installer, update checker, `verify-packaging.sh` pin, README/docs URLs; `release.yml` change in `docs/pending-changes/.github__workflows__release.yml.patch` (apply from a Mac session, delete the patch in that commit).
- Left for stream B (`site/`): memory sentence in `site/src/content/docs/docs/space/memory.mdx` ("old `~/.kira-memory`"), and "Backups of one-time upgrades live in" in `site/src/content/docs/docs/start/how-it-works.md` (becomes "Config sync backups live in"). Pages links need `base: '/kira-space'` in `site/astro.config.mjs`.
- By hand, user: delete `~/.kira-space/backups/p557-*`, `~/.kira-space/config/workflows/.p526-backup/`, `~/.kira-space/.legacy-mcp-removed`.
- Mac hand-off: `go vet ./internal/secrets && go test ./internal/secrets -run Keychain` (`keyring_darwin.go` not buildable on Linux).

## P565 Presentation site: redesign, repo URLs, mobile zoom, drop carousel

- Ask, one phase for all `site/` work:
  - Redesign: modern, flashy look that grabs attention. Subtle animation (scroll reveals, hero motion, hover states); respect `prefers-reduced-motion`; no heavy runtime cost. Planner picks an open-source animation approach (CSS first, a small library only against a named need).
  - Repo URLs: `REPO_URL` (`astro.config.mjs`, `src/data/nav.ts`), install snippets, release links in `site/`.
  - Mobile: images zoom in too far on phones. Fix so images fit the viewport.
  - Build with Kira page: drop the carousel (`BuildCarousel.astro`); keep the rest of the page.
  - Memory: remove mentions from the site's first page (landing) only. Other pages, site docs and all app code stay as is.
- Verify with Playwright at phone width.

## P566 Close chapter

- Fold chapter into `docs/HISTORY.md` by hand per `CLAUDE.md`.
