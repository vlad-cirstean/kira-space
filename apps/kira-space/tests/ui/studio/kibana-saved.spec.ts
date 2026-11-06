import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import { savedQuerySchema } from '@shared/domain/queries';
import { browserTimeZone, resolveTime } from '../../../frontend/src/studio/views/console/kibana/time';
import { expect, test } from './fixtures';
import { contract } from './support/contract';
import { acceptConfirm, cancelConfirm } from './support/dialogs';
import { IPC } from './support/ipcChannels';
import { executeHits, kibanaControl, logsNode, logsView } from './support/kibanaFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// UI half of the kibana-saved scenario; the Go half is dbflow TestKibanaSavedQueriesJourney.
// Bridge answers are that test's recorded results.

const NOW = new Date('2026-01-15T12:00:00.000Z');
const saved = <T extends object = Record<string, unknown>>(key: string) =>
  contract<T>('kibana-saved', key);

const first = saved<{ id: string; name: string; body: Record<string, unknown> }>(
  'QueriesService.SaveDiscover#first',
);
const renamedList = saved<(typeof first)[]>('QueriesService.ListDiscover#afterRename');
const renamed = saved<typeof first>('QueriesService.Update#renamed');
const renameTaken = saved<{ code: string; message: string }>('QueriesService.Update#nameTaken');
const nameTaken = saved<{ code: string; message: string }>('QueriesService.SaveDiscover#nameTaken');
const EXISTS_STATUS = { field: 'status', operator: 'exists' as const, values: [], disabled: false };
// The mock keys nested args by key order, so a body is spelled the way the view sends it.
const ordered = (b: Record<string, unknown>) =>
  Object.fromEntries(
    [
      'query',
      'dataViewId',
      'dataViewName',
      'language',
      'time',
      'filters',
      'columns',
      'sort',
      'sampleSize',
      'interval',
    ].map((k) => [k, b[k]]),
  );
// The update answer, with the body this spec's own tab state holds.
const updated = savedQuerySchema.parse({
  ...saved('QueriesService.UpdateDiscover#updated'),
  connectionId: 'conn-kibana',
  body: { ...first.body, filters: [EXISTS_STATUS] },
});
const listed = savedQuerySchema.parse({ ...updated, connectionId: 'conn-kibana' });

function statement(patch: Partial<KibanaDiscoverRequest> = {}): string {
  const { columns, ...rest }: KibanaDiscoverRequest = {
    dataView: logsView.id,
    query: '',
    language: 'kuery',
    timeRange: resolveTime({ mode: 'preset', preset: 'last15m' }, NOW),
    filters: [],
    sort: [],
    size: 500,
    timeZone: browserTimeZone(),
    ...patch,
  };
  return JSON.stringify(columns ? { ...rest, columns } : rest);
}

const QUERY = 'level:error';
const COLUMNS = ['level'];

