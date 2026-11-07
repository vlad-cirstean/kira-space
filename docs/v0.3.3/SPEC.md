# v0.3.3

Cleanup chapter. Branch `chapter/v0.3.3`, tag `v0.3.3`. First `P` is P564 (highest prior: P563).

| Phase | Title | Status |
|---|---|---|
| P564 | Cleanup: drop v0.3.2 one-time migrations, repo rename URLs | Done |
| P565 | Presentation site: redesign, repo URLs, mobile zoom, drop carousel | Done |
| P566 | Presentation site: second design pass (identity, color, bento) | Done |
| P567 | Close chapter | Pending |

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
- Result: done. Repo URLs and Pages base point at `kira-space`. Carousel and detail crop removed; window shots serve the whole framed image with a 1136 px `srcset` derivative. Memory removed from `/` only. Redesign is CSS only (aurora hero, scroll-driven reveals, hover states) gated by `prefers-reduced-motion`; no new dependency. Site Playwright run: 215 pass; the only failure is the README `github.io/kira-space` check, pending P564. Visual baselines refreshed from real shots.

## P566 Presentation site: second design pass (identity, color, bento)

- Ask: user verdict on P565: grey cards and text, no personality. Give the landing page and /build/ a real identity: bold brand palette from the app icon, memorable hero, varied bento tiles with CSS/SVG visuals, scroll story, voice in copy. Zero client JS, reduced-motion respected.
- Plan: `plans/P566.md`. User reviews design shots before implementation.
- Result: done. Brand tokens from the app icon, flashy hero stage, engine marquee, bento of CSS and SVG mini UIs, scroll-held story, light Build band, install bookend, `/build/` carries the sparkle thread. Banner now reads "Personal project. Use with care." Large `blur`, `backdrop-filter` and `mask-image` layers stalled WebKit (landing under 2 fps, tests timed out); replaced by gradient fades, now WebKit holds 15 to 50 fps. Site Playwright in WebKit: 223 pass, 1 skip; Chromium site and build specs: 68 pass. Visual baselines refreshed from real shots. Zero client JS, no new dependency.

## P567 Close chapter

- Fold chapter into `docs/HISTORY.md` by hand per `CLAUDE.md`.
