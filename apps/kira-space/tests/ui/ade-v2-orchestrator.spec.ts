import type { Page } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { answeredOk, liveItem } from '@workbench/testing/ui/approvals';
import type { ContractJson } from '@workbench/testing/ui/contract';
import { expect, test } from './fixtures';
import { adeBoard, adeFixture, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Orchestrator tasks: kind choice, card and children group, parent links, the Run next pass dialog,
// child rows in the chain and the task.create approval body. Contract ade-orchestrator; backend
// half: flows/adeflow TestOrchestrator (orchestrator_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const ORCH = 'T_orch';
const CHILD = 'T_child';

type Json = ContractJson;

interface BoardFx {
  tasks: Json[];
  statuses: Json[];
  repos: Json[];
  [key: string]: unknown;
}

/** The contract board with real ids: both tasks on the fixture day, no branches. `edit` adjusts them. */
function orchestratorBoard(edit: (orch: Json, child: Json) => void = () => {}) {
  const fx = contract<BoardFx>('ade-orchestrator', 'AdeTaskService.Board#orchestrator');
  const base = adeFixture<BoardFx>('board');
  return adeBoard((board) => {
    const [orch, child] = fx.tasks as [Json, Json];
    orch.id = ORCH;
    orch.workflowId = 'orch';
    orch.workflow = { ...orch.workflow, id: 'orch' };
    orch.workflow.steps[0].id = 'plan';
    for (const r of orch.runs) Object.assign(r, { id: 'run_orch', taskId: ORCH, stepId: 'plan' });
    child.id = CHILD;
    child.parentId = ORCH;
    child.handoverIn = { ...child.handoverIn, taskId: CHILD, fromTaskId: ORCH };
    for (const task of [orch, child]) {
      task.createdAt = FIXED_NOW - 864e5;
      task.branchIds = [];
    }
    edit(orch, child);
    board.tasks = [orch, child] as typeof board.tasks;
    board.statuses = base.statuses;
    board.repos = base.repos;
    board.branches = [];
    board.history = [];
    board.plan = {
      backlog: [],
      day: { [ORCH]: '2026-09-22', [CHILD]: '2026-09-22' },
      order: [ORCH, CHILD],
      queuedAfter: {},
      unpushed: {},
    };
  });
}

const boardSnapshot = (board = orchestratorBoard()): ControlSnapshot => ({
  channel: IPC.adeTaskBoard,
  response: board,
});

async function openDetails(page: Page, taskId: string): Promise<void> {
  await openTask(page, taskId);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

function taskCreateItem(): ApprovalItem {
  const [it] = contract<ApprovalItem[]>('ade-orchestrator', 'ApprovalsService.List#taskCreate');
  if (!it) throw new Error('contract has no item');
  return liveItem(it, 1800, FIXED_NOW);
}

test('task.create approval shows title, parent, workflow, repos and brief', async ({
  relaunch,
}) => {
  const it = taskCreateItem();
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('approval-origin'))).toContainText('Ship login');
  await expect(dialog.locator(t('approval-target-name'))).toHaveText('API login');
  const body = dialog.locator(t(`approval-item-${it.id}`));
  await expect(body.locator(t('ade-task-create-workflow'))).toContainText('Flow');
  const repo = body.locator(t('ade-task-create-repo'));
  await expect(repo).toHaveCount(1);
  await expect(repo).toContainText('api');
  await expect(body.locator(t('ade-task-create-base'))).toContainText('main');
  await expect(body.locator(t('ade-task-create-brief'))).toContainText(
    'Wire the login form to the API.',
  );
  await expect(body.locator(t('ade-task-create-brief'))).toContainText(
    'Hand back the endpoint list.',
  );
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toMatchObject({
    ids: [it.id],
    approve: true,
  });
});

