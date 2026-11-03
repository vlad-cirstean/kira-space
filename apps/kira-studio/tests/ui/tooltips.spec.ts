import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot, PortSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import {
  installFakeTimers,
  runPastTooltipDelay,
  runToJustBeforeTooltipDelay,
  settleTooltipSkipDelay,
} from './support/clock';
import { connectionCreateArgs, createConnection } from './support/connect';
import { IPC } from './support/ipcChannels';
import {
  COMPOSITE_PK_META,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { assertTooltipShows, tooltipContent } from './support/tooltip';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// Ported from tests/e2e/tooltips.spec.ts (P57 D16), against the same real-captured
// app.composite_pk fixture mutations.spec.ts uses (a genuine PK, no inbound FK — here it's the PK
// checkbox in ColumnsMenu that scenario 3 needs). The original's own `beforeAll`/`afterAll` and
// `test.describe.configure({ timeout: 300_000 })` existed only to stand up and tear down a real
// Docker Postgres container — nothing to port, there is no container in this tier. Its
// `createConnection` helper was a raw `page.evaluate(() => window.kira.connectionsCreate(...))`
// call, the pre-migration escape hatch — `window.kira` no longer exists post-M2/M3 (CLAUDE.md's
// P57 finding), so both connections here are created through the real dialog instead, the same
// flow mutations.spec.ts's own read-only-connection scenario already uses. Everything else is one
// continuous session with no relaunch(), so it all ports unchanged.
//
// P104 §6.5: scenarios 1-4 (below) exercise controls already converted to the real
// Tooltip/TooltipTrigger/TooltipContent trio (reka's TooltipProvider owns delay/rearm now —
// state/tooltip.ts's own timers are gone from these call sites), hit-testing a disabled control
// (scenario 2, §6.3's span-wrapper pattern) and a control inside an already-open popover
// (scenario 3, F3(a) — the popover's own backdrop must not swallow the hit test). Scenario 5 and
// the two geometry tests below exercise SlickGrid's own header cells (§6.4's one residue): that
// DOM is SlickGrid's, not Vue's, so AttributeTooltip.vue drives the same TooltipProvider/
// TooltipContent through a pointermove listener scoped to `.slick-header-columns` instead of a
// real TooltipTrigger. A-final (P104 §8.3) is what finished converting scenario 5 and the
// geometry tests off the deleted AppTooltip.vue singleton onto this — the last real users of the
// old mechanism, caught by the same acceptance-gate grep as the primitive call sites.

const DB_PATH = 'database:kira_test';
const APP_PATH = `${DB_PATH}/schema:app`;

const CONNECTION_ID = 'conn-tooltips';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Tooltips DB', 'blue');
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);

const RO_CONNECTION_ID = 'conn-tooltips-ro';
const RO_CONNECTION_SUMMARY = {
  ...postgresConnectionSummary(RO_CONNECTION_ID, 'Tooltips DB (RO)', 'red'),
  readOnly: true,
};
const RO_FIXTURE = compositePkConnectAndOpen(RO_CONNECTION_ID);

// Split out so the geometry tests below (which need a real grid open, for `.slick-header-columns`
// to exist, but no read-only connection) can relaunch with just the RW half rather than the full
// scenario-1 fixture set.
const RW_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tooltips DB'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
];
const RW_PORT: PortSnapshot[] = [...FIXTURE.port];

const CONTROL: ControlSnapshot[] = [
  ...RW_CONTROL,
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tooltips DB (RO)'),
    response: RO_CONNECTION_SUMMARY,
  },
  ...RO_FIXTURE.control,
];

const PORT: PortSnapshot[] = [...RW_PORT, ...RO_FIXTURE.port];

// Every scenario, including 5 and the geometry tests: the real reka tooltip, rendered through a
// TooltipPortal — `[data-slot="tooltip-content"]` is `ui/tooltip`'s own TooltipContent.vue marker
// (the migration selector), not tied to any one call site's DOM. AttributeTooltip.vue
// (the grid-header bridge) renders through this exact same component. `tooltipContent`/
// `assertTooltipShows` live in `./support/tooltip` — the ipc/e2e-real specs' status-dot and
// toolbar-button tooltip assertions share this exact helper, not a re-copy.

/** Scenario 1's own connect-and-open steps, factored out for the geometry tests below: they need
 *  a real grid open (so `.slick-header-columns` exists in the DOM for AttributeTooltip.vue's own
 *  container prop, §6.4) but not the read-only connection scenario 2 goes on to add. */
