import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { expect, test } from './fixtures';

// Catalog wording on the phone: the take over confirm, the empty Later lists, the pairing screen.

const t = (id: string) => `[data-testid="${id}"]`;

interface BoardFx {
  tasks: { runs: { id: string; state: string }[] }[];
}

function stuckBoard(): BoardFx {
  const board = JSON.parse(
    readFileSync(resolve(__dirname, '../fixtures/ade-v2/board.json'), 'utf8'),
  ) as BoardFx;
  for (const task of board.tasks) {
    for (const run of task.runs) if (run.id.startsWith('r_T_bill_impl_')) run.state = 'stuck';
  }
  return board;
}

test('the pairing screen titles itself with the product name', async ({ page, app }) => {
  await app();
  await expect(page.locator(t('pair-screen')).getByRole('heading', { level: 1 })).toHaveText(
    'Kira Space',
  );
  await expect(page.locator(t('pair-request'))).toHaveText('Request access');
});

test('empty Later and Backlog lists say so', async ({ page, app, server }) => {
  server.state.auth = 'ok';
  const board = JSON.parse(
    readFileSync(resolve(__dirname, '../fixtures/ade-v2/board.json'), 'utf8'),
  ) as { tasks: unknown[] };
  server.state.replies.board = { ...board, tasks: [] };
  await app();
  await page.locator(t('tab-later')).click();
  await expect(page.locator(`${t('later-shelf-later')} ${t('later-empty')}`)).toHaveText(
    'Nothing in later yet',
  );
  await expect(page.locator(`${t('later-shelf-backlog')} ${t('later-empty')}`)).toHaveText(
    'Nothing on the backlog yet',
  );
});

test.describe('with write access', () => {
  test.beforeEach(async ({ app, server }) => {
    server.state.auth = 'ok';
    server.state.permissions = { write: true, agentInput: true };
    server.state.replies.board = stuckBoard();
    await app();
  });

  test('take over asks with a title, an explanation and Take over button and the X', async ({
    page,
  }) => {
    await page.locator(t('needs-take-over')).first().click();
    const dialog = page.locator(t('confirm-dialog'));
    await expect(dialog).toContainText('Take over run?');
    await expect(dialog).toContainText(
      'Taking over stops the background run and continues it in Claude Code on your computer.',
    );
    await expect(page.locator(t('confirm-yes'))).toHaveText('Take over');
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toHaveCount(0);
  });
});
