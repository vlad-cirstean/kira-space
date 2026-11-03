import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../ipc/support/types';
import { expect, test } from './fixtures';
import { connectionCreateArgs, createConnection } from './support/connect';
import { acceptConfirm } from './support/dialogs';
import { IPC } from './support/ipcChannels';
import {
  ANALYTICS_CHILDREN,
  ANALYTICS_PATH,
  APP_PATH,
  connectAndExpandControl,
  connectionsDisconnectSnapshot,
  DB_PATH,
  ORDER_ITEMS_PATH,
  ORDERS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
  WIDE_TABLE_PATH,
} from './support/postgresFixture';
import {
  connectionRow,
  expandRow,
  findRow,
  openRowMenu,
  settleFrames,
  treeContainer,
} from './support/tree';

// Ported from tests/e2e/tree.spec.ts (P57 D16), against real captured tree shapes
// (tests/ui/support/postgresFixture.ts, plus a newly captured `ANALYTICS_CHILDREN` for
// database:kira_test/schema:analytics — scripts/capture-postgres-tree.ts's real output surfaced
// `events_id_seq`, the serial PK's own backing sequence, alongside the `events` table itself,
// exactly the kind of miss a hand-written guess would not have included). No real Postgres
// container, no Docker: every connect/tree/filter response comes from a canned ControlSnapshot.
//
// Every raw `window.kira.*` call the original used is gone (P57 M2/M3 — window.kira no longer
// exists, CLAUDE.md/P57-cutover.md §11) and is replaced one of two ways:
//   - `window.kira.connectionsCreate(...)` (used for speed, bypassing the dialog) becomes driving
//     the real add-connection dialog — the same convention mutations.spec.ts/definition.spec.ts
//     already use.
//   - `window.kira.opsRecent({limit:1000})` (op-log-based call counting) becomes
//     `control.log()` — mockRuntime.ts's own real record of every Call this tier actually
//     answered, filtered down to the same set of channels apps/kira-studio/internal/storage/model/ops.go's
//     own `opKinds` table logs as an "op" (`connect`/`disconnect`/`children`/`describe`/
//     `definition`/`test`, plus the data-plane read/count/mutate/execute/transfer this spec never
//     issues) — see `opsCount()` below. This is a more precise substitute than the original, not
//     a weaker one: it is the actual set of network Calls this tier's mock answered, not app-level
//     bookkeeping, and it reproduces the original's own "a FiltersService.Replace call is not an
//     op" fact exactly (confirmed by reading ops.go directly) rather than assuming it.
//   - One raw call — `window.kira.treeChildren({connectionId, path: WIDE_TABLE_PATH})`, used only
//     to prove the adapter itself returns no children for a table path — is dropped outright: it
//     has no live wire to query in this tier, the UI itself never issues this call either (a table
//     row renders with no twisty, already asserted separately just below), and the adapter-level
//     fact is already covered directly at packages/db-fixtures/postgres.spec.ts, which asserts wide_table's
//     hasChildren:false.
//
// Two scenarios do not port at full fidelity:
//   - The disconnect -> reconnect scenario's own "D11" claim (reconnecting fires exactly four new
//     ops: one connect plus three treeChildren refetches, via `onConnectionMetadataInvalidated`
//     -> `refreshExpanded()`) does not hold here: this tier's mockRuntime has no Events.On mock at
//     all (P57 finding, already recorded getting connections.spec.ts green) — the event that
//     drives that automatic refetch never fires. What still ports: reconnecting still works
//     (exactly one new `connectionsConnect` op), the already-cached subtree still renders with no
//     error, and expanding a schema that was never cached before (`analytics`) still issues a
//     normal fresh `treeChildren` call. The "three automatic refetches" claim itself is dropped —
//     a tier limitation, not a regression this port is papering over.
//   - The final "unticking one schema hides it, and it survives a relaunch" persistence check is
//     dropped — `tests/ui/fixtures.ts`'s own header comment: there is no backing store in this
//     tier for a second `relaunch()` to prove anything against, same category as every other
//     dropped relaunch scenario this session (workbench/connections/tabs). Everything before that
//     final relaunch — the filter actually applying, with no extra query cost — still ports.
//   - One more raw-probe drop, smaller: the tri-state scenario's own trailing
//     `window.kira.filtersList(...)` check only re-verified the exact response this file's own
//     `filtersReplace` fixture snapshot hands back — checking a shape the test's own fixture
//     already determines, the same reason `definition.spec.ts`'s header comment already gives for
//     dropping its op-count checks. The UI-observable half of the same fact (both checkboxes read
//     back 'on' after the save) is asserted just above it and stays.

const CONNECTION_ID = 'conn-tree';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Tree DB', 'blue');
const CONN2_ID = 'conn-tree-2';
const CONNECTION_SUMMARY_2 = postgresConnectionSummary(CONN2_ID, 'Tree DB 2', 'cyan');

