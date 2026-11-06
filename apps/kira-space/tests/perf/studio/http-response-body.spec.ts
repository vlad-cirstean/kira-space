import { RssSampler, startFrames, stopFrames } from '@workbench/testing/ui/perfProbe';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { httpResponse, openHttpModeAndNewRequest } from '../../ui/studio/support/apiMode';
import { IPC } from '../../ui/support/ipcChannels';

// HTTP response viewer on a large body. BODYKIND: json (default) | text-80col | text-short-lines |
// text-1line; NITEMS sizes the json body (100000 is about 12 MB). Reports time to first text, the
// longest main-thread stall and the RSS peak on receive: the expensive part is the first render.

const KIND = process.env.BODYKIND ?? 'json';
const NITEMS = Number(process.env.NITEMS ?? 20_000);
const TEXT_BYTES = 12_000_000;
const LINE = 'lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor\n';

function makeBody(): { body: string; contentType: string; needle: string } {
  switch (KIND) {
    case 'text-80col':
      return {
        body: LINE.repeat(TEXT_BYTES / LINE.length),
        contentType: 'text/plain',
        needle: 'lorem',
      };
    case 'text-short-lines':
      return {
        body: 'row 1234567 abc\n'.repeat(TEXT_BYTES / 16),
        contentType: 'text/plain',
        needle: 'row',
      };
    case 'text-1line':
      return { body: 'x'.repeat(TEXT_BYTES), contentType: 'text/plain', needle: 'xxxx' };
    default: {
      const items = Array.from({ length: NITEMS }, (_, i) => ({
        id: i,
        name: `item-${i}`,
        tags: ['a', 'b', 'c'],
        nested: { x: i, y: 'lorem ipsum dolor sit amet', z: [1, 2, 3] },
      }));
      return { body: JSON.stringify(items), contentType: 'application/json', needle: '"id"' };
    }
  }
}

test('perf: http response viewer, large body', async ({ relaunch }) => {
  test.setTimeout(240_000);
  const { body, contentType, needle } = makeBody();
  const response = httpResponse({
    body,
    bodyEncoding: 'utf8',
    bodyBytes: body.length,
    bodyTruncated: false,
    headers: [{ name: 'Content-Type', value: contentType }],
  });
  const control: ControlSnapshot[] = [{ channel: IPC.httpSend, response }];
  const { window: page } = await relaunch({ control });
  await openHttpModeAndNewRequest(page);
  await page.fill('[data-testid="http-url"]', 'https://api.example.com/big');

  const rss = new RssSampler();
  rss.start();
  await page.waitForTimeout(1500);
  rss.phase('send');
  await startFrames(page);
  const started = Date.now();
  await page.click('[data-testid="http-send"]');
  await expect(page.locator('[data-testid="http-status"]')).toContainText('200', {
    timeout: 120_000,
  });
  await expect(
    page.locator('[data-testid="http-response-pane"] .view-lines').first(),
  ).toContainText(needle, { timeout: 120_000 });
  const shownMs = Date.now() - started;
  const frames = await stopFrames(page);
  await page.waitForTimeout(2000);
  const stats = rss.stats('send');
  rss.stop();
  console.log(
    `PERF http ${KIND} ${(body.length / 1e6).toFixed(1)} MB: text visible ${shownMs} ms | ` +
      `longest main-thread stall ${frames.maxMs.toFixed(0)} ms | ` +
      `RSS ${stats.startMb.toFixed(0)} -> peak ${stats.peakMb.toFixed(0)} MB (+${stats.riseMb.toFixed(0)}), ` +
      `kept ${stats.endMb.toFixed(0)} MB`,
  );
});