test('orchestrator card groups children and child links to parent', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [boardSnapshot()]);
  const orch = page.locator(`${t('ade-card')}[data-task-id="${ORCH}"]`);
  await expect(orch.locator(t('ade-orchestrator-badge'))).toBeVisible();
  const children = orch.locator(t('ade-card-children'));
  await expect(children).toBeVisible();
  const row = children.locator(`${t('ade-card-child')}[data-task-id="${CHILD}"]`);
  await expect(row).toContainText('API login');
  await expect(children.locator(t('ade-card-child'))).toHaveCount(1);

  const child = page.locator(`${t('ade-card')}[data-task-id="${CHILD}"]`);
  const chip = child.locator(t('ade-parent-chip'));
  await expect(chip).toContainText('Ship login');
  await chip.click();
  await expect(orch).toHaveAttribute('aria-current', 'true');

  await row.click();
  await expect(child).toHaveAttribute('aria-current', 'true');
  await expect(page.locator(t('ade-task-parent-row'))).toContainText('Ship login');
});

test('finished orchestrator Run opens pass dialog and sends message', async ({ relaunch }) => {
  const args = contract<Json>('ade-orchestrator', 'args:AdeTaskService.NextPass');
  const board = orchestratorBoard((orch) => {
    orch.entryStepId = 'done';
    orch.statusId = 'done';
    orch.runs = [];
  });
  const { window: page, control } = await openPlan(relaunch, [
    boardSnapshot(board),
    {
      channel: IPC.adeTaskNextPass,
      response: contract('ade-orchestrator', 'AdeTaskService.NextPass'),
    },
  ]);
  const orch = page.locator(`${t('ade-card')}[data-task-id="${ORCH}"]`);
  await expect(orch.getByLabel('Pass done')).toBeVisible();
  await orch.locator(t('ade-task-action-nextPass')).click();
  const dialog = page.locator(t('ade-stage-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('ade-stage-pass-target'))).toContainText('Plan');
  await dialog.locator(`${t('ade-stage-notes')} .ProseMirror`).fill(args.notes);
  await dialog.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskNextPass)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskNextPass)[0]?.args).toEqual({
    ...args,
    taskId: ORCH,
    fromStepId: 'done',
  });
  await expect(dialog).toBeHidden();
});

test('parent chain renders child handover with link', async ({ relaunch }) => {
  const fx = contract<{ handovers: Json[] }>('ade-orchestrator', 'AdeTaskService.Handovers#parent');
  const handovers = fx.handovers.map((h, i) => ({
    ...h,
    id: `h_${i}`,
    taskId: ORCH,
    ...(h.source === 'child' ? { fromTaskId: CHILD } : {}),
  }));
  const { window: page } = await openPlan(relaunch, [
    boardSnapshot(),
    { channel: IPC.adeTaskHandovers, response: { handovers } },
  ]);
  await openDetails(page, ORCH);
  const rows = page.locator(`${t('ade-task-chain')} ${t('ade-note-row')}[data-source="child"]`);
  await expect(rows).toHaveCount(2);
  await expect(
    page.locator(t('ade-task-chain')).getByText('Endpoints: POST /login.'),
  ).toBeVisible();
  await expect(rows.locator(t('ade-handover-edit'))).toHaveCount(0);
  await expect(rows.locator(t('ade-handover-child-empty')).first()).toContainText(
    'Finished without a handover',
  );
  const link = rows.locator(t('ade-handover-child-link'));
  await expect(link.first()).toBeVisible();
  await link.first().click();
  await expect(page.locator(`${t('ade-card')}[data-task-id="${CHILD}"]`)).toHaveAttribute(
    'aria-current',
    'true',
  );
});

test('add popover creates orchestrator task', async ({ relaunch }) => {
  const created = { ...adeFixture<Json>('task'), id: ORCH, kind: 'orchestrator' };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskCreateTask, response: created },
  ]);
  await page.locator(t('ade-add')).click();
  await page.locator(t('ade-nw-title')).fill('Ship login');
  await page.locator(t('ade-nw-kind-orchestrator')).click();
  await page.locator(t('ade-nw-add')).click();
  await expect.poll(() => calls(control, IPC.adeTaskCreateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskCreateTask)[0]?.args).toMatchObject({
    title: 'Ship login',
    kind: 'orchestrator',
  });
});