async function openConnectionAndGrid(page: Page): Promise<void> {
  await createConnection(page, 'postgres', 'Tooltips DB');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
}

test('tooltips — app-owned surface: delay, disabled controls, popovers, a11y', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });

  await openConnectionAndGrid(page);
  const connRow = connectionRow(page);

  // --- scenario 1: an enabled control — hidden before the delay, shown after, hides on leave ---
  // A cold hover, not a scan: settle the pointer away from the tree first and outlast
  // TooltipProvider's own skipDelayDuration (300ms), or this hover lands in the rearm window left
  // by the dblclick above and opens immediately instead of waiting the full delayDuration.
  await installFakeTimers(page);
  await page.mouse.move(4, 4);
  await settleTooltipSkipDelay(page);
  const refreshButton = page.locator('[data-testid="toolbar-refresh"]');
  await refreshButton.hover();
  await runToJustBeforeTooltipDelay(page);
  await expect(tooltipContent(page)).toHaveCount(0);
  await runPastTooltipDelay(page);
  await expect(tooltipContent(page)).toBeVisible({ timeout: 3_000 });
  // reka's own TooltipContent renders a visually-hidden a11y mirror span alongside the visible
  // text, so a bare .textContent read (toHaveText) sees "RefreshRefresh" — toContainText reads
  // the visible text node correctly without depending on that internal duplication.
  await expect(tooltipContent(page)).toContainText('Refresh');

  await page.mouse.move(4, 4);
  await expect(tooltipContent(page)).toHaveCount(0);
  await page.clock.resume();

  // --- scenario 3: over an overlay — a popover's own backdrop must not swallow the hit test ---
  // (F3(a)). ColumnsMenu's PK checkbox carries a hint only when it's the locked one. Checkbox is
  // reka's own CheckboxRoot now (role="checkbox" on a <button>, not a native <input>).
  await page.click('[data-testid="toolbar-columns"]');
  await expect(page.locator('[data-testid="columns-menu"]')).toBeVisible();
  const pkCheckbox = page
    .locator('.columns-menu-item.is-pk [data-testid="columns-menu-item"]')
    .first();
  await expect(pkCheckbox).toBeVisible();
  await assertTooltipShows(page, pkCheckbox, /Primary key, always shown/);
  // reka's DismissableLayer dismisses only its own (highest) layer per Escape press — with the
  // tooltip's layer stacked above the menu's, one Escape closes the tooltip, not both at once.
  // Move off the trigger first (a real mouseleave, same as scenario 1's own close) so the
  // tooltip's layer is already gone and Escape reaches the menu's layer instead.
  await page.mouse.move(4, 4);
  await expect(tooltipContent(page)).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="columns-menu"]')).toHaveCount(0);

  // --- scenario 2: a disabled control (F5/D3) — a naive mouseenter implementation never sees
  // this hover at all, since Blink dispatches no pointer events on a disabled form control.
  await connRow.locator('[data-testid="tree-twisty"]').click(); // collapse — keeps the two connections' tree paths distinct
  await createConnection(page, 'postgres', 'Tooltips DB (RO)');

  const roConnRow = connectionRow(page, 'Tooltips DB (RO)');
  await expect(roConnRow).toBeVisible();
  await roConnRow.click({ button: 'right' });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  await page.click('[data-testid="menu-item-connect"]');
  await expect(roConnRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await roConnRow.locator('[data-testid="tree-twisty"]').click();
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
  await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();

  const addRowButton = page.locator('[data-testid="toolbar-add-row"]');
  await expect(addRowButton).toBeDisabled();
  // §6.3's own pattern: the disabled <button> itself receives no pointer events in Blink, so the
  // never-disabled wrapper <span> is the real trigger and hit target.
  await assertTooltipShows(page, addRowButton.locator('xpath=..'), 'Connection is read-only');

  // --- scenario 4: pointer-events: none (D4/disable-hoverable-content) + accessibility (D7) ---
  const projectPanel = page.locator('[data-testid="project-panel"]');
  const wasVisible = (await projectPanel.count()) > 0;
  const toggleButton = page.locator('[data-testid="toggle-project-panel"]');
  await assertTooltipShows(page, toggleButton, 'Connections');
  // reka generates the content id itself (no fixed "app-tooltip" id any more) — assert the
  // trigger's aria-describedby actually resolves to real, matching content, not a literal string.
  const describedById = await toggleButton.getAttribute('aria-describedby');
  expect(describedById).toBeTruthy();
  await expect(page.locator(`#${describedById}`)).toContainText('Connections');
  await expect(toggleButton).toHaveAttribute('aria-label', 'Connections');

  // The tooltip sits at a higher z-index than everything else in the app, directly over the
  // button it describes — if it intercepted pointer events, this click would hit the tooltip
  // instead and the panel would never toggle.
  await toggleButton.click();
  await expect(projectPanel).toHaveCount(wasVisible ? 0 : 1);
  await toggleButton.click(); // restore, so the tree is still usable if anything runs after this
  await expect(projectPanel).toHaveCount(wasVisible ? 1 : 0);

  // --- scenario 5: a structured tooltip — the grid header's own name/type/
  // description renders as three separate elements (AttributeTooltip.vue's title/meta/body),
  // while data-kira-tip stays the exact same newline-joined plain text every existing assertion
  // (and the a11y mirror) already reads. -----
  const tenantIdHeader = page.locator('[data-testid="grid-header-cell"][data-column="tenant_id"]');
  await tenantIdHeader.hover();
  await expect(tooltipContent(page)).toBeVisible({ timeout: 3_000 });
  await expect(tooltipContent(page).locator('[data-testid="tooltip-title"]')).toHaveText(
    'tenant_id',
  );
  await expect(tooltipContent(page).locator('[data-testid="tooltip-meta"]')).not.toBeEmpty();
  await expect(tooltipContent(page).locator('[data-testid="tooltip-body"]')).not.toBeEmpty();
  const meta = (
    await tooltipContent(page).locator('[data-testid="tooltip-meta"]').innerText()
  ).trim();
  const body = (
    await tooltipContent(page).locator('[data-testid="tooltip-body"]').innerText()
  ).trim();
  const tags = await tooltipContent(page).locator('[data-testid="tooltip-tag"]').allInnerTexts();
  expect(tags).toEqual(['PK', 'NOT NULL']);
  await expect(tooltipContent(page).locator('[data-testid="tooltip-note"]')).toHaveCount(0);
  const plain = ['tenant_id', meta, ...tags, body].join('\n');
  await expect(tenantIdHeader).toHaveAttribute('data-kira-tip', plain);
  await expect(tenantIdHeader).toHaveAttribute('aria-label', plain);

  // A hover elsewhere closes it — AttributeTooltip's own `leave()`, same rearm-window behaviour as
  // every other tooltip in this app (TooltipProvider's own skipDelayDuration).
  await page.mouse.move(4, 4);
  await expect(tooltipContent(page)).toHaveCount(0);

  // A plain-string tooltip elsewhere is unaffected — still one text node, no parts.
  await page.clock.pauseAt((await page.evaluate(() => Date.now())) + 10_000);
  await settleTooltipSkipDelay(page);
  await refreshButton.hover();
  await runToJustBeforeTooltipDelay(page);
  await runPastTooltipDelay(page);
  await expect(tooltipContent(page)).toContainText('Refresh');
  await expect(tooltipContent(page).locator('[data-testid="tooltip-title"]')).toHaveCount(0);

  expect(consoleErrors).toEqual([]);
});