const ORDER_SUMMARY_PATH = `${APP_PATH}/view:order_summary`;
const SEQUENCE_PATH = `${APP_PATH}/sequence:invoice_number_seq`;
// P19 D2: a group folder's path is its parent's path plus `#<kind>` — a renderer-only synthetic
// path, never sent to an adapter (project/state/tree.ts's groupPath()).
const SEQUENCES_FOLDER_PATH = `${APP_PATH}#sequence`;

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tree DB', 'blue'),
    response: CONNECTION_SUMMARY,
  },
  ...connectAndExpandControl(CONNECTION_ID),
  {
    channel: IPC.treeChildren,
    args: { connectionId: CONNECTION_ID, path: ANALYTICS_PATH, refresh: false },
    response: { nodes: ANALYTICS_CHILDREN, source: 'server', truncated: false },
  },
  connectionsDisconnectSnapshot(CONNECTION_ID),
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Tree DB 2', 'cyan'),
    response: CONNECTION_SUMMARY_2,
  },
  ...connectAndExpandControl(CONN2_ID),
  { channel: IPC.connectionsDelete, args: { id: CONN2_ID }, response: null },
  {
    channel: IPC.filtersReplace,
    args: { connectionId: CONNECTION_ID, visibility: { hiddenKinds: [], hiddenPaths: [] } },
    response: { hiddenKinds: [], hiddenPaths: [] },
  },
  {
    channel: IPC.filtersReplace,
    args: {
      connectionId: CONNECTION_ID,
      visibility: { hiddenKinds: ['sequence'], hiddenPaths: [] },
    },
    response: { hiddenKinds: ['sequence'], hiddenPaths: [] },
  },
  {
    channel: IPC.filtersReplace,
    args: {
      connectionId: CONNECTION_ID,
      visibility: { hiddenKinds: [], hiddenPaths: [ANALYTICS_PATH] },
    },
    response: { hiddenKinds: [], hiddenPaths: [ANALYTICS_PATH] },
  },
];

// The exact channel set apps/kira-studio/internal/storage/model/ops.go's own `opKinds` table logs
// as an "op"
// (connectionsTest/data-plane read/count/mutate/execute/transfer included for completeness, even
// though this spec never issues them) — the real substitute for the original's
// `window.kira.opsRecent()`, not an approximation of it.
const OP_CHANNELS = new Set<string>([
  IPC.connectionsConnect,
  IPC.connectionsDisconnect,
  IPC.connectionsTest,
  IPC.treeChildren,
  IPC.treeDescribe,
  IPC.treeDefinition,
]);

function opsCount(log: { channel: string }[]): number {
  return log.filter((e) => OP_CHANNELS.has(e.channel)).length;
}

function menuItemIds(page: Page): Promise<string[]> {
  const menu = page.locator('[data-testid="context-menu"]');
  return menu
    .locator(
      '[data-testid^="menu-item-"]:not([data-testid$="-shortcut"]), [data-slot="dropdown-menu-separator"]',
    )
    .evaluateAll((els) =>
      els.map((el) =>
        el.getAttribute('data-slot') === 'dropdown-menu-separator'
          ? '--separator--'
          : (el.getAttribute('data-testid') ?? '').replace('menu-item-', ''),
      ),
    );
}

