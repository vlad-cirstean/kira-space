# P565 Presentation site: redesign, repo URLs, mobile zoom, drop carousel

Plan only. Base commit `8ede593c`. Single implementer, no streams.
Discovery: CodeGraph MCP failed to connect this session (`codegraph` not in `$PATH`); all lookups used `rg` and direct reads.

## Scope

- Everything under `site/`, plus site screenshot tooling: `apps/kira-space/tests/screenshots/support/shoot.ts`, `packages/workbench/src/testing/screenshots/capture.ts`.
- Site bullets in `docs/ARCHITECTURE.md` "Presentation site (P280)" and `docs/DEV_ENVIRONMENT.md` "Presentation screenshots and site check (P280)".
- Not touched: app code, `README.md`, workflows, `docs/HISTORY.md`, any docs page under `site/src/content/docs/**` except the repo URL lines named below.
- Parallel phases share the checkout. Stage and commit explicit paths only (`git add <paths>`, `git commit <paths>`), never `git add -A`. Before committing `docs/ARCHITECTURE.md` or `docs/DEV_ENVIRONMENT.md`, run `git diff <file>`. If it holds hunks that are not yours, wait for that phase to commit, then commit.

## 1. Repo URL and Pages base

GitHub Pages has no rename redirect. The site moves to `https://vlad-cirstean.github.io/kira-space/`.

- `site/astro.config.mjs`: `REPO_URL` to `https://github.com/vlad-cirstean/kira-space`; `base: '/kira-space'`; both redirect targets to `/kira-space/...`. `site` stays `https://vlad-cirstean.github.io`.
- `site/src/data/nav.ts`: `REPO_URL` to `.../kira-space`. Footer "Source of this page" derives from it.
- `site/src/data/content.ts`: `INSTALL.command` to `https://raw.githubusercontent.com/vlad-cirstean/kira-space/main/scripts/install.sh`.
- `site/src/content/docs/docs/start/install.md`: line 20 install URL, line 50 README link.
- `site/src/content/docs/docs/reference/features.md`: line 11 releases link.
- Tests: `site/playwright.config.ts` `webServer.url`; `BASE`/`URL` in `site/tests/site.spec.ts`, `build.spec.ts`, `docs.spec.ts`, `visual.spec.ts`; `site.spec.ts:216` regex to `/kira-space/#install$`; `site.spec.ts:244` to `https://vlad-cirstean.github.io/kira-space/#install`.
- `site.spec.ts:244` reads `README.md` line 1. README is owned by P564 (repo rename, non-site). If README still says `kira-studio` when this test runs, stop and report to the orchestrator. Do not edit README.
- Keep `kira-studio` where it names an MCP server (`kira-studio`, `kira-studio-session` in docs `database-mcp.mdx`, `mcp-tools.md`, `mcp.md`, `how-it-works.md`). These are app identifiers, not URLs.
- Assets and links already route through `import.meta.env.BASE_URL`/`href()`; no hard-coded `/kira-studio/` exists in `site/src` beyond the config. Confirm with the gone-check.

Gone-check (expect no output):

```sh
rg -n "vlad-cirstean/kira-studio|github\.io/kira-studio|/kira-studio/|'/kira-studio'" site --glob '!node_modules'
```

## 2. Build with Kira: drop the carousel

- `git rm site/src/components/BuildCarousel.astro`. It holds the only client script on the marketing pages; removing it restores zero client JS on `/` and `/build/`.
- `site/src/pages/build.astro`: remove import and `<BuildCarousel>`. Keep heading, intro, sticky step nav, sections, gallery, install CTA. Adjust step nav top margin now that no block sits above it.
- `site/src/data/build.ts`: delete `BUILD.carouselLabel`. Reword the `shots` doc comment: first shot leads the section and supplies `og:image`, so it must be a window shot.
- `site/src/lib/build.ts`: keep `lead` (used for `og:image` via `first?.lead.social`). No logic change.
- `site/src/data/icons.ts`: delete `pause` and `play` entries and their `debug-pause.svg`/`play.svg` imports. Only the carousel used them (`rg "'pause'|'play'|name=\"pause\"|name=\"play\""` to confirm). Keep `play-circle`.
- `site/tests/build.spec.ts`: delete `dots`, `active`, `paused` helpers, the slides test, arrows test, rotation-control test, `autoplay` describe and reduced-motion autoplay test. Keep sections, step nav and image tests. Add one assertion: no `[data-carousel]` and no `<script>` element in `main` on `/build/`.