// P104 A-final: this pair used to drive the app-owned computeFloatPosition wrapper through the
// now-deleted AppTooltip.vue singleton. Grid-header tooltips (§6.4) are reka's own TooltipContent
// now (AttributeTooltip.vue), so the geometry that needs a regression test is reka's own
// flip/shift, reached the same way production does: a synthetic `data-kira-tip` cell inside the
// real grid's `.slick-header-columns` (AttributeTooltip's own `container`), not `document.body` —
// the pointermove listener is scoped to that element (§6.4), and bubbling is DOM-tree-based, so a
// fixed-position child still reaches it regardless of where in the viewport it visually sits.
// reka's own TooltipContentImpl defaults to `side: "top"` (not this app's old 'bottom-start'), so
// the two cases below test the flip/shift this app's markup actually exercises today.
//
// Real bug found live, this session: slickTheme.css's own `.slick-header-columns { will-change:
// transform }` (P22's measured header-scroll-flicker fix, unrelated to this test and not to be
// touched) makes that element the CONTAINING BLOCK for a `position: fixed` descendant — the exact
// same effect a real `transform` has (CSS spec) — so a `top`/`left`/`right`/`bottom` given to
// `style` below is no longer viewport-relative once appended; converting the desired
// viewport-relative box into a `.slick-header-columns`-relative one before assigning it (the same
// math the containing-block reassignment implies) restores that. That alone still isn't enough,
// though: the reassignment also means this "fixed" child no longer escapes the normal box/paint
// tree the way a real viewport-fixed element does, so it's now clipped by `.slick-pane`'s own
// `overflow: hidden` (slick.grid.css) two ancestors up whenever it's positioned outside that
// pane's own ~29px header band — confirmed live (`getBoundingClientRect()` reports the intended
// coordinate correctly; `elementFromPoint()` at that same coordinate finds an unrelated element
// underneath instead). A real `.hover()` therefore can't reach it for either geometry scenario
// below (both intentionally place it away from the header's own natural position, to force reka's
// flip/shift). `hoverInjectedTrigger` dispatches a real, bubbling `pointermove` DOM event targeted
// at the (correctly, if invisibly, positioned) element directly instead of asking Playwright to
// hit-test it on screen — same DOM-tree bubble path AttributeTooltip.vue's own listener reads
// (`closest('[data-kira-tip]')`), same `getBoundingClientRect()` reka's own popper math reads for
// flip/shift, just without requiring the pixel to be paint-visible for Playwright's own
// actionability check, which the pane's clipping defeats independent of anything under test here.
async function injectHeaderTooltipTrigger(
  page: Page,
  style: { top?: string; bottom?: string; left?: string; right?: string },
): Promise<Locator> {
  await page.evaluate((s) => {
    const header = document.querySelector('.slick-header-columns');
    if (!header) throw new Error('.slick-header-columns not found — grid not open');
    const rect = header.getBoundingClientRect();
    const px = (v: string | undefined): number | null =>
      v === undefined ? null : Number.parseFloat(v);
    const relative: Record<string, string> = {};
    const top = px(s.top);
    if (top !== null) relative.top = `${top - rect.top}px`;
    const left = px(s.left);
    if (left !== null) relative.left = `${left - rect.left}px`;
    const right = px(s.right);
    if (right !== null) relative.right = `${right - (window.innerWidth - rect.right)}px`;
    const bottom = px(s.bottom);
    if (bottom !== null) relative.bottom = `${bottom - (window.innerHeight - rect.bottom)}px`;
    const btn = document.createElement('button');
    btn.id = 'g20-tooltip-trigger';
    btn.textContent = 'x';
    btn.setAttribute('data-kira-tip', 'Geometry test tooltip');
    Object.assign(btn.style, {
      position: 'fixed',
      width: '20px',
      height: '20px',
      ...relative,
    });
    header.appendChild(btn);
  }, style);
  return page.locator('#g20-tooltip-trigger');
}