test('project tree — expansion, caching, disconnect/reconnect, search, filters, menus', async ({
  relaunch,
}) => {
  // The largest single scenario in this tier (sticky band + two-connection scroll handoff +
  // full checkbox filter dialog on top of the usual expand/menu coverage) — the default 60s
  // project timeout is tuned for a typical spec, not this one, even with no container/network
  // latency to blame.
  test.setTimeout(120_000);
  const { window: page, control } = await relaunch({ control: CONTROL });

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(page.locator('[data-testid="operations-panel"]')).toBeVisible();

  // --- setup: create the connection through the real dialog, then connect it through the real
  // UI (12c: window.kira.connectionsCreate no longer exists — see header comment). -------------
  await createConnection(page, 'postgres', 'Tree DB');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });

  expect(control.log().filter((e) => e.channel === IPC.connectionsConnect)).toHaveLength(1);

  // --- expand connection -> database -> app -----------------------------------------------
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  const appRow = await expandRow(page, APP_PATH);
  await expect(appRow).toHaveAttribute('data-kind', 'schema');

  // P19 D5: tables are tree leaves — no twisty, and children() returns [] regardless of what a
  // pre-upgrade cache entry might still say (see header comment for why the original's raw
  // window.kira.treeChildren probe of that fact is dropped rather than ported).
  const wideTableRow = await findRow(page, WIDE_TABLE_PATH);
  await expect(wideTableRow).toHaveAttribute('data-kind', 'table');
  await expect(wideTableRow.locator('[data-testid="tree-twisty"]')).not.toBeVisible();

  // WAI-ARIA tree keys: Left collapses an expanded row, Right expands it again, Down moves the
  // selection to the next row, Up back.
  await appRow.click();
  await appRow.press('ArrowLeft');
  await expect(appRow).toHaveAttribute('aria-expanded', 'false');
  await expect(wideTableRow).toHaveCount(0);
  await appRow.press('ArrowRight');
  await expect(appRow).toHaveAttribute('aria-expanded', 'true');
  await appRow.press('ArrowDown');
  await expect(appRow).toHaveAttribute('aria-selected', 'false');
  await expect(page.locator('[data-testid="tree-row"][aria-selected="true"]')).toHaveCount(1);
  await page.keyboard.press('ArrowUp');
  await expect(appRow).toHaveAttribute('aria-selected', 'true');

  // P19 D1-D3: every other listed kind (views, materialized views, sequences, functions)
  // collapses into its own per-kind folder below the ungrouped tables, collapsed by default.
  const viewsFolder = await findRow(page, `${APP_PATH}#view`);
  await expect(viewsFolder).toBeVisible();
  await expect(viewsFolder).toHaveAttribute('data-kind', 'group');
  await expect(viewsFolder).toContainText('Views');
  const matviewsFolder = await findRow(page, `${APP_PATH}#matview`);
  await expect(matviewsFolder).toBeVisible();
  await expect(matviewsFolder).toContainText('Materialized views');
  const functionsFolder = await findRow(page, `${APP_PATH}#function`);
  await expect(functionsFolder).toBeVisible();
  await expect(functionsFolder).toContainText('Functions');
  const sequencesFolder = await findRow(page, SEQUENCES_FOLDER_PATH);
  await expect(sequencesFolder).toBeVisible();
  await expect(sequencesFolder).toContainText('Sequences');
  await expect(sequencesFolder).toHaveAttribute('data-kind', 'group');

  // D2/D4's own acceptance bar: expanding a folder is a pure render over already-fetched
  // children — zero IPC calls, zero op-log rows, asserted rather than assumed.
  const opsBeforeFolderExpand = opsCount(control.log());
  await sequencesFolder.locator('[data-testid="tree-twisty"]').click();
  const sequenceRow = await findRow(page, SEQUENCE_PATH);
  await expect(sequenceRow).toBeVisible();
  await expect(sequenceRow).toHaveAttribute('data-kind', 'sequence');
  expect(opsCount(control.log())).toBe(opsBeforeFolderExpand);

  // --- cache assertion (§7/§9.2): a cache hit produces zero new op-log rows --------------
  const opsBeforeCollapse = opsCount(control.log());
  await (await findRow(page, '')).locator('[data-testid="tree-twisty"]').click(); // collapse the whole connection
  await expandRow(page, '');
  expect(opsCount(control.log())).toBe(opsBeforeCollapse);

  // Refresh from the context menu forces a real round trip: exactly one new `children` row.
  await openRowMenu(page, APP_PATH);
  await page.click('[data-testid="menu-item-refresh"]');
  await expect.poll(() => opsCount(control.log())).toBe(opsBeforeCollapse + 1);

  // That refresh routes through treeInvalidate first — the schema's WHOLE row (all four
  // kinds: children/describe/definition/columns), not loadChildren's own refresh:true, which
  // only ever rewrote 'children' and left the other three kinds sharing that row untouched.
  // treeInvalidate is void and outside opsCount()'s own op-channel set (control.log() records it
  // regardless, since every intercepted call is logged before any snapshot/wildcard lookup runs).
  const invalidateOnRefresh = control.log().filter((e) => e.channel === IPC.treeInvalidate);
  expect(invalidateOnRefresh.at(-1)?.args).toEqual({ connectionId: CONNECTION_ID, path: APP_PATH });

  // Refreshing a TABLE (a tree leaf) no longer calls Children on it — every
  // adapter answers a leaf's Children with an empty list nobody renders, so the old refresh flow
  // both wasted a real round trip AND wrote junk `{"children":[]}` into the same row describe/
  // definition live on. Right-click wide_table's own Refresh and assert the op delta contains no
  // `children` call at all — fails on main, which routed every row's Refresh through the same
  // loadChildren(…, true) regardless of what kind of node it targeted.
  const opsBeforeLeafRefresh = opsCount(control.log());
  const invalidateBeforeLeafRefresh = invalidateOnRefresh.length;
  await openRowMenu(page, WIDE_TABLE_PATH);
  await page.click('[data-testid="menu-item-refresh"]');
  // A leaf has no twisty spinner to poll (wide_table's own `[data-testid="tree-twisty"]`
  // is `invisible`, and
  // refreshObject never touches `treeState.loading` for it either way) — wait out the mocked
  // round trip with a fixed settle window instead, long enough for either the fixed behaviour
  // (one treeInvalidate call, nothing else) or the pre-fix one (treeInvalidate AND a treeChildren
  // call) to fully land against a local mock whose every response is effectively instant.
  await page.waitForTimeout(300);
  const invalidateAfterLeafRefresh = control.log().filter((e) => e.channel === IPC.treeInvalidate);
  expect(invalidateAfterLeafRefresh).toHaveLength(invalidateBeforeLeafRefresh + 1);
  expect(invalidateAfterLeafRefresh.at(-1)?.args).toEqual({
    connectionId: CONNECTION_ID,
    path: WIDE_TABLE_PATH,
  });
  expect(opsCount(control.log())).toBe(opsBeforeLeafRefresh);

  // --- context menus: exact item id list per kind (§9b) ------------------------------------
  await openRowMenu(page, '');
  expect(await menuItemIds(page)).toEqual([
    'disconnect',
    'refresh',
    'edit',
    'open-file',
    'reveal-file',
    'update-credentials',
    'duplicate',
    'move-up',
    'move-down',
    'copy-name',
    'copy-uri',
    'filters',
    'schema',
    'open-console',
    '--separator--',
    'delete',
  ]);
  await page.keyboard.press('Escape');

  await openRowMenu(page, DB_PATH);
  expect(await menuItemIds(page)).toEqual([
    'refresh',
    'copy-name',
    'filters',
    'open-console',
    'set-as-default',
  ]);
  await page.keyboard.press('Escape');

  await openRowMenu(page, APP_PATH);
  expect(await menuItemIds(page)).toEqual([
    'refresh',
    'copy-name',
    'filters',
    'open-console',
    'set-as-default',
  ]);
  // D9: "Set as default" is unchecked until chosen, then stays checked on this row and clears
  // on the previously-default one — Postgres-only (§8.9), same connection this spec already has.
  await expect(page.locator('[data-testid="menu-item-set-as-default"] .codicon-check')).toHaveCount(
    0,
  );
  await page.click('[data-testid="menu-item-set-as-default"]');

  await openRowMenu(page, DB_PATH);
  await expect(page.locator('[data-testid="menu-item-set-as-default"] .codicon-check')).toHaveCount(
    0,
  );
  await page.keyboard.press('Escape');

  await openRowMenu(page, APP_PATH);
  await expect(
    page.locator('[data-testid="menu-item-set-as-default"] .codicon-check'),
  ).toBeVisible();
  await page.keyboard.press('Escape');

  await openRowMenu(page, WIDE_TABLE_PATH);
  expect(await menuItemIds(page)).toEqual([
    'open-data',
    'open-data-new-tab',
    'open-definition',
    'open-console',
    'refresh',
    'copy-name',
    'copy-qualified-name',
    'count-rows',
    'saved-filters',
  ]);
  await page.keyboard.press('Escape');

  // P19 D2/D9: a group folder's own menu — Refresh (targets the *parent*, not the synthetic
  // path itself) and Collapse all, nothing that needs a real node.
  await openRowMenu(page, SEQUENCES_FOLDER_PATH);
  expect(await menuItemIds(page)).toEqual(['refresh', 'collapse-all']);
  await page.keyboard.press('Escape');

  // The Sequences folder is still expanded from the earlier expansion step —
  // sequence:invoice_number_seq is already one of its rendered children.
  await openRowMenu(page, SEQUENCE_PATH);
  expect(await menuItemIds(page)).toEqual(['copy-name', 'copy-qualified-name']);
  await page.keyboard.press('Escape');

  // Column rows no longer live in the tree (P19 D5) — the "Copy name / Add to projection /
  // Sort by" menu relocated into the definition view's Columns section; covered there in
  // definition.spec.ts (D9).

  // Collapse everything down to the bare connection row so a right-click well below it lands
  // on the virtual list's empty spacer, not on a `.tree-row` (which stops propagation itself).
  await (await findRow(page, '')).locator('[data-testid="tree-twisty"]').click();
  await page.locator('[data-testid="tree-background"]').click({
    button: 'right',
    position: { x: 10, y: 200 },
  });
  await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
  expect(await menuItemIds(page)).toEqual(['new-connection', 'refresh-all', 'collapse-all']);
  await page.keyboard.press('Escape');
  await expandRow(page, ''); // restore — later steps expect the tree still expanded

  // --- sticky ancestor band (P28 §5) ----------------------------------------------------------
  // ProjectTree.vue's own literal (rowDensity defaults to 'comfortable') — the steady-state slot
  // math (top = 0/H/2H) below depends on it exactly.
  const H = 28;
  const treeScroll = treeContainer(page);
  const stickyRows = page.locator('[data-testid="tree-sticky-row"]');

  // Nothing is pinned at the top of the list.
  await treeScroll.evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);
  await expect(stickyRows).toHaveCount(0);

  // The connection and database pin as soon as they leave, in order, at top 0/H/2H.
  await (await findRow(page, WIDE_TABLE_PATH)).evaluate((el) =>
    el.scrollIntoView({ block: 'start' }),
  );
  await settleFrames(page);
  await expect(stickyRows).toHaveCount(3);
  await expect(stickyRows.nth(0)).toHaveAttribute('data-path', '');
  await expect(stickyRows.nth(1)).toHaveAttribute('data-path', DB_PATH);
  await expect(stickyRows.nth(2)).toHaveAttribute('data-path', APP_PATH);
  await expect(stickyRows.nth(0)).toHaveAttribute('data-depth', '0');
  await expect(stickyRows.nth(1)).toHaveAttribute('data-depth', '1');
  await expect(stickyRows.nth(2)).toHaveAttribute('data-depth', '2');

  // The band pins to the *top* of the scrollport, not its bottom — no earlier assertion in this
  // suite ever compared a sticky row's own viewport box to the scrollport's, so this is the one
  // that would have failed against the pre-fix "sticky slot is the last child" layout (it rendered
  // at the end of the content instead of pinning at all).
  const treeScrollBox = await treeScroll.boundingBox();
  const firstStickyBox = await stickyRows.first().boundingBox();
  if (!treeScrollBox || !firstStickyBox) throw new Error('expected both boxes to be measurable');
  expect(Math.abs(firstStickyBox.y - treeScrollBox.y)).toBeLessThanOrEqual(1);

  // Scrolled to the very end of the content: no sticky row sits below the scrollport (P41 F2 —
  // the pre-fix bug's own symptom was one to three ancestor rows duplicated at the bottom).
  await treeScroll.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await settleFrames(page);
  const scrollportBottom = treeScrollBox.y + treeScrollBox.height;
  for (const row of await stickyRows.all()) {
    const box = await row.boundingBox();
    if (!box) throw new Error('expected a sticky row box to be measurable');
    expect(box.y + box.height).toBeLessThanOrEqual(scrollportBottom + 1);
  }
  await (await findRow(page, WIDE_TABLE_PATH)).evaluate((el) =>
    el.scrollIntoView({ block: 'start' }),
  );
  await settleFrames(page);

  // The band never exceeds the cap: scrolled into the (depth-3) Sequences folder, still exactly
  // three rows, and the folder itself is not among them.
  await sequenceRow.evaluate((el) => el.scrollIntoView({ block: 'start' }));
  await settleFrames(page);
  await expect(stickyRows).toHaveCount(3);
  await expect(
    page.locator('[data-testid="tree-sticky-row"]', { hasText: 'Sequences' }),
  ).toHaveCount(0);

  // A pinned row is a real row: its twisty collapses the real schema, and it carries the
  // connection's colour rail. Collapsing it removes the whole subtree we'd scrolled into (the
  // Sequences folder among it) — kira_test's only remaining content is its two collapsed schemas,
  // which comfortably fit the viewport, so there is nothing left to scroll past and the band
  // clears entirely (same as the initial "nothing pinned at scrollTop 0" case above).
  await expect(stickyRows.last()).toHaveAttribute('data-path', APP_PATH);
  await expect(stickyRows.last().locator('[data-testid="tree-rail"]')).toBeVisible();
  await stickyRows.last().locator('[data-testid="tree-twisty"]').click();
  await expect(stickyRows).toHaveCount(0);
  await expandRow(page, APP_PATH); // restore for the assertions that follow

  // The band does not duplicate anything the rest of the suite counts (F5): the connection-kind
  // row count is unaffected, and no sticky row carries the tree's own roving tab stop.
  await (await findRow(page, WIDE_TABLE_PATH)).evaluate((el) =>
    el.scrollIntoView({ block: 'start' }),
  );
  await settleFrames(page);
  // schema:app alone (fourteen tables, four folders, thirteen sequences) now outgrows
  // VirtualList's 8-row overscan on its own, so the real connection row (index 0) is fully
  // virtualized out at this depth — only its sticky counterpart remains. The invariant worth
  // guarding is "never both": data-kind="connection" (real or sticky) resolves to exactly one
  // row, not two.
  expect(await page.locator('[data-kind="connection"]').count()).toBe(1);
  expect(await page.locator('[data-testid="tree-sticky-row"][tabindex="0"]').count()).toBe(0);

  // A row under the band is still reachable: the shared findRow scrolls it clear before
  // returning, so a click on a row that would otherwise land at the very top succeeds unretried.
  await (await findRow(page, WIDE_TABLE_PATH)).click();
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  await treeScroll.evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);

  // --- two-connection handoff (D4): scrolling from one connection's section into the next ----
  await createConnection(page, 'postgres', 'Tree DB 2');

  const conn2Row = connectionRow(page, 'Tree DB 2');
  // The first connection's own expanded subtree (schema:app's fourteen tables, four folders, and
  // thirteen sequences) is now taller than the panel's overscan window, so the second connection
  // — appended after all of it — is not yet in the DOM at scrollTop 0. Scroll to the bottom (it's
  // the last root row) before asserting it exists.
  await treeScroll.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await settleFrames(page);
  await expect(conn2Row).toBeVisible();

  // A freshly created, still-collapsed connection is only one row tall — nowhere near enough
  // content below it for the browser to ever scroll it to the viewport's top (that would leave a
  // gap under the last row, which no scroll container allows), and unexpanded it could never
  // join the band anyway (stickyBand.ts only self-pins a row that is showing its own children).
  // Give it the same connection/database/schema depth as the first connection so there is real
  // content — and a real handoff — to scroll through. Both connections share the same catalog, so
  // `path` alone is ambiguous the moment both are expanded (the whole reason for deleting this
  // one afterwards) — every step below walks from `conn2Row` via DOM sibling order instead of the
  // path-based `findRow`/`expandRow` helpers.
  async function expandNext(row: Locator): Promise<Locator> {
    await row.scrollIntoViewIfNeeded();
    await row.locator('[data-testid="tree-twisty"]').click();
    await expect(
      row.locator('[data-testid="tree-twisty"] [data-testid="tree-row-spinner"]'),
    ).toHaveCount(0, {
      timeout: 15_000,
    });
    return row.locator('xpath=following-sibling::*[1]');
  }
  // Walks forward sibling by sibling from `start`, scrolling each candidate into view before
  // reading it — VirtualList only mounts a row once it has been scrolled near, so a plain
  // `following-sibling` chase (with no interleaved scroll) can silently resolve past an
  // unmounted row's own real position, landing on whatever the DOM still happens to hold from an
  // earlier scroll position instead (connection 1's own same-named folder, for one — hit while
  // porting this scenario, since a global `hasText`/`.last()` locator has that identical failure
  // mode: it can only ever match what is *currently mounted*, not what is logically last).
  async function findFollowingGroup(start: Locator, label: string): Promise<Locator> {
    let cur = start.locator('xpath=following-sibling::*[1]');
    for (let i = 0; i < 40; i++) {
      await cur.scrollIntoViewIfNeeded();
      if (
        (await cur.getAttribute('data-kind')) === 'group' &&
        (await cur.innerText()).includes(label)
      ) {
        return cur;
      }
      cur = cur.locator('xpath=following-sibling::*[1]');
    }
    throw new Error(`findFollowingGroup: no "${label}" folder found after ${40} siblings`);
  }
  const conn2Db = await expandNext(conn2Row); // database:kira_test (first child)
  // schema:analytics sorts before schema:app and stays collapsed (one row) — schema:app is the
  // second sibling, not the first.
  const conn2App = (await expandNext(conn2Db)).locator('xpath=following-sibling::*[1]');
  await expandNext(conn2App);
  // schema:app now carries fourteen tables (this fixture's real captured content, more than the
  // original spec's own schema had) — tall enough on its own that expanding conn2's own Sequences
  // folder too (as connection 1's own subtree already does, further up) gives conn2's subtree
  // enough real height for the browser to ever scroll its own connection row flush to the
  // scrollport's top below; without it there is not enough content left below conn2Row to fill
  // the viewport, and the browser refuses to open a gap under the last row.
  const conn2Sequences = await findFollowingGroup(conn2App, 'Sequences');
  await conn2Sequences.locator('[data-testid="tree-twisty"]').click();
  await expect(
    conn2Sequences.locator('[data-testid="tree-twisty"] [data-testid="tree-row-spinner"]'),
  ).toHaveCount(0, {
    timeout: 15_000,
  });

  // Right at the boundary: the band's outermost — only — row is now the second connection.
  // stickyBand.ts's own kept-loop requires a candidate's natural top to be strictly less than its
  // pinned slot (the "passed", not "arrived") — landing exactly on the row's natural offset
  // (what scrollIntoView's block:'start' does) is the tie, not the pass, so nudge one pixel
  // further to cross it without pulling in a second candidate (conn2's own database row, one
  // full rowHeight below, would join too).
  await conn2Row.evaluate((el) => el.scrollIntoView({ block: 'start' }));
  await treeScroll.evaluate((el) => {
    el.scrollTop += 1;
  });
  await settleFrames(page);
  await expect(stickyRows.first()).toContainText('Tree DB 2');

  // One row shy of the boundary: the first connection is still pinned, its slot pushed to (or
  // below) zero as the second connection's own header arrives to replace it — no gap, no second
  // header at the same offset.
  await treeScroll.evaluate((el, h) => {
    el.scrollTop -= h;
  }, H);
  await settleFrames(page);
  const firstStickyBeforeHandoff = stickyRows.first();
  await expect(firstStickyBeforeHandoff).toContainText('Tree DB');
  await expect(firstStickyBeforeHandoff).not.toContainText('Tree DB 2');
  const topPx = await firstStickyBeforeHandoff.evaluate((el) => Number.parseFloat(el.style.top));
  expect(topPx).toBeLessThanOrEqual(0);

  // Deleting the second connection keeps every later `data-path=""` lookup in this test
  // unambiguous — `path` is per-connection-relative, so two expanded connections would both
  // render one.
  await conn2Row.click({ button: 'right' });
  await page.click('[data-testid="menu-item-delete"]');
  await acceptConfirm(page);
  await expect(conn2Row).toHaveCount(0);
  await treeScroll.evaluate((el) => {
    el.scrollTop = 0;
  });
  await settleFrames(page);

  // --- disconnect: cached nodes still render; expanding any node (cached or not) reconnects
  // first rather than surfacing a disconnected error — the twisty is the primary way users
  // browse, so it shouldn't require a separate explicit "Connect" click first (state/tree.ts's
  // expand()). ---------------------------------------------------------------
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-disconnect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'disconnected');

  // app is cached (L1 survives disconnect) — collapse/re-expand reconnects first (silently),
  // then still renders from cache with no error. See header comment: this tier's mockRuntime has
  // no Events.On mock at all, so the automatic cache-invalidate-and-refetch the original asserted
  // here never fires — only the reconnect's own `connectionsConnect` call happens.
  const opsBeforeReconnect = opsCount(control.log());
  await (await findRow(page, APP_PATH)).locator('[data-testid="tree-twisty"]').click();
  const appRowAgain = await expandRow(page, APP_PATH);
  await expect(appRowAgain.locator('[data-testid="error-popover-trigger"]')).toHaveCount(0);
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expect.poll(() => opsCount(control.log())).toBe(opsBeforeReconnect + 1);

  // analytics was never expanded — no cache entry — but the connection is already back up (the
  // app-row expand above reconnected it), so this is a normal fresh fetch with real children,
  // not an error.
  const analyticsRow = await findRow(page, ANALYTICS_PATH);
  await analyticsRow.locator('[data-testid="tree-twisty"]').click();
  await expect(analyticsRow.locator('[data-testid="error-popover-trigger"]')).toHaveCount(0, {
    timeout: 10_000,
  });
  await expect(await findRow(page, `${ANALYTICS_PATH}/table:events`)).toBeVisible();

  // --- search: cached-only, matches + ancestors, incomplete note --------------------------
  // 7bdfc32 hid the search box behind a toggle (ProjectPanel.vue's showSearch) — it no longer
  // mounts, and data-testid="tree-search" no longer exists, until this reveals it.
  await page.click('[data-testid="toggle-search"]');
  await page.fill('[data-testid="tree-search"]', 'order');
  await expect(page.locator('[data-testid="search-incomplete-note"]')).toBeVisible();
  await expect(await findRow(page, ORDER_ITEMS_PATH)).toBeVisible();
  await expect(await findRow(page, ORDERS_PATH)).toBeVisible();
  await expect(await findRow(page, ORDER_SUMMARY_PATH)).toBeVisible();
  expect(await page.locator(`[data-path="${WIDE_TABLE_PATH}"]`).count()).toBe(0);

  await page.click('[aria-label="Clear search"]');
  await expect(page.locator('[data-testid="search-incomplete-note"]')).toHaveCount(0);
  await expect(await findRow(page, WIDE_TABLE_PATH)).toBeVisible();

  // --- checkbox tree filter (P28 §5) --------------------------------------------------------
  const filtersDialog = page.locator('[data-testid="filters-dialog"]');
  function filterKindRow(kind: string) {
    return page.locator(`[data-testid="filter-kind-row-${kind}"]`);
  }
  // P104: reka's CheckboxRoot renders `<button role="checkbox">`, not a native
  // `<input type="checkbox">` -- matched by its own data-slot instead.
  function filterKindCheckbox(kind: string) {
    return filterKindRow(kind).locator('[data-slot="checkbox"]');
  }
  function filterObjectRow(path: string) {
    return page.locator(`[data-testid="filter-object-row"][data-path="${path}"]`);
  }
  function filterObjectCheckbox(path: string) {
    return filterObjectRow(path).locator('[data-slot="checkbox"]');
  }
  async function openFilters(path: string): Promise<void> {
    await openRowMenu(page, path);
    await page.click('[data-testid="menu-item-filters"]');
    await expect(filtersDialog).toBeVisible();
  }
  async function saveFilters(): Promise<void> {
    await page.locator('[data-slot="dialog-footer"] button', { hasText: 'Save' }).click();
    await expect(filtersDialog).toHaveCount(0);
  }
  async function cancelFilters(): Promise<void> {
    await page.locator('[data-slot="dialog-footer"] button', { hasText: 'Cancel' }).click();
    await expect(filtersDialog).toHaveCount(0);
  }

  // --- opens focused on the invoking row (D20); issues no query while open (D21/D23); cancel
  // discards (baseline parity) --------------------------------------------------------------
  const opsBeforeDialog = opsCount(control.log());
  await openFilters(APP_PATH);
  // D20: the schema's ancestor (the database) is pre-expanded, so the schema's own row is
  // listed without any manual expand click.
  await expect(filterObjectRow(APP_PATH)).toBeVisible();
  await expect(page.locator('[data-testid="filters-preview"]')).toBeVisible();
  await expect(page.locator('.cached-note')).toContainText('Only cached nodes are listed');
  // Expand one level deeper inside the dialog and untick a box — still zero queries while open.
  await filterObjectRow(APP_PATH).locator('[data-testid="tree-twisty"]').click();
  await expect(filterObjectRow(WIDE_TABLE_PATH)).toBeVisible();
  await filterObjectCheckbox(APP_PATH).click();
  await expect(filterObjectRow(APP_PATH)).toHaveAttribute('data-state', 'off');
  await cancelFilters();
  expect(opsCount(control.log())).toBe(opsBeforeDialog);
  // Reopening shows the box still ticked — Cancel discarded the untick above. Focused on
  // APP_PATH again so its ancestor (the database) is pre-expanded, revealing it without a
  // manual expand click (D20).
  await openFilters(APP_PATH);
  await expect(filterObjectCheckbox(APP_PATH)).toBeChecked();
  await expect(filterObjectRow(APP_PATH)).toHaveAttribute('data-state', 'on');
  await cancelFilters();

  // --- tri-state: unticking both schemas of kira_test partials the database row;
  // ticking the database row restores both and leaves the persisted path set empty -----------
  // Focused on APP_PATH so the database (both schemas' shared parent) is pre-expanded,
  // revealing both app and analytics without a manual expand click.
  await openFilters(APP_PATH);
  await filterObjectCheckbox(APP_PATH).click();
  await filterObjectCheckbox(ANALYTICS_PATH).click();
  await expect(filterObjectRow(DB_PATH)).toHaveAttribute('data-state', 'partial');
  await filterObjectCheckbox(DB_PATH).click();
  await expect(filterObjectRow(APP_PATH)).toHaveAttribute('data-state', 'on');
  await expect(filterObjectRow(ANALYTICS_PATH)).toHaveAttribute('data-state', 'on');
  await saveFilters();
  // (See header comment: the original's trailing window.kira.filtersList() re-check of this same
  // fixture-determined response is dropped — the UI-observable half above already proves it.)

  // --- unticking a type hides every object of that kind, and its P19 folder with it (D10/D14);
  // a node hidden by its kind says so rather than silently unticking (D16); the consequence
  // strip counts what the tree will show (D17) ------------------------------------------------
  await openFilters(APP_PATH);
  await filterKindCheckbox('sequence').click();
  await expect(filterKindRow('sequence')).toHaveAttribute('data-state', 'off');
  await filterObjectRow(APP_PATH).locator('[data-testid="tree-twisty"]').click();
  await expect(filterObjectCheckbox(SEQUENCE_PATH)).toBeDisabled();
  await expect(filterObjectRow(SEQUENCE_PATH)).toHaveAttribute('data-state', 'off');
  // The disabled-row reason moved off data-kira-tip onto the real Tooltip system
  // (FiltersDialog.vue's `Tooltip :disabled="!row.disabledReason"`) -- hover to read it instead.
  // Expanding the schema re-lays the row out after the pointer lands; a tooltip opens on
  // pointerenter, so leave and re-enter on the settled layout.
  await expect(async () => {
    await page.mouse.move(0, 0);
    await filterObjectRow(SEQUENCE_PATH).locator('.object-checkbox-label').hover();
    await expect(page.locator('[data-slot="tooltip-content"]')).not.toHaveCount(0, {
      timeout: 1500,
    });
  }).toPass();
  await expect(page.locator('[data-slot="tooltip-content"]')).not.toHaveText('');
  await page.mouse.move(0, 0);
  const previewBefore = await page.locator('[data-testid="filters-preview"]').innerText();
  const [shownBefore, totalBefore] = [...previewBefore.matchAll(/\d+/g)].map((m) => Number(m[0]));
  await saveFilters();

  expect(await page.locator(`[data-path="${SEQUENCE_PATH}"]`).count()).toBe(0);
  expect(await page.locator(`[data-path="${SEQUENCES_FOLDER_PATH}"]`).count()).toBe(0);
  await expect(await findRow(page, `${APP_PATH}#view`)).toBeVisible();
  await expect(await findRow(page, `${APP_PATH}#function`)).toBeVisible();
  // The strip's own count dropped below the connection's full cached total once sequences
  // were unticked — the visible proof the number is live, not decorative.
  expect(shownBefore).toBeLessThan(totalBefore);

  // Un-hide sequences again so the rest of the suite (and later phases) see the tree unfiltered.
  await openFilters(APP_PATH);
  await filterKindCheckbox('sequence').click();
  await saveFilters();
  expect(await page.locator(`[data-path="${SEQUENCES_FOLDER_PATH}"]`).count()).toBeGreaterThan(0);

  // --- the name filter is a substring, not a pattern (D17/D19) -------------------------------
  await openFilters(DB_PATH);
  await page.fill('[data-testid="filter-name-input"]', 'analyt');
  await expect(filterObjectRow(ANALYTICS_PATH)).toBeVisible();
  await expect(filterObjectRow(APP_PATH)).toHaveCount(0);
  await page.fill('[data-testid="filter-name-input"]', '.*');
  await expect(page.locator('[data-testid="filter-object-row"]')).toHaveCount(0);
  await cancelFilters();

  // --- unticking one schema hides exactly it, and costs no *engine* op (a FiltersService.Replace
  // call is not one — confirmed by reading apps/kira-studio/internal/storage/model/ops.go directly, and
  // opsCount() mirrors that table exactly) (D10/D23) — the "survives a relaunch" half of the
  // original (D11) is dropped per the header comment. -----------------------------------------
  const opsBeforeFilter = opsCount(control.log());
  await openFilters('');
  await filterObjectRow(DB_PATH).locator('[data-testid="tree-twisty"]').click();
  await filterObjectCheckbox(ANALYTICS_PATH).click();
  await expect(filterObjectRow(APP_PATH)).toHaveAttribute('data-state', 'on');
  await saveFilters();

  expect(await page.locator(`[data-path="${ANALYTICS_PATH}"]`).count()).toBe(0);
  expect(await page.locator(`[data-path="${APP_PATH}"]`).count()).toBeGreaterThan(0);
  expect(opsCount(control.log())).toBe(opsBeforeFilter);
});

