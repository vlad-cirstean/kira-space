import type { ApprovalItem } from '@shared/domain/approvals';
import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import { liveItem } from '@workbench/testing/ui/approvals';
import { AGENT_PANEL_ON } from '../../ui/fixtures';
import { reviewStep } from '../../ui/support/adeReviewStep';
import { adeBoard, adeFixture, adeV2Control } from '../../ui/support/adeV2';
import { contract } from '../../ui/support/contract';
import type { GitStreamMockArgs } from '../../ui/support/gitStreamMock';
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
      b.worktree = `~/kira-space-wts/${b.id === STORY.webBranchId ? 'acme-web' : 'acme-api'}/orders-csv-export`;
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

export const REVIEW_AGENT = adeFixture<{ session: { terminalId: string } }>('review-agent');
const CSV_FILE = 'src/orders/exportCsv.ts';
const SHA = 'a'.repeat(40);

/** The story's source file at line `n`; the diff shows lines 97 to 103 and the comment sits on 12. */
const CODE: Record<number, string> = {
  1: "import { db } from '../db';",
  3: '',
  4: '/** Columns of the orders export, in file order. */',
  12: 'export const COLUMNS = [',
  97: '  const rows = await db.query(ORDERS_QUERY, [month]);',
  98: "  const header = COLUMNS.map((c) => c.title).join(',');",
  99: "  const lines = rows.map((row) => COLUMNS.map((c) => escapeCell(row[c.key])).join(','));",
  100: "  return [header, ...lines].join('\\n');",
  101: '}',
  102: '',
  103: 'export function escapeCell(value: unknown): string {',
};
const CHANGED_100 = `  return \`\${[header, ...lines].join('\\r\\n')}\\r\\n\`;`;

const codeLine = (n: number, changed: boolean) =>
  n === 100 && changed ? CHANGED_100 : (CODE[n] ?? '');

const blob = (changed: boolean) => ({
  content: Array.from({ length: 140 }, (_, i) => codeLine(i + 1, changed)).join('\n'),
});

function storyDiff() {
  const hunk = contract<{ body: { hunks: Rec[] } & Rec } & Rec>(
    'git-review-hunks',
    'git:review.fileDiff#range',
  );
  const text = (line: Rec) => ({
    ...line,
    text: codeLine(
      (line.newLine ?? line.oldLine) as number,
      line.kind === 'add' || line.kind === 'context',
    ),
  });
  return {
    ...hunk,
    path: CSV_FILE,
    body: {
      ...hunk.body,
      hunks: hunk.body.hunks.map((h) => ({
        ...h,
        heading: 'export function toCsv',
        lines: (h.lines as Rec[]).map(text),
      })),
    },
  };
}

const comment = (id: number, over: Rec = {}): Rec => ({
  id: String(id),
  path: CSV_FILE,
  range: { start: 12, end: 12 },
  body: '',
  anchor: 'exact',
  anchorSha: SHA,
  createdAt: CAPTURE_NOW - 3_600_000,
  kind: 'change',
  state: 'open',
  reply: '',
  repliedAt: null,
  sentAt: null,
  ...over,
});

export const STORY_COMMENTS: Rec[] = [
  comment(1, {
    range: { start: 100, end: 100 },
    body: 'Excel expects CRLF line ends and a trailing newline.',
    state: 'addressed',
    reply: 'Switched to CRLF and added the trailing newline.',
    sentAt: 1,
  }),
  comment(2, {
    kind: 'question',
    body: 'Do paid orders include refunded ones?',
    state: 'addressed',
    reply: 'No. Refunded orders have the status refunded.',
    sentAt: 1,
  }),
  comment(3, { body: 'Name the columns once, not per row.' }),
];

const REVIEW_FILES = {
  branchTip: SHA,
  mergeBase: 'b'.repeat(40),
  files: [
    {
      change: { kind: 'modified', path: CSV_FILE, similarity: 0 },
      review: { kind: 'none', changedSinceReview: false },
    },
    {
      change: { kind: 'modified', path: 'src/orders/OrdersPage.vue', similarity: 0 },
      review: { kind: 'full', changedSinceReview: false },
    },
    {
      change: { kind: 'added', path: 'src/orders/ExportButton.vue', similarity: 0 },
      review: { kind: 'full', changedSinceReview: false },
    },
  ],
};

