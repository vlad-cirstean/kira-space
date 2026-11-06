import type { Locator, Page } from '@playwright/test';
import { expect } from '../fixtures';

// P107 I2-25: modeTab/openHttpModeAndNewRequest/grpcTab/httpResponse were copied, byte-identically
// or nearly so, across 15/9/2/3 spec files respectively. One copy here, imported everywhere.

export function modeTab(page: Page, mode: 'studio' | 'api' | 'automations' | 'docker'): Locator {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

export async function openHttpModeAndNewRequest(page: Page): Promise<void> {
  await modeTab(page, 'api').click();
  await expect(page.locator('[data-testid="api-start"]')).toBeVisible();
  await page.click('[data-testid="new-request-start"]');
}

/** `base` overrides the default state fields (before `state` itself is applied) — grpc-
 *  request.spec.ts's own tests rely on `responsePane: 'messages'` as the default pane (its own
 *  comment: "Both calls happen with the Messages pane showing (the default)"), diverging from
 *  every other caller's `'history'`; that file supplies its own thin local wrapper passing
 *  `{ responsePane: 'messages' }` as `base` rather than changing the shared default. */
export function grpcTab(
  state: Record<string, unknown> = {},
  base: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    id: 'tab-grpc-1',
    connectionId: null,
    path: 'request',
    kind: 'grpc-request',
    order: 0,
    active: true,
    state: {
      target: '',
      tlsMode: 'tls',
      caFile: '',
      serverName: '',
      descriptorMode: 'reflection',
      protoPath: '',
      importPaths: [],
      service: '',
      method: '',
      message: '',
      metadata: [],
      itemId: null,
      name: '',
      requestPane: 'message',
      responsePane: 'history',
      requestPaneHeight: 0,
      ...base,
      ...state,
    },
  };
}

/** `overrides` wins over `base` — pass a file's own `base` for a response fixture whose fields a
 *  test relies on without overriding every one of them explicitly (http-raw.spec.ts's exact-
 *  fidelity captures, http-timeline.spec.ts's timeline entries); omit it for the plain default. */
export function httpResponse(
  overrides: Record<string, unknown> = {},
  base: Record<string, unknown> = {
    status: 200,
    statusText: 'OK',
    proto: 'HTTP/1.1',
    headers: [],
    body: '',
    bodyEncoding: 'utf8',
    bodyBytes: 0,
    bodyTruncated: false,
    elapsedMs: 5,
    finalUrl: 'https://api.example.com/v1',
    redirects: [],
  },
): Record<string, unknown> {
  return { ...base, ...overrides };
}

export async function openHttpMode(page: Page): Promise<void> {
  await modeTab(page, 'api').click();
}

export async function openHttpRequestView(page: Page): Promise<void> {
  await openHttpModeAndNewRequest(page);
  await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();
}
