import { expect, test } from './fixtures';
import { adeFixture, openPlan, openSessions, openWorkflows, saved } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// ADE ids are UUIDv7: a session shows the last 4 characters of its id (the head is a timestamp),
// and a new workflow step gets a prefixed handle. Contract ade-ids.
// Backend half: adeflow TestAdeIDs.

const t = (id: string) => `[data-testid="${id}"]`;

interface SessionsFx {
  sessions: Record<string, unknown>[];
}

test('the contract board with UUID task keys renders', async ({ relaunch }) => {
  const board = contract<{ tasks: { id: string }[] }>('ade-ids', 'AdeTaskService.Board');
  expect(board.tasks.length).toBeGreaterThan(0);
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
  ]);
  await expect(page.locator(t('ade-plan'))).toBeVisible();
});

test('two v7 sessions sharing a timestamp head show distinct short ids', async ({ relaunch }) => {
  const base = adeFixture<SessionsFx>('sessions').sessions.find((s) => s.taskId === 'T_bill');
  const head = '0192f0a8-1b2c-7000-8000-';
  const sessions = ['00000000aaaa', '00000000bbbb'].map((tail) => ({
    ...base,
    id: head + tail,
    agentSessionId: head + tail,
    state: 'running',
  }));
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: { sessions } },
  ]);
  await openSessions(page, 'T_bill');
  await page.locator(t('ade-sessions-menu-trigger')).click();
  const rows = page.locator(`${t('ade-sessions-menu-popover')} ${t('ade-sessions-row')}`);
  await expect(rows).toHaveCount(2);
  await expect(rows.nth(0)).toContainText('aaaa');
  await expect(rows.nth(1)).toContainText('bbbb');
  await expect(rows.nth(0)).not.toContainText('0192');
});

test('Add step saves a stp_ id and keeps existing step ids', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  const out = await saved<{ workflow: { steps: { id: string }[] } }>(page, control, 1);
  const ids = out.workflow.steps.map((s) => s.id);
  expect(ids.at(-1)).toMatch(/^stp_[0-9a-hjkmnp-tv-z]{12}$/);
  expect(ids).toContain('plan');
});
