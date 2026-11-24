import { expect, test } from './fixtures';
import { openDetails, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';

// Details auto-update: a board push alone refreshes the disk banner and the task proofs, with no
// focus, Refresh or proof event. Contract ade-details-autoupdate; backend half: flows/adeflow
// TestDetailsAutoUpdateIdleSave and TestDetailsAutoUpdateAgentProofEdit (detailsautoupdate_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const TASK = 'T_alerts';
const SCENARIO = 'ade-details-autoupdate';

interface ProofFx {
  proofId: string;
  sessionId: string;
  steps: { id: string; view: string }[];
  [key: string]: unknown;
}

const board = (key: string) => contract<{ refs: boolean; taskIds: string[] }>(SCENARIO, key);

/** The proof as the chain fixture holds it (two steps, rev 1). */
const oldProofs = () => {
  const fx = contract<{ proofs: ProofFx[] }>('ade-proof-chain', 'AdeTaskService.TaskProofs#chain');
  return {
    proofs: fx.proofs.map((p) => ({
      ...p,
      proofId: 'P_login',
      sessionId: '',
      steps: p.steps.map((s, i) => ({ ...s, id: `s${i + 1}` })),
    })),
  };
};

/** The proof after the agent's edit: same rev (accepted edits rewrite it in place), one step. */
const editedProofs = () => {
  const fx = contract<{ proofs: ProofFx[] }>(SCENARIO, 'AdeTaskService.TaskProofs#edited');
  return {
    proofs: fx.proofs.map((p) => ({
      ...p,
      proofId: 'P_login',
      sessionId: '',
      steps: p.steps.map((s, i) => ({ ...s, id: `s${i + 1}` })),
    })),
  };
};

test('contract: an outside save shows in the disk banner without focus or Refresh', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskDetailsDiskChanges, response: [] },
  ]);
  await openDetails(page, TASK);
  await expect(page.locator(t('ade-details-disk'))).toBeVisible();
  await expect(page.locator(t('ade-details-disk-banner'))).toHaveCount(0);

  const changes = contract<{ path: string }[]>(SCENARIO, 'AdeTaskService.DetailsDiskChanges#idle');
  control.respond(IPC.adeTaskDetailsDiskChanges, changes);
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, board('event:kira:adetask:board'));
  await expect(page.locator(t('ade-details-disk-banner'))).toBeVisible();
  await page.locator(t('ade-details-disk-review')).click();
  await expect(page.locator(t('ade-details-disk-file'))).toContainText(changes[0]?.path ?? '');
});

test('contract: a board push refetches the task proofs and redraws the edited proof', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskProofs, response: oldProofs() },
    {
      channel: IPC.proofStep,
      response: contract('ade-proof-chain', 'ProofService.Step#diagram'),
    },
  ]);
  await openTask(page, TASK);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
  const steps = page.locator(`${t('ade-task-chain')} ${t('ade-proof-step')}`);
  await expect(steps).toHaveCount(2);
  await expect(page.locator(`${t('ade-proof-step')}[data-view="diagram"]`)).toHaveCount(1);

  const before = calls(control, IPC.adeTaskProofs).length;
  control.respond(IPC.adeTaskProofs, editedProofs());
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, board('event:kira:adetask:board#proofEdit'));
  await expect.poll(() => calls(control, IPC.adeTaskProofs).length).toBeGreaterThan(before);
  await expect(steps).toHaveCount(1);
  await expect(page.locator(`${t('ade-proof-step')}[data-view="diagram"]`)).toHaveCount(0);
});

test('a board push for another task does not refetch this task proofs', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskProofs, response: oldProofs() },
    {
      channel: IPC.proofStep,
      response: contract('ade-proof-chain', 'ProofService.Step#diagram'),
    },
  ]);
  await openTask(page, TASK);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(`${t('ade-task-chain')} ${t('ade-proof-step')}`)).toHaveCount(2);
  const before = calls(control, IPC.adeTaskProofs).length;
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, { refs: false, taskIds: ['T_other'] });
  await page.waitForTimeout(500);
  expect(calls(control, IPC.adeTaskProofs).length).toBe(before);
});

test('a board push refetches the open diagram step: an accepted edit keeps the rev', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskProofs, response: oldProofs() },
    {
      channel: IPC.proofStep,
      response: contract('ade-proof-chain', 'ProofService.Step#diagram'),
    },
  ]);
  await openTask(page, TASK);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-proof-diagram'))).toBeVisible();
  const before = calls(control, IPC.proofStep).length;
  await emitWailsEvent(page, IPC.adeTaskBoardChanged, board('event:kira:adetask:board#proofEdit'));
  await expect.poll(() => calls(control, IPC.proofStep).length).toBeGreaterThan(before);
});
