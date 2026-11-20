import { expect, test } from './fixtures';
import { adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// One PR glyph per branch row: shape, tone and checks dot from the status the host carries. Contract
// ade-pr-glyph; backend half: flows/adeflow TestChangeRequestGlyphStatus.

const t = (id: string) => `[data-testid="${id}"]`;

interface Cr {
  state: string;
}
interface CrResult {
  branches: Record<string, Cr>;
  [rest: string]: unknown;
}

// The contract is keyed by branch name; the board fixture names the same branches.
function changeRequests(): CrResult {
  const board = adeFixture<{ branches: { id: string; name: string }[] }>('board');
  const byName = contract<CrResult>('ade-pr-glyph', 'AdeTaskService.ChangeRequests');
  const fx = adeFixture<CrResult>('change-requests');
  const branches: Record<string, Cr> = {};
  for (const [name, cr] of Object.entries(byName.branches)) {
    const id = board.branches.find((b) => b.name === name)?.id;
    if (!id) throw new Error(`board fixture has no branch ${name}`);
    branches[id] = cr;
  }
  return { ...fx, branches };
}

const control = (): ControlSnapshot[] => [
  { channel: IPC.adeTaskChangeRequests, response: changeRequests() },
];

const glyph = (id: string) =>
  `${t('ade-branch-row')}[data-branch-id="${id}"] ${t('ade-branch-pr')}`;

test('each PR shows its state, tone and checks', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, control());

  const approved = page.locator(glyph('b_hooks'));
  await expect(approved).toHaveAttribute('data-pr-state', 'open');
  await expect(approved).toHaveAttribute('data-tone', 'purple');
  await expect(approved).toHaveAttribute('data-checks', 'success');
  await expect(approved).toHaveAttribute('data-review', 'approved');
  await expect(approved).toHaveAttribute('data-mergeable', 'mergeable');
  await expect(approved).toHaveAttribute('data-auto-merge', 'true');

  const conflicted = page.locator(glyph('b_bill'));
  await expect(conflicted).toHaveAttribute('data-tone', 'red');
  await expect(conflicted).toHaveAttribute('data-checks', 'failure');
  await expect(conflicted).toHaveAttribute('data-review', 'changesRequested');
  await expect(conflicted).toHaveAttribute('data-mergeable', 'conflicting');

  const draft = page.locator(glyph('b_authui'));
  await expect(draft).toHaveAttribute('data-pr-state', 'draft');
  await expect(draft).toHaveAttribute('data-tone', 'grey');
  await expect(draft).toHaveAttribute('data-checks', 'pending');

  const merged = page.locator(glyph('b_cart'));
  await expect(merged).toHaveAttribute('data-pr-state', 'merged');
  await expect(merged).toHaveAttribute('data-tone', 'purple');
  await expect(merged).not.toHaveAttribute('data-checks', /.+/);
});

test('one tooltip lists every reported part, one line each', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, control());
  await page.locator(glyph('b_hooks')).hover();
  const tip = page.locator('[data-slot="tooltip-content"]').first();
  await expect(tip).toContainText('Outgoing webhooks');
  for (const part of ['checks', 'review', 'mergeable', 'autoMerge']) {
    await expect(tip.locator(`[data-part="${part}"]`)).toHaveCount(1);
  }
  await expect(tip.locator('[data-part="checks"]')).toHaveText('Checks passed');
  await expect(tip.locator('[data-part="review"]')).toHaveText('Approved');
  await expect(tip.locator('[data-part="mergeable"]')).toHaveText('No conflicts');
  await expect(tip.locator('[data-part="autoMerge"]')).toHaveText('Auto-merge on');
});

test('the pending dot pulses, and stops pulsing under reduced motion while staying visible', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, control());
  const dot = page.locator(`${glyph('b_authui')} [data-slot="status-dot"]`);
  await expect(dot).toBeVisible();
  const animation = () => dot.evaluate((el) => getComputedStyle(el).animationName);
  expect(await animation()).not.toBe('none');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect.poll(animation).toBe('none');
  await expect(dot).toBeVisible();
});
