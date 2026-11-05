import type { Page } from '@playwright/test';
import { expect, reviewTest, test } from './fixtures';
import {
  ALL_ADDRESSED,
  IMPL_ID,
  openWindow,
  ROUND_ID,
  RUN_ID,
  reviewStep,
  t,
} from './support/adeReviewStep';
import { type AdeBoardFx, adeBoard, openPlan } from './support/adeV2';
import { gitStreamParams } from './support/gitStreamMock';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The review step: the board chip and Needs-you item, and the Workflow review tab of the review
// window. Contract ade-review-step; backend half: adeflow TestReviewStep* (reviewstep_test.go). The
// task, branch and run ids are the fixture board's; every other field is the backend's.

function boardWithReview(key: string): ControlSnapshot[] {
  const step = reviewStep(key);
  return [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b: AdeBoardFx) => {
        const bill = b.tasks.find((x) => x.id === 'T_bill');
        if (!bill) throw new Error('fixture task missing');
        bill.runs = bill.runs.map((r) =>
          r.id === RUN_ID ? { ...step.run, startedAt: r.startedAt } : r,
        );
      }),
    },
  ];
}

async function openTask(page: Page): Promise<void> {
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await expect(page.locator(t('ade-panel'))).toBeVisible();
}

test('a waiting review shows on the run line and opens the review window', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, boardWithReview('waiting'));
  await openTask(page);
  const line = page
    .locator(`${t('ade-step-block')}[data-step-id="impl"]`)
    .locator(`${t('ade-run-line')}[data-branch-id="b_billdash"]`);
  await expect(line.locator(t('ade-run-status'))).toHaveText('Waiting for review');
  await line.locator(t('ade-run-review')).click();
  await expect.poll(() => calls(control, IPC.adeTaskOpenReviewWindow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskOpenReviewWindow)[0]?.args).toEqual({ branchId: 'b_billdash' });
});

test('a waiting review is a Needs-you item with a Review action', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, boardWithReview('waiting'));
  await page.locator(t('ade-tab-needs')).click();
  const item = page.locator(`${t('ade-needs-item')}[data-kind="review"]`);
  await expect(item).toHaveCount(1);
  await expect(item.locator(t('ade-needs-action'))).toHaveText('Review');
  await item.locator(t('ade-needs-action')).click();
  await expect.poll(() => calls(control, IPC.adeTaskOpenReviewWindow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskOpenReviewWindow)[0]?.args).toEqual({ branchId: 'b_billdash' });
});

test('a running round shows Agent working', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, boardWithReview('working'));
  await openTask(page);
  const line = page.locator(`${t('ade-run-line')}[data-branch-id="b_billdash"]`);
  await expect(line.locator(t('ade-run-status'))).toHaveText('Agent working');
});

reviewTest('contract: answered comments show replies and Addressed', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'answered');
  await expect(page.locator(t('ade-review-tab-workflow'))).toBeVisible();
  const rows = page.locator(t('ade-review-comment'));
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(0).locator(t('ade-review-comment-reply'))).toHaveText(
    'Renamed to chargeFor.',
  );
  await expect(rows.nth(0)).toContainText('Addressed');
  await expect(rows.nth(1)).toContainText('Question');
  await expect(rows.nth(2)).toContainText('Open');
  await expect(page.locator(t('ade-review-round'))).toHaveText('Round 1');
});

reviewTest('Reopen sends review.comment.setState', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'answered');
  await page
    .locator(t('ade-review-comment'))
    .first()
    .locator(t('ade-review-comment-state'))
    .click();
  await expect
    .poll(async () => gitStreamParams(page, 'review.comment.setState'))
    .toEqual([expect.objectContaining({ id: '1', state: 'open' })]);
});

reviewTest('the Ask agent tab shows the review agent', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'answered');
  await page.locator(t('ade-review-tab-ask')).click();
  await expect(page.locator(t('ade-review-agent'))).toBeVisible();
  await expect(page.locator(t('ade-review-workflow'))).toHaveCount(0);
});

reviewTest('contract: Send is off while a round runs', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'working');
  await expect(page.locator(t('ade-review-phase'))).toHaveText('Agent working');
  await expect(page.locator(t('ade-review-send-agent'))).toBeDisabled();
  await expect(page.locator(t('ade-review-stop'))).toBeVisible();
  await expect(page.locator(t('ade-review-finish-approved'))).toBeDisabled();
});

reviewTest('Send to agent sends the implementer route by default', async ({ relaunch }) => {
  const { page, control } = await openWindow(relaunch, 'waiting');
  await expect(page.locator(t('ade-review-target'))).toHaveValue(IMPL_ID);
  await page.locator(t('ade-review-send-agent')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSendReview)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSendReview)[0]?.args).toEqual({
    runId: RUN_ID,
    route: 'implementer',
    targetRunId: IMPL_ID,
  });
});

reviewTest('picking a new review agent sends route new', async ({ relaunch }) => {
  const { page, control } = await openWindow(relaunch, 'waiting');
  await page.locator(t('ade-review-route-new')).click();
  await page.locator(t('ade-review-send-agent')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSendReview)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSendReview)[0]?.args).toEqual({
    runId: RUN_ID,
    route: 'new',
    targetRunId: '',
  });
});

reviewTest('Send needs an open comment', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'answered', ALL_ADDRESSED);
  await expect(page.locator(t('ade-review-send-agent'))).toBeDisabled();
});

reviewTest(
  'Finish with open comments asks first, then sends the result and note',
  async ({ relaunch }) => {
    const { page, control } = await openWindow(relaunch, 'answered');
    await page.locator(t('ade-review-finish-note')).fill('Ship it after CI.');
    await page.locator(t('ade-review-finish-approved')).click();
    await expect(page.locator(t('ade-confirm-dialog'))).toContainText('1 comments are still open');
    expect(calls(control, IPC.adeTaskFinishReview)).toHaveLength(0);
    await page.locator(t('ade-confirm-yes')).click();
    await expect.poll(() => calls(control, IPC.adeTaskFinishReview)).toHaveLength(1);
    expect(calls(control, IPC.adeTaskFinishReview)[0]?.args).toEqual({
      runId: RUN_ID,
      result: 'approved',
      note: 'Ship it after CI.',
    });
  },
);

reviewTest('Finish with every comment addressed sends at once', async ({ relaunch }) => {
  const { page, control } = await openWindow(relaunch, 'answered', ALL_ADDRESSED);
  await page.locator(t('ade-review-finish-rework')).click();
  await expect.poll(() => calls(control, IPC.adeTaskFinishReview)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskFinishReview)[0]?.args).toEqual({
    runId: RUN_ID,
    result: 'rework',
    note: '',
  });
});

reviewTest('contract: a lost session shows the fallback note', async ({ relaunch }) => {
  const { page } = await openWindow(relaunch, 'fallback');
  await expect(page.locator(t('ade-review-note'))).toHaveText(
    'The implementer session is gone. Sent to a new review agent.',
  );
});

reviewTest('contract: a stopped round offers Resume', async ({ relaunch }) => {
  const { page, control } = await openWindow(relaunch, 'stuck');
  await expect(page.locator(t('ade-review-phase'))).toHaveText('Review agent stopped');
  await page.locator(t('ade-review-resume')).click();
  await expect.poll(() => calls(control, IPC.adeTaskResumeRun)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskResumeRun)[0]?.args).toEqual({ runId: ROUND_ID });
});
