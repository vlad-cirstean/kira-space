import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { gitStreamParams } from './support/gitStreamMock';
import {
  COMMIT_DETAIL,
  openPortGraph,
  PICKER_RESULTS,
  type Relaunch,
  rootRow,
  SHA_A,
  SHA_B,
  singleRowChunks,
} from './support/gitUiPortFixtures';
import { pressArrow } from './support/radioKeys';

// P667: single-choice dialogs render their options as cards (whole card is the radio). UI half
// only: the dialogs' op arguments are asserted on the wire; the backend ops have their own tests.

const tid = (page: Page, id: string) => page.locator(`[data-testid="${id}"]`);
const gridRow = (page: Page) =>
  page.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]');

const HEAD = { kind: 'branch', name: 'main' };
const PARENTS = [
  { parentNumber: 1, sha: SHA_A, subject: 'Mainline work' },
  { parentNumber: 2, sha: SHA_B, subject: 'Side branch work' },
];

const RESULTS = {
  ...PICKER_RESULTS,
  'commit.detail': COMMIT_DETAIL,
  'status.get': {
    head: HEAD,
    upstream: null,
    counts: { staged: 0, unstaged: 1, untracked: 0, unmerged: 0 },
    isClean: false,
    dirtyPaths: ['a.ts'],
    dirtyTruncated: false,
    inProgress: null,
    autoFetch: null,
  },
  'op.run': { ok: true, error: undefined, undo: null, head: HEAD, inProgress: null },
  'preflight.reset': {
    target: SHA_A,
    targetSubject: 'A commit',
    mode: 'mixed',
    currentHead: SHA_B,
    branch: 'main',
    leaving: 1,
    gaining: 0,
    leavingCommits: [{ sha: SHA_B, subject: 'A later commit' }],
    leavingTruncated: false,
    dirty: { staged: [], unstaged: ['a.ts'], untracked: [] },
    destroys: [],
    inProgress: null,
    requiresTypedConfirmation: false,
    routes: ['stashFirst'],
    verdict: 'clean',
    blockers: [],
  },
  'preflight.cherryPick': {
    sha: SHA_A,
    subject: 'A merge commit',
    mainlineRequired: PARENTS,
    prediction: { kind: 'clean' },
    alreadyApplied: false,
    inProgress: null,
    detachedHead: false,
    verdict: 'clean',
    blockers: [],
  },
  'preflight.revert': {
    shas: [SHA_A],
    mainlineRequired: [{ sha: SHA_A, parents: PARENTS }],
    dirtyPaths: [],
    inProgress: null,
    prediction: { kind: 'clean' },
    predictedFor: SHA_A,
    detachedHead: false,
    verdict: 'clean',
    blockers: [],
  },
};

async function openMenuItem(relaunch: Relaunch, item: string): Promise<Page> {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: RESULTS,
  });
  await expect(gridRow(page)).toBeVisible();
  await gridRow(page).locator('[data-testid="message-subject"]').click({ button: 'right' });
  await tid(page, `menu-item-${item}`).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  return page;
}

const checked = async (card: Locator, on: boolean) => {
  await expect(card).toHaveAttribute('aria-checked', String(on));
  await expect(card).toHaveAttribute('data-state', on ? 'checked' : 'unchecked');
};

test('reset mode cards select by click and arrows, and run the chosen mode', async ({
  relaunch,
}) => {
  const page = await openMenuItem(relaunch, 'resetToThisCommit');
  const [soft, mixed, hard] = ['soft', 'mixed', 'hard'].map((m) => tid(page, `reset-mode-${m}`));
  if (!(soft && mixed && hard)) throw new Error('unreachable');

  await checked(mixed, true);
  await checked(soft, false);
  await checked(hard, false);
  await expect(page.getByRole('radio', { name: 'Hard' })).toHaveAttribute(
    'data-testid',
    'reset-mode-hard',
  );

  await soft.click();
  await checked(soft, true);
  await checked(mixed, false);

  await soft.focus();
  await pressArrow(page, 'ArrowDown');
  await pressArrow(page, 'ArrowDown');
  await checked(hard, true);
  await expect(hard).toBeFocused();
  await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();

  await pressArrow(page, 'ArrowUp');
  await pressArrow(page, 'ArrowUp');
  await checked(soft, true);

  await tid(page, 'reset-confirm').click();
  await expect
    .poll(async () => gitStreamParams(page, 'op.run'))
    .toEqual([
      expect.objectContaining({ op: expect.objectContaining({ kind: 'reset', mode: 'soft' }) }),
    ]);
});

test('cherry-pick mainline cards gate the confirm and send the chosen parent', async ({
  relaunch,
}) => {
  const page = await openMenuItem(relaunch, 'cherryPickThisCommit');
  const first = tid(page, 'cherry-pick-mainline-1');
  const second = tid(page, 'cherry-pick-mainline-2');
  await checked(first, false);
  await checked(second, false);
  await expect(tid(page, 'cherry-pick-confirm')).toBeDisabled();

  await second.click();
  await checked(second, true);
  await expect
    .poll(async () => gitStreamParams(page, 'preflight.cherryPick'))
    .toContainEqual(expect.objectContaining({ mainline: 2 }));
  await expect(tid(page, 'cherry-pick-confirm')).toBeEnabled();

  await tid(page, 'cherry-pick-confirm').click();
  await expect
    .poll(async () => gitStreamParams(page, 'op.run'))
    .toEqual([
      expect.objectContaining({
        op: expect.objectContaining({ kind: 'cherryPick', mainline: 2 }),
      }),
    ]);
});

test('revert mainline cards pick by keyboard and send the chosen parent', async ({ relaunch }) => {
  const page = await openMenuItem(relaunch, 'revertThisCommit');
  const first = tid(page, 'revert-mainline-1');
  const second = tid(page, 'revert-mainline-2');
  await expect(tid(page, 'revert-confirm')).toBeDisabled();

  // The dialog focuses its own first control; Tab moves on until the group's first card.
  for (let i = 0; i < 4 && !(await first.evaluate((el) => el === document.activeElement)); i++) {
    await page.keyboard.press('Tab');
  }
  await expect(first).toBeFocused();
  await page.keyboard.press('Space');
  await checked(first, true);
  await pressArrow(page, 'ArrowDown');
  await checked(second, true);
  await expect
    .poll(async () => gitStreamParams(page, 'preflight.revert'))
    .toContainEqual(expect.objectContaining({ mainline: 2 }));

  await tid(page, 'revert-confirm').click();
  await expect
    .poll(async () => gitStreamParams(page, 'op.run'))
    .toEqual([
      expect.objectContaining({
        op: expect.objectContaining({ kind: 'revert', mainline: 2 }),
      }),
    ]);
});
