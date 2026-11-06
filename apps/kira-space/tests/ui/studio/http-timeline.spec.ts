import type { Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { httpResponse as buildHttpResponse, openHttpModeAndNewRequest } from './support/apiMode';
import { editorText } from './support/editorText';

// The real built bundle, real WebKit, both wire planes mocked — the waterfall,
// the SPEC's own "reused, not zero" requirement, the two jump affordances, a stored
// entry's real timeline vs. Raw's own empty state, and a failed send's partial timeline.

// This file's own fixture defaults diverge from the shared canonical shape (below) — passed as
// `base` rather than changed on the shared default (http-raw.spec.ts's own reasoning applies).
function httpResponse(overrides: Record<string, unknown>): Record<string, unknown> {
  return buildHttpResponse(overrides, {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [{ name: 'Content-Type', value: 'application/json' }],
    body: '{"id":1}',
    bodyEncoding: 'utf8',
    bodyBytes: 8,
    bodyTruncated: false,
    elapsedMs: 45,
    finalUrl: 'https://api.example.com/final',
    redirects: [],
  });
}

// F1's own shape: a same-host 301->302->307->200 chain, hop 0 fresh, hops 1-3 reused — exactly
// the case the SPEC's "shown as reused, not as an instant/zero one" wording exists to explain.
const TIMELINE_4HOPS = {
  totalMs: 45.2,
  hops: [
    {
      index: 0,
      method: 'GET',
      url: 'https://api.example.com/start',
      status: 301,
      statusText: 'Moved Permanently',
      proto: 'HTTP/1.1',
      reused: false,
      connAttempts: 1,
      startOffsetMs: 0,
      totalMs: 10.2,
      dns: { startOffsetMs: 0, durationMs: 2.1 },
      connect: { startOffsetMs: 2.1, durationMs: 0.8 },
      tls: { startOffsetMs: 2.9, durationMs: 5.5 },
      wait: { startOffsetMs: 8.4, durationMs: 1.5 },
      download: { startOffsetMs: 9.9, durationMs: 0.3 },
      headers: [{ name: 'Location', value: 'https://api.example.com/mid' }],
    },
    {
      index: 1,
      method: 'GET',
      url: 'https://api.example.com/mid',
      status: 302,
      statusText: 'Found',
      proto: 'HTTP/1.1',
      reused: true,
      idleMs: 0.13,
      connAttempts: 1,
      startOffsetMs: 10.2,
      totalMs: 5.0,
      wait: { startOffsetMs: 10.2, durationMs: 4.5 },
      download: { startOffsetMs: 14.7, durationMs: 0.5 },
      headers: [{ name: 'Location', value: 'https://api.example.com/next' }],
    },
    {
      index: 2,
      method: 'GET',
      url: 'https://api.example.com/next',
      status: 307,
      statusText: 'Temporary Redirect',
      proto: 'HTTP/1.1',
      reused: true,
      idleMs: 0.2,
      connAttempts: 1,
      startOffsetMs: 15.2,
      totalMs: 5.0,
      wait: { startOffsetMs: 15.2, durationMs: 4.5 },
      download: { startOffsetMs: 19.7, durationMs: 0.5 },
      headers: [{ name: 'Location', value: 'https://api.example.com/final' }],
    },
    {
      index: 3,
      method: 'GET',
      url: 'https://api.example.com/final',
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      reused: true,
      idleMs: 0.11,
      connAttempts: 1,
      startOffsetMs: 20.2,
      totalMs: 25.0,
      wait: { startOffsetMs: 20.2, durationMs: 24.0 },
      download: { startOffsetMs: 44.2, durationMs: 1.0 },
    },
  ],
};

async function sendRedirectChain(page: Page): Promise<void> {
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/start');
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
}

test('Http timeline — the waterfall', async ({ relaunch }) => {
  const RESPONSE = httpResponse({
    redirects: [
      { status: 301, url: 'https://api.example.com/start' },
      { status: 302, url: 'https://api.example.com/mid' },
      { status: 307, url: 'https://api.example.com/next' },
    ],
    timeline: TIMELINE_4HOPS,
  });
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await sendRedirectChain(page);
  await page.click('[data-testid="http-response-pane-timeline"]');

  const hops = page.locator('[data-testid="http-timeline-hop"]');
  await expect(hops).toHaveCount(4);

  const hop0Caption = hops.nth(0).locator('[data-testid="http-timeline-hop-caption"]');
  await expect(hop0Caption).toContainText('GET');
  await expect(hop0Caption).toContainText('https://api.example.com/start');
  await expect(hop0Caption).toContainText('301');

  for (const key of ['dns', 'connect', 'tls', 'wait', 'download']) {
    const cell = hops.nth(0).locator(`[data-testid="http-timeline-phase-${key}"]`);
    await expect(cell).toBeVisible();
    await expect(cell).toContainText('ms');
  }

  await expect(page.locator('[data-testid="http-timeline-summary"]')).toContainText('4 hops');
});

test('Http timeline — a reused hop is shown as reused, not as zero', async ({ relaunch }) => {
  const RESPONSE = httpResponse({
    redirects: [
      { status: 301, url: 'https://api.example.com/start' },
      { status: 302, url: 'https://api.example.com/mid' },
      { status: 307, url: 'https://api.example.com/next' },
    ],
    timeline: TIMELINE_4HOPS,
  });
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await sendRedirectChain(page);
  await page.click('[data-testid="http-response-pane-timeline"]');

  const hop1 = page.locator('[data-testid="http-timeline-hop"]').nth(1);
  const dnsCell = hop1.locator('[data-testid="http-timeline-phase-dns"]');
  await expect(dnsCell).toContainText('DNS —');
  await expect(dnsCell).not.toContainText(/(^|\D)0ms/);
  const connectCell = hop1.locator('[data-testid="http-timeline-phase-connect"]');
  await expect(connectCell).toContainText('Connect —');
  await expect(connectCell).not.toContainText(/(^|\D)0ms/);
  const tlsCell = hop1.locator('[data-testid="http-timeline-phase-tls"]');
  await expect(tlsCell).toContainText('TLS —');
  await expect(tlsCell).not.toContainText(/(^|\D)0ms/);

  const reuseNote = hop1.locator('[data-testid="http-timeline-reuse-note"]');
  await expect(reuseNote).toBeVisible();
  await expect(reuseNote).toContainText('Reused an existing connection');
});

test('Http timeline — the elapsed figure and the redirect caption open the timeline', async ({
  relaunch,
}) => {
  const RESPONSE = httpResponse({
    redirects: [{ status: 301, url: 'https://api.example.com/start' }],
    timeline: {
      totalMs: 10,
      hops: [
        {
          index: 0,
          method: 'GET',
          url: 'https://api.example.com/start',
          status: 301,
          statusText: 'Moved Permanently',
          proto: 'HTTP/1.1',
          reused: false,
          connAttempts: 1,
          startOffsetMs: 0,
          totalMs: 5,
        },
        {
          index: 1,
          method: 'GET',
          url: 'https://api.example.com/final',
          status: 200,
          statusText: 'OK',
          proto: 'HTTP/1.1',
          reused: true,
          connAttempts: 1,
          startOffsetMs: 5,
          totalMs: 5,
        },
      ],
    },
  });
  const CONTROL: ControlSnapshot[] = [{ channel: IPC.httpSend, response: RESPONSE }];
  const { window: page } = await relaunch({ control: CONTROL });

  await sendRedirectChain(page);
  await expect(page.locator('[data-testid="http-timeline-pane"]')).toHaveCount(0);

  await page.click('[data-testid="http-elapsed"]');
  await expect(page.locator('[data-testid="http-timeline-pane"]')).toBeVisible();

  await page.click('[data-testid="http-response-pane-body"]');
  await expect(page.locator('[data-testid="http-timeline-pane"]')).toHaveCount(0);

  await page.click('[data-testid="http-redirects"]');
  await expect(page.locator('[data-testid="http-timeline-pane"]')).toBeVisible();
});

// This test used to contrast Timeline (has real data for a stored entry) against Raw
// (nothing at all, the gap) — Raw now reconstructs its two documents from the same stored
// snapshot, so the two panes are no longer "one works, one doesn't"; both do, from different
// stored fields (the timeline vs the request/response reconstruction).
test('Http timeline — a stored history entry has a real timeline, and Raw reconstructs its own view', async ({
  relaunch,
}) => {
  const ENTRY = {
    id: 'e1',
    itemId: null,
    tabId: 't1',
    sentAt: '2024-06-01T10:00:00.000Z',
    method: 'GET',
    url: 'https://api.example.com/orders',
    environment: '',
    status: 200,
    statusText: 'OK',
    elapsedMs: 40,
    bodyBytes: 22,
    storedBytes: 340,
  };
  const STORED_TIMELINE = {
    totalMs: 40,
    hops: [
      {
        index: 0,
        method: 'GET',
        url: 'https://api.example.com/orders',
        status: 200,
        statusText: 'OK',
        proto: 'HTTP/1.1',
        reused: false,
        connAttempts: 1,
        startOffsetMs: 0,
        totalMs: 40,
        dns: { startOffsetMs: 0, durationMs: 3 },
        connect: { startOffsetMs: 3, durationMs: 1 },
        wait: { startOffsetMs: 4, durationMs: 30 },
        download: { startOffsetMs: 34, durationMs: 6 },
      },
    ],
  };
  const SNAPSHOT = {
    entry: ENTRY,
    request: {
      method: 'GET',
      url: 'https://api.example.com/orders',
      headers: [],
      body: {
        mode: 'none',
        raw: '',
        code: '',
        codeLanguage: '',
        urlEncoded: [],
        formData: [],
        file: '',
      },
    },
    // No `wire` key — D7's own stripped-before-storage shape — but `timeline` present, D10's
    // whole point: the two panes behave differently for the same stored entry.
    response: httpResponse({ elapsedMs: 40, bodyBytes: 22, timeline: STORED_TIMELINE }),
    bodyStored: true,
    bodyStorageTruncated: false,
    requestBodyStorageTruncated: false,
  };
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.historyList, response: [ENTRY] },
    { channel: IPC.historyGet, response: SNAPSHOT },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.click('[data-testid="http-response-pane-history"]');
  await page.locator('[data-testid="http-history-row"]').first().click();
  await expect(page.locator('[data-testid="http-history-band"]')).toBeVisible();

  await page.click('[data-testid="http-response-pane-timeline"]');
  await expect(page.locator('[data-testid="http-timeline-stored-note"]')).toBeVisible();
  await expect(page.locator('[data-testid="http-timeline-hop"]')).toHaveCount(1);
  await expect(page.locator('[data-testid="http-timeline-phase-download"]').first()).toContainText(
    'ms',
  );

  // P18 D8: the same stored entry's Raw pane now reconstructs its own view too, from the
  // snapshot's stage-1 request/response fields rather than the timeline's own separate data.
  await page.click('[data-testid="http-response-pane-raw"]');
  await expect(page.locator('[data-testid="http-raw-reconstructed"]')).toBeVisible();
  const requestEditor = page.locator('[data-testid="http-wire-request-editor"]');
  expect(await editorText(requestEditor)).toContain('GET https://api.example.com/orders HTTP/1.1');
});