/** Review window boot: control, git stream and storage. Pass `reviewStep` keys of contract ade-review-step. */
export function storyReview(key = 'answered'): {
  control: ControlSnapshot[];
  gitStream: GitStreamMockArgs;
  storage: Record<string, string>;
} {
  const target = {
    ...adeFixture<{ codeRepoId: string; gitRepoId: string }>('review-target'),
    branch: STORY.branch,
    worktree: '~/kira-space-wts/acme-web/orders-csv-export',
  };
  return {
    storage: AGENT_PANEL_ON,
    control: storyBoard([
      { channel: IPC.adeTaskReviewWindowTarget, response: target },
      { channel: IPC.adeTaskReviewAgent, response: REVIEW_AGENT },
      { channel: IPC.adeTaskReviewStep, response: reviewStep(key) },
      { channel: IPC.windowsEnsure, response: { mode: 'git' } },
      {
        channel: IPC.codeWorkspaceListRepos,
        response: [
          {
            id: target.codeRepoId,
            name: 'acme-web',
            root: '/tmp/acme-web',
            repoId: target.gitRepoId,
            sortOrder: 1,
            color: 'none',
            createdAt: '2026-01-01T00:00:00.000Z',
          },
        ],
      },
    ]),
    gitStream: {
      repoId: target.gitRepoId as string,
      extraResults: {
        'review.resolveBase': {
          branch: STORY.branch,
          base: 'main',
          reason: 'override',
          range: { kind: 'ready', commitCount: 3 },
          candidates: [],
        },
        'review.files': REVIEW_FILES,
        'review.session.load': null,
        'repo.open': null,
        'file.read': { __sequence: [blob(false), blob(true)] },
        'review.snapshot': { kind: 'none', text: null, reviewedAtSha: null },
        'review.fileDiff': storyDiff(),
        'review.comment.list': { at: SHA, comments: STORY_COMMENTS },
        'review.comment.setState': { changed: true },
        'changeRequest.reviewState': contract(
          'ade-review-actions',
          'git:changeRequest.reviewState',
        ),
        'changeRequest.submitReview': contract(
          'ade-review-actions',
          'git:changeRequest.submitReview',
        ),
      },
    },
  };
}

const PROOF_ID = 'orders-export';
const PROOF_PENDING_ID = 'orders-export-edit';
const HOUR = 3_600_000;
const DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

const proofStepFx = (base: string, key: string, ordinal: number, patch: Rec = {}): Rec => ({
  ...contract<Rec>(base, `ProofService.Step#${key}`),
  ordinal,
  ...patch,
});

const proofSteps = (changed: boolean): Rec[] => {
  const perDay = [412, 388, 455, 420, 502, 96, 71];
  return [
    proofStepFx('proof', 'table', 1, {
      description: 'Orders in the export',
      query: "select id, status, total, created_at from orders where status = 'paid' limit 8",
      data: {
        columns: [
          { name: 'id', type: 'number' },
          { name: 'status', type: 'text' },
          { name: 'total', type: 'number' },
          { name: 'created_at', type: 'text' },
        ],
        rows: [
          [10482, 'paid', 129.9, '2026-10-26 09:14'],
          [10483, 'paid', 54.0, '2026-10-26 09:31'],
          [10485, 'paid', 18.5, '2026-10-26 10:02'],
          [10488, 'paid', 249.0, '2026-10-26 10:47'],
          [10491, 'paid', 76.25, '2026-10-26 11:20'],
          [10494, 'paid', 33.0, '2026-10-26 11:58'],
          [10496, 'paid', 412.8, '2026-10-26 12:15'],
          [10499, 'paid', 61.4, '2026-10-26 12:40'],
        ],
      },
    }),
    proofStepFx('proof-chart', 'bar', 2, {
      description: 'Exports per day',
      source: 'postgres',
      data: {
        horizontal: false,
        stacked: false,
        type: 'bar',
        x: { name: 'day', type: 'category' },
        y: { name: 'exports', unit: '' },
        series: [
          {
            name: 'exports',
            points: DAYS.map((d, i) => [d, i === 4 && changed ? 540 : perDay[i]]),
          },
        ],
      },
    }),
    proofStepFx('proof-diagram', 'flowchart', 3, {
      description: 'Export flow',
      data: {
        type: 'flowchart',
        source:
          'flowchart LR\n  Button[Export button] --> Route[GET /orders.csv]\n  Route --> Query[(orders)]\n  Query --> Csv[CSV file]',
      },
    }),
    proofStepFx('proof', 'keyvalue', 4, {
      description: 'Export summary',
      data: {
        entries: [
          { field: 'rows', value: 84000 },
          { field: 'durationMs', value: 1840 },
          { field: 'fileKb', value: 6120 },
        ],
      },
    }),
  ];
};

