import { expect, type Page } from '@playwright/test';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<'linkOpenExternal' | 'terminal' | 'terminalOpen'>;

export interface Options {
  extraBase: readonly ControlSnapshot[];
  /** Opens a new terminal tab and leaves it active. */
  openTerminal: (page: Page) => Promise<void>;
}

// Cmd/Ctrl-click opens a terminal link through LinkService.OpenExternal. Backend half: appflow
// TestTerminalLinkOpen, whose contract terminal-link is the call these specs expect.
// The UI harness runs a macOS user agent, so the primary modifier is Meta.
export function registerTerminalLinkScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;
  const { url } = contract<{ url: string }>('terminal-link', 'args:LinkService.OpenExternal');

  const BASE: ControlSnapshot[] = [
    ...options.extraBase,
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
    { channel: IPC.linkOpenExternal, response: null },
  ];

  async function printLine(page: Page, line: string): Promise<void> {
    await options.openTerminal(page);
    await expect(page.locator('.xterm-rows')).toBeVisible();
    const terminalId = await page
      .locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-active="true"]')
      .getAttribute('data-tab-id');
    expect(terminalId).not.toBeNull();
    await emitWailsEvent(page, IPC.terminal, {
      terminalId,
      data: Buffer.from(line).toString('base64'),
      exited: false,
    });
  }

  const opened = (control: { log(): { channel: string; args?: unknown }[] }) =>
    control.log().filter((e) => e.channel === IPC.linkOpenExternal);
  const hint = (page: Page) => page.locator('[data-testid="terminal-link-hint"]');

  test('contract: plain-text link opens only on Cmd/Ctrl-click, with a hover hint', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await printLine(page, `${url}\r\n`);
    const row = page.locator('.xterm-rows > div', { hasText: url });
    await expect(row).toBeVisible();

    await row.hover({ position: { x: 20, y: 6 }, force: true });
    await expect(hint(page)).toBeVisible();
    await expect(hint(page)).toContainText(url);
    await expect(hint(page)).toContainText('-click to open');

    await row.click({ position: { x: 20, y: 6 }, force: true });
    await row.click({ position: { x: 20, y: 6 }, modifiers: ['Meta'], force: true });
    await expect.poll(() => opened(control)).toHaveLength(1);
    expect(opened(control)[0]?.args).toEqual({ url });
    await page.locator('.xterm-rows').hover({ position: { x: 300, y: 200 }, force: true });
    await expect(hint(page)).toBeHidden();
  });

  test('contract: OSC 8 hyperlink opens its target on Cmd/Ctrl-click', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await printLine(page, `\x1b]8;;${url}\x07open PR\x1b]8;;\x07\r\n`);
    const row = page.locator('.xterm-rows > div', { hasText: 'open PR' });
    await expect(row).toBeVisible();

    await row.hover({ position: { x: 20, y: 6 }, force: true });
    await expect(hint(page)).toContainText(url);

    await row.click({ position: { x: 20, y: 6 }, modifiers: ['Meta'], force: true });
    await expect.poll(() => opened(control)).toHaveLength(1);
    expect(opened(control)[0]?.args).toEqual({ url });
  });

  test('contract: hint of a link on the last row stays inside the terminal', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: BASE });
    await printLine(page, `${'filler\r\n'.repeat(80)}${url}`);
    const row = page.locator('.xterm-rows > div', { hasText: url });
    await expect(row).toBeVisible();

    await row.hover({ position: { x: 20, y: 6 }, force: true });
    await expect(hint(page)).toBeVisible();
    const screen = await page.locator('.xterm-screen').boundingBox();
    const box = await hint(page).boundingBox();
    expect(box && screen && box.y + box.height <= screen.y + screen.height).toBe(true);
  });

  // Inline stub: the package cannot import the app's support helpers.
  async function stubClipboard(page: Page): Promise<() => Promise<string[]>> {
    await page.evaluate(() => {
      const w = window as unknown as { __clipboard: string[] };
      w.__clipboard = [];
      navigator.clipboard.writeText = async (text: string) => {
        w.__clipboard.push(text);
      };
    });
    return () => page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard);
  }

  for (const [name, printed, label] of [
    ['plain-text link', `${url}\r\n`, url],
    ['OSC 8 hyperlink', `\x1b]8;;${url}\x07open PR\x1b]8;;\x07\r\n`, 'open PR'],
  ] as const) {
    test(`contract: right-click on a ${name} offers Open and Copy link`, async ({ relaunch }) => {
      const { window: page, control } = await relaunch({ control: BASE });
      await printLine(page, printed);
      const row = page.locator('.xterm-rows > div', { hasText: label });
      await expect(row).toBeVisible();
      const clipboard = await stubClipboard(page);

      await row.hover({ position: { x: 20, y: 6 }, force: true });
      await expect(hint(page)).toBeVisible();
      await row.click({ position: { x: 20, y: 6 }, button: 'right', force: true });
      await expect(page.locator('[data-testid="context-menu"]')).toBeVisible();
      expect(opened(control)).toHaveLength(0);

      await page.locator('[data-testid="menu-item-copy-link"]').click();
      await expect.poll(clipboard).toEqual([url]);
      await expect(page.locator('[data-testid="context-menu"]')).toBeHidden();

      // xterm skips a re-hover of the cell it last saw, so move to another cell of the link.
      await row.hover({ position: { x: 36, y: 6 }, force: true });
      await expect(hint(page)).toBeVisible();
      await row.click({ position: { x: 36, y: 6 }, button: 'right', force: true });
      await page.locator('[data-testid="menu-item-open-link"]').click();
      await expect.poll(() => opened(control)).toHaveLength(1);
      expect(opened(control)[0]?.args).toEqual({ url });
    });
  }
}
