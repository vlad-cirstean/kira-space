import type { Page } from '@playwright/test';
import { CHANNEL } from '@shared/protocol/events';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { installClipboardSpy } from './support/clipboard';
import { editorText } from './support/editorText';
import {
  bigTable,
  diffClipboardImage,
  explainedControl,
  jpegHeight,
  LONG_QUERY,
  openProofWindow,
  PENDING_ID,
  pendingSession,
  proofControl,
  RESULT_VIEWPORT,
  SESSION_ID,
  session,
  sessions,
  shotFolder,
  shotLocation,
  stepSnapshot,
  viewState,
} from './support/proof';

// P420 Part 2. The Go half (internal/flows/proofflow) proves what the backend stores and serves;
// this half proves the window renders it: step order, literal data, saved view, screenshots, the
// edit review and the history list. Fixtures are the same tests/contract/proof*.json files.

// A capture clones thousands of styled nodes; headless WebKit takes seconds per tab.
const CAPTURE_MS = 60_000;

const copiedImageCount = (page: Page) =>
  page.evaluate(
    () => (window as unknown as { __clipboardImages: Blob[] }).__clipboardImages.length,
  );

const tabTitles = (page: Page) =>
  page.locator('[data-testid="proof-tabs"] [data-testid="tab"]').allInnerTexts();
// The first render waits on the bundle load and the bridge round-trip; WebKit is slow on a busy runner.
const stepsReady = (page: Page) =>
  expect(page.locator('[data-testid="proof-step"][data-ready="true"]').first()).toBeVisible({
    timeout: 20_000,
  });
const setViewStates = (log: { channel: string; args: unknown }[]) =>
  log.filter((c) => c.channel === IPC.proofSetViewState).map((c) => c.args as Json);

// biome-ignore lint/suspicious/noExplicitAny: logged bridge args
type Json = Record<string, any>;

const threeSteps = () => [
  stepSnapshot(SESSION_ID, 1, 1, 'table'),
  stepSnapshot(SESSION_ID, 1, 2, 'document'),
  stepSnapshot(SESSION_ID, 1, 3, 'keyvalue'),
];

test('the explanation and summary show in full at the larger size', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page } = await relaunch({ control: explainedControl() });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const detail = contract('proof-explained', 'ProofService.Session') as unknown as {
    summary: string;
    current: { steps: { description: string }[] };
  };
  const summary = page.locator('[data-testid="proof-summary"]');
  const description = page.locator('[data-testid="proof-step-description"]');
  await expect(summary).toHaveText(detail.summary);
  await expect(description).toHaveText(detail.current.steps[0]?.description ?? '');

  const clipped = await description.evaluate((el) => ({
    fits: el.scrollWidth <= el.clientWidth,
    overflow: getComputedStyle(el).textOverflow,
  }));
  expect(clipped.fits).toBe(true);
  expect(clipped.overflow).not.toBe('ellipsis');

  const size = (loc: typeof description) =>
    loc.evaluate((el) => Number.parseFloat(getComputedStyle(el).fontSize));
  expect(await size(description)).toBeGreaterThan(
    await size(page.locator('[data-testid="proof-title"]')),
  );
});

test('tabs follow step order and the data renders literally', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const titles = await tabTitles(page);
  expect(titles.map((t) => t.trim())).toEqual(['#1 Orders before', '#2 Doc hit', '#3 Stats']);
  await expect(page.locator('[data-testid="proof-summary"]')).toHaveCount(0);

  const result = page.locator('[data-testid="proof-step-result"]');
  await expect(
    result.locator('[data-testid="console-result-cell"]', { hasText: '<b>bold</b>' }),
  ).toBeVisible();
  await expect(result.locator('b')).toHaveCount(0);
  expect(await editorText(page.locator('[data-testid="proof-step-query"]'))).toBe(
    'select id, name, tags from orders',
  );

  await page.locator('[data-testid="tab"]', { hasText: '#2 Doc hit' }).click();
  await expect(result.locator('[data-testid="console-result-doc-row"]')).toHaveCount(2);
  await page.locator('[data-testid="tab"]', { hasText: '#3 Stats' }).click();
  await expect(result.locator('[data-testid="console-result-kv-row"]')).toHaveCount(2);
});