const proofSummary = (s: Rec, diff: string): Rec => ({
  bytes: s.bytes,
  description: s.description,
  diff,
  language: s.language,
  ordinal: s.ordinal,
  originOrdinal: diff === 'added' ? null : s.ordinal,
  source: s.source,
  view: s.view,
});

function proofRev(rev: number, changed: boolean, diffs: string[]): Rec {
  return {
    createdAt: CAPTURE_NOW - 3 * HOUR,
    reason: changed ? 'Friday spike checked against the audit log' : '',
    rev,
    steps: proofSteps(changed).map((s, i) => proofSummary(s, diffs[i] as string)),
  };
}

const CLOSED = CAPTURE_NOW - 2 * HOUR;

/** Proof window boot for the story: one closed session, or a pending edit when `pending`. */
export function storyProof(pending = false): ControlSnapshot[] {
  const base = contract<Rec>('proof', 'ProofService.Session');
  const id = pending ? PROOF_PENDING_ID : PROOF_ID;
  const added = ['added', 'added', 'added', 'added'];
  const session: Rec = {
    ...base,
    id,
    name: 'Orders export',
    closedAt: CLOSED,
    current: proofRev(1, false, added),
    pending: pending ? proofRev(2, true, ['unchanged', 'changed', 'unchanged', 'unchanged']) : null,
  };
  const step = (rev: number, s: Rec): ControlSnapshot => ({
    channel: IPC.proofStep,
    args: { sessionId: id, rev, ordinal: s.ordinal },
    response: s,
  });
  return [
    { channel: IPC.proofWindowTarget, response: { sessionId: id } },
    {
      channel: IPC.proofSessions,
      response: [{ id, name: 'Orders export', closedAt: CLOSED, steps: 4, pending, seen: true }],
    },
    { channel: IPC.proofSession, response: session },
    ...proofSteps(false).map((s) => step(1, s)),
    ...(pending
      ? proofSteps(true).map((s) =>
          step(2, { ...s, diff: s.ordinal === 2 ? 'changed' : 'unchanged' }),
        )
      : []),
  ];
}

/** Task Details chain: one agent proof plus the story's handover notes. */
export function storyProofChain(): ControlSnapshot[] {
  const steps = proofSteps(false).filter((s) => s.view !== 'diagram');
  return [
    {
      channel: IPC.adeTaskProofs,
      response: {
        proofs: [
          {
            closedAt: CLOSED,
            linkedAt: CLOSED,
            name: 'Orders export',
            pending: false,
            proofId: 'P_orders',
            rev: 1,
            sessionId: '',
            source: 'agent',
            stepId: 'impl',
            stepName: 'Implement',
            steps: steps.map((s) => ({
              description: s.description,
              ordinal: s.ordinal,
              source: s.source,
              view: s.view,
            })),
            summary: '',
          },
        ],
      },
    },
    { channel: IPC.adeTaskHandovers, ...{ response: storyHandovers() } },
  ];
}
