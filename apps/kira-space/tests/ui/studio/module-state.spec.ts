import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { TIMING } from '@workbench/behaviour/timing';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { apiTest, expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { roundTrip } from '../support/moduleRoundTrip';
import { modeTab } from './support/apiMode';
import { WIDE_TABLE_COLUMNS, WIDE_TABLE_ROWS } from './support/cellEditorCaptures';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { container, openDocker, seedContainers, setupDocker } from './support/dockerFixture';
import { gridScroller, headerNames } from './support/grid';
import {
  activePanel,
  k,
  loc,
  openKind,
  openKubernetes,
  setupKubernetes,
} from './support/kubernetesFixture';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
  WIDE_TABLE_PATH,
} from './support/postgresFixture';
import { findRow, treeContainer } from './support/tree';

// P660: Studio, API, Docker and Kubernetes keep view state across a module round trip
// (search bar and text, list and grid scroll, column filter, find bars). No backend half: the
// state is frontend-only and session-only, never sent over IPC.

const t = (id: string) => `[data-testid="${id}"]`;
const scrollTop = (el: Locator) => el.evaluate((e) => e.scrollTop);

async function scrollTo(el: Locator, top: number): Promise<number> {
  await el.evaluate((e, y) => {
    e.scrollTop = y;
  }, top);
  await expect.poll(() => scrollTop(el)).toBeGreaterThan(0);
  return scrollTop(el);
}

const CONNECTION_ID = 'conn-module-state';
const FIXTURE = bigRowsFixture(CONNECTION_ID);
const NAMES = WIDE_TABLE_COLUMNS.map((c) => c.name);
const HEADERS = t('grid-header-cell');

const WIDE_META = {
  path: WIDE_TABLE_PATH,
  kind: 'table',
  name: 'wide_table',
  qualifiedName: 'app.wide_table',
  columns: WIDE_TABLE_COLUMNS.map((c, i) => ({
    name: c.name,
    position: i + 1,
    dataType: c.dataType,
    nullable: c.nullable,
    defaultExpr: null,
    isPrimaryKey: c.isPrimaryKey,
    comment: null,
  })),
  primaryKey: ['id'],
  foreignKeys: [],
  referencedBy: [],
  indexes: [],
  rowEstimate: WIDE_TABLE_ROWS.length,
  comment: null,
};

const STUDIO_CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('State DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'State DB', 'green'),
  },
  ...FIXTURE.control,
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: WIDE_TABLE_PATH, refresh: false, tabId: null },
    response: { meta: WIDE_META, source: 'server' },
  },
];

const STUDIO_PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: WIDE_TABLE_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: WIDE_TABLE_COLUMNS,
        rows: WIDE_TABLE_ROWS,
        position: {
          offset: 0,
          pageSize: 100,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'keyset',
        },
        truncatedCells: 0,
      },
      source: 'server',
    },
  },
];

test('Studio: tree search and scroll, grid scroll and column filter survive a module round trip', async ({
  relaunch,
}) => {
  test.setTimeout(90_000);
  const { window: page } = await relaunch({ control: STUDIO_CONTROL, stream: STUDIO_PORT });
  await page.setViewportSize({ width: 1200, height: 420 });
  await connectAndExpand(page, 'State DB', 'green');

  // Grid scroll on big_rows, then the column filter on wide_table, then back in-module.
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  await expect(gridScroller(page)).toBeVisible();
  const gridScrolled = await scrollTo(gridScroller(page), 400);
  // The grid saves its scroll into the tab after a debounce (an unmount inside it drops the write).
  await page.waitForTimeout(TIMING.scrollPersist + 200);
  await (await findRow(page, WIDE_TABLE_PATH)).dblclick();
  const filter = t('column-filter-input');
  await page.locator(filter).fill('date');
  await expect.poll(async () => (await headerNames(page)).length).toBeLessThan(NAMES.length);
  const narrowed = await headerNames(page);
  await page
    .locator(`${t('tab')}[data-tab-active="true"], ${t('tab')}`)
    .first()
    .click();
  await expect(gridScroller(page)).toBeVisible();
  await page.locator(t('tab')).nth(1).click();
  // Opening rows scrolls the tree to them; read the position the round trip must keep.
  const treeScrolled = await scrollTo(treeContainer(page), 160);

  await roundTrip(page, 'api', 'studio');

  await expect.poll(() => scrollTop(treeContainer(page))).toBe(treeScrolled);
  await expect(page.locator(filter)).toHaveValue('date');
  expect(await headerNames(page)).toEqual(narrowed);
  await page.locator(t('tab')).first().click();
  await expect.poll(() => scrollTop(gridScroller(page))).toBe(gridScrolled);

  // Search bar and text.
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('wide');
  await roundTrip(page, 'api', 'studio');
  await expect(page.locator(t('tree-search'))).toHaveValue('wide');
});

