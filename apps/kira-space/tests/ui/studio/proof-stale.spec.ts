import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { calls } from '../support/ipcCalls';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import {
  openProofWindow,
  PENDING_ID,
  pendingSession,
  SESSION_ID,
  shotFolder,
  staleSession,
  stepSnapshot,
} from './support/proof';

// P753. The Go half (internal/flows/proofflow/stale_test.go) proves every content write of a proof
// raises kira:proof:changed and moves Detail.Version; this half proves the open window marks itself
// stale from that, never reloads on its own and reloads on request. Contract proof-stale.

const CAPTURE_MS = 60_000;

const changedEvent = () => contract('proof-stale', 'event:kira:proof:changed#acceptedEdit');
const deletedEvent = () => ({
  ...(contract('proof-stale', 'event:kira:proof:changed#deleted') as object),
  sessionId: SESSION_ID,
});

const stepsReady = (page: import('@playwright/test').Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible({
    timeout: 20_000,
  });
const tabTitles = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="proof-tabs"] [data-testid="tab"]').allInnerTexts();
const strip = (page: import('@playwright/test').Page) =>
  page.locator('[data-testid="proof-stale"]');

// The window loads `loaded`; the next two reads (the probe after an event, then a reload) answer `changed`.
const staleControl = () => [
  { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
  { channel: IPC.proofSession, args: { sessionId: SESSION_ID }, response: staleSession('loaded') },
  { channel: IPC.proofSession, args: { sessionId: SESSION_ID }, response: staleSession('changed') },
  stepSnapshot(SESSION_ID, 1, 1, 'table', { description: 'rows' }),
  stepSnapshot(SESSION_ID, 2, 1, 'table', { description: 'rows changed' }),
];

test('contract: an accepted edit marks the open window stale and Reload shows the new proof', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page, control } = await relaunch({ control: staleControl() });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);
  await expect(strip(page)).toHaveCount(0);
  expect((await tabTitles(page)).map((t) => t.trim())).toEqual(['#1 rows']);

  await emitWailsEvent(page, IPC.proofChanged, changedEvent());
  const marker = page.locator('[data-testid="proof-centre"] [data-testid="proof-stale"]');
  await expect(marker).toHaveAttribute('data-state', 'changed');
  // Nothing reloaded: the probe is the only extra read, the old tabs and step stay.
  expect(calls(control, IPC.proofSession)).toHaveLength(2);
  expect(calls(control, IPC.proofStep)).toHaveLength(1);
  expect((await tabTitles(page)).map((t) => t.trim())).toEqual(['#1 rows']);
  await expect(page.locator('[data-testid="proof-step-description"]')).toHaveText('rows');

  await page.locator('[data-testid="proof-stale-reload"]').click();
  await expect(strip(page)).toHaveCount(0);
  await expect
    .poll(async () => (await tabTitles(page)).map((t) => t.trim()))
    .toEqual(['#1 rows changed']);
  await expect(page.locator('[data-testid="proof-step-description"]')).toHaveText('rows changed');
});

test('an event for another proof leaves the window alone', async ({ relaunch, uiServer }) => {
  const { window: page, control } = await relaunch({ control: staleControl() });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await emitWailsEvent(page, IPC.proofChanged, { sessionId: 'other' });
  await page.waitForTimeout(500);
  expect(calls(control, IPC.proofSession)).toHaveLength(1);
  await expect(strip(page)).toHaveCount(0);
});

test('a deleted proof shows the deleted marker without Reload', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
      {
        channel: IPC.proofSession,
        args: { sessionId: SESSION_ID },
        response: staleSession('loaded'),
      },
      {
        channel: IPC.proofSession,
        args: { sessionId: SESSION_ID },
        error: { code: 'E_NOT_FOUND', message: 'no such proof session' },
      },
      stepSnapshot(SESSION_ID, 1, 1, 'table', { description: 'rows' }),
    ],
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await emitWailsEvent(page, IPC.proofChanged, deletedEvent());
  await expect(strip(page)).toHaveAttribute('data-state', 'removed');
  await expect(page.locator('[data-testid="proof-stale-reload"]')).toHaveCount(0);
});

test("the window's own accept leaves no marker", async ({ relaunch, uiServer }) => {
  const pending = pendingSession();
  const accepted = { ...pending, current: pending.pending, pending: null, removed: [] };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: PENDING_ID } },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: pending },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: accepted },
      ...['keep', 'replace', 'drop'].map((d, i) =>
        stepSnapshot(PENDING_ID, 1, i + 1, 'keyvalue', { description: d }),
      ),
      ...['keep', 'replaced', 'brand new'].map((d, i) =>
        stepSnapshot(PENDING_ID, 2, i + 1, 'keyvalue', { description: d }),
      ),
    ],
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await page.locator('[data-testid="proof-accept"]').click();
  await expect(page.locator('[data-testid="proof-pending"]')).toHaveCount(0);
  // Go also announces the accept; the probe reads the same version the window already shows.
  await emitWailsEvent(page, IPC.proofChanged, { sessionId: PENDING_ID });
  await expect.poll(() => calls(control, IPC.proofSession).length).toBeGreaterThanOrEqual(3);
  await page.waitForTimeout(300);
  await expect(strip(page)).toHaveCount(0);
});

test('Reload is off while a screenshot runs', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({
    control: [
      ...staleControl(),
      { channel: IPC.proofOpenShotFolder, hold: true, response: shotFolder() },
    ],
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);
  await emitWailsEvent(page, IPC.proofChanged, changedEvent());
  const reload = page.locator('[data-testid="proof-stale-reload"]');
  await expect(reload).toBeEnabled();

  await page.locator('[data-testid="proof-shot"]').click();
  await expect.poll(() => calls(control, IPC.proofOpenShotFolder)).toHaveLength(1);
  await expect(reload).toBeDisabled();

  control.release(IPC.proofOpenShotFolder);
  await expect(reload).toBeEnabled({ timeout: CAPTURE_MS });
  await expect(strip(page)).toHaveAttribute('data-state', 'changed');
});