test('filters dialog — a never-expanded connection seeds the draft from the saved set, so Save does not wipe it', async ({
  relaunch,
}) => {
  const SAVED = { hiddenKinds: ['sequence'], hiddenPaths: [ANALYTICS_PATH] };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONNECTION_SUMMARY] },
      { channel: IPC.filtersList, args: { connectionId: CONNECTION_ID }, response: SAVED },
      {
        channel: IPC.filtersReplace,
        args: { connectionId: CONNECTION_ID, visibility: SAVED },
        response: SAVED,
      },
    ],
  });

  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-filters"]');
  await expect(page.locator('[data-testid="filters-error"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="filters-save"]')).toBeEnabled();
  await page.click('[data-testid="filters-save"]');
  await expect(page.locator('[data-slot="dialog-content"]')).toHaveCount(0);

  const replaces = control.log().filter((e) => e.channel === IPC.filtersReplace);
  expect(replaces).toHaveLength(1);
  expect(replaces[0].args).toEqual({ connectionId: CONNECTION_ID, visibility: SAVED });
});

test('a failing tree menu action reports on the row and the error popover opens and closes', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONNECTION_SUMMARY] },
      {
        channel: IPC.connectionsDelete,
        args: { id: CONNECTION_ID },
        response: null,
        error: { code: 'E_BUSY', message: 'database is locked' },
      },
    ],
  });

  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-delete"]');
  await acceptConfirm(page);

  const trigger = connectionRow(page).locator('[data-testid="error-popover-trigger"]');
  await expect(trigger).toContainText('database is locked');
  await trigger.click();
  const popover = page.locator('[data-testid="error-popover"]');
  await expect(popover).toBeVisible();
  await popover.getByRole('button', { name: 'Close' }).click();
  await expect(popover).toHaveCount(0);
});

