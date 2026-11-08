# v0.3.5

User-requested fixes after v0.3.4. Chapter stays open: more phases get added later. Branch `chapter/v0.3.5`, tag `v0.3.5`. First `P` is P575 (highest prior: P574).

| Phase | Title | Status |
|---|---|---|
| P575 | Presentation site and README: headline, share image, screenshot click, install copy, one icon, header Install button, README images | Planned |
| P576 | Title bar: app title after window controls | Planned |
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

## P576 Title bar: app title after window controls

- Ask (user): app title bar shows the app title "Kira Space" immediately after the window control buttons (macOS traffic-light buttons).
- Spelling: "Kira Space" is in `PROPER_NOUNS` (`scripts/copy/catalogRules.ts`); copy-catalog sentence-case rules apply and the catalog lint must pass.
- Acceptance: title sits directly after the window controls, no gap element between.

## P577 Terminal-less run logs and sessions: scroll, select, copy

- Ask (user):
  - (a) A script run started by an AI (agent run, headless MCP start, log tab opened from Runs/Operations) shows a log view the user cannot scroll. Fix scrolling.
  - (b) Text in those logs cannot be selected or copied. Fix selection and copy.
  - (c) Non-interactive Claude sessions (stream/transcript view of headless Claude sessions) have the same selection and copy problem. Fix it too.
- Tests: where a backend half exists, flow test plus UI spec on the same scenario, split at the IPC boundary (CLAUDE.md Tests).
