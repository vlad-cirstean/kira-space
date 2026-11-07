# v0.3.3 code review, round 1 of 1

Base: `23788ccc` (chapter v0.3.2 merge, PR #34; last commit of prior review session's fixes and close). Diff
`git diff 23788ccc..HEAD`, HEAD `c6ee9401`. Base checked: `git log 23788ccc..HEAD` holds only v0.3.3 commits.

Scope: P564 drop of v0.3.2 one-time moves (Go, flow tests, docs), repo rename URLs (installer, update
checker, packaging pin, README/docs, pending `release.yml` patch), P565/P566 site redesign (hero, marquee,
bento, story, Build band, install, /build/, CSS motion, tests, 4 visual baselines), banner wording, test fix
`52fcde80`, CLAUDE.md split into `docs/WORKING_AGREEMENT.md` plus repo `CLAUDE.md`. Not in scope: P567
worktrees under `.claude/worktrees/`.

Checks run: `go build ./internal/... ./apps/kira-space/internal/...` clean; darwin pure-Go vet of
`internal/secrets` clean; gone-symbol grep (`MCPRemover|RunOneTimeMoves|wtmove|MoveSecretsFile|ProjectDirName|moveLegacyKey|ReadOnlyURI|legacy-mcp`)
empty outside history; `kira-studio` URL grep: only the pending `release.yml` line (patch present) and the
deliberate ARCHITECTURE rename note. Fresh `site` build plus Playwright probes (Chromium, 390 and 1280 px,
motion on and reduced) for findings 2 to 4. Baselines: 4 PNGs, 0.8 to 2.6 MB, sane.

CLAUDE.md split: every rule of `git show 23788ccc:CLAUDE.md` is present in `CLAUDE.md` or
`docs/WORKING_AGREEMENT.md`. Dropped text is examples only (P110 split rejection, P108 base `771512bc`, P107
CodeGraph counts, AG Grid, P98/P99, P101). Hook `postcompact-style-reminder.sh`, `checkprompts/style.go`,
ARCHITECTURE, DEV_ENVIRONMENT and README point at the new file. One stale pointer left (finding 8).

## Findings

### 1. High: v0.3.1 install updating straight to v0.3.3 silently loses every stored secret

- Where: `internal/secrets/keyring_darwin.go:28` (Keychain move gone), `apps/kira-space/main.go:87`,
  `internal/sqlitex/sqlitex.go:202` (baseline guard), `apps/kira-space/internal/storage/migrations/` (0090
  baseline, 0091 step), `internal/startupfail/classify.go:125`.
- What: the dropped moves were the only bridge from v0.3.1 data. Nothing now refuses a v0.3.1 home. The
  baseline guard only rejects schema below 0090; a v0.3.1 `kira.db` sits at 0090, so 0091 applies and boot
  continues.
- Failure: a v0.3.1 user who never opened v0.3.2 clicks Update (the v0.3.1 checker follows GitHub's rename
  redirect to the kira-space `releases/latest`, now v0.3.3). `loadOrCreateKey` finds no "Kira Space Secrets"
  item and mints a new key; every `secret_values` row (connection passwords, Api secrets) sealed under the old
  "Kira Studio Secrets" key fails to decrypt. `agent-secrets.db` is never read, so agent credentials vanish.
  Task worktrees under `~/wt` are no longer found by ADE. No alert; the user sees blank credentials.
- Fix: refuse a kira.db at exactly 0090 before 0091 applies (before `secrets.New`). Simplest: fold
  0090 and 0091 into one kira.db baseline at 0091, so a 0090 DB hits `SchemaTooOldError`; make the
  startupfail headline name the needed release per DB ("open with v0.3.2 first" for kira.db). Update the
  embed.go comment and the ARCHITECTURE baseline bullet (line 423).

### 2. Medium: landing "scroll-held story" never holds; desktop gets large empty gaps

- Where: `site/src/components/Story.astro:37,66`, `site/src/layouts/Landing.astro:28`.
- What: two causes. `story-hold relative` and `story-shot relative` put `relative` on the same element as the
  `@utility` that sets `position: sticky`; `relative` wins (computed `position: relative` in Chromium at
  1280 px). Even without it, `<main class="... overflow-hidden">` is a scroll container, which disables
  sticky for every descendant. `/build/` uses `overflow-x-clip` and does not have this problem.
- Failure: probe at 1280 px scrolls the story by 1200 px; the shot and the text column move by exactly the
  scroll distance, never pinned. `story-chapter` still forces `min-height: 85svh` per chapter, so each of
  the 4 chapters leaves a tall blank band under short content. ARCHITECTURE line 3866 and the P566 Result
  claim a held story.
- Fix: drop `relative` where the story utility sets position (put `relative` inside the utility's non-sticky
  branch, or wrap), and change Landing `main` to `overflow-x-clip` (as /build/). Add a site spec at 1280 px
  that scrolls the story and asserts the shot's top stays fixed.

### 3. Medium: reduced motion clips most engine names off screen

- Where: `site/src/components/EngineMarquee.astro:61,67`.
- What: under `prefers-reduced-motion: reduce` the row wrapper becomes `w-auto flex-wrap justify-center`,
  but the visible `<ul>` keeps `shrink-0`, so it stays at max-content width (1404 px for engines, 905 px for
  the second row) and never wraps. The wrapper centres it and the row's `overflow-hidden` clips both ends.
- Failure: at 390 px reduced motion, engines row spans x = -507 to 897: PostgreSQL, MariaDB, MySQL, SQLite,
  Kafka, SQS, S3, HTTP, gRPC and others are cut off or invisible; only about 3 pills show per row. At 1280 px
  the engines row still starts at x = -124. The "Ten engines" claim has no visible engines for these users.
  Existing reduced-motion test checks only `getAnimations()`, so it passes.
- Fix: add `motion-reduce:shrink` (or `motion-reduce:w-full motion-reduce:shrink`) to the `<ul>`. Extend the
  reduced-motion site spec to assert every marquee `li` lies within the viewport.

### 4. Medium: landing and /build/ ship client JS; the no-script test cannot see it

- Where: `site/astro.config.mjs` (no `prefetch` key), `site/tests/build.spec.ts:40`, `docs/ARCHITECTURE.md:3862`.
- What: Starlight sets `prefetch: config.prefetch ?? { prefetchAll: true }`, so Astro injects
  `<script type="module" src="/kira-space/_astro/page.*.js">` (prefetch, about 2.5 KB) into `<head>` of every
  page, `/` and `/build/` included (confirmed in a fresh build). The new test checks only `main script`, so
  it passes while the page runs a script. Predates this chapter, but P565/P566 asked for zero client JS and
  the SPEC Results and ARCHITECTURE state it holds.
- Failure: every landing visit runs a hover/viewport prefetch script and fetches docs pages on hover,
  contrary to the stated rule; a future script anywhere outside `main` also passes the test.
- Fix: set `prefetch: false` in `astro.config.mjs` (Starlight docs pages keep working without it), and assert
  `document.scripts.length === 0` on `/` and `/build/` in `site.spec.ts`.

### 5. Low: bento link names include an arrow glyph

- Where: `site/src/components/Bento.astro:94`.
- What: the `→` span sits inside the tile link without `aria-hidden`.
- Failure: screen readers announce "Git graph, right arrow" for each of 11 tiles.
- Fix: `aria-hidden="true"` on that span (or use the `arrow-right` icon, already `aria-hidden` by `Icon`).

### 6. Low: bento and hero fixture data is invented despite the "never invented" claim

- Where: `site/src/data/visuals.ts:1-2`, `site/src/components/bento/AgentsMini.astro:42`,
  `site/src/components/HeroStage.astro:59`.
- What: the header says values match the screenshot fixtures. Agent rows ("Add order search", "Fix grid
  paging", ...), MCP names (`acme-postgres`, ...), graph commits (`a91c3e2 Add search`), file tree and
  "13 agents, 2 need input" appear in no fixture under `apps/kira-space/tests/screenshots` (grep). Only the
  SQL and API values (Alice Morgan, 4217) match. P566 plan says the chip comes "from the board fixture's
  status bar"; it does not exist there.
- Failure: marketing mini UIs drift from the real app; the comment misleads the next editor.
- Fix: take names from the board fixture (`adeV2Control`, e.g. task `T_search`, "Make CI green 3/6") and the
  space fixture (`feature/search`, `v1.4.0`), or reword the comment to what is true.

### 7. Low: dead content fields after FeatureGrid and AppHero removal

- Where: `site/src/data/content.ts:4,6,25,28,36` and every feature's `icon`/`body`, `APP.icon`, `APP.summary`.
- What: `Feature.icon`, `Feature.body`, `AppContent.icon`, `AppContent.summary` and `SITE.title` have no
  reader left (grep across `site/src`, `site/tests`, `site/scripts`). knip does not flag object properties.
- Failure: copy edits land in fields nothing renders; `body` duplicates `line` per tile.
- Fix: delete the fields and their values.

### 8. Low: stale process pointers after the split

- Where: `site/scripts/docsRules.ts:2` ("Mirrors CLAUDE.md 'Communication style'");
  `docs/ARCHITECTURE.md:3863` (infinite-animation list omits the `gradient-text` sweep and the hero ring's
  `animate-breathe`, both infinite).
- Failure: a reader looking for the style rules in `CLAUDE.md` finds only an import line; the animation list
  under-reports running animations.
- Fix: point docsRules.ts at `docs/WORKING_AGREEMENT.md` "Communication style"; add sweep and breathe to the
  list.

## Dimensions with nothing to report

- Performance outside the known WebKit cost (P567): Go change is deletion only, no hot path touched. Site CSS
  (`Base.*.css` 92 KB raw) and inline SVG minis are in line with the page; no other real issue found.
- URL rename: complete for installer, update checker, packaging pin, Pages base, Playwright config, README,
  docs and site; `release.yml` change waits in `docs/pending-changes/` by design.
- Migration removal: no dangling callers, imports, flags or env overrides (`KIRA_HOME`, `KIRA_MEMORY_HOME`
  only in testdata); `flowharness` and `main.go` startup order consistent.