async function hoverInjectedTrigger(trigger: Locator): Promise<void> {
  await trigger.evaluate((el) => {
    const rect = el.getBoundingClientRect();
    el.dispatchEvent(
      new PointerEvent('pointermove', {
        bubbles: true,
        clientX: rect.x + rect.width / 2,
        clientY: rect.y + rect.height / 2,
      }),
    );
  });
}

test('tooltips — flips below the trigger when there is no room above', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page } = await relaunch({ control: RW_CONTROL, stream: RW_PORT });
  await openConnectionAndGrid(page);
  await page.setViewportSize({ width: 1000, height: 400 });

  // Almost no room above (4px), plenty below (376px) — reka's default `side: "top"` cannot fit,
  // so avoidCollisions must flip it below instead.
  const trigger = await injectHeaderTooltipTrigger(page, { top: '4px', left: '400px' });
  await hoverInjectedTrigger(trigger);
  const tip = tooltipContent(page);
  await expect(tip).toBeVisible({ timeout: 3_000 });
  const tipBox = await tip.boundingBox();
  const triggerBox = await trigger.boundingBox();
  if (!tipBox || !triggerBox) throw new Error('tooltip or trigger has no box');

  expect(
    tipBox.y,
    'flip: the tooltip renders below the trigger, not merely clamped on-screen above it',
  ).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height - 1);
  expect(
    tipBox.y + tipBox.height,
    'the flipped tooltip must itself stay on-screen',
  ).toBeLessThanOrEqual(400 + 1);

  expect(consoleErrors).toEqual([]);
});

