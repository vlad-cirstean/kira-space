# v0.3.5

User-requested fixes after v0.3.4. Chapter stays open: more phases get added later. Branch `chapter/v0.3.5`, tag `v0.3.5`. First `P` is P575 (highest prior: P574).

| Phase | Title | Status |
|---|---|---|
| P575 | Presentation site and README: headline, share image, screenshot click, install copy, one icon, header Install button, README images | Implemented |
| P576 | Title bar: app title after window controls | Implemented |
| P577 | Terminal-less run logs and sessions: scroll, select, copy | Planned |

Phases run in table order, one at a time, unless the user asks for a batch.

## P575 Presentation site and README: headline, share image, screenshot click, install copy, one icon, header Install button, README images

- Ask (user):
  - Hero headline becomes "Everything agents need, in one place", replacing the current one. Root `README.md` carries the same headline (normal prose, outward-facing).
  - Social and link thumbnail (`og:image` and the twitter image) is not the first screenshot. It is a purpose-made image with the Kira logo (the one site icon below).
  - Clicking a screenshot must not navigate to the image file (user has to press back): no link to the image file, no navigation. A lightbox or zoom is not asked for; opening nothing is acceptable. Planner chooses.
  - Install command on the site gets a copy icon button: copies the command without the prompt, brief copied feedback, accessible name, keyboard operable. Reuse the site's icon set; small dependency-free script, no new dependency.
  - The `$` prompt before any shell snippet on the site is not selectable and not read by screen readers: selecting and copying the command yields the command only.
  - One icon across the site: the rounded-border icon the site header uses. Favicon, docs (Starlight logo and favicon), apple-touch icon and share image all derive from it; other icon sources leave the site. A site check fails when the built site references more than one icon source.
  - Header Install button looks different from the site's other buttons: give it the shared button style.
  - `README.md` shows the memory screenshot; replace it. README opens with the one Kira icon and the new headline, then the primary screenshot (agents board); other screenshots follow. Planner proposes the replacement shot; user may overrule. Image paths resolve on GitHub (committed files or the published `shots/` URLs).
- Acceptance: new headline renders on the site and in `README.md`; `og:image` and twitter image meta point to the new image; no screenshot is wrapped in a link to its file; copy button copies the bare command; selecting the command yields no `$`; landing and docs `<link rel="icon">` resolve to the same asset; header Install button matches the shared button style; `README.md` shows no memory screenshot and every README image resolves; `bun site/scripts/check-docs.ts` reports 0 violations.
- Out of scope: app icon variants (`apps/kira-space/build/appicon.icon/`, macOS bundle icons).
- Pointers: `site/` (Astro), `docs/DEV_ENVIRONMENT.md`.
- Result: headline "Everything agents need, in one place." on the hero (two lines) and as the README's first paragraph. Screenshots sit in no link (`Shot.astro`, `Hero.astro`); clicking one opens nothing. Install card has a copy button (Codicon `copy`, swaps to `check`, `role="status"` text, native button so Enter and Space work); the `$` prompts are `aria-hidden` plus `select-none`, selection yields the bare command (WebKit test). One icon, `apps/kira-space/build/appicon.png`: header, footer, hero, `/favicon.png` (64 px endpoint, replaces `favicon.svg.ts`), apple-touch, Starlight logo and favicon, share image; `site/scripts/check-icons.ts` runs in `site:build` (28 pages, 0 violations). Share image `site/src/assets/share/kira-share.webp` from `bun run site:share-image` (two runs `cmp` equal), PNG built in `Base.astro` for `og:image` and `twitter:image`; `social` and `lead` removed. Header Install uses `site/src/lib/buttons.ts`, shared with Hero and the Build CTA. README opens with the icon, headline and agents board; `space-memory` replaced by `space-runs`; README test checks every image path. ARCHITECTURE and DEV_ENVIRONMENT updated. Checks: `bun run lint`, `bun run typecheck:site`, `typecheck:docs-check`, biome, `check-docs` (0 violations), full site Playwright suite on real shots from `bun run screenshots` (239 passed, 1 skipped). Visual baselines refreshed locally (`fc-match sans-serif` is Inter) after a look at both widths.
  - User may overrule: trailing period in the headline; no lightbox (images are plain, non-focusable); one small script on the site, so `ships no document script` became `ships only the copy script` and ARCHITECTURE no longer says zero client JavaScript; WebP share image committed (guard N15 allows it) with the PNG built at site build; README image order and `space-runs` as the memory replacement.
  - Deviations from the plan: share image headline wraps over three lines at the 640 px column (Inter 76 px), tagline uses `text-wrap: balance`; hero `h1` widened to `max-w-6xl` so the desktop headline breaks after the comma; "no image inside a link" tests scope to `main` because the header and Starlight logos link home; added `site/scripts/raw.d.ts` so `typecheck:docs-check` accepts `content.ts` through `icons.ts`; check-icons skips meta-refresh redirect pages; README test allows the PR badge URL.

## P576 Title bar: app title after window controls

- Ask (user): app title bar shows the app title "Kira Space" immediately after the window control buttons (macOS traffic-light buttons).
- Spelling: "Kira Space" is in `PROPER_NOUNS` (`scripts/copy/catalogRules.ts`); copy-catalog sentence-case rules apply and the catalog lint must pass.
- Acceptance: title sits directly after the window controls, no gap element between.
- Result:
  - Title is first child of the `TitleBar` slot in `WorkbenchTitleBar` (`appTitle` prop, `data-testid="titlebar-app-title"`), at the 78 px traffic-light inset. Stays in the drag region (no `wails-no-drag`). Spec asserts first child, offset equals inset 78, `--wails-draggable: drag`.
  - App.vue passes existing catalog key `common.product.kiraSpace.label`; no new key. Stale "No app title" comment in `ModeSwitcher.vue` dropped.
  - Rendered on every platform, no sniff (macOS only target). Linux dev builds show the title twice (native frame plus bar).
  - Spec pins no overlap or clipping at the 1024 px window floor.
  - No visual baseline changed (no `*-snapshots/` touched).
  - Checks: `window-chrome studio/workbench copy-coverage` ui specs, 76 passed; typecheck and lint clean.

## P577 Terminal-less run logs and sessions: scroll, select, copy

- Ask (user):
  - (a) A script run started by an AI (agent run, headless MCP start, log tab opened from Runs/Operations) shows a log view the user cannot scroll. Fix scrolling.
  - (b) Text in those logs cannot be selected or copied. Fix selection and copy.
  - (c) Non-interactive Claude sessions (stream/transcript view of headless Claude sessions) have the same selection and copy problem. Fix it too.
- Tests: where a backend half exists, flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).