test('the history lists sessions, marks one seen when opened, and deletes after confirming', async ({
  relaunch,
  uiServer,
}) => {
  const other = { ...sessions()[0], id: 'session-9', name: 'Second run', seen: true };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
      { channel: IPC.proofSessions, response: [...sessions(), other] },
      { channel: IPC.proofSessions, response: [other] },
      { channel: IPC.proofSession, response: session() },
      ...threeSteps(),
    ],
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const row = page.locator(`[data-testid="proof-session-${SESSION_ID}"]`);
  await expect(row).toContainText('Checkout fix');
  await expect(row.locator('[data-testid="proof-session-unseen"]')).toBeVisible();
  await row.click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((c) => c.channel === IPC.proofMarkSeen)
        .map((c) => c.args),
    )
    .toEqual([{ sessionId: SESSION_ID }]);

  await page.locator(`[data-testid="proof-session-delete-${SESSION_ID}"]`).click({ force: true });
  await expect(page.locator('[data-testid="confirm-dialog-message"]')).toContainText(
    'Checkout fix',
  );
  await page.locator('[data-testid="confirm-dialog-confirm"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((c) => c.channel === IPC.proofDelete)
        .map((c) => c.args),
    )
    .toEqual([{ sessionId: SESSION_ID }]);
  await expect(page.locator(`[data-testid="proof-session-${SESSION_ID}"]`)).toHaveCount(0);
  await expect(page.locator('[data-testid="proof-session-session-9"]')).toBeVisible();
});

test('scroll and the highlighted cell persist per tab and across a reload', async ({
  relaunch,
  uiServer,
}) => {
  const control = () =>
    proofControl([
      stepSnapshot(SESSION_ID, 1, 1, 'table', { data: bigTable(), query: LONG_QUERY }),
      stepSnapshot(SESSION_ID, 1, 2, 'document'),
    ]);
  const { window: page, control: log } = await relaunch({ control: control() });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const viewport = page.locator(RESULT_VIEWPORT);
  await viewport.evaluate((el) => {
    el.scrollTop = 1200;
    el.scrollLeft = 260;
  });
  const rows = page.locator('[data-testid="proof-step-result"] [data-testid="console-result-row"]');
  await expect
    .poll(async () => Number(await rows.first().getAttribute('data-row')))
    .toBeGreaterThan(10);
  const cell = page
    .locator('[data-testid="proof-step-result"] [data-testid="console-result-row"][data-row="48"]')
    .locator('[data-testid="console-result-cell"][data-column="column_2"]');
  const selectedText = await cell.innerText();
  await cell.click();
  await page.locator('[data-testid="proof-step-query"]').hover();
  await page.mouse.wheel(0, 300);
  const firstQueryLine = (win: Page = page) =>
    win.locator('[data-testid="proof-step-query"] .view-line').first().innerText();
  await expect.poll(() => firstQueryLine()).not.toMatch(/line_1,?$/);
  const scrolledQueryLine = await firstQueryLine();

  await expect
    .poll(() => {
      const last = setViewStates(log.log()).at(-1)?.state;
      return (
        last &&
        last.selection?.kind === 'cell' &&
        last.resultScrollTop > 0 &&
        last.queryScrollTop > 0
      );
    })
    .toBeTruthy();
  const saved = setViewStates(log.log()).at(-1) as Json;
  expect(saved).toMatchObject({ sessionId: SESSION_ID, rev: 1, ordinal: 1 });
  expect(saved.state.resultScrollLeft).toBeGreaterThan(0);

  const expectRestored = async (win: Page) => {
    await stepsReady(win);
    const viewport = win.locator(RESULT_VIEWPORT);
    await expect.poll(() => viewport.evaluate((el) => el.scrollTop)).toBeGreaterThan(1000);
    expect(await viewport.evaluate((el) => el.scrollLeft)).toBeGreaterThan(50);
    const picked = win.locator('[data-testid="proof-step-result"] .kira-cell-selected');
    await expect(picked).toHaveCount(1);
    await expect(picked).toHaveText(selectedText);
    await expect.poll(() => firstQueryLine(win)).toBe(scrolledQueryLine);
  };

  // Another tab and back: the cache carries the state.
  await page.locator('[data-testid="tab"]', { hasText: '#2 Doc hit' }).click();
  await expect(page.locator('[data-testid="console-result-doc-row"]').first()).toBeVisible();
  await page.locator('[data-testid="tab"]', { hasText: '#1 Orders before' }).click();
  await expectRestored(page);

  // A reload: the backend's saved view comes back through ViewState.
  const { window: reloaded } = await relaunch({
    control: [
      ...control(),
      { channel: IPC.proofViewState, response: [{ ordinal: 1, state: saved.state }] },
    ],
  });
  await openProofWindow(reloaded, uiServer.url);
  await expectRestored(reloaded);
});

