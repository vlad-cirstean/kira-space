import type { KibanaDiscoverRequest } from '@shared/domain/kibana';
import { DATA_OP } from '@shared/protocol/dataOps';
import { browserTimeZone, resolveTime } from '../../../frontend/src/studio/views/console/kibana/time';
import { expect, test } from './fixtures';
import { typeInto } from './support/editor';
import {
  executeColumns,
  executeHits,
  histogram,
  kibanaControl,
  logsNode,
  logsView,
} from './support/kibanaFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

// UI half of the kibana-discover scenario; the Go half is dbflow TestKibanaDiscoverJourney.
// Answers are that test's recorded results, so both halves assert the same numbers.

const NOW = new Date('2026-01-15T12:00:00.000Z');

function statement(patch: Partial<KibanaDiscoverRequest> = {}): string {
  const { columns, ...request }: KibanaDiscoverRequest = {
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
  return JSON.stringify(columns ? { ...request, columns } : request);
}

const EXISTS_STATUS = {
  field: 'status',
  operator: 'exists' as const,
  values: [],
  disabled: false,
};

test('Discover runs the typed query, suggests fields and applies a sidebar filter', async ({
  relaunch,
}) => {
  const { window: page, stream } = await relaunch({
    control: kibanaControl(),
    stream: [
      executeHits(statement(), 200),
      executeHits(statement({ query: 'level:error' }), 50),
      executeHits(statement({ query: 'level:error', filters: [EXISTS_STATUS] }), 30),
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

  await expect(page.locator('[data-testid="kibana-discover"]')).toBeVisible();
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible({ timeout: 15_000 });
  await expect(page.locator('[data-testid="kibana-total"]')).toContainText(
    histogram.total.toLocaleString(),
  );
  await expect(page.locator('[data-testid="kibana-field-level"]')).toBeVisible();

  const query = page.locator('[data-testid="kibana-query"]');
  await typeInto(query, page, 'lev');
  const suggestions = page.locator('.suggest-widget');
  await expect(suggestions).toBeVisible({ timeout: 10_000 });
  await expect(suggestions).toContainText('level');

  await page.keyboard.press('Escape');
  await page.keyboard.type('el:error');
  await page.keyboard.press('Escape');
  await page.keyboard.press('Enter');
  await expect(page.locator('[data-testid="kibana-results"]')).toBeVisible();

  await page.click('[data-testid="kibana-field-exists-status"]');

  await expect
    .poll(async () => (await stream.ops()).filter((o) => o.op === DATA_OP.execute).length, {
      timeout: 15_000,
    })
    .toBe(3);
  const executed = (await stream.ops())
    .filter((o) => o.op === DATA_OP.execute)
    .map((o) => (o.payload as { statements: string[] }).statements[0]);
  expect(executed).toEqual([
    statement(),
    statement({ query: 'level:error' }),
    statement({ query: 'level:error', filters: [EXISTS_STATUS] }),
  ]);
});

test('Discover shows the selected columns as a table', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: kibanaControl(),
    stream: [
      executeHits(statement(), 200),
      executeColumns(statement({ columns: ['level'] }), 50),
      executeColumns(statement({ columns: ['level', 'status'] }), 50),
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

  await page.click('[data-testid="kibana-field-column-level"]');
  await page.click('[data-testid="kibana-field-column-status"]');
  await expect(page.locator('[data-testid="kibana-field-column-status"]')).toHaveAccessibleName(
    /Remove/,
  );
  const results = page.locator('[data-testid="kibana-results"]');
  await expect(results.getByText('2026-01-01T00:03:00.000Z')).toBeVisible({ timeout: 15_000 });
  await expect(results.getByText('503').first()).toBeVisible();
});
