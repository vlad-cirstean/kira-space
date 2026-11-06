import { expect, type Page } from '@playwright/test';

// P107 I2-25: installClipboardSpy/lastClipboardWrite were copied, byte-identically, across 3
// spec files. One copy here, imported everywhere.

/**
 * `lazy` also captures `clipboard.write` of ClipboardItems (console "Copy all"). Off by default:
 * Monaco's own copy calls `write` too and fails on the stub. `image` captures `image/png` items
 * as blobs in `window.__clipboardImages` instead (the proof window's screenshot copy).
 */
export async function installClipboardSpy(
  page: Page,
  opts: { lazy?: boolean; image?: boolean } = {},
): Promise<void> {
  await page.evaluate(
    ({ lazy, image }) => {
      const w = window as unknown as { __clipboard: string[]; __clipboardImages: Blob[] };
      w.__clipboard = [];
      w.__clipboardImages = [];
      navigator.clipboard.writeText = (text: string) => {
        w.__clipboard.push(text);
        return Promise.resolve();
      };
      if (lazy) {
        navigator.clipboard.write = async (items: ClipboardItem[]) => {
          for (const item of items) {
            const blob = await item.getType('text/plain');
            w.__clipboard.push(await blob.text());
          }
        };
      }
      if (image) {
        navigator.clipboard.write = async (items: ClipboardItem[]) => {
          for (const item of items) w.__clipboardImages.push(await item.getType('image/png'));
        };
      }
    },
    { lazy: opts.lazy ?? false, image: opts.image ?? false },
  );
}

export async function lastClipboardWrite(page: Page): Promise<string> {
  return page.evaluate(
    () => (window as unknown as { __clipboard: string[] }).__clipboard.at(-1) ?? '',
  );
}

/** Replaces `navigator.clipboard` with an in-memory stub (WebKit refuses an ungestured
 *  `readText()` even with the permission granted). Install before the page loads. */
export const CLIPBOARD_SHIM = `(() => {
  let text = '';
  const clip = {
    writeText: (t) => { text = String(t); return Promise.resolve(); },
    readText: () => Promise.resolve(text),
  };
  Object.defineProperty(navigator, 'clipboard', { value: clip, configurable: true });
})();`;

export async function installClipboardShim(page: Page): Promise<void> {
  await page.addInitScript(CLIPBOARD_SHIM);
  await page.reload();
  await page.waitForSelector('[data-testid="status-bar"]');
}

// Real-clipboard coverage (grid-copy-menu.spec.ts). Chromium reads the OS clipboard; WebKit refuses
// an ungestured `readText()`, so there a pass-through recorder wraps the real `writeText` and logs
// each write only once its promise settles. A rejection fails the read, keeping WebKit's gesture
// rule under test.
const SENTINEL = '__kira-clipboard-sentinel__';

type RealClipboardLog = { entries: { ok?: string; err?: string }[]; read: number };

export async function prepareRealClipboard(page: Page, browserName: string): Promise<void> {
  if (browserName === 'chromium') {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    return;
  }
  await page.addInitScript(() => {
    const log: RealClipboardLog = { entries: [], read: 0 };
    (window as unknown as { __realClipboard: RealClipboardLog }).__realClipboard = log;
    const clip = navigator.clipboard;
    const orig = clip.writeText.bind(clip);
    clip.writeText = async (text: string) => {
      try {
        await orig(text);
        log.entries.push({ ok: text });
      } catch (err) {
        log.entries.push({ err: String(err) });
        throw err;
      }
    };
  });
}

/** Chromium only: blank the clipboard so a menu item that writes nothing cannot pass on stale text. */
export async function clipboardReset(page: Page, browserName: string): Promise<void> {
  if (browserName !== 'chromium') return;
  await page.evaluate((s) => navigator.clipboard.writeText(s), SENTINEL);
}

/** The next clipboard write, waiting for it to land. Throws on a rejected WebKit write. */
export async function clipboardRead(page: Page, browserName: string): Promise<string> {
  if (browserName === 'chromium') {
    let text = SENTINEL;
    await expect
      .poll(
        async () => {
          text = await page.evaluate(() => navigator.clipboard.readText());
          return text;
        },
        { timeout: 10_000, message: 'clipboard was never written' },
      )
      .not.toBe(SENTINEL);
    return text;
  }
  const handle = await page.waitForFunction(() => {
    const log = (window as unknown as { __realClipboard: RealClipboardLog }).__realClipboard;
    const entry = log.entries[log.read];
    if (!entry) return null;
    log.read++;
    return entry;
  });
  const entry = (await handle.jsonValue()) as { ok?: string; err?: string };
  if (entry.err !== undefined) throw new Error(`clipboard write rejected: ${entry.err}`);
  return entry.ok ?? '';
}
