import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { adeBoard, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Adopting an existing branch: the request dialog and the attach glyphs on branch rows.
// Contract ade-branch-adopt; backend half: flows/adeflow TestBranchAdopt (branchadopt_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface Pl {
  items: { id: string; branch: string; kind: string; holder?: { worktree: string } }[];
}

function item(key: string): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('ade-branch-adopt', `ApprovalsService.List#${key}`);
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

const only = (it: ApprovalItem) => {
  const [row] = (it.payload as Pl).items;
  if (!row) throw new Error('item has no row');
  return row;
};

async function openRequest(relaunch: Parameters<typeof openPlan>[0], key: string) {
  const it = item(key);
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  return { page, control, it, dialog, row: only(it) };
}

test('contract: an adopt item names the holder and Approve sends it', async ({ relaunch }) => {
  const { control, it, dialog, row } = await openRequest(relaunch, 'adopt');
  expect(row.kind).toBe('adopt');
  const line = dialog.locator(t(`ade-branch-request-item-${row.id}`));
  await expect(line).toContainText(row.branch);
  await expect(line).toContainText(row.holder?.worktree ?? 'no holder');
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({
    ids: [it.id],
    approve: true,
    options: {
      [it.id]: { itemIds: [row.id], remoteItemIds: [], shownItemIds: [row.id] },
    },
  });
});

test('contract: taking the main checkout warns about its dirty files', async ({ relaunch }) => {
  const { dialog, row } = await openRequest(relaunch, 'adoptMain');
  await expect(dialog.locator(t(`ade-branch-request-holder-${row.id}`))).toBeVisible();
  await expect(dialog.locator(t(`ade-branch-request-dirty-${row.id}`))).toBeVisible();
});

test('contract: a free existing branch is a plain checkout item', async ({ relaunch }) => {
  const { dialog, row } = await openRequest(relaunch, 'checkout');
  expect(row.kind).toBe('checkout');
  await expect(dialog.locator(t(`ade-branch-request-item-${row.id}`))).toContainText(row.branch);
  await expect(dialog.locator(t(`ade-branch-request-holder-${row.id}`))).toHaveCount(0);
});

interface Row extends Record<string, unknown> {
  detached?: { worktree: string; holder: Record<string, unknown> };
  shared: Record<string, unknown>[];
}

const DONOR_TASK = 'T_cart';
const ADOPTER_TASK = 'T_search';
const DONOR_ROW = 'b_cart';
const ADOPTER_ROW = 'b_search';
const ADOPTER_TREE = '~/kira-space-wts/web-app/search-v2';

// Masked ids in the contract become the fixture board's ids.
function adoptedBoard() {
  const [donor, adopter] = contract<Row[]>('ade-branch-adopt', 'AdeTaskBoard.Board#adopted');
  if (!donor?.detached || !adopter) throw new Error('contract has no adopted pair');
  return adeBoard((b) => {
    const patch = (id: string, fx: Row) => {
      const at = b.branches.findIndex((x) => x.id === id);
      const cur = b.branches[at];
      if (!cur) throw new Error(`${id} missing`);
      b.branches[at] = { ...cur, name: fx.name, detached: fx.detached, shared: fx.shared };
    };
    const peer = (kind: 'donor' | 'adopter') =>
      kind === 'donor'
        ? { attached: true, branchId: ADOPTER_ROW, taskId: ADOPTER_TASK }
        : { attached: false, branchId: DONOR_ROW, taskId: DONOR_TASK };
    patch(DONOR_ROW, {
      ...donor,
      detached: {
        worktree: ADOPTER_TREE,
        holder: {
          kind: 'task',
          taskId: ADOPTER_TASK,
          branchId: ADOPTER_ROW,
          worktree: ADOPTER_TREE,
        },
      },
      shared: [peer('donor')],
    });
    patch(ADOPTER_ROW, { ...adopter, detached: undefined, shared: [peer('adopter')] });
  });
}

const rowOf = (page: import('@playwright/test').Page, testid: string, id: string) =>
  page.locator(`${t(testid)}[data-branch-id="${id}"]`);

test('contract: a detached row keeps its branch name and names the holder', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: adoptedBoard() },
  ]);
  const donor = rowOf(page, 'ade-branch-row', DONOR_ROW);
  await expect(donor.locator(t('ade-branch-name'))).toHaveText('feat/x');
  await expect(donor.locator(t('ade-branch-name'))).not.toHaveText('HEAD');
  const glyph = donor.locator(t('ade-branch-detached'));
  await expect(glyph).toBeVisible();
  await expect(glyph).toHaveAttribute('aria-label', /Search v2/);
  await expect(glyph).toHaveAttribute(
    'aria-label',
    new RegExp(ADOPTER_TREE.split('/').pop() ?? ''),
  );
  await expect(donor.locator(t('ade-branch-shared'))).toHaveCount(0);
});

test('contract: the holder row shows the shared glyph with the other task', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: adoptedBoard() },
  ]);
  const adopter = rowOf(page, 'ade-branch-row', ADOPTER_ROW);
  await expect(adopter.locator(t('ade-branch-name'))).toHaveText('feat/x');
  const glyph = adopter.locator(t('ade-branch-shared'));
  await expect(glyph).toBeVisible();
  await expect(glyph).toHaveAttribute('aria-label', /Cart total test fails intermittently/);
  await expect(adopter.locator(t('ade-branch-detached'))).toHaveCount(0);
});

test('the task panel Main tab shows the same glyphs', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: adoptedBoard() },
  ]);
  await openTask(page, DONOR_TASK);
  const donor = rowOf(page, 'ade-task-branch', DONOR_ROW);
  await expect(donor.locator(t('ade-branch-name'))).toHaveText('feat/x');
  await expect(donor.locator(t('ade-branch-detached'))).toBeVisible();
  await openTask(page, ADOPTER_TASK);
  await expect(
    rowOf(page, 'ade-task-branch', ADOPTER_ROW).locator(t('ade-branch-shared')),
  ).toBeVisible();
});