## 3. Mobile: images zoom in too far

Root cause (verified by reading code): `site/src/components/ShotPicture.astro` serves `shot.detail` below 768 px. That file is a 440 x 330 CSS px crop (`DETAIL_SIZE`, `capture.ts:28`) of the 1024 x 640 window at a per-shot offset (`shotCapture.ts` `detail: {x, y}`). A phone sees under half the window width, enlarged to full column width: the "zoom". Shot.astro adds a phone-only rounded clip box around it.
Ruled out: viewport meta is `width=device-width, initial-scale=1` (`Base.astro`); no lightbox (shot links open the raw file); no `object-fit: cover`; crop shots render at half natural size with `max-w-full`; the Keychain image is `h-auto max-w-full`.

Fix: serve the whole framed window at every width, scaled to fit.

- `ShotPicture.astro`: drop `<picture>`/`<source>`. One `<img>` with `src` the framed WebP, `srcset` of a 1136 px wide derivative plus the original, `sizes="(min-width: 1024px) 1024px, 100vw"`, `width`/`height` from the framed meta, `h-auto w-full`, no forced aspect class.
- `site/src/lib/shots.ts`: drop `detail`, `detailWidth`, `detailHeight` from `ResolvedWindowShot`; add `framedSmall` (1136 px wide WebP via `getImage({ src, width: 1136, format: 'webp' })`, build time, sharp already a dependency). Placeholder mode: `framedSmall` is the placeholder src. `placeholder` flag from the main file only.
- `Shot.astro`: window link loses the phone-only `overflow-hidden rounded-xl bg-chrome shadow-shot ring-1` and `md:` resets. The framed art carries its own frame, rim and shadow on a transparent margin, so a box behind it shows as a grey rectangle.
- `site/src/data/shotCapture.ts`: window kind becomes `{ readonly kind: 'window' }`; delete every `detail: {...}`. `shotFiles` returns `{ main }` only.
- `site/scripts/shots-fill.ts`: drop the `phone` branch.
- `apps/kira-space/tests/screenshots/support/shoot.ts`: call `captureShot(page, { id, consoleErrors })`.
- `packages/workbench/src/testing/screenshots/capture.ts`: delete `DETAIL_SIZE`, `ShotOptions.detail`, the clip screenshot and `saveShot(`${id}-detail`)`; fix the `captureShot` doc comment. `rg -n "captureShot|DETAIL_SIZE|-detail" apps packages site --glob '!**/node_modules/**'` must show no stale caller.
- `pages.yml` cache key already hashes `shotCapture.ts`, so CI recaptures; a fallback cache with stale `-detail` files is harmless. No workflow edit.
- `site/tests/shotImage.ts`: window shot on any width: `currentSrc` contains the id and not `-detail`; rendered box shows the full frame: `clientWidth/clientHeight` within 1 % of `naturalWidth/naturalHeight`, computed `object-fit` not `cover`, box inside the viewport. Drop the `phone` parameter. Update callers in `site.spec.ts`, `build.spec.ts`, `docs.spec.ts`; rename the site.spec test to "every shot shows the whole image and fits the viewport".

Gone-check (expect no output): `rg -n "detail(Width|Height)?\b|-detail|DETAIL_SIZE" site/src site/scripts site/tests apps/kira-space/tests/screenshots packages/workbench/src/testing/screenshots | rg -v "commit-detail|detail-region|ade-details"`.

## 4. Memory: off the landing page only

Final scope from the orchestrator: remove memory mentions from the first page only (`/`, `site/src/pages/index.astro` and data or components used only by it). Every other page, all docs pages, nav and the docs sidebar stay as they are.

Inventory of `memor` hits in `site/` outside `site/src/content/docs/**`, classified:

- `site/src/data/content.ts:41` `APP.description` (landing meta and og description): landing. Drop ", memory".
- `content.ts:43` `APP.summary` (hero): landing. Drop "memory" from the list ("with coding agents and a terminal").
- `content.ts:66-70` features card "Memory": landing. Delete the entry (11 cards left; grid stays `sm:2 lg:3`).
- `content.ts:130-138` highlight `space-memory`: landing. Delete it (3 highlights; flip alternation still works).
- `site/tests/site.spec.ts:24` landing route `shots` lists `space-memory`: landing test. Remove it.
- `site/src/data/shots.ts:56-61` `space-memory` copy: keep. `docs/space/memory.mdx` renders it via DocShot.
- `site/src/data/shotCapture.ts:22` `space-memory`: keep, same reason.
- `site/astro.config.mjs:51` sidebar "Git, agents and memory": docs nav. Keep.
- `site/tests/docs.spec.ts:56,184`, `site/scripts/docsRules.spec.ts:47`: docs tests. Keep.
- `icons.ts` `lightbulb`: keep, `shots.ts` still uses it.

Gone-check, after `site:build`: `rg -i -c memor site/dist/index.html` prints nothing (exit 1).

Cross-phase risk for the orchestrator: the memory removal phase deletes the app memory module, so `apps/kira-space/tests/screenshots/space.shot.ts:365` can no longer capture `space-memory`, and `docs/space/memory.mdx` then fails the site build (missing shot). That phase must decide the shot's fate; P565 does not.

## 5. Redesign

Goal: modern, flashy, attention-grabbing; slight motion; zero client JS kept; no new hues (ARCHITECTURE: existing tokens only, `--tint`, `--kira-accent`, `--kira-info`).

Animation approach: CSS only. No new dependency.
- Hero motion: `@keyframes` with `animation-fill-mode: both`.
- Scroll reveals: CSS scroll-driven animations (`animation-timeline: view()`, `animation-range: entry 0% cover 30%`), inside `@supports (animation-timeline: view())`. Without support, content renders static and visible (progressive enhancement). Safari 26 supports it, and Safari is the only browser the target audience runs (macOS 26+).
- Hover states: transitions on `transform`, `opacity`, `box-shadow`, `border-color`.
- Libraries declined: Motion (MIT) and AOS (MIT) both need a client script, which breaks the zero-client-JS invariant; CSS meets all three asks (reveal, hero motion, hover). No named need remains.

Rules:
- Every animation and transition sits under `@media (prefers-reduced-motion: no-preference)` (or Tailwind `motion-safe:`). Under `reduce`, nothing animates and nothing starts hidden.
- Animate `transform` and `opacity` only (compositor); no layout properties, no `filter` animation on large boxes, no global `will-change`.
- Reveal motion is vertical only (`translateY`, `scale`): horizontal offsets break the `box.x >= 0` image check and risk horizontal scroll.
- Infinite animations limited to the hero background (slow, 18 to 24 s, alternate).

Where it lives:
- `site/src/styles/global.css`: keyframes in `@theme` as `--animate-*` (Tailwind 4), plus `@utility reveal`, `@utility hero-in` (delay via `--d` custom property for stagger), `@utility shine`. Keep `scroll-behavior: smooth` only under `no-preference`.
- `site/src/layouts/Landing.astro`: hero backdrop becomes an aurora: two or three blurred radial blobs (tint, accent, info) drifting on transform, plus a faint masked dot grid. Stays inside `main.isolate.overflow-hidden`.
- `site/src/components/AppHero.astro`: larger heading (`sm:text-7xl`), gradient-clipped text (fg to tint), staggered `hero-in` on eyebrow, heading, summary, buttons, shot. Primary button gets a hover lift and a shine sweep; secondary a border glow. Hero shot settles from a slight `perspective` tilt to flat, glow behind it breathes slowly.
- `FeatureGrid.astro`: cards reveal with stagger (nth-child delay), hover lift `-translate-y-1`, tint border, icon scale, radial spotlight layer.
- `Highlight.astro`: text and shot reveal; shot scales 0.96 to 1. Divider becomes a tint gradient line.
- `BuildTeaser.astro`: gradient border card (tint to accent), step links with hover arrow nudge.
- `Install.astro`: section reveal; terminal card gets a subtle top glow; copy unchanged.
- `Header.astro`: link hover underline grows from center; layout unchanged (44 px targets).
- `site/src/pages/build.astro`: hero heading matches landing style with a small backdrop; each section reveals; step number becomes a tint badge. Give `main` `overflow-x-clip` if a backdrop is added (html/body stay unclipped: site.spec forbids hidden/clip there).
- `Footer.astro`, `404.astro`, `BetaNotice.astro`: style consistency only, no motion needed.
- Keep: 16 px minimum body text on phone, 44 px tap targets, visible focus rings, `aria-hidden` on decorative layers and icons, `text-base` floor on `p`/`li`.

