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
  const base: ControlSnapshot[] = [
    { channel: IPC.codeWorkspaceListRepos, response: STORY_REPOS },
    { channel: IPC.adeTaskBoard, response: storyBoardFx() },
    { channel: IPC.adeTaskWorkflows, response: storyWorkflows() },
  ];
  const overridden = new Set(extra.map((s) => s.channel));
  return adeV2Control([...base.filter((s) => !overridden.has(s.channel)), ...extra]);
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

const HANDOVER_BODIES = {
  spec: 'Spec signed off: export paid orders only, one CSV file per month, newest first.',
  plan: 'Plan agreed: one export endpoint in acme-api, a download button in acme-web. Rows stream, no paging.',
} as const;

interface HandoverFx extends Rec {
  id: string;
}

/** Handover draft of the story task, as the board carries it while the session finishes a stage. */
export function storyHandoverDraft(): HandoverFx {
  const board = contract<{ tasks: { handoverDrafts: HandoverFx[] }[] }>(
    'ade-handover',
    'AdeTaskService.Board#draft',
  );
  const draft = board.tasks[0]?.handoverDrafts[0];
  if (!draft) throw new Error('contract board has no draft');
  return {
    ...draft,
    id: 'h_draft',
    taskId: STORY.taskId,
    fromStepId: 'plan',
    fromStepName: 'Plan from spec',
    toStepId: 'impl',
    toStepName: 'Implement',
    body: HANDOVER_BODIES.plan,
    updatedAt: 1,
  };
}

/** Handover chain of the story task: the draft first, then the earlier stage. */
export function storyHandovers(): { handovers: Rec[] } {
  const { handovers } = contract<{ handovers: Rec[] }>('ade-handover', 'AdeTaskService.Handovers');
  const [first] = handovers;
  return {
    handovers: [
      storyHandoverDraft(),
      {
        ...first,
        id: 'h_spec',
        taskId: STORY.taskId,
        fromStepId: 'spec',
        fromStepName: 'Spec',
        toStepId: 'plan',
        toStepName: 'Plan from spec',
        body: HANDOVER_BODIES.spec,
        createdAt: CAPTURE_NOW - 2 * 3_600_000,
        updatedAt: CAPTURE_NOW - 2 * 3_600_000,
      },
    ],
  };
}

/** Board whose story task carries the handover draft. */
export function storyBoardWithDraft() {
  const board = storyBoardFx();
  const task = board.tasks.find((t) => t.id === STORY.taskId);
  if (task) {
    task.handoverDrafts = [storyHandoverDraft()];
    task.runs = [];
  }
  return board;
}

const artifact = (id: string, kind: string, title: string, over: Rec = {}): Rec => ({
  createdAt: CAPTURE_NOW - 3_600_000,
  id,
  kind,
  ref: id,
  repo: '',
  sessionId: '',
  source: 'host',
  status: '',
  title,
  url: '',
  ...over,
});

/** Artifacts of the story task: PR, issue, proof, branches and a deployment. */
export function storyArtifacts(): { artifacts: Rec[]; gaps: Rec[] } {
  return {
    artifacts: [
      artifact('a_pr', 'changeRequest', 'Export orders as CSV', {
        repo: 'acme-api',
        status: 'open',
        url: 'https://forge.test/acme/acme-api/-/merge_requests/41',
      }),
      artifact('a_pr_web', 'changeRequest', 'Export button on the orders page', {
        repo: 'acme-web',
        status: 'open',
        url: 'https://forge.test/acme/acme-web/-/merge_requests/87',
      }),
      artifact('a_issue', 'issue', 'PAY-102', {
        source: 'jira',
        url: 'https://acme.atlassian.net/browse/PAY-102',
      }),
      artifact('a_doc', 'doc', 'CSV format notes', {
        source: 'user',
        url: 'https://wiki.example/orders-csv',
      }),
      artifact('a_branch_api', 'branch', STORY.branch, {
        repo: 'acme-api',
        source: 'git',
        status: 'pushed',
        url: 'https://forge.test/acme/acme-api/-/tree/feat/orders-csv-export',
      }),
      artifact('a_deploy', 'deployment', 'staging', {
        repo: 'acme-api',
        source: 'git',
        status: 'deployed',
      }),
    ],
    gaps: [],
  };
}
