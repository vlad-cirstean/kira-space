import type { ProofViewState } from '@shared/domain/proof';
import { expect, studioTest as test } from '../../ui/fixtures';
import {
  bigTable,
  chartControl,
  LONG_QUERY,
  openProofWindow,
  PENDING_ID,
  pendingSession,
  proofControl,
  SESSION_ID,
  sessions,
  stepSnapshot,
  viewState,
} from '../../ui/studio/support/proof';
import { IPC } from '../../ui/support/ipcChannels';

// P420 Part 2: the proof window follows the review window's chrome; these baselines catch a
// layout or token drift in the three surfaces a reader sees.

const WINDOW = '[data-testid="proof-window"]';
const ready = (page: import('@playwright/test').Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible();

test('proof window, table step with a scrolled grid and a highlighted cell (P420)', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page } = await relaunch({
    control: proofControl([
      stepSnapshot(SESSION_ID, 1, 1, 'table', { data: bigTable(), query: LONG_QUERY }),
      stepSnapshot(SESSION_ID, 1, 2, 'document'),
      stepSnapshot(SESSION_ID, 1, 3, 'keyvalue'),
      {
        channel: IPC.proofViewState,
        response: [
          viewState(1, {
            resultScrollTop: 600,
            resultScrollLeft: 60,
            selection: { kind: 'cell', row: 30, col: 2 },
          }),
        ],
      },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await ready(page);
  await expect(page.locator('.kira-cell-selected')).toHaveCount(1);
  await expect(page.locator(WINDOW)).toHaveScreenshot('proof-table-step.png');
});

test('proof window, document step with an expanded row (P420)', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({
    control: proofControl([
      stepSnapshot(SESSION_ID, 1, 1, 'table'),
      stepSnapshot(SESSION_ID, 1, 2, 'document', {
        data: {
          documents: [
            { id: 'doc-1', body: { a: { nested: true }, z: 1 } },
            { id: 'doc-2', body: { title: 'Second' } },
          ],
        },
      }),
      stepSnapshot(SESSION_ID, 1, 3, 'keyvalue'),
      {
        channel: IPC.proofViewState,
        response: [viewState(2, { expanded: ['doc-1'], selectedRow: 0 })],
      },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await page.locator('[data-testid="tab"]', { hasText: '#2' }).click();
  await ready(page);
  await expect(page.locator('[data-testid="console-result-doc-row"]').first()).toBeVisible();
  await expect(page.locator(WINDOW)).toHaveScreenshot('proof-document-step.png');
});

test('proof window, compare mode with a pending edit (P420)', async ({ relaunch, uiServer }) => {
  const pending = pendingSession();
  const summary = { ...sessions()[0], id: PENDING_ID, name: 'Edit me', pending: true };
  const steps = [
    ...['keep', 'replace', 'drop'].map((d, i) =>
      stepSnapshot(PENDING_ID, 1, i + 1, 'keyvalue', { description: d }),
    ),
    ...['keep', 'replaced', 'brand new'].map((d, i) =>
      stepSnapshot(PENDING_ID, 2, i + 1, 'keyvalue', { description: d }),
    ),
  ];
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: PENDING_ID } },
      { channel: IPC.proofSessions, response: [summary] },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: pending },
      ...steps,
    ],
  });
  await openProofWindow(page, uiServer.url);
  await ready(page);
  await page.locator('[data-testid="proof-view-compare"]').click();
  await page.locator('[data-testid="tab"]', { hasText: '#2 replaced' }).click();
  await expect(page.locator('[data-testid="proof-compare"]')).toBeVisible();
  await ready(page);
  await expect(page.locator(WINDOW)).toHaveScreenshot('proof-compare-pending.png');
});

// P426: one baseline per chart family a reader sees; payloads come from the Go-written contract.
test.describe('proof chart', () => {
  test.use({ timezoneId: 'UTC' });

  const chartShot = async (
    relaunch: (o: Record<string, unknown>) => Promise<{ window: import('@playwright/test').Page }>,
    url: string,
    tab: number,
    state: NonNullable<ProofViewState['chart']>,
    name: string,
  ) => {
    const { window: page } = await relaunch({
      control: chartControl([
        {
          channel: IPC.proofViewState,
          response: [viewState(tab, { chart: state })],
        },
      ]),
      timezoneId: 'UTC',
    });
    await openProofWindow(page, url);
    await page
      .locator('[data-testid="tab"]')
      .nth(tab - 1)
      .click();
    await ready(page);
    await expect(page.locator('[data-testid="proof-chart"]')).toBeVisible();
    await expect(page.locator(WINDOW)).toHaveScreenshot(name);
  };

  test('line, zoomed with a hidden series (P426)', async ({ relaunch, uiServer }) => {
    await chartShot(
      relaunch,
      uiServer.url,
      1,
      { hidden: ['after <b>fix</b>'], selected: null, zoom: { start: 25, end: 75 } },
      'proof-chart-line.png',
    );
  });

  test('stacked bar with a selected bar (P426)', async ({ relaunch, uiServer }) => {
    await chartShot(
      relaunch,
      uiServer.url,
      2,
      { hidden: [], selected: { series: 0, index: 1 }, zoom: null },
      'proof-chart-bar-stacked.png',
    );
  });

  test('pie (P426)', async ({ relaunch, uiServer }) => {
    await chartShot(
      relaunch,
      uiServer.url,
      6,
      { hidden: [], selected: null, zoom: null },
      'proof-chart-pie.png',
    );
  });
});