test('contract: Discover saves, updates and reopens a query, and flags unsaved edits', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      ...kibanaControl(),
      { channel: IPC.queriesListDiscover, response: [listed] },
      {
        channel: IPC.queriesSaveDiscover,
        args: {
          connectionId: 'conn-kibana',
          name: first.name,
          body: ordered(first.body),
          pinned: false,
        },
        response: { ...first, connectionId: 'conn-kibana' },
      },
      {
        channel: IPC.queriesSaveDiscover,
        args: {
          connectionId: 'conn-kibana',
          name: 'Errors Last Hour',
          body: ordered(updated.body),
          pinned: false,
        },
        error: { code: nameTaken.code, message: nameTaken.message },
      },
      { channel: IPC.queriesUpdateDiscover, response: updated },
      {
        channel: IPC.queriesUpdate,
        args: { id: updated.id, name: 'errors last hour', pinned: null },
        error: { code: renameTaken.code, message: renameTaken.message },
      },
      {
        channel: IPC.queriesUpdate,
        args: { id: updated.id, name: '5xx', pinned: null },
        response: { ...updated, name: '5xx' },
      },
      { channel: IPC.queriesDelete, response: null },
    ],
    stream: [
      executeHits(statement(), 200),
      executeHits(statement({ query: QUERY }), 50),
      executeHits(statement({ query: QUERY, columns: COLUMNS }), 50),
      executeHits(statement({ query: QUERY, columns: COLUMNS, filters: [EXISTS_STATUS] }), 30),
      executeHits(
        statement({ query: QUERY, columns: ['level', 'status'], filters: [EXISTS_STATUS] }),
        30,
      ),
    ],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);

  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await (await findRow(page, logsNode.path)).dblclick();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });

  const dirty = page.locator('[data-testid="kibana-saved-dirty"]');
  const name = page.locator('[data-testid="kibana-saved-name"]');
  await expect(name).toHaveCount(0);

  await page.locator('[data-testid="kibana-query"]').click();
  await page.keyboard.type(QUERY);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="kibana-total"]')).toBeVisible();
  await page.click('[data-testid="kibana-field-column-level"]');

  // Save with no link prompts for a name, then the tab is linked and clean.
  await page.click('[data-testid="kibana-save"]');
  await page.fill('[data-testid="text-prompt-input"]', first.name);
  await page.click('[data-testid="text-prompt-ok"]');
  await expect(name).toContainText(first.name);
  await expect(dirty).toHaveCount(0);

  // An edit marks the tab dirty; Ctrl/Cmd+S updates the same entry.
  await page.click('[data-testid="kibana-field-exists-status"]');
  await expect(dirty).toBeVisible();
  await page.locator('[data-testid="kibana-query"]').click();
  // Monaco maps its Cmd chord to Meta under the UI suite's Mac user agent.
  await page.keyboard.press('Meta+s');
  await expect(dirty).toHaveCount(0);

  // Save as with the existing name shows the conflict and keeps the link.
  await page.click('[data-testid="kibana-save-more"]');
  await page.click('[data-testid="kibana-save-as"]');
  await page.fill('[data-testid="text-prompt-input"]', 'Errors Last Hour');
  await page.click('[data-testid="text-prompt-ok"]');
  await expect(page.locator('[data-testid="kibana-saved-error"]')).toContainText(nameTaken.message);
  await expect(name).toContainText(first.name);

  // An extra column dirties the tab; Revert restores the saved body.
  await page.click('[data-testid="kibana-field-column-status"]');
  await expect(dirty).toBeVisible();
  await page.click('[data-testid="kibana-save-more"]');
  await page.click('[data-testid="kibana-revert"]');
  await expect(dirty).toHaveCount(0);
  await expect(page.locator('[data-testid="kibana-field-column-status"]')).toBeVisible();

  // The saved list shows the entry; a taken name keeps the old one, a free name relinks the tab.
  await page.click('[data-testid="kibana-saved-toggle"]');
  const entry = page.locator('[data-testid="kibana-saved-entry"]');
  await expect(entry).toContainText(first.name);
  await entry.hover();
  await page.click('[data-testid="kibana-saved-rename"]');
  await page.fill('[data-testid="text-prompt-input"]', 'errors last hour');
  await page.click('[data-testid="text-prompt-ok"]');
  await expect(page.locator('[data-testid="kibana-saved-menu-error"]')).toContainText(
    renameTaken.message,
  );
  await entry.hover();
  await page.click('[data-testid="kibana-saved-rename"]');
  await page.fill('[data-testid="text-prompt-input"]', '5xx');
  await page.click('[data-testid="text-prompt-ok"]');
  await expect(name).toContainText('5xx');

  // Deleting the linked entry detaches the tab but keeps its state.
  await entry.hover();
  await entry.getByRole('button', { name: 'Delete' }).click();
  await page.click('[data-testid="confirm-dialog-confirm"]');
  await expect(name).toHaveCount(0);
  await expect(page.locator('[data-testid="kibana-field-column-level"]')).toBeVisible();
});

