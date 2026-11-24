import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { gitStreamRequests } from './support/gitStreamMock';
import {
  COMMIT_DETAIL,
  openCommitDetail,
  openPortGraph,
  openRefsPopover,
  PICKER_RESULTS,
  type Relaunch,
  refRow,
  rootRow,
  SHA_A,
  SHA_B,
  singleRowChunks,
} from './support/gitUiPortFixtures';

// P267 C9: behaviour the UI leftovers pass unified on the git surfaces: search combobox keys, the
// picker trigger and row menus, the capped file tree, grid refresh keys and the checkout dialog.

const gridRow = (p: Page) => p.locator('[data-testid="commit-grid"] .slick-row[data-row="0"]');

test('search: arrows move the active option, the first Esc closes the dropdown and keeps the query', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: PICKER_RESULTS,
  });
  await expect(gridRow(page)).toBeVisible();
  await gridRow(page).click();
  await page.keyboard.press('Control+f');

  const input = page.locator('[data-testid="search-input"] input');
  await expect(input).toBeFocused();
  await input.fill('feature');
  await expect(page.locator('[data-testid="search-results"]')).toBeVisible();
  await expect(input).not.toHaveAttribute('aria-activedescendant', /./);

  await page.keyboard.press('ArrowDown');
  await expect(input).toHaveAttribute('aria-activedescendant', /./);

  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="search-results"]')).toHaveCount(0);
  await expect(input).toHaveValue('feature');
});

test('branch picker: the trigger is the shared picker trigger', async ({ relaunch }) => {
  const page = await openRefsPopover(relaunch);
  // PopoverTrigger and TooltipTrigger overwrite data-slot, so assert the shared look instead.
  const trigger = page.locator('[data-testid="branch-trigger"]');
  await expect(trigger).toBeVisible();
  await expect(trigger).toHaveClass(/rounded-kira-sm/);
  await expect(trigger).toHaveClass(/h-control/);
});

const WITH_REMOTE = {
  ...PICKER_RESULTS,
  'refs.list': {
    ...PICKER_RESULTS['refs.list'],
    remoteBranches: [
      refRow('origin/main', { refname: 'refs/remotes/origin/main', kind: 'remoteBranch' }),
    ],
    // A first tag takes the roving tabindex by default, so v1 starts at -1.
    tags: [
      refRow('v0', { refname: 'refs/tags/v0', kind: 'tag' }),
      refRow('v1', { refname: 'refs/tags/v1', kind: 'tag' }),
    ],
  },
};

test('tags: a right-click selects the row; the disabled push item shows its reason', async ({
  relaunch,
}) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit')]),
    results: WITH_REMOTE,
  });
  await expect(gridRow(page)).toBeVisible();
  await page.locator('[data-testid="branch-trigger"]').click();
  await page.getByRole('button', { name: /^Tags/ }).click();

  const tag = page.locator('[data-row-id="tag:refs/tags/v1"]');
  await expect(tag).toHaveAttribute('tabindex', '-1');
  await tag.locator('[data-testid="row-actions"]').click({ button: 'right' });
  await expect(tag).toHaveAttribute('tabindex', '0');

  const push = page.locator('[data-testid="menu-item-pushRef:origin"]');
  await expect(push).toHaveAttribute('data-disabled', '');
  await push.hover();
  await expect(
    page.locator('[data-slot="tooltip-content"]').filter({ hasText: 'Not supported yet.' }),
  ).toBeVisible();
});

const MANY_FILES = Array.from({ length: 1100 }, (_, i) => ({
  kind: 'modified',
  path: `src/file-${String(i).padStart(4, '0')}.ts`,
  originalPath: undefined,
  similarity: undefined,
}));

test('file tree renders a window of 1100 files', async ({ relaunch }) => {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, COMMIT_DETAIL.subject)]),
    results: { 'commit.detail': { ...COMMIT_DETAIL, files: MANY_FILES } },
  });
  await gridRow(page).click();
  const tree = page.locator('[data-testid="file-tree"]');
  const rows = tree.locator('[data-testid="file-tree-row"]');
  await expect(rows.first()).toBeVisible();
  expect(await rows.count()).toBeLessThan(100);
  await expect(tree.getByRole('button', { name: /^Show more/ })).toHaveCount(0);

  await tree.getByRole('tree').evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await expect(rows.filter({ hasText: 'file-1099.ts' })).toBeVisible();
  expect(await rows.count()).toBeLessThan(100);
});

test('graph keys: Ctrl+R does nothing, F5 refreshes', async ({ relaunch }) => {
  const page = await openCommitDetail(relaunch);
  await gridRow(page).focus();
  const refreshes = async () =>
    (await gitStreamRequests(page)).filter((m) => m === 'graph.refresh').length;
  const before = await refreshes();

  await page.keyboard.press('Control+r');

  // F5 is the barrier: exactly one refresh proves Ctrl+R sent none.
  await page.keyboard.press('F5');
  await expect.poll(refreshes).toBeGreaterThan(before);
  expect(await refreshes()).toBe(before + 1);
});

const BLOCKED = {
  'preflight.checkout': {
    target: { kind: 'sha', name: SHA_A },
    detaches: true,
    createsTracking: undefined,
    carried: [],
    blockers: [{ kind: 'blockedByTracked', paths: ['a.ts'] }],
    verdict: 'blocked',
    routes: ['discard', 'stashAndCarry'],
  },
};

async function openBlockedCheckout(relaunch: Relaunch): Promise<Page> {
  const page = await openPortGraph(relaunch, {
    chunks: singleRowChunks([rootRow(SHA_A, 'A commit', { parents: [SHA_B] })]),
    results: { ...PICKER_RESULTS, ...BLOCKED },
  });
  await expect(gridRow(page)).toBeVisible();
  await gridRow(page).locator('[data-testid="message-subject"]').click({ button: 'right' });
  await page.locator('[data-testid="menu-item-checkoutDetached"]').click();
  return page;
}

test('checkout dialog: exactly one primary action', async ({ relaunch }) => {
  const page = await openBlockedCheckout(relaunch);
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-variant="dialog-primary"]')).toHaveCount(1);
});
