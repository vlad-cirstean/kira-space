# P492 ADE: agents widget shows active-agents indicator

Plan for SPEC row P492. Planned against `e2aabe5b` (P491 result). Stream C, last of P488, P491,
P492. One sequential implementer. Discovery used `codegraph_explore` over the status bar agents item
(`AdeAgentsStatusItem`, `AdeAgentsStatusList`, `AdeAgentsStatusRow`, `agentsSummary`,
`StatusBarItem`, `StatusBar.vue`), the activity model (`activityKind`, `withTuiActivity`,
`ACTIVITY_RANK`, `AdeActivityIcon`, `useSessions`, `AgentActivity`) and the existing indicator
primitives (`StatusDot`, `RunsStatusItem`). Then grep for `status-agents`, `space.statusBar.agents`,
`animate-pulse` and `prefers-reduced-motion`, specs and docs included.

## 1. Requirement and current behaviour

- SPEC row: the agents widget shows a distinct indicator when agents are running. Plan picks pulse
  dot, count badge, or both.
- Today `AdeAgentsStatusItem.vue` renders `StatusBarItem icon="robot"` with text `N agents` (plus
  `· M needs input`) only while `summary.total > 0`. `total` counts sessions with
  `state === 'running'`, idle ones included. Tone turns `warn` on input. Nothing else changes with
  activity: two idle agents and two busy agents look identical.
- So "running" cannot mean `Session.state === 'running'`: the item already exists only then, and an
  indicator keyed on it would duplicate the item's own visibility, not be distinct. The useful
  signal is an agent actively running a turn: `activityKind(s) === 'working'` (headless `activity`
  from the server, TUI phase from the agent-hook store via `withTuiActivity`). `summary.byKind.working`
  already holds that count.
- Decision: the indicator shows while `summary.byKind.working > 0`. Idle, waiting-on-monitor and
  needs-input agents show no dot. Needs-input keeps its existing warn tone and label. If the user
  meant "any running agent", the change is the one `v-if` condition (`summary.total > 0`); flag it in
  the Result, not a design rework.

## 2. Indicator: pulse dot, no count badge

Pick: one pulsing green dot, trailing the label. Reasons:

- Count badge declined: the label already prints the count (`2 agents`). A badge would repeat a
  number in a 22px bar. A second number (working count) next to the total reads as a contradiction
  ("2 agents" beside "1"). The tooltip already lists counts per activity (`Working 1`, `Idle 1`).
- Pulse dot: motion marks "live" at a glance without reading. It matches the ADE working glyph
  (green dot, `AdeActivityIcon`).
- Primitive: `@theme/components/StatusDot.vue` with `tone="ok"` and `pulse`. Guard U23
  (`scripts/check-ui-primitives.sh:166`) forbids raw `animate-pulse` outside `StatusDot`/`Spinner`,
  so no hand-rolled `<span class="animate-pulse ...">`. Tailwind only, no `<style>` block.
- Reduced motion: already handled globally. `packages/theme/src/base.css:96` sets
  `[data-slot="status-dot"] { animation: none; }` under `prefers-reduced-motion: reduce`. The dot
  stays visible and static, so the indicator stays distinct without motion. No new CSS.
- Placement: inside `StatusBarItem`'s default slot, after the label text. Reading order then gives
  the accessible name `2 agents 1 working`. `StatusBarItem.vue` needs no slot or prop change, so P492
  does not edit `packages/workbench` at all.
- Accessibility: `StatusDot :label` sets `role="img"` and `aria-label`. Label text reuses existing
  key `space.ade.allSessions.summaryWorking.label` (`{count} working`), the same family the item
  already reuses for `summaryInput`. No new i18n key; `en.json` untouched.
- Text unchanged: the dot has no text content, so `toHaveText('2 agents')` assertions stay true.

## 3. Change

`apps/kira-space/frontend/src/ade/v2/status/AdeAgentsStatusItem.vue` only:

1. Import `StatusDot from '@theme/components/StatusDot.vue'`.
2. Update the header comment: one clause that a pulsing dot marks agents mid-turn (working).
3. In the default slot of `StatusBarItem`, after `{{ label }}`:

   ```vue
   <StatusDot
     v-if="summary.byKind.working > 0"
     tone="ok"
     pulse
     :label="t('space.ade.allSessions.summaryWorking.label', { count: summary.byKind.working })"
     data-testid="status-agents-working"
   />
   ```

   `StatusDot` keeps default attribute inheritance, so `data-testid` lands on its root span.
   `StatusBarItem`'s Button has a flex gap (`size="status"`), so no margin class is needed; the
   implementer checks the rendered spacing in the visual baseline and adds none unless it is off.