test('the copied screenshot matches the screen, scroll and highlight included', async ({
  relaunch,
  uiServer,
}) => {
  const { window: page } = await relaunch({
    control: proofControl([
      stepSnapshot(SESSION_ID, 1, 1, 'table', { data: bigTable(), query: LONG_QUERY }),
      {
        channel: IPC.proofViewState,
        response: [
          viewState(1, {
            queryScrollTop: 160,
            resultScrollTop: 900,
            resultScrollLeft: 220,
            selection: { kind: 'cell', row: 40, col: 3 },
          }),
        ],
      },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);
  await expect(page.locator('[data-testid="proof-step-result"] .kira-cell-selected')).toHaveCount(
    1,
  );
  await installClipboardSpy(page, { image: true });

  await page.locator('[data-testid="proof-shot-more"]').click();
  await page.locator('[data-testid="proof-shot-copy"]').click();

  await expect.poll(() => copiedImageCount(page), { timeout: CAPTURE_MS }).toBe(1);

  await page.mouse.move(0, 0);
  const shot = await page.locator('[data-testid="proof-centre"]').screenshot();
  const diff = await diffClipboardImage(page, shot);
  expect(diff.width).toBeGreaterThan(800);
  expect(diff.over).toBeLessThan(0.02);
  expect(diff.mean).toBeLessThan(6);
});

const savedFiles = (log: { channel: string; args: unknown }[]) =>
  log.filter((c) => c.channel === IPC.proofSaveJpegTo).map((c) => c.args as Json);

test('the copied screenshot includes an open cell editor', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);
  await page
    .locator('[data-testid="proof-step-result"] [data-testid="console-result-row"][data-row="0"]')
    .locator('[data-testid="console-result-cell"][data-column="name"]')
    .click();
  await expect(page.locator('[data-testid="cell-editor"]')).toBeVisible();
  await installClipboardSpy(page, { image: true });

  await page.locator('[data-testid="proof-shot-more"]').click();
  await page.locator('[data-testid="proof-shot-copy"]').click();
  await expect.poll(() => copiedImageCount(page), { timeout: CAPTURE_MS }).toBe(1);

  await page.mouse.move(0, 0);
  const shot = await page.locator('[data-testid="proof-centre"]').screenshot();
  const diff = await diffClipboardImage(page, shot);
  expect(diff.over).toBeLessThan(0.02);
  expect(diff.mean).toBeLessThan(6);
});

test('save screenshots writes every tab into a new Desktop folder with no picker', async ({
  relaunch,
  uiServer,
}) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({
    control: proofControl([
      ...threeSteps(),
      { channel: IPC.proofOpenShotFolder, response: shotFolder() },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  // Measured before the save: the status strip it raises shrinks the panes.
  const centre = await page.locator('[data-testid="proof-centre"]').boundingBox();
  await page.locator('[data-testid="proof-shot"]').click();
  await expect.poll(() => savedFiles(control.log()).length, { timeout: CAPTURE_MS }).toBe(3);

  const files = savedFiles(control.log());
  expect(files.map((f) => f.filename)).toEqual([
    '01-Orders-before.jpg',
    '02-Doc-hit.jpg',
    '03-Stats.jpg',
  ]);
  const dpr = await page.evaluate(() => window.devicePixelRatio);
  for (const f of files) {
    expect(f.folderId).toBe('shot_test');
    expect(f.jpegBase64.startsWith('/9j/')).toBe(true);
    expect(Math.abs(jpegHeight(f.jpegBase64) - (centre?.height ?? 0) * dpr)).toBeLessThanOrEqual(3);
  }
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofCloseShotFolder).length)
    .toBe(1);
  expect(control.log().filter((c) => c.channel === IPC.proofChooseShotLocation)).toHaveLength(0);
  // The tab the user was on is active again.
  await expect(
    page.locator('[data-testid="proof-tabs"] [data-testid="tab"][data-active="true"]'),
  ).toContainText('#1');

  const status = page.locator('[data-testid="proof-shot-status"]');
  await expect(status).toContainText(String(shotFolder().path));
  await page.locator('[data-testid="proof-shot-reveal"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofRevealShotFolder))
    .toHaveLength(1);
  expect(
    control.log().find((c) => c.channel === IPC.proofRevealShotFolder)?.args as Json,
  ).toMatchObject({ folderId: 'shot_test' });
});

test('the menu shows and changes the save location', async ({ relaunch, uiServer }) => {
  const { window: page, control } = await relaunch({
    control: proofControl([
      ...threeSteps(),
      { channel: IPC.proofShotLocation, response: shotLocation() },
      {
        channel: IPC.proofChooseShotLocation,
        response: { canceled: false, location: shotLocation(true) },
      },
      { channel: IPC.proofResetShotLocation, response: shotLocation() },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const more = page.locator('[data-testid="proof-shot-more"]');
  const location = page.locator('[data-testid="proof-shot-location"]');
  await more.click();
  await expect(location).toContainText(String(shotLocation().path));
  await expect(location).toContainText('Default');
  await expect(page.locator('[data-testid="proof-shot-location-reset"]')).toHaveCount(0);

  await page.locator('[data-testid="proof-shot-location-choose"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofChooseShotLocation))
    .toHaveLength(1);
  await more.click();
  await expect(location).toContainText(String(shotLocation(true).path));
  await page.locator('[data-testid="proof-shot-location-reset"]').click();
  await expect
    .poll(() => control.log().filter((c) => c.channel === IPC.proofResetShotLocation))
    .toHaveLength(1);
});

test('a missing location shows the error and saves nothing', async ({ relaunch, uiServer }) => {
  const message = 'The save location /Users/test/Gone no longer exists.';
  const { window: page, control } = await relaunch({
    control: proofControl([
      ...threeSteps(),
      {
        channel: IPC.proofOpenShotFolder,
        error: { code: 'E_BAD_REQUEST', message },
      },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await page.locator('[data-testid="proof-shot"]').click();
  const status = page.locator('[data-testid="proof-shot-status"]');
  await expect(status).toContainText(message);
  await expect(status).toHaveAttribute('data-tone', 'error');
  expect(savedFiles(control.log())).toHaveLength(0);
  await page.locator('[data-testid="proof-shot-status-dismiss"]').click();
  await expect(status).toHaveCount(0);
});

test('saving does not wait on a step that failed to load', async ({ relaunch, uiServer }) => {
  test.setTimeout(180_000);
  const { window: page, control } = await relaunch({
    control: proofControl([
      stepSnapshot(SESSION_ID, 1, 1, 'table'),
      {
        channel: IPC.proofStep,
        args: { sessionId: SESSION_ID, rev: 1, ordinal: 2 },
        error: { code: 'E_INTERNAL', message: 'step unreadable' },
      },
      stepSnapshot(SESSION_ID, 1, 3, 'keyvalue'),
      { channel: IPC.proofOpenShotFolder, response: shotFolder() },
    ]),
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const started = Date.now();
  await page.locator('[data-testid="proof-shot"]').click();
  await expect.poll(() => savedFiles(control.log()).length, { timeout: CAPTURE_MS }).toBe(3);
  // The ready timeout is 15 s; a failed step must not burn it.
  expect(Date.now() - started).toBeLessThan(14_000);
});

test('the cell editor shows a clicked cell read-only', async ({ relaunch, uiServer }) => {
  const { window: page } = await relaunch({ control: proofControl(threeSteps()) });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  const key = await page.locator('[data-testid="proof-step"]').getAttribute('data-step-key');
  const dock = page.locator(`[data-testid="cell-editor"][data-tab-id="${key}"]`);
  await expect(dock).toHaveCount(0);

  const cell = page
    .locator('[data-testid="proof-step-result"] [data-testid="console-result-row"][data-row="0"]')
    .locator('[data-testid="console-result-cell"][data-column="name"]');
  await cell.click();
  await expect(dock).toBeVisible();
  await expect(dock.locator('[data-testid="cell-editor-panel"]')).toHaveAttribute(
    'data-read-only',
    'true',
  );
  await expect
    .poll(() => editorText(dock.locator('[data-testid="cell-editor-encoded"]')))
    .toBe('alpha');

  // Another tab and back: the selection is kept per step.
  await page.locator('[data-testid="tab"]', { hasText: '#2 Doc hit' }).click();
  await expect(page.locator('[data-testid="console-result-doc-row"]').first()).toBeVisible();
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);
  await page.locator('[data-testid="tab"]', { hasText: '#1 Orders before' }).click();
  await expect(dock).toBeVisible();

  // A key-value row opens it too.
  await page.locator('[data-testid="tab"]', { hasText: '#3 Stats' }).click();
  await page
    .locator('[data-testid="proof-step-result"] [data-testid="console-result-kv-row"]')
    .first()
    .click();
  const kvDock = page.locator('[data-testid="cell-editor"]');
  await expect(kvDock).toBeVisible();
  await expect(kvDock.locator('[data-testid="cell-editor-panel"]')).toHaveAttribute(
    'data-read-only',
    'true',
  );
});

test('a pending edit shows the diff, compares old to new, and accepts', async ({
  relaunch,
  uiServer,
}) => {
  const pending = pendingSession();
  const accepted = { ...pending, current: pending.pending, pending: null, removed: [] };
  const pendingSummary = { ...sessions()[0], id: PENDING_ID, name: 'Edit me', pending: true };
  const steps = [
    ...['keep', 'replace', 'drop'].map((d, i) =>
      stepSnapshot(PENDING_ID, 1, i + 1, 'keyvalue', { description: d }),
    ),
    ...['keep', 'replaced', 'brand new'].map((d, i) =>
      stepSnapshot(PENDING_ID, 2, i + 1, 'keyvalue', { description: d }),
    ),
  ];
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: PENDING_ID } },
      { channel: IPC.proofSessions, response: [pendingSummary] },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: pending },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: accepted },
      ...steps,
    ],
  });
  await openProofWindow(page, uiServer.url);
  await stepsReady(page);

  await expect(page.locator('[data-testid="proof-pending"]')).toBeVisible();
  await expect(page.locator('[data-testid="proof-change-reason"]')).toHaveText('tighten');
  // Proposed by default.
  expect((await tabTitles(page)).map((t) => t.trim())).toEqual([
    '#1 keep',
    '#2 replaced',
    '#3 brand new',
  ]);
  await expect(page.locator('[data-testid="proof-change-step-2"]')).toHaveAttribute(
    'data-diff',
    'changed',
  );
  await expect(page.locator('[data-testid="proof-change-step-3"][data-diff="added"]')).toHaveCount(
    1,
  );
  await expect(
    page.locator('[data-testid="proof-change-step-3"][data-diff="removed"]'),
  ).toHaveCount(1);

  await page.locator('[data-testid="proof-view-current"]').click();
  expect((await tabTitles(page)).map((t) => t.trim())).toEqual([
    '#1 keep',
    '#2 replace',
    '#3 drop',
  ]);

  // Compare lists the proposed steps, then the one the edit removes.
  await page.locator('[data-testid="proof-view-compare"]').click();
  expect((await tabTitles(page)).map((t) => t.trim())).toEqual([
    '#1 keep',
    '#2 replaced',
    '#3 brand new',
    '#3 drop',
  ]);
  await page.locator('[data-testid="tab"]', { hasText: '#2 replaced' }).click();
  await expect(page.locator('[data-testid="proof-compare-old"]')).toContainText('replace');
  await expect(page.locator('[data-testid="proof-compare-new"]')).toContainText('replaced');
  await page.locator('[data-testid="tab"]', { hasText: '#3 brand new' }).click();
  await expect(
    page.locator('[data-testid="proof-compare-old"] [data-testid="proof-step"]'),
  ).toHaveCount(0);
  await expect(page.locator('[data-testid="proof-compare-new"]')).toContainText('brand new');
  await page.locator('[data-testid="tab"]', { hasText: '#3 drop' }).click();
  await expect(page.locator('[data-testid="proof-compare-old"]')).toContainText('drop');
  await expect(
    page.locator('[data-testid="proof-compare-new"] [data-testid="proof-step"]'),
  ).toHaveCount(0);

  await page.locator('[data-testid="proof-accept"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((c) => c.channel === IPC.proofAcceptEdit)
        .map((c) => c.args),
    )
    .toEqual([{ sessionId: PENDING_ID }]);
  await expect(page.locator('[data-testid="proof-change"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="proof-pending"]')).toHaveCount(0);
});

test('rejecting a pending edit calls RejectEdit', async ({ relaunch, uiServer }) => {
  const pending = pendingSession();
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: PENDING_ID } },
      {
        channel: IPC.proofSessions,
        response: [{ ...sessions()[0], id: PENDING_ID, pending: true }],
      },
      { channel: IPC.proofSession, args: { sessionId: PENDING_ID }, response: pending },
      {
        channel: IPC.proofSession,
        args: { sessionId: PENDING_ID },
        response: { ...pending, pending: null, removed: [] },
      },
      ...[1, 2].map((o) => stepSnapshot(PENDING_ID, 2, o, 'keyvalue')),
    ],
  });
  await openProofWindow(page, uiServer.url);
  await page.locator('[data-testid="proof-reject"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((c) => c.channel === IPC.proofRejectEdit)
        .map((c) => c.args),
    )
    .toEqual([{ sessionId: PENDING_ID }]);
  await expect(page.locator('[data-testid="proof-change"]')).toHaveCount(0);
});

test('a ready prompt opens the proof window, and a pushed select switches session', async ({
  relaunch,
  uiServer,
}) => {
  const other = { ...sessions()[0], id: 'session-9', name: 'Second run' };
  const { window: page, control } = await relaunch({
    control: [
      { channel: IPC.proofWindowTarget, response: { sessionId: SESSION_ID } },
      {
        channel: IPC.proofSessions,
        response: [{ ...sessions()[0], taskTitle: 'Fix checkout' }, other],
      },
      { channel: IPC.proofSession, args: { sessionId: SESSION_ID }, response: session() },
      {
        channel: IPC.proofSession,
        args: { sessionId: other.id },
        response: { ...session(), id: other.id, name: other.name },
      },
      ...threeSteps(),
      ...[1, 2, 3].map((o) => ({ ...stepSnapshot(other.id, 1, o, 'keyvalue') })),
    ],
  });
  await emitPrompts(page, [routed('proof', SESSION_ID, { title: 'Proof ready: Checkout fix' })]);
  await expect(
    page.locator('[data-testid="proof-ready-dialog"] [data-slot="dialog-title"]'),
  ).toHaveText('Fix checkout');
  await expect(page.locator('[data-testid="proof-ready-proof"]')).toHaveText('Checkout fix');
  await page.locator('[data-testid="proof-ready-open"]').click();
  await expect
    .poll(() =>
      control
        .log()
        .filter((c) => c.channel === IPC.proofOpenWindow)
        .map((c) => c.args),
    )
    .toEqual([{ sessionId: SESSION_ID }]);
  await expect(page.locator('[data-testid="proof-ready-dialog"]')).toHaveCount(0);

  await openProofWindow(page, uiServer.url);
  await expect(page.locator('[data-testid="proof-title"]')).toHaveText('Checkout fix');
  await emitWailsEvent(page, CHANNEL.proofSelect, { sessionId: other.id });
  await expect(page.locator('[data-testid="proof-title"]')).toHaveText('Second run');
});

test('a ready prompt for a session with no task is titled by the entry', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.proofSessions, response: sessions() }],
  });
  await emitPrompts(page, [routed('proof', SESSION_ID, { title: 'Proof ready: Checkout fix' })]);
  await expect(page.locator('[data-testid="proof-ready-dialog"]')).toContainText(
    'Proof ready: Checkout fix',
  );
  await expect(page.locator('[data-testid="proof-ready-proof"]')).toHaveCount(0);
});