test('Http timeline — a failed send carries the timeline it got as far as', async ({
  relaunch,
}) => {
  const FAILED_TIMELINE = {
    totalMs: 5.2,
    hops: [
      {
        index: 0,
        method: 'GET',
        url: 'https://api.example.com/orders',
        status: 0,
        statusText: '',
        proto: '',
        reused: false,
        connAttempts: 1,
        startOffsetMs: 0,
        totalMs: 5.2,
        error: 'connect: connection refused',
        connect: { startOffsetMs: 0, durationMs: 5.2 },
      },
    ],
  };
  const CONTROL: ControlSnapshot[] = [
    {
      channel: IPC.httpSend,
      error: {
        code: 'E_TRANSPORT',
        message: 'connect: connection refused',
        details: FAILED_TIMELINE,
      },
    },
  ];
  const { window: page } = await relaunch({ control: CONTROL });

  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/orders');
  await page.click('[data-testid="http-send"]');

  const errorStrip = page.locator('[data-testid="http-send-error"]');
  await expect(errorStrip).toBeVisible();
  await expect(errorStrip).toContainText('connect: connection refused');

  await page.click('[data-testid="http-response-pane-timeline"]');
  const failureNote = page.locator('[data-testid="http-timeline-failure-note"]');
  await expect(failureNote).toBeVisible();
  await expect(failureNote).toContainText('The request failed during');

  const hops = page.locator('[data-testid="http-timeline-hop"]');
  await expect(hops).toHaveCount(1);
  await expect(hops.first().locator('[data-testid="http-timeline-phase-connect"]')).toContainText(
    'ms',
  );

  // Finding 9: phaseTooltip used to fall through every unhandled phase — Download included — to
  // the Wait explanation, even for a hop like this one where Download is legitimately absent
  // because the connect itself never completed.
  await hops.first().locator('[data-testid="http-timeline-phase-download"]').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText(
    'No download because no response bytes ever arrived',
  );

  // The error strip at the top of the pane is unaffected by this — the same message a failed
  // send has always shown, still shown, alongside the new detail rather than instead of it.
  await expect(errorStrip).toBeVisible();
});