Not touched: `agentsSummary.ts` (already counts `working`), `AdeAgentsStatusList.vue`,
`AdeAgentsStatusRow.vue`, `activity.ts`, `AdeActivityIcon.vue`, `StatusBarItem.vue`, `StatusDot.vue`,
`base.css`, mobile (`apps/kira-space/frontend/mobile/**`, no status bar), Go, wire, contracts, i18n.

## 4. Backend or frontend: frontend only

Decision: no backend change, no backend flow test.

- Every input already reaches the client: `useSessions` (`AdeTaskService.Sessions`) carries headless
  `activity`; TUI phase arrives on the agent event channel into `useAgentSessionsStore`. P492 adds
  no field, IPC call or event.
- CLAUDE.md "split at the IPC boundary" asks for a backend half when the feature crosses IPC. This
  one does not. The payload the UI spec reads is contract `ade-sessions-running`, already asserted
  backend-side by `adeflow` `TestRunningSessionsList` (`internal/flows/adeflow/sessions_test.go:347`);
  its `#mixed` sample already holds a working headless run. A new flow test would assert nothing
  P492 changed.

## 5. Library rules

- Component primitive: `StatusDot` (theme), inside shadcn-vue-backed `StatusBarItem`/`Button`. No
  hand-rolled dot or animation.
- Styling: Tailwind utilities through `StatusDot`; no scoped `<style>`.
- State: `summary` is a computed over the existing TanStack Query `useSessions` and the existing
  Pinia `useAgentSessionsStore`; no new store, query or composable. No VueUse need
  (`prefers-reduced-motion` is CSS, so `usePreferredReducedMotion` would add JS for what a media
  query already does).
- No new dependency.

## 6. Tests

UI spec as the SPEC row asks; backend half declined (4).

`apps/kira-space/tests/ui/status-agents.spec.ts`:

- Helper: extend `sessions(running = true)` with an option to force every session's `activity` to
  `'idle'` (e.g. `sessions({ running, idle })`), keeping the existing call sites' meaning. Use a
  locator `const dot = (page) => page.locator(t('status-agents-working'))`.
- Extend the first contract test (`the bar counts the running agents...`): after `toHaveText('2
  agents')`, assert `dot` visible, `aria-label` `1 working`, class matches `/(^|\s)animate-pulse(\s|$)/`,
  and the item text is still exactly `2 agents`.
- New test `contract: the working dot shows only while an agent works and follows the TUI
  activity`: sessions with every activity `idle`. Steps:
  1. Item text `2 agents`; `dot` count 0.
  2. `emitAgentSessions(page, ['term-qa01'])`, `emitAgentEvent(page, 'term-qa01', 'PreToolUse')`:
     `dot` visible, `aria-label` `1 working`, item `data-kind` `working`.
  3. `emitAgentEvent(page, 'term-qa01', 'Stop')`: `dot` count 0, item still visible with `2 agents`.
- New test `contract: reduced motion keeps the working dot but stops its pulse`:
  `page.emulateMedia({ reducedMotion: 'reduce' })` before asserting (fixture default sessions, one
  working). Assert `dot` visible and computed `animation-name` is `none`
  (`dot.evaluate((el) => getComputedStyle(el).animationName)`). Also assert, with
  `reducedMotion: 'no-preference'`, that `animation-name` is not `none`, so the test proves the
  media query, not a missing class.
- Existing `needs input` test: add one assertion that `dot` stays visible beside the warn tone
  (qa02 headless is still working), so the two signals stay independent.
- Visual: `apps/kira-space/tests/visual/status-bar.spec.ts` `status bar with agents, runs, update and
  metrics` changes (fixture has a working run, so the dot renders). Re-record only
  `status-bar.spec.ts-snapshots/status-bar*.png` per `docs/DEV_ENVIRONMENT.md` "`tests/visual/*`
  pixel diffs" (Inter font check first; never update from a mismatched font setup). `toHaveScreenshot`
  disables infinite animations, so the baseline is deterministic. Tooltip and popover baselines
  must not change; if they diff, investigate, do not re-record.