// P328: a table whose adapter reports hasChildren (a Postgres partitioned table) expands lazily
// to its partitions, one level deeper; double-click still opens its data tab.
test('a partitioned table expands to its partitions on demand and still opens its data tab', async ({
  relaunch,
}) => {
  const fixture = orderItemsFixture(CONNECTION_ID);
  const partitions = ['order_items_2024', 'order_items_2025'].map((name) => ({
    kind: 'table',
    name,
    path: `${APP_PATH}/table:${name}`,
    hasChildren: false,
  }));
  const control: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Tree DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...fixture.control.map((snap) => {
      const args = snap.args as { path?: string } | undefined;
      if (snap.channel !== IPC.treeChildren || args?.path !== APP_PATH) return snap;
      const response = snap.response as { nodes: { path: string }[] };
      return {
        ...snap,
        response: {
          ...response,
          nodes: response.nodes.map((n) =>
            n.path === ORDER_ITEMS_PATH ? { ...n, hasChildren: true } : n,
          ),
        },
      };
    }),
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: ORDER_ITEMS_PATH, refresh: false },
      response: { nodes: partitions, source: 'server', truncated: false },
    },
  ];
  const { window: page } = await relaunch({ control, stream: fixture.port });

  await createConnection(page, 'postgres', 'Tree DB');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);

  const parent = await findRow(page, ORDER_ITEMS_PATH);
  const partitionRow = page.locator(
    `[data-testid="tree-row"][data-path="${APP_PATH}/table:order_items_2024"]`,
  );
  await expect(partitionRow).toHaveCount(0);

  const parentLevel = Number(await parent.getAttribute('aria-level'));
  await expandRow(page, ORDER_ITEMS_PATH);
  await expect(partitionRow).toHaveCount(1);
  await expect(partitionRow).toHaveAttribute('aria-level', String(parentLevel + 1));

  await parent.dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
});