test('tooltips — shifts back on-screen near a horizontal viewport edge', async ({
  relaunch,
  consoleErrors,
}) => {
  const { window: page } = await relaunch({ control: RW_CONTROL, stream: RW_PORT });
  await openConnectionAndGrid(page);
  await page.setViewportSize({ width: 400, height: 400 });
  // The injected trigger's offsets read the header's rect, so let the resize's layout settle first.
  await page.evaluate(
    () => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))),
  );

  // reka's default `align: "center"` grows the tooltip both ways from the trigger's own centre —
  // parking the trigger 4px from the right edge of a 400px-wide viewport forces real overflow past
  // the right edge without shift().
  const trigger = await injectHeaderTooltipTrigger(page, { top: '200px', right: '4px' });
  await hoverInjectedTrigger(trigger);
  const tip = tooltipContent(page);
  await expect(tip).toBeVisible({ timeout: 3_000 });
  const viewportWidth = await page.evaluate(() => window.innerWidth);
  // The resize above can land after the first paint, so the shifted position settles by polling.
  await expect
    .poll(
      async () => {
        const box = await tip.boundingBox();
        return box ? box.x >= 0 && box.x + box.width <= viewportWidth : false;
      },
      { message: 'shift: the tooltip stays on-screen (both edges)' },
    )
    .toBe(true);

  expect(consoleErrors).toEqual([]);
});

const TENANT_COMMENT =
  'Owning tenant; never reused after a tenant is deleted. Set by the provisioning service.\nSee the tenants table for the full lifecycle.';

test('tooltips — column header separates explanation from comment', async ({
  relaunch,
  consoleErrors,
}) => {
  const meta = {
    ...COMPOSITE_PK_META,
    columns: COMPOSITE_PK_META.columns.map((c) =>
      c.name === 'tenant_id' ? { ...c, comment: TENANT_COMMENT } : c,
    ),
  };
  const control = RW_CONTROL.map((c) =>
    c.channel === IPC.treeDescribe ? { ...c, response: { meta, source: 'server' } } : c,
  );
  const { window: page } = await relaunch({ control, stream: RW_PORT });
  await openConnectionAndGrid(page);

  const tip = tooltipContent(page);
  const part = (id: string): Locator => tip.locator(`[data-testid="${id}"]`);
  const tenantIdHeader = page.locator('[data-testid="grid-header-cell"][data-column="tenant_id"]');
  await tenantIdHeader.hover();
  await expect(tip).toBeVisible({ timeout: 3_000 });

  await expect(part('tooltip-title')).toHaveText('tenant_id');
  await expect(part('tooltip-meta')).toHaveText('integer');
  expect(await part('tooltip-tag').allInnerTexts()).toEqual(['PK', 'NOT NULL']);
  const body = (await part('tooltip-body').innerText()).trim();
  expect(body).not.toBe('');
  expect(body).not.toContain('Owning tenant');
  await expect(part('tooltip-note-label')).toHaveText('Comment');
  expect(await part('tooltip-note-text').innerText()).toBe(TENANT_COMMENT);

  const box = async (id: string) => {
    const b = await part(id).boundingBox();
    if (!b) throw new Error(`${id} has no box`);
    return b;
  };
  const [header, tagsBox, bodyBox, noteBox, tipBox] = [
    await box('tooltip-header'),
    await box('tooltip-tags'),
    await box('tooltip-body'),
    await box('tooltip-note'),
    await tip.boundingBox(),
  ];
  if (!tipBox) throw new Error('tooltip has no box');
  expect(header.y + header.height).toBeLessThanOrEqual(tagsBox.y + 1);
  expect(tagsBox.y + tagsBox.height).toBeLessThanOrEqual(bodyBox.y + 1);
  expect(bodyBox.y + bodyBox.height).toBeLessThanOrEqual(noteBox.y + 1);
  expect(tipBox.width).toBeLessThanOrEqual(385);

  const plain = ['tenant_id', 'integer', 'PK', 'NOT NULL', body, 'Comment', TENANT_COMMENT].join(
    '\n',
  );
  await expect(tenantIdHeader).toHaveAttribute('data-kira-tip', plain);
  await expect(tenantIdHeader).toHaveAttribute('aria-label', plain);

  await page.mouse.move(4, 4);
  await expect(tip).toHaveCount(0);

  await page.locator('[data-testid="grid-header-cell"][data-column="name"]').hover();
  await expect(tip).toBeVisible({ timeout: 3_000 });
  await expect(part('tooltip-title')).toHaveText('name');
  expect(await part('tooltip-tag').allInnerTexts()).toEqual(['Nullable']);
  await expect(part('tooltip-note')).toHaveCount(0);

  expect(consoleErrors).toEqual([]);
});