test('contract: a saved Discover query opens in a new tab with its exact state', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [...kibanaControl(), { channel: IPC.queriesListDiscover, response: [listed] }],
    stream: [
      executeHits(statement(), 200),
      // A tab opened from the saved menu has no data-view node path.
      executeHits(statement({ query: QUERY, columns: COLUMNS, filters: [EXISTS_STATUS] }), 30, ''),
    ],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);

  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await (await findRow(page, logsNode.path)).dblclick();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });

  await page.click('[data-testid="kibana-saved-toggle"]');
  const entry = page.locator('[data-testid="kibana-saved-entry"]');
  await entry.hover();
  await page.click('[data-testid="kibana-saved-open-new"]');

  await expect(page.locator('[data-testid="kibana-saved-name"]')).toContainText(listed.name);
  await expect(page.locator('[data-testid="kibana-saved-dirty"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="kibana-field-column-level"]')).toBeVisible();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });
});

test('contract: pinning a saved Discover query moves it first', async ({ relaunch }) => {
  const unpinned = renamedList.map((row) => ({ ...row, pinned: false })).reverse();
  const listArgs = { connectionId: 'conn-kibana' };
  const { window: page, control } = await relaunch({
    control: [
      ...kibanaControl(),
      {
        channel: IPC.queriesListDiscover,
        args: listArgs,
        response: unpinned.map((row) => ({ ...row, connectionId: 'conn-kibana' })),
      },
      {
        channel: IPC.queriesListDiscover,
        args: listArgs,
        response: renamedList.map((row) => ({ ...row, connectionId: 'conn-kibana' })),
      },
      {
        channel: IPC.queriesUpdate,
        args: { id: renamed.id, name: null, pinned: true },
        response: { ...renamed, connectionId: 'conn-kibana' },
      },
    ],
    stream: [executeHits(statement(), 200)],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);

  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await (await findRow(page, logsNode.path)).dblclick();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });

  await page.click('[data-testid="kibana-saved-toggle"]');
  const entries = page.locator('[data-testid="kibana-saved-entry"]');
  await expect(entries).toHaveCount(2);
  const names = renamedList.map((row) => row.name);
  await expect(entries.first()).toContainText(names[1]);
  await expect(entries.first().locator('.codicon-star-full')).toHaveCount(0);

  await entries.filter({ hasText: names[0] }).locator('[data-testid="saved-entry-pin"]').click();
  await expect(entries.first()).toContainText(names[0]);
  await expect(entries.first().locator('.codicon-star-full')).toHaveCount(1);
  await expect(entries.nth(1)).toContainText(names[1]);
  expect(
    control
      .log()
      .filter((e) => e.channel === IPC.queriesUpdate)
      .map((e) => e.args),
  ).toEqual([{ id: renamed.id, name: null, pinned: true }]);
});

test('contract: closing a Discover tab with unsaved changes asks to discard', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [...kibanaControl(), { channel: IPC.queriesListDiscover, response: [listed] }],
    stream: [
      executeHits(statement(), 200),
      executeHits(statement({ query: QUERY, columns: COLUMNS, filters: [EXISTS_STATUS] }), 30, ''),
      executeHits(
        statement({ query: QUERY, columns: ['level', 'status'], filters: [EXISTS_STATUS] }),
        30,
        '',
      ),
    ],
  });
  await page.clock.install({ time: NOW });
  await page.clock.setFixedTime(NOW);

  const connRow = connectionRow(page);
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await (await findRow(page, logsNode.path)).dblclick();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });

  await page.click('[data-testid="kibana-saved-toggle"]');
  await page.locator('[data-testid="kibana-saved-entry"]').hover();
  await page.click('[data-testid="kibana-saved-open-new"]');
  const dirty = page.locator('[data-testid="kibana-saved-dirty"]');
  await expect(page.locator('[data-testid="kibana-saved-name"]')).toContainText(listed.name);
  await expect(dirty).toHaveCount(0);

  const tabs = page.locator('[data-testid="tab"]');
  const before = await tabs.count();
  await page.click('[data-testid="kibana-field-column-status"]');
  await expect(dirty).toBeVisible();

  const active = page.locator('[data-testid="tab"][data-active="true"]');
  await active.hover();
  await active.locator('[data-testid="tab-close"]').click();
  await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toContainText('Discard');
  await cancelConfirm(page);
  await expect(tabs).toHaveCount(before);
  await expect(dirty).toBeVisible();

  await active.locator('[data-testid="tab-close"]').click();
  await acceptConfirm(page);
  await expect(tabs).toHaveCount(before - 1);
});
