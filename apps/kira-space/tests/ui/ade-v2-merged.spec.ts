import { expect, test } from './fixtures';
import { type AdeBoardFx, adeBoard, adeFixture, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { IPC } from './support/ipcChannels';

// Contract ade-merged. Backend half: adeflow TestMergedIntoMain. Fixture ids replace the backend's;
// every other field is the backend's.

const t = (id: string) => `[data-testid="${id}"]`;
const row = (id: string) => `[data-testid="ade-branch-row"][data-branch-id="${id}"]`;

type Fact = {
  mergedIntoMain: boolean;
  mergedAt: number | null;
  ahead: number;
  behind: number;
  conflictCheck: string;
  conflictsIfRebased: string[];
  base: string;
  baseBranchId: string;
};

/** The backend's branch facts on the fixture's feat/oauth-login row. */
async function open(relaunch: Parameters<typeof openPlan>[0], key: string) {
  const fact = contract<Fact>('ade-merged', `AdeTaskService.Branch#${key}`);
  const board = adeBoard((b: AdeBoardFx) => {
    const br = b.branches.find((x) => x.id === 'b_auth') as Record<string, unknown>;
    Object.assign(br, {
      mergedIntoMain: fact.mergedIntoMain,
      mergedAt: fact.mergedAt,
      ahead: fact.ahead,
      behind: fact.behind,
      conflictCheck: fact.conflictCheck,
      conflictsIfRebased: fact.conflictsIfRebased,
      base: fact.baseBranchId ? 'feat/usage-billing' : fact.base,
      baseBranchId: fact.baseBranchId ? 'b_bill' : '',
    });
  });
  // No change requests: a merged PR's glyph would replace the merged tag.
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: board },
    {
      channel: IPC.adeTaskChangeRequests,
      response: { ...adeFixture<object>('change-requests'), branches: {} },
    },
  ]);
  const fixtureRow = page.locator(row('b_auth'));
  await fixtureRow.scrollIntoViewIfNeeded();
  return { page, fixtureRow };
}

for (const key of [
  'stacked-parent-merged',
  'develop-into-main',
  'commit-after-merge',
  'pr-other-head',
]) {
  test(`contract: ${key} is not shown merged and still offers Change base`, async ({
    relaunch,
  }) => {
    const { page, fixtureRow } = await open(relaunch, key);
    await expect(fixtureRow.locator(`${t('ade-tag')}[data-kind="merged"]`)).toHaveCount(0);
    await fixtureRow.click();
    await expect(page.locator(t('ade-panel-action-change-base'))).toBeVisible();
  });
}

for (const key of ['merged', 'squash-pr-head']) {
  test(`contract: ${key} is shown merged and offers no Change base`, async ({ relaunch }) => {
    const { page, fixtureRow } = await open(relaunch, key);
    await expect(fixtureRow.locator(`${t('ade-tag')}[data-kind="merged"]`)).toHaveCount(1);
    await fixtureRow.click();
    await expect(page.locator(t('ade-panel-action-change-base'))).toHaveCount(0);
  });
}