## 6. Tests and verification

New assertions (in `site/tests/site.spec.ts`, per route), each guarding something easy to break:
- Reduced motion: with `emulateMedia({ reducedMotion: 'reduce' })`, `document.getAnimations()` is empty after load and after a full scroll.
- No hidden content: with motion on, scroll to the bottom in steps, then every `.reveal` element has computed opacity 1. A broken view timeline would otherwise leave sections invisible.
- Phone-width full-frame check: covered by the new `expectShotImage` (section 3), run in the `phone` project (375 x 812, scale 2) and `desktop` (1280 x 800).

Run order:
1. Fast, per commit: `bun run site:build` (astro check plus build), `bun run lint` (biome, check-docs), `bun test site/scripts`.
2. Real shots for the mobile fix: `bun run screenshots` (needs WebKit; `/opt/pw-browsers/webkit-2370` exists; if launch fails, `scripts/prepare-ui-tests.sh`). If capture is blocked, use `KIRA_SITE_SHOTS=placeholder bun run site:build` and say so in the result. Placeholder shots cannot prove the zoom fix: record that as unverified.
3. `bunx playwright test --config=site/playwright.config.ts` (site, build, docs specs; phone and desktop projects, WebKit).
4. Visual baselines: intended change, so refresh all four `site/tests/visual.spec.ts-snapshots/*.png` with `--update-snapshots`, then rerun once without it. Note in the commit body whether real or placeholder shots were built.
5. Manual screenshots for the orchestrator: a scratchpad script (not committed; guard N15 forbids PNG under `site/` outside baselines) captures `/` and `/build/` full page at 375 px (`isMobile`, scale 2) and 1280 px, in both Chromium and WebKit, motion on and reduced. Write to the session scratchpad and list the paths in the result.
6. Gone-checks from sections 1, 3, 4.

## 7. Docs

`docs/ARCHITECTURE.md` "Presentation site (P280)":
- URL to `https://vlad-cirstean.github.io/kira-space/`; base `'/kira-space'` in the routing bullet. If the repo-rename phase already changed the URL line, leave it.
- "zero client JavaScript" stays true; add one bullet: motion is CSS only (keyframes, scroll-driven `view()` timelines under `@supports`), gated by `prefers-reduced-motion: no-preference`; no animation library.
- Replace phone-crop wording: window shots serve the framed image at every width with a 1136 px derivative in `srcset`; no `-detail` file. Fix the `shotCapture.ts` bullet (`window` has no `detail`), the screenshot output bullet (drop the `<id>-detail.webp` 440 x 330 crop) and the `site:check` bullet ("phone crop and desktop framed window" becomes "whole framed window at both widths").
- Remove the stale "two static routes: / is Studio and /space/ is Space ... AppTabs" bullet only if it is plainly false today (no AppTabs component exists; `/space` redirects). It is site-owned and stale.

`docs/DEV_ENVIRONMENT.md` site section: nothing names `-detail`; check after edits with `rg -n "detail" docs/DEV_ENVIRONMENT.md`. Line 56 go-live URL belongs to the repo-rename phase.

## Commits (Conventional, explicit paths)

1. `fix(site): point repo links and Pages base at kira-space`
2. `refactor(site): drop Build with Kira carousel`
3. `fix(site): show whole framed window on phones, drop detail crop`
4. `feat(site): remove memory from the landing page`
5. `feat(site): redesign with CSS motion and reduced-motion fallback` (may split per component group)
6. `test(site): reduced-motion, reveal and full-frame checks; refresh baselines`
7. `docs: update presentation site notes for P565`

## Done when

- All gone-checks clean; `site:build`, `lint`, `bun test site/scripts` and the full site Playwright run pass on rerun.
- `/` and `/build/` ship no `<script>`; no `BuildCarousel` reference anywhere.
- Phone screenshots show whole windows, not crops.
