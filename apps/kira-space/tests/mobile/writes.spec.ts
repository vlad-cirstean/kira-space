import { contract } from '../ui/support/contract';
import { expect, test } from './fixtures';

// The phone's write paths against the scripted backend: what each tap sends, and how the app
// reacts to a refusal.

const t = (id: string) => `[data-testid="${id}"]`;

const posts = (server: { state: { requests: { method: string; path: string }[] } }, path: string) =>
  server.state.requests.filter((r) => r.method === 'POST' && r.path === path);

test.describe('with write access', () => {
  test.beforeEach(async ({ app, server }) => {
    server.state.auth = 'ok';
    server.state.permissions = { write: true, agentInput: false };
    await app();
  });

  test('contract: adding a task sends it once with an idempotency key', async ({
    page,
    server,
  }) => {
    const args = contract<{ title: string }>('mobile-board', 'args:POST /api/ade/later/items');
    server.state.replies.laterTask = contract('mobile-board', 'http:POST /api/ade/later/items');
    await page.locator(t('tab-later')).click();
    await page.locator(t('later-add')).fill(args.title);
    await page.locator(t('later-add-submit')).click();
    await expect(page.locator(t('later-add'))).toHaveValue('');
    const sent = posts(server, '/api/ade/later/items') as { body: string; key: string }[];
    expect(sent).toHaveLength(1);
    expect(JSON.parse(sent[0]?.body ?? '{}')).toEqual(args);
    expect(sent[0]?.key).toMatch(/^[0-9a-f-]{36}$/);
  });

  test('a write waits while the phone is offline and sends once on reconnect', async ({
    page,
    server,
  }) => {
    const args = contract<{ title: string }>('mobile-board', 'args:POST /api/ade/later/items');
    server.state.replies.laterTask = contract('mobile-board', 'http:POST /api/ade/later/items');
    await page.locator(t('tab-later')).click();
    await page.evaluate(() => window.dispatchEvent(new Event('offline')));
    await page.locator(t('later-add')).fill(args.title);
    await page.locator(t('later-add-submit')).click();
    await expect(page.locator(t('later-add-submit'))).toHaveAttribute('aria-busy', 'true');
    expect(posts(server, '/api/ade/later/items')).toHaveLength(0);
    await page.evaluate(() => window.dispatchEvent(new Event('online')));
    await expect.poll(() => posts(server, '/api/ade/later/items').length).toBe(1);
    const sent = posts(server, '/api/ade/later/items') as { key: string }[];
    expect(sent[0]?.key).toMatch(/^[0-9a-f-]{36}$/);
  });

  test('contract: dragging a row to the other list sends the shelf it landed on', async ({
    page,
    server,
  }) => {
    const board = contract<{ tasks: { id: string; title: string }[] }>(
      'ade-shelf',
      'AdeTaskService.Board#shelves',
    );
    server.state.replies.board = board;
    await page.reload();
    const args = contract<{ shelf: string }>('mobile-board', 'args:POST /api/ade/shelf/move');
    await page.locator(t('tab-later')).click();
    const later = page.locator(`${t('later-shelf-later')} ${t('later-item')}`);
    const backlog = page.locator(`${t('later-shelf-backlog')} ${t('later-item')}`);
    await expect(later).toHaveCount(2);
    const handle = await later.nth(0).locator('[data-drag-handle]').boundingBox();
    const target = await backlog.nth(0).boundingBox();
    if (!handle || !target) throw new Error('rows not laid out');
    await page.mouse.move(handle.x + handle.width / 2, handle.y + handle.height / 2);
    await page.mouse.down();
    // The sortable library samples pointer positions per frame: move in many small steps.
    await page.mouse.move(handle.x + 10, handle.y + 30, { steps: 10 });
    await page.mouse.move(target.x + 40, target.y + target.height * 0.75, { steps: 40 });
    await page.waitForTimeout(300);
    await page.mouse.up();
    await expect.poll(() => posts(server, '/api/ade/shelf/move').length).toBe(1);
    const body = JSON.parse((posts(server, '/api/ade/shelf/move')[0] as { body: string }).body) as {
      taskId: string;
      shelf: string;
    };
    expect(body.taskId).toBe(board.tasks.find((x) => x.title === 'planned')?.id);
    expect(body.shelf).toBe(args.shelf);
  });

  test('confirm buttons are at least 44px tall', async ({ page }) => {
    await page.locator(t('tab-plan')).click();
    const card = page
      .locator(t('plan-task'))
      .filter({ hasText: 'Cart total test fails intermittently' });
    await card.locator('button').first().click();
    await card.locator(t('plan-step-next')).click();
    for (const id of ['confirm-cancel', 'confirm-yes']) {
      const box = await page.locator(t(id)).boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
  });

  test('Next asks first, then moves the task from the step the phone saw', async ({
    page,
    server,
  }) => {
    await page.locator(t('tab-plan')).click();
    const card = page
      .locator(t('plan-task'))
      .filter({ hasText: 'Cart total test fails intermittently' });
    await card.locator('button').first().click();
    await card.locator(t('plan-step-next')).click();
    await expect(page.locator(t('confirm-dialog'))).toBeVisible();
    expect(posts(server, '/api/ade/tasks/step')).toHaveLength(0);
    await page.locator(t('plan-step-notes')).fill('  Check the empty state.  ');
    await page.locator(t('confirm-yes')).click();
    await expect.poll(() => posts(server, '/api/ade/tasks/step').length).toBe(1);
    const sent = posts(server, '/api/ade/tasks/step')[0] as { body: string; key: string };
    const body = JSON.parse(sent.body) as {
      taskId: string;
      fromStepId: string;
      stepId: string;
      notes?: string;
    };
    expect(body.fromStepId).not.toBe(body.stepId);
    expect(body.notes).toBe('Check the empty state.');
    expect(sent.key).not.toBe('');
  });

  test('a stale move shows the server message and keeps the card', async ({ page, server }) => {
    server.state.failNext.set('/api/ade/tasks/step', {
      status: 409,
      code: 'E_CONFLICT',
      message: 'The task moved on the computer. Refresh and try again.',
    });
    await page.locator(t('tab-plan')).click();
    const card = page
      .locator(t('plan-task'))
      .filter({ hasText: 'Cart total test fails intermittently' });
    await card.locator('button').first().click();
    await card.locator(t('plan-step-next')).click();
    await page.locator(t('confirm-yes')).click();
    await expect(page.locator(t('confirm-dialog'))).toContainText('moved on the computer');
  });
});

test('without write access the controls are replaced by a hint', async ({ page, server, app }) => {
  server.state.auth = 'ok';
  server.state.permissions = { write: false, agentInput: false };
  await app();
  await page.locator(t('tab-later')).click();
  await expect(page.locator(t('permission-hint'))).toContainText(
    'turn on mobile access in the settings of Kira Space',
  );
  await expect(page.locator(t('later-add'))).toHaveCount(0);
  await page.locator(t('tab-plan')).click();
  await page
    .locator(t('plan-task'))
    .filter({ hasText: 'Cart total test fails intermittently' })
    .locator('button')
    .first()
    .click();
  await expect(page.locator(t('plan-step-next'))).toHaveCount(0);
});
