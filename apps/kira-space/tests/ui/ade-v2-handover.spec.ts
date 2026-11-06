import type { Page } from '@playwright/test';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import { type AdeBoardFx, adeBoard, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Stage handover: the routed prompt, the stage dialog, the Details notes chain and the Run dialog block.
// Contract ade-handover; backend half: flows/adeflow TestStageHandover (handover_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface HandoverFx {
  id: string;
  taskId: string;
  fromStepId: string;
  fromStepName: string;
  toStepId: string;
  toStepName: string;
  body: string;
  source: string;
  state: string;
  finished: boolean;
  dismissed: boolean;
  edited: boolean;
  createdAt: number;
  updatedAt: number;
}

const BODY = 'Form wired; validation missing.';

function recorded(): { draft: HandoverFx; handed: HandoverFx[] } {
  const board = contract<{ tasks: { handoverDrafts: HandoverFx[] }[] }>(
    'ade-handover',
    'AdeTaskService.Board#draft',
  );
  const draft = board.tasks[0]?.handoverDrafts[0];
  if (!draft) throw new Error('contract board has no draft');
  const history = contract<{ handovers: HandoverFx[] }>('ade-handover', 'AdeTaskService.Handovers');
  return { draft, handed: history.handovers };
}

/** T_alerts sits on its manual Spec step, with the session's finished draft. */
function withDraft(b: AdeBoardFx): void {
  const alerts = b.tasks.find((x) => x.id === 'T_alerts');
  if (!alerts) throw new Error('T_alerts missing');
  alerts.handoverDrafts = [
    {
      ...recorded().draft,
      id: 'h_draft',
      taskId: 'T_alerts',
      fromStepId: 'spec',
      fromStepName: 'Spec',
      updatedAt: 1,
    },
  ];
}

function snapshots(edit: (b: AdeBoardFx) => void): ControlSnapshot[] {
  const { handed } = recorded();
  return [
    { channel: IPC.adeTaskBoard, response: adeBoard(edit) },
    {
      channel: IPC.adeTaskHandovers,
      response: {
        handovers: [
          {
            ...recorded().draft,
            id: 'h_draft',
            taskId: 'T_alerts',
            fromStepId: 'spec',
            fromStepName: 'Spec',
          },
          ...handed.map((h, i) => ({ ...h, id: `h_${i}`, taskId: 'T_alerts' })),
        ],
      },
    },
  ];
}

async function openRouted(
  relaunch: Parameters<typeof openPlan>[0],
): Promise<{ page: Page; control: Awaited<ReturnType<typeof openPlan>>['control'] }> {
  const entry = routed('handover', 'T_alerts');
  const { window: page, control } = await openPlan(relaunch, [
    ...snapshots(withDraft),
    { channel: IPC.promptsList, response: [entry] },
  ]);
  await emitPrompts(page, [entry]);
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  return { page, control };
}

test('contract: the routed handover prompt shows the draft and Move sends StepDone with edited notes', async ({
  relaunch,
}) => {
  const { page, control } = await openRouted(relaunch);
  const notes = page.locator(t('ade-stage-notes')).locator('.ProseMirror');
  await expect(notes).toHaveText(BODY);
  await expect(page.locator(t('ade-stage-not-now'))).toBeVisible();

  await notes.fill(`${BODY} Check C.`);
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({
    taskId: 'T_alerts',
    notes: `${BODY} Check C.`,
  });
});

test('Move with the draft untouched sends no notes', async ({ relaunch }) => {
  const { page, control } = await openRouted(relaunch);
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStepDone)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStepDone)[0]?.args).toEqual({ taskId: 'T_alerts' });
});

test('Not now keeps the draft and sends DismissHandover', async ({ relaunch }) => {
  const { page, control } = await openRouted(relaunch);
  await page.locator(t('ade-stage-not-now')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDismissHandover)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDismissHandover)[0]?.args).toEqual({ taskId: 'T_alerts' });
  expect(calls(control, IPC.adeTaskStepDone)).toHaveLength(0);
  await expect(page.locator(t('ade-stage-dialog'))).toBeHidden();
});

test('the step mover opens the dialog and sends SetTaskStep with the typed notes', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, snapshots(withDraft));
  await openTask(page, 'T_auth');
  await page.locator(t('ade-step-back')).click();
  const notes = page.locator(t('ade-stage-notes')).locator('.ProseMirror');
  await expect(notes).toHaveText('');
  await notes.fill('Redo the PR text.');
  await page.locator(t('ade-stage-move')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSetTaskStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSetTaskStep)[0]?.args).toMatchObject({
    taskId: 'T_auth',
    stepId: 'pr',
    notes: 'Redo the PR text.',
  });
});

test('Task tab shows no handover; Notes lists the draft and moves; an edit sends UpdateHandover', async ({
  relaunch,
}) => {
  const { handed } = recorded();
  const { window: page, control } = await openPlan(relaunch, snapshots(withDraft));
  await openTask(page, 'T_alerts');
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  await expect(page.locator(t('ade-main-tab'))).not.toContainText(BODY);
  await expect(page.locator(t('ade-task-handover-row'))).toHaveCount(0);
  await page.locator(t('ade-panel-tab-details')).click();
  const list = page.locator(t('ade-notes-chain'));
  await expect(list.locator(t('ade-handover-draft'))).toContainText(BODY);
  await expect(list.locator(t('ade-handover-row'))).toHaveCount(handed.length);
  await expect(list.locator(t('ade-handover-row')).first()).toContainText(handed[0]?.body ?? '');

  await list.locator(t('ade-handover-row')).first().locator(t('ade-handover-edit')).click();
  await list.locator(t('ade-handover-text')).locator('.ProseMirror').fill('Corrected note.');
  await list.locator(t('ade-handover-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateHandover)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateHandover)[0]?.args).toEqual({
    id: 'h_0',
    body: 'Corrected note.',
  });
});

test('Move on in the Notes tab opens the stage dialog', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots(withDraft));
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await page.locator(t('ade-handover-move-on')).click();
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  await expect(page.locator(t('ade-stage-not-now'))).toHaveCount(0);
});

test('the Run dialog shows the handover the step gets', async ({ relaunch }) => {
  const { handed } = recorded();
  const { window: page } = await openPlan(
    relaunch,
    snapshots((b) => {
      const alerts = b.tasks.find((x) => x.id === 'T_alerts');
      if (!alerts) throw new Error('T_alerts missing');
      alerts.entryStepId = 'plan';
      alerts.workflowId = 'standard';
      alerts.runs = [];
      alerts.notes = 'Mind the rate limit.';
      alerts.handoverIn = { ...(handed[0] as HandoverFx), toStepId: 'plan', fromStepName: 'Spec' };
    }),
  );
  await page
    .locator(`${t('ade-task')}[data-task-id="T_alerts"] ${t('ade-task-action-run')}`)
    .click();
  const dialog = page.locator(t('ade-run-dialog'));
  await expect(dialog.locator(t('ade-run-handover'))).toContainText('Handover from Spec');
  await expect(dialog.locator(t('ade-run-handover'))).toContainText(handed[0]?.body ?? '');
  await expect(dialog.locator(t('ade-run-notes'))).toContainText('Mind the rate limit.');
});
