import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { liveItem } from '@workbench/testing/ui/approvals';
import { adeFixture } from '../../ui/support/adeV2';
import { contract } from '../../ui/support/contract';
import { STORY, STORY_REPOS, storyBoardFx } from './buildStory';

// Phone fixtures of the story: the board, sessions and pull requests the phone backend answers with,
// cut down to the story task and two quiet neighbours. The ids and shapes are the ADE fixtures'.

type Rec = Record<string, unknown>;

const KEEP_TASKS = new Set([STORY.taskId, 'T_notif', 'T_deps']);
const KEEP_BRANCHES = new Set([STORY.apiBranchId, STORY.webBranchId, 'b_notif', 'b_deps']);

/** The story board without the other fixture tasks. */
export function phoneBoard(): Rec {
  const board = storyBoardFx() as unknown as Rec & {
    tasks: Rec[];
    branches: Rec[];
    plan: { day: Record<string, string>; order: string[] } & Rec;
  };
  const tasks = board.tasks.filter((t) => KEEP_TASKS.has(t.id as string));
  for (const task of tasks) {
    task.branchIds = (task.branchIds as string[]).filter((id) => KEEP_BRANCHES.has(id));
    task.jira = null;
  }
  return {
    ...board,
    tasks,
    branches: board.branches.filter((b) => KEEP_BRANCHES.has(b.id as string)),
    pairs: [],
    history: [],
    plan: {
      ...board.plan,
      day: Object.fromEntries(Object.entries(board.plan.day).filter(([id]) => KEEP_TASKS.has(id))),
      order: board.plan.order.filter((id) => KEEP_TASKS.has(id)),
    },
  };
}

const PULL_REQUESTS: Record<string, Rec> = {
  [STORY.apiBranchId]: {
    number: 218,
    title: 'Export endpoint for orders as CSV',
    state: 'open',
    url: 'https://github.com/acme/acme-api/pull/218',
    baseRef: 'main',
    checksState: 'SUCCESS',
    reviewDecision: 'APPROVED',
    mergeable: 'mergeable',
  },
  [STORY.webBranchId]: {
    number: 1427,
    title: 'Download button for the orders export',
    state: 'open',
    url: 'https://github.com/acme/acme-web/pull/1427',
    baseRef: 'main',
    checksState: 'PENDING',
    mergeable: 'mergeable',
  },
  b_deps: {
    number: 1431,
    title: 'Bump dependencies',
    state: 'open',
    url: 'https://github.com/acme/acme-web/pull/1431',
    baseRef: 'main',
    checksState: 'SUCCESS',
    mergeable: 'mergeable',
  },
};

/** The `/api/ade/sessions` answer: the live sessions of the kept tasks. */
function phoneSessions(): { sessions: Rec[] } {
  const all = adeFixture<{ sessions: Rec[] }>('sessions').sessions;
  const keep = ['b1c2', '9d10', 'm111', 'tk01', '71b0', 'aa12'];
  return {
    sessions: all
      .filter((s) => keep.includes(s.id as string))
      .map((s) => ({
        ...s,
        cwd: (s.cwd as string).replace('/web-app/', '/acme-web/').replace('/api/', '/acme-api/'),
      })),
  };
}

/** Routes the phone reads the backend mock cannot take from `replies`: pull requests and repositories. */
export async function routePhoneReads(page: Page): Promise<void> {
  const fixture = adeFixture<{ repos: Rec[]; branches: Record<string, Rec>; branchUrls: Rec }>(
    'change-requests',
  );
  const requests = {
    ...fixture,
    branches: PULL_REQUESTS,
    branchUrls: {},
  };
  const json = (body: unknown) => ({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
  await page.route('**/api/ade/change-requests', (route) => route.fulfill(json(requests)));
  await page.route('**/api/ade/repos', (route) =>
    route.fulfill(
      json({
        repos: STORY_REPOS.map((r) => ({ codeRepoId: r.id, name: r.name, nickname: '' })),
      }),
    ),
  );
}

export const phoneReplies = () => ({ board: phoneBoard(), sessions: phoneSessions() });

/** An agent's request to rename the branches of the story task. */
export function phoneBranchRequest(): ApprovalItem {
  const [base] = contract<ApprovalItem[]>(
    'mobile-approvals',
    'http:GET /api/approvals#pending',
  ).filter((i) => i.kind === 'branch.request');
  if (!base) throw new Error('contract has no branch.request item');
  const item = (id: string, repoId: string, repo: string, from: string) => ({
    ...((base.payload as { items: Rec[] }).items[0] as Rec),
    id,
    kind: 'rename',
    codeRepoId: repoId,
    repo,
    branch: STORY.branch,
    from,
    holder: undefined,
  });
  return liveItem(
    {
      ...base,
      id: 'branch.request:story-rename',
      origin: {
        client: 'Claude Code 2.1.318',
        cwd: '',
        source: '',
        step: 'Implement',
        task: STORY.taskTitle,
      },
      payload: {
        taskId: STORY.taskId,
        taskTitle: STORY.taskTitle,
        items: [
          item('c1i1', 'repo-api', 'acme-api', 'orders-csv'),
          item('c1i2', 'repo-web-app', 'acme-web', 'orders-csv'),
        ],
      },
      target: { color: '', kind: '', location: '', name: STORY.taskTitle, readOnly: false },
    } as ApprovalItem,
    1800,
    CAPTURE_NOW,
  );
}
