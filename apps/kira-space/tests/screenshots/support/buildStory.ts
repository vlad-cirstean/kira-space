import type { ApprovalItem } from '@shared/domain/approvals';
import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { liveItem } from '@workbench/testing/ui/approvals';
import { adeBoard, adeFixture, adeV2Control } from '../../ui/support/adeV2';
import { contract } from '../../ui/support/contract';
import { IPC } from '../../ui/support/ipcChannels';
import type { ControlSnapshot } from '../../ui/support/types';

// One story for every Build with Kira shot: the same task, workflow, branch and database.

export const STORY = {
  taskId: 'T_bill',
  taskTitle: 'Export orders as CSV',
  workflowName: 'Feature',
  branch: 'feat/orders-csv-export',
  webBranchId: 'b_billdash',
  apiBranchId: 'b_meter',
  connection: 'acme-orders',
  proofSession: 'orders-export',
} as const;

const repo = (id: string, name: string, order: number, color: string) => ({
  id,
  name,
  root: `/Users/ana/code/${name}`,
  repoId: `/Users/ana/code/${name}`,
  sortOrder: order,
  color,
  createdAt: '2026-01-01T00:00:00.000Z',
  hidden: false,
});

export const STORY_REPOS = [
  repo('repo-web-app', 'acme-web', 1, 'blue'),
  repo('repo-api', 'acme-api', 2, 'green'),
  repo('repo-mobile', 'acme-mobile', 3, 'violet'),
];

type Rec = Record<string, unknown>;

function storyBoardFx() {
  return adeBoard((board) => {
    const task = board.tasks.find((t) => t.id === STORY.taskId);
    if (!task) throw new Error('story task missing from the board fixture');
    task.title = STORY.taskTitle;
    task.branchIds = [STORY.apiBranchId, STORY.webBranchId];
    task.runs = task.runs.filter((r) => r.branchId !== 'b_bill');
    board.branches = board.branches.filter((b) => b.id !== 'b_bill');
    for (const b of board.branches as Rec[]) {
      if (b.id !== STORY.webBranchId && b.id !== STORY.apiBranchId) continue;
      b.name = STORY.branch;
      b.worktree = `~/wt/${b.id === STORY.webBranchId ? 'acme-web' : 'acme-api'}/orders-csv-export`;
      b.base = 'main';
      b.baseBranchId = '';
    }
  });
}

function storyWorkflows() {
  const fx = adeFixture<{ workflows: { workflow: { name: string } | null }[] }>('workflows');
  fx.workflows = fx.workflows.filter((w) => w.workflow);
  const first = fx.workflows[0];
  if (first?.workflow) first.workflow.name = STORY.workflowName;
  return fx;
}

/** Main window in the ade mode with the story board; `extra` replaces a default channel. */
export function storyBoard(extra: readonly ControlSnapshot[] = []): ControlSnapshot[] {
  return adeV2Control([
    { channel: IPC.codeWorkspaceListRepos, response: STORY_REPOS },
    { channel: IPC.adeTaskBoard, response: storyBoardFx() },
    { channel: IPC.adeTaskWorkflows, response: storyWorkflows() },
    ...extra,
  ]);
}

export const STORY_LAUNCH = CAPTURE_LAUNCH;

/** Approval items of the story: an agent write and a lifted limit on the story database. */
export function storyApprovals(): { write: ApprovalItem; exemption: ApprovalItem } {
  const [write] = contract<ApprovalItem[]>('dbmcp', 'ApprovalsService.List');
  const exemption = contract<ApprovalItem>('dbmcp-expensive-query', 'item');
  if (!write) throw new Error('contract dbmcp has no item');
  const target = {
    color: 'indigo',
    kind: 'postgres',
    location: 'localhost:5432 · orders',
    name: STORY.connection,
    readOnly: false,
  };
  const origin = {
    client: 'Claude Code 2.1.318',
    cwd: '',
    source: '',
    step: 'Implement',
    task: STORY.taskTitle,
  };
  const plan = {
    estimatedRowsRead: 1200,
    issues: [
      {
        code: 'wide-scan',
        message: 'Seq Scan on orders is estimated to read 1,200 rows.',
        severity: 'warn',
      },
    ],
    issuesOmitted: 0,
    overThreshold: false,
    thresholdRows: 5000,
  };
  return {
    write: liveItem(
      {
        ...write,
        id: 'db.query:orders-write',
        target,
        origin,
        payload: {
          ...(write.payload as object),
          statement: "update orders set exported_at = now() where status = 'paid'",
          plan,
        },
      } as ApprovalItem,
      1800,
      CAPTURE_NOW,
    ),
    exemption: liveItem(
      {
        ...exemption,
        id: 'db.exemption:orders-export',
        target,
        origin,
        payload: {
          ...(exemption.payload as object),
          limits: [{ limit: 'expensive_query', max: 5000, measured: 84000, unit: 'rows' }],
          plan: { ...plan, estimatedRowsRead: 84000, overThreshold: true },
          reason: 'The CSV export reads every paid order of the month.',
          statement: "select id, customer_id, total, created_at from orders where status = 'paid'",
        },
      } as ApprovalItem,
      1800,
      CAPTURE_NOW,
    ),
  };
}

/** DB audit trail entries of the story task, newest first. */
export function storyDbAudit(): Rec[] {
  const raw = JSON.stringify(
    contract<{ entries: Rec[] }>('ade-db-audit', 'AdeTaskService.DbAudit#list'),
  )
    .replaceAll('audit db', STORY.connection)
    .replaceAll('quarterly report needs the full table', 'the CSV export reads every paid order')
    .replaceAll('people', 'orders')
    .replaceAll('email', 'status')
    .replaceAll('"Work"', '"Implement"');
  const fx = JSON.parse(raw) as { entries: Rec[] };
  return fx.entries.map((e, i) => {
    const at = CAPTURE_NOW - (i + 1) * 60_000;
    return {
      ...e,
      id: `c_${i}`,
      sessionId: 'sp22',
      startedAt: at,
      durationMs: 12,
      approvals: (e.approvals as Rec[]).map((a, j) => ({
        ...a,
        id: `a_${i}_${j}`,
        raisedAt: at,
        resolvedAt: at + 5,
      })),
    };
  });
}
