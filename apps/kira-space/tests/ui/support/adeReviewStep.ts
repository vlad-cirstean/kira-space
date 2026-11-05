import type { Page } from '@playwright/test';
import { AGENT_PANEL_ON, type reviewTest } from '../fixtures';
import { adeFixture, adeV2Control } from './adeV2';
import { contract } from './contract';
import { IPC } from './ipcChannels';

// Shared by the review step UI and visual specs: the backend's ReviewStep answers (contract
// ade-review-step) mapped onto the fixture board, and the review window boot.

export const t = (id: string) => `[data-testid="${id}"]`;
export const target = adeFixture<{ gitRepoId: string }>('review-target');
export const IDS = { taskId: 'T_bill', stepId: 'impl', branchId: 'b_billdash' };
export const RUN_ID = 'r_T_bill_impl_b_billdash';
export const ROUND_ID = 'r_round_1';
export const IMPL_ID = 'r_T_bill_impl_b_meter';

export type Wire = Record<string, unknown>;
export interface StepWire extends Wire {
  results: Wire[];
  candidates: Wire[];
  run: Wire | null;
  round: ({ run: Wire } & Wire) | null;
}

export function reviewStep(key: string): StepWire {
  const s = contract<StepWire>('ade-review-step', `AdeTaskService.ReviewStep#${key}`);
  const ids = ['approved', 'rework', 'rejected'];
  const run = (r: Wire, id: string): Wire => ({
    ...r,
    ...IDS,
    id,
    attempt: 1,
    startedAt: 1_790_036_400_000,
  });
  return {
    ...s,
    results: s.results.map((r, i) => ({ ...r, id: ids[i] })),
    candidates: s.candidates.map((c) => ({ ...c, runId: IMPL_ID, attempt: 1 })),
    targetRunId: IMPL_ID,
    defaultTargetRunId: IMPL_ID,
    run: s.run && run(s.run, RUN_ID),
    round: s.round && { ...s.round, run: run(s.round.run, ROUND_ID) },
  };
}

export const SHA = 'a'.repeat(40);
function comment(id: number, over: Wire = {}): Wire {
  return {
    id: String(id),
    path: 'src/billing.ts',
    range: { start: 10 * id, end: 10 * id },
    body: `Comment ${id}`,
    anchor: 'exact',
    anchorSha: SHA,
    createdAt: 1_790_036_500_000,
    kind: 'change',
    state: 'open',
    reply: '',
    repliedAt: null,
    sentAt: null,
    ...over,
  };
}
export const ANSWERED = [
  comment(1, { state: 'addressed', reply: 'Renamed to chargeFor.', sentAt: 1 }),
  comment(2, { kind: 'question', state: 'addressed', reply: 'The API returns cents.', sentAt: 1 }),
  comment(3),
];
export const ALL_ADDRESSED = ANSWERED.map((c) => ({ ...c, state: 'addressed' }));

const GIT = (comments: Wire[]) => ({
  repoId: target.gitRepoId,
  extraResults: {
    'review.resolveBase': {
      branch: 'feat/billing-dashboard',
      base: 'main',
      reason: 'override',
      range: { kind: 'ready', commitCount: 3 },
      candidates: [],
    },
    'review.files': { branchTip: SHA, mergeBase: 'b'.repeat(40), files: [] },
    'review.session.load': null,
    'review.comment.list': { at: SHA, comments },
    'review.comment.setState': { changed: true },
    'repo.open': null,
  },
});

export async function openWindow(
  relaunch: Parameters<Parameters<typeof reviewTest>[2]>[0]['relaunch'],
  key: string,
  comments: Wire[] = ANSWERED,
): Promise<{ page: Page; control: Awaited<ReturnType<typeof relaunch>>['control'] }> {
  const { window: page, control } = await relaunch({
    storage: AGENT_PANEL_ON,
    control: adeV2Control([
      { channel: IPC.adeTaskReviewWindowTarget, response: target },
      { channel: IPC.adeTaskReviewAgent, response: { session: null, hostWindowKey: '' } },
      { channel: IPC.adeTaskReviewStep, response: reviewStep(key) },
    ]),
    gitStream: GIT(comments),
  });
  await page.locator(t('ade-review-workflow')).waitFor();
  return { page, control };
}