- Unit: none (CLAUDE.md bar: one condition).
- Run: `bun run build:test:space`, then `npx playwright test
  --config=apps/kira-space/playwright.config.ts --project=ui status-agents.spec.ts`, then the visual
  project for `status-bar.spec.ts`. Fast checks per commit through the pre-commit hook (includes
  `check-ui-primitives`).

## 7. Docs

- `docs/ARCHITECTURE.md:1606` (status bar bullet): after "shows `N agents · M needs input` from
  `useSessions`", add: "plus a pulsing `StatusDot` while any agent is working (static under reduced
  motion)". Nothing else in that bullet.
- No `DEV_ENVIRONMENT.md`, site or README change.

## 8. Commits

1. `feat: status bar agents item pulses while an agent works` (`AdeAgentsStatusItem.vue`,
   `status-agents.spec.ts`).
2. `test: re-record status bar visual baseline` (`status-bar.spec.ts-snapshots/status-bar*.png`).
3. `docs: agents item working dot` (ARCHITECTURE).
4. `docs: P492 result` (SPEC row Done plus Result).

## 9. File ownership (Stream C, P492)

Owned by P492 only. Stream A (P485-P487, `apps/kira-studio` Kubernetes) and Stream B (P490 Kira
Space file-open window shell, sidebar, workspace) touch none of them:

- `apps/kira-space/frontend/src/ade/v2/status/AdeAgentsStatusItem.vue`
- `apps/kira-space/tests/ui/status-agents.spec.ts`
- `apps/kira-space/tests/visual/status-bar.spec.ts-snapshots/status-bar*.png` (the bar baseline only)
- `docs/v0.2.29/plans/P492-agents-working-pulse.md`

Released from P491's reservation, not edited by P492: `AdeAgentsStatusList.vue`,
`AdeAgentsStatusRow.vue`, `agentsSummary.ts`, `activity.ts`, `AdeActivityIcon.vue`,
`packages/workbench/src/components/StatusBarItem.vue`. P492 needs no `StatusBarItem` change, so
the P491 flag on it is void for this chapter; Stream B still must not change it while P492 is in
flight, since the visual baseline above depends on its rendering.

Read, not edited: `StatusDot.vue`, `base.css`, `StatusBar.vue` (Kira Space workbench),
`tests/ui/support/adeV2.ts`, `tests/visual/status-bar.spec.ts`, contract `ade-sessions-running`.

Shared, with resolution:

- `docs/v0.2.29/SPEC.md`: each stream edits only its own rows (status cell, Result under its own
  `### P<n>`). On rebase keep both sides' hunks; never rewrite another stream's row.
- `docs/ARCHITECTURE.md`: P492 edits only the status bar bullet (`:1606`). Stream A edits Kubernetes
  sections, Stream B Kira Space window/file-open sections. If Stream B's window shell work edits the
  same status bar bullet (e.g. the file-open window hides the bar), keep both edits in one bullet.
  A textual conflict otherwise means adjacent hunks: keep both.
- `packages/theme/src/i18n/en.json`: P492 makes no change (reuses existing keys). Nothing to
  resolve.
- Contracts (`apps/*/tests/contract/*.json`), Wails bindings, `bridge/index.ts`, `ipcChannels.ts`,
  `mockRuntime.ts`, `go.mod`, `package.json`, lockfiles: P492 touches none.
- `apps/kira-space/frontend/src/workbench/StatusBar.vue`: P492 reads only. If Stream B's file-open
  window changes it (hide or trim the bar there), that is Stream B's file; P492's visual baseline is
  the main window's bar, so re-run `status-bar.spec.ts` after rebase and re-record only if Stream B's
  change is the diff's cause and Stream B has not already re-recorded it.
- Stream C runs in the main checkout on `chapter/v0.2.29`. Other streams land by rebase; commit only
  P492's own files and retry on `.git/index.lock`.

## 10. Ordering consequences

None for Stream A. Stream B: no shared file except the docs above; see the `StatusBar.vue` and
baseline note in 9. P492 is the last implementation phase of Stream C; the chapter's one full suite
run (CLAUDE.md) follows the last implementation phase across all streams, before P493.
