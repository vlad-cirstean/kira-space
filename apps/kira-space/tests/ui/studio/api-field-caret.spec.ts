import type { Locator, Page } from '@playwright/test';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, apiTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { grpcTab, openHttpModeAndNewRequest } from './support/apiMode';

// Ranges-only AutocompleteField fields (`{{variable}}` tint, no language): the native
// <input>/<textarea> paints glyphs and caret in one layout, the overlay only tints behind it. These
// specs prove that by construction (computed styles) and by walking the real caret. They do not
// reproduce the platform-specific drift that motivated the design: that needs the shipped engine.

const SINGLE = 'https://{{host}}/a  b {{x}} c';
const MULTI = 'line one {{x}}\nline  two with  spaces\nthree';

interface Kind {
  name: string;
  value: string;
  open: (page: Page, value: string) => Promise<Locator>;
}

async function headerValue(page: Page, value: string): Promise<Locator> {
  await page.click('[data-testid="http-request-pane-headers"]');
  const row = page.locator('[data-testid="http-header-row"]').first();
  await row.locator('[data-testid="http-header-name"]').fill('X-A');
  const field = row.locator('[data-testid="http-header-value"]');
  await field.fill(value);
  return field;
}

const KINDS: Kind[] = [
  {
    name: 'http-url',
    value: SINGLE,
    open: async (page, value) => {
      const f = page.locator('[data-testid="http-url"]');
      await f.fill(value);
      return f;
    },
  },
  {
    name: 'http-header-name',
    value: 'X-{{x}}-Long Name',
    open: async (page, value) => {
      await page.click('[data-testid="http-request-pane-headers"]');
      const f = page.locator('[data-testid="http-header-name"]').first();
      await f.fill(value);
      return f;
    },
  },
  { name: 'http-header-value', value: MULTI, open: headerValue },
  {
    name: 'http-param-value',
    value: 'one{{x}}two',
    open: async (page) => {
      await page.fill('[data-testid="http-url"]', 'https://h.io/a?k=one{{x}}two');
      return page.locator('[data-testid="http-param-value"]').first();
    },
  },
  {
    name: 'http-urlencoded-value',
    value: MULTI,
    open: async (page, value) => {
      await page.click('[data-testid="http-request-pane-body"]');
      await page.click('[data-testid="http-body-mode-urlencoded"]');
      const f = page.locator('[data-testid="http-urlencoded-value"]').first();
      await f.fill(value);
      return f;
    },
  },
  {
    name: 'http-formdata-value',
    value: MULTI,
    open: async (page, value) => {
      await page.click('[data-testid="http-request-pane-body"]');
      await page.click('[data-testid="http-body-mode-formdata"]');
      const f = page.locator('[data-testid="http-formdata-value"]').first();
      await f.fill(value);
      return f;
    },
  },
];

const GRPC_CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.tabsList,
    response: [
      grpcTab({
        target: SINGLE,
        requestPane: 'metadata',
        metadata: [{ name: 'x-a', value: MULTI, enabled: true }],
      }),
    ],
  },
];

async function caretWalk(page: Page, field: Locator): Promise<void> {
  const expected = await field.inputValue();
  await field.focus();
  await field.evaluate((el: HTMLInputElement | HTMLTextAreaElement) => el.setSelectionRange(0, 0));
  const sel = () => field.evaluate((el: HTMLInputElement) => el.selectionStart);
  for (let i = 0; i < expected.length; i++) {
    await page.keyboard.press('ArrowRight');
    expect(await sel()).toBe(i + 1);
  }
  for (let i = expected.length; i > 0; i--) {
    await page.keyboard.press('ArrowLeft');
    expect(await sel()).toBe(i - 1);
  }
  await expect(field).toHaveValue(expected);
}

async function ownership(field: Locator): Promise<void> {
  const m = await field.evaluate((el) => {
    const wrap = el.parentElement as HTMLElement;
    const overlay = wrap.querySelector('.highlight-overlay') as HTMLElement;
    const unknown = overlay.querySelector('.kira-ed-var-unknown') as HTMLElement;
    const own = getComputedStyle(el);
    const ov = getComputedStyle(overlay);
    const span = getComputedStyle(unknown);
    return {
      controlColor: own.color,
      controlZ: own.zIndex,
      overlayIsUnderlay: overlay.classList.contains('is-underlay'),
      overlayEvents: ov.pointerEvents,
      overlayColor: ov.color,
      spanColor: span.color,
      spanBg: span.backgroundColor,
      overlayText: overlay.textContent,
      value: (el as HTMLInputElement).value,
    };
  });
  expect(m.controlColor).not.toBe('rgba(0, 0, 0, 0)');
  expect(m.controlZ).toBe('1');
  expect(m.overlayIsUnderlay).toBe(true);
  expect(m.overlayEvents).toBe('none');
  expect(m.overlayColor).toBe('rgba(0, 0, 0, 0)');
  expect(m.spanColor).toBe('rgba(0, 0, 0, 0)');
  expect(m.spanBg).not.toBe('rgba(0, 0, 0, 0)');
  expect(m.overlayText).toBe(m.value.replace(/\r\n?/g, '\n'));
}

for (const kind of KINDS) {
  test(`${kind.name}: native control owns glyphs and caret`, async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await openHttpModeAndNewRequest(page);
    const field = await kind.open(page, kind.value);
    await field.focus();
    await ownership(field);
    await caretWalk(page, field);
  });
}

test('grpc-target and grpc-metadata-value: native control owns glyphs and caret', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: GRPC_CONTROL });
  for (const id of ['grpc-target', 'grpc-metadata-value']) {
    const field = page.locator(`[data-testid="${id}"]`).first();
    await expect(field).toBeVisible();
    await field.focus();
    await ownership(field);
    await caretWalk(page, field);
  }
});

test('multi-line cell keeps native Up/Down; one-line cell still steps rows', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: [] });
  await openHttpModeAndNewRequest(page);
  const field = await headerValue(page, 'a\nb');
  await field.focus();
  await field.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(3, 3));
  await page.keyboard.press('ArrowUp');
  await expect(field).toBeFocused();
  expect(await field.evaluate((el: HTMLTextAreaElement) => el.selectionStart)).toBeLessThanOrEqual(
    1,
  );

  await field.fill('one line');
  await field.focus();
  await page.keyboard.press('ArrowDown');
  const second = page.locator('[data-testid="http-header-row"]').nth(1);
  await expect(second.locator('[data-testid="http-header-value"]')).toBeFocused();
});

test('a lone CR in a param value grows the box to two lines', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: [] });
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://x.io/a?k=one%20{{x}}%0Dtwo%20words');
  const value = page.locator('[data-testid="http-param-value"]').first();
  await expect(value).toBeVisible();
  const fits = await value.evaluate((el) => el.scrollHeight <= el.clientHeight);
  expect(fits).toBe(true);
  expect(await value.evaluate((el) => el.clientHeight)).toBeGreaterThan(30);
});