apiTest('API: response find and body scroll survive a module round trip', async ({ relaunch }) => {
  const body = JSON.stringify(
    Array.from({ length: 400 }, (_, i) => ({ id: i, name: `item-${i}` })),
    null,
    2,
  );
  const NOW = '2026-01-01T00:00:00.000Z';
  const tree = {
    collections: [
      {
        id: 'col-1',
        name: 'Bulk API',
        file: 'bulk-api.yaml',
        sortOrder: 0,
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
    items: Array.from({ length: 40 }, (_, i) => ({
      id: `item-${i}`,
      collectionId: 'col-1',
      parentId: null,
      kind: 'request',
      name: `Request ${String(i).padStart(2, '0')}`,
      sortOrder: i,
      method: 'GET',
      url: `https://api.example.com/r/${i}`,
      createdAt: NOW,
      updatedAt: NOW,
    })),
  };
  const control: ControlSnapshot[] = [
    { channel: IPC.collectionsList, response: tree },
    {
      channel: IPC.httpSend,
      response: {
        status: 200,
        statusText: 'OK',
        proto: 'HTTP/1.1',
        headers: [{ name: 'Content-Type', value: 'application/json' }],
        body,
        bodyEncoding: 'utf8',
        bodyBytes: body.length,
        bodyTruncated: false,
        elapsedMs: 12,
        finalUrl: 'https://api.example.com/items',
        redirects: [],
      },
    },
  ];
  const { window: page } = await relaunch({ control });
  await page.setViewportSize({ width: 1200, height: 700 });
  await modeTab(page, 'api').click();
  await expect(page.locator(t('toggle-search'))).toBeVisible();
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('Request');
  await page.locator(`${t('collection-row')}[data-id="col-1"] ${t('tree-twisty')}`).click();
  const apiTree = page.locator(`${t('tree-background')} .virtual-list`);
  const treeScrolled = await scrollTo(apiTree, 300);
  await page.click(t('new-request-start'));
  await page.fill(t('http-url'), 'https://api.example.com/items');
  await page.click(t('http-send'));
  await expect(page.locator(t('http-status'))).toContainText('200');

  // Response find bar and its query.
  await page.locator(t('http-find-toggle')).click();
  await page.locator(t('http-find-input')).fill('item-3');

  // Response body scroll: Monaco scrolls by wheel; its first rendered line moves.
  const firstLine = () =>
    page.locator(`${t('http-request-view')} .monaco-editor .view-line`).evaluateAll(
      (els) =>
        els
          .map((e) => ({
            top: Number.parseFloat((e as HTMLElement).style.top),
            text: e.textContent,
          }))
          .sort((a, b) => a.top - b.top)[0]?.text,
    );
  await expect.poll(firstLine).toContain('[');
  const atTop = await firstLine();
  await page
    .locator(`${t('http-request-view')} .monaco-editor`)
    .last()
    .hover();
  await page.mouse.wheel(0, 2400);
  await expect.poll(firstLine).not.toBe(atTop);
  // The find bar re-reveals its match after the wheel settles; compare the settled position.
  await page.waitForTimeout(1000);
  const scrolled = await firstLine();
  expect(scrolled).not.toBe(atTop);

  await roundTrip(page, 'studio', 'api');

  await expect(page.locator(t('tree-search'))).toHaveValue('Request');
  await expect
    .poll(() => scrollTop(page.locator(`${t('tree-background')} .virtual-list`)))
    .toBe(treeScrolled);
  await expect(page.locator(t('http-find-input'))).toHaveValue('item-3');
  await expect.poll(firstLine).toBe(scrolled);
});

test('Docker: panel search, list scroll and logs query survive a module round trip', async ({
  relaunch,
}) => {
  const { page, docker, state } = await setupDocker(relaunch);
  state.containers = [
    ...seedContainers(),
    ...Array.from({ length: 40 }, (_, i) =>
      container(`c-bulk-${i}`, `bulk-job-${String(i).padStart(2, '0')}`, 'running'),
    ),
  ];
  await page.setViewportSize({ width: 1200, height: 520 });
  await openDocker(page);

  // Logs query on a container tab first: opening a row scrolls the list to it.
  await page.locator(t('docker-row')).first().click();
  await expect(page.locator(t('docker-container-detail'))).toBeVisible();
  await page.locator(t('docker-tab-logs')).click();
  await expect.poll(() => docker.calls('LogsOpen').length).toBe(1);
  await page.locator(t('docker-logs-find')).click();
  await page.locator(t('docker-logs-find-input')).fill('boom');
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('o');

  const list = page.locator(`${t('docker-container-list')}`);
  const scrolled = await scrollTo(list, 300);

  await roundTrip(page, 'studio', 'docker');

  await expect(page.locator(t('tree-search'))).toHaveValue('o');
  await expect.poll(() => scrollTop(page.locator(t('docker-container-list')))).toBe(scrolled);
  await expect(page.locator(t('docker-logs-find-input'))).toHaveValue('boom');
  await emitWailsEvent(page, 'kira:docker:logs', { streamId: 'none', lines: [], ended: false });
});

test('Kubernetes: panel search, resource scroll and describe filter survive a module round trip', async ({
  relaunch,
}) => {
  type List = { rows: { key: string; name: string; cells: string[] }[] };
  const base = () => k<List>('KubernetesService.List');
  const { page } = await setupKubernetes(relaunch, {
    List: () => {
      const list = base();
      const pad = Array.from({ length: 50 }, (_, i) => ({
        ...list.rows[0],
        key: `uid-pad-${i}`,
        name: `pad-${i}`,
        cells: [`pad-${i}`, ...list.rows[0].cells.slice(1)],
      }));
      return { ...list, rows: [...list.rows, ...pad] };
    },
  });
  await page.setViewportSize({ width: 1200, height: 520 });
  await openKubernetes(page);
  await page.locator(t('toggle-search')).click();
  await page.locator(t('tree-search')).fill('kind');
  await openKind(page, 'kind-dev', 'v1/pods');

  // Describe filter first: opening a row scrolls the table to it.
  await page.locator(`${t('kube-row')}[data-name="web-1"]`).click();
  await expect(activePanel(page)).toBeVisible();
  await loc(page, 'kube-describe-filter').fill('Scheduled');

  await roundTrip(page, 'studio', 'kubernetes');

  await expect(page.locator(t('tree-search'))).toHaveValue('kind');
  await expect(loc(page, 'kube-describe-filter')).toHaveValue('Scheduled');
});
