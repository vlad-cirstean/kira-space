import { devices } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { CAPTURE_NOW, installCaptureFonts } from '@workbench/testing/screenshots/capture';
import { expect, test } from '../mobile/fixtures';
import { contract } from '../ui/support/contract';
import { STORY } from './support/buildStory';
import { phoneBranchRequest, phoneReplies, routePhoneReads } from './support/phoneStory';
import { PHONE_TERMINAL_BYTES } from './support/phoneTerminal';
import { shoot } from './support/shoot';

// P734: the phone app against the scripted phone backend (tests/mobile/support/mockServer.ts),
// at iPhone 15 size. Run by `bun run screenshots`; output goes to /screenshots/phone.

test.use({ ...devices['iPhone 15'], deviceScaleFactor: 2, locale: 'en-US', timezoneId: 'UTC' });

const t = (id: string) => `[data-testid="${id}"]`;
const SESSION = 'tk01';

test.beforeEach(async ({ page, app: _app, server }) => {
  await installCaptureFonts(page);
  await routePhoneReads(page);
  server.state.auth = 'ok';
  server.state.permissions = { write: true, agentInput: true };
  server.state.agentInputGlobal = true;
  Object.assign(server.state.replies, phoneReplies());
});

const errors = (page: import('@playwright/test').Page): string[] => {
  const list: string[] = [];
  page.on('pageerror', (e) => list.push(e.message));
  page.on('console', (m) => {
    if (m.type() === 'error') list.push(m.text());
  });
  return list;
};

async function still(page: import('@playwright/test').Page): Promise<void> {
  await page.clock.setFixedTime(CAPTURE_NOW + 1000);
}

test('phone plan', async ({ app, page }) => {
  const consoleErrors = errors(page);
  await app();
  await page.locator(t('tab-plan')).click();
  await expect(page.locator(t('plan-task')).first()).toBeVisible();
  await still(page);
  await shoot(page, 'phone-plan', consoleErrors);
});

test('phone needs you', async ({ app, page, server }) => {
  const consoleErrors = errors(page);
  const request = phoneBranchRequest();
  server.state.replies.approvals = [request];
  await app();
  await expect(page.locator(t(`approval-item-${request.id}`))).toBeVisible();
  await still(page);
  await shoot(page, 'phone-needs', consoleErrors);
});

test('phone terminal', async ({ page, server }) => {
  const consoleErrors = errors(page);
  await page.routeWebSocket(/\/api\/agent\/sessions\/[^/]+\/terminal/, (ws) => {
    ws.send(JSON.stringify({ type: 'hello', offset: 0 }));
    ws.send(Buffer.from(PHONE_TERMINAL_BYTES));
    ws.onMessage(() => undefined);
  });
  await page.clock.install({ time: CAPTURE_NOW });
  await page.clock.resume();
  await page.goto(`${server.url}terminal/${SESSION}`);
  await expect(page.locator(t('term-status'))).toHaveText('Live');
  await expect(page.locator('.xterm-rows')).toContainText('and open the pull request?');
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await still(page);
  await shoot(page, 'phone-terminal', consoleErrors);
});

test('phone activity', async ({ app, page, server }) => {
  const consoleErrors = errors(page);
  const run = contract<{ runs: Record<string, unknown>[] }>(
    'mobile-status',
    'http:GET /api/runs#failed',
  ).runs[0];
  const computer = contract<{ docker: Record<string, unknown> }>(
    'mobile-status',
    'http:GET /api/computer#docker',
  );
  server.state.replies.runs = {
    runs: [
      {
        ...run,
        id: 'run-export',
        scriptName: 'export-orders',
        taskTitle: STORY.taskTitle,
        createdAt: CAPTURE_NOW - 600_000,
        startedAt: CAPTURE_NOW - 600_000,
        finishedAt: CAPTURE_NOW - 590_000,
      },
    ],
  };
  server.state.replies.computer = {
    docker: { ...computer.docker, state: 'ok', reason: '', running: 3, stopped: 1, total: 4 },
  };
  await app();
  await page.locator(t('tab-activity')).click();
  await expect(page.locator(t('phone-run'))).toHaveCount(1);
  await expect(page.locator(t('activity-docker'))).toBeVisible();
  await still(page);
  await shoot(page, 'phone-activity', consoleErrors);
});
