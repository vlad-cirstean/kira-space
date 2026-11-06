import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Proof as an artifact chain in the task details panel. Contract ade-proof-chain; backend half:
// flows/adeflow TestProofChain (proofchain_test.go). The contract masks ids and times, so the
// specs fill them in.

const t = (id: string) => `[data-testid="${id}"]`;
const HOUR = 3_600_000;

interface ProofFx {
  proofId: string;
  name: string;
  closedAt: number;
  sessionId: string;
  steps: { ordinal: number; view: string }[];
  [key: string]: unknown;
}

function proofs(closedAt = FIXED_NOW - 2 * HOUR): ProofFx[] {
  const fx = contract<{ proofs: ProofFx[] }>('ade-proof-chain', 'AdeTaskService.TaskProofs#chain');
  return fx.proofs.map((p) => ({
    ...p,
    proofId: 'P_login',
    sessionId: '',
    closedAt,
    linkedAt: closedAt,
  }));
}

/** A second proof with a table step only. */
function otherProof(closedAt: number): ProofFx {
  const base = proofs()[0] as ProofFx;
  return {
    ...base,
    proofId: 'P_other',
    name: 'Perf checks',
    closedAt,
    linkedAt: closedAt,
    steps: base.steps.filter((s) => s.view !== 'diagram'),
  };
}

function note(taskId: string, createdAt: number) {
  const fx = contract<{ handovers: Record<string, unknown>[] }>(
    'ade-proof-chain',
    'AdeTaskService.Handovers#chain',
  );
  return fx.handovers.map((h, i) => ({
    ...h,
    id: `h_${i}`,
    taskId,
    createdAt,
    updatedAt: createdAt,
  }));
}

const proofsSnapshot = (list: ProofFx[]): ControlSnapshot => ({
  channel: IPC.adeTaskProofs,
  response: { proofs: list },
});
const notesSnapshot = (taskId: string, createdAt: number): ControlSnapshot => ({
  channel: IPC.adeTaskHandovers,
  response: { handovers: note(taskId, createdAt) },
});
const diagramStep: ControlSnapshot = {
  channel: IPC.proofStep,
  response: contract('ade-proof-chain', 'ProofService.Step#diagram'),
};
const choices = (): ControlSnapshot => {
  const fx = contract<{ proofs: Record<string, unknown>[] }>(
    'ade-proof-chain',
    'AdeTaskService.ProofChoices',
  );
  return {
    channel: IPC.adeTaskProofChoices,
    response: {
      proofs: fx.proofs.map((p) => ({ ...p, proofId: 'P_legacy', closedAt: FIXED_NOW - 9 * HOUR })),
    },
  };
};

async function openChain(page: Page, taskId = 'T_alerts'): Promise<void> {
  await openTask(page, taskId);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

const kinds = (page: Page) =>
  page
    .locator(`${t('ade-task-chain')} [data-kind]`)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));

test('Details order: links, the task chain, then the timeline', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  const order = await page
    .locator(
      `${t('ade-details-tab')} > :is(${t('ade-details-links')}, ${t('ade-task-chain')}, ${t('ade-details-timeline-toggle')})`,
    )
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid')));
  expect(order.slice(0, 2)).toEqual(['ade-details-links', 'ade-task-chain']);
  const chain = page.locator(t('ade-task-chain'));
  const toggle = page.locator(t('ade-details-timeline-toggle'));
  const [cy, ty] = await Promise.all([
    chain.boundingBox().then((b) => b?.y ?? 0),
    toggle.boundingBox().then((b) => b?.y ?? 0),
  ]);
  expect(cy).toBeLessThan(ty);
});

test('contract: the chain mixes a note and an agent proof newest first', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    notesSnapshot('T_alerts', FIXED_NOW - HOUR),
    diagramStep,
  ]);
  await openChain(page);
  expect(await kinds(page)).toEqual(['handover', 'proof']);
  const entry = page.locator(t('ade-proof-entry'));
  await expect(entry.locator(t('ade-proof-open'))).toHaveText('Login checks');
  await expect(entry.locator(t('ade-note-author'))).toHaveAttribute(
    'aria-label',
    'Written by an agent',
  );
  await expect(entry.locator(t('ade-note-stage'))).toContainText('During Design');
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(2);
  await expect(entry.locator(`${t('ade-proof-step')}[data-view="table"]`)).toContainText(
    'Login checks step 2',
  );
});

test('the same time orders a diagram proof, then the note, then other proof', async ({
  relaunch,
}) => {
  const at = FIXED_NOW - 2 * HOUR;
  const { window: page } = await openPlan(relaunch, [
    proofsSnapshot([otherProof(at), ...proofs(at)]),
    notesSnapshot('T_alerts', at),
    diagramStep,
  ]);
  await openChain(page);
  expect(await kinds(page)).toEqual(['proof', 'handover', 'proof']);
  await expect(page.locator(t('ade-proof-open')).first()).toHaveText('Login checks');
  await expect(page.locator(t('ade-proof-open')).last()).toHaveText('Perf checks');
});

test('contract: the diagram step draws inline', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  const diagram = page.locator(`${t('ade-proof-diagram')} ${t('proof-diagram')}`);
  await expect(diagram).toHaveAttribute('data-diagram-type', 'flowchart');
  await expect(diagram.locator('svg text', { hasText: 'Check' }).first()).toBeVisible();
});

test('the proof name opens the proof window on that session', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    diagramStep,
  ]);
  await openChain(page);
  await page.locator(t('ade-proof-open')).click();
  await expect.poll(() => calls(control, IPC.proofOpenWindow)).toHaveLength(1);
  expect(calls(control, IPC.proofOpenWindow)[0]?.args).toEqual({ sessionId: 'P_login' });
});

test('Attach proof lists unattached sessions; choosing one and removing one send the calls', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    choices(),
    diagramStep,
  ]);
  await openChain(page);
  await page.locator(t('ade-proof-attach')).click();
  const item = page.locator(t('ade-proof-attach-item'));
  await expect(item).toHaveCount(1);
  await expect(item).toContainText('Legacy');
  await item.click();
  await expect.poll(() => calls(control, IPC.adeTaskLinkProof)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskLinkProof)[0]?.args).toEqual({
    taskId: 'T_alerts',
    proofId: 'P_legacy',
  });
  await page.locator(t('ade-proof-remove')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUnlinkProof)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUnlinkProof)[0]?.args).toEqual({
    taskId: 'T_alerts',
    proofId: 'P_login',
  });
});

test('a proof changed push refetches the task proofs', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    diagramStep,
  ]);
  await openChain(page);
  await expect.poll(() => calls(control, IPC.adeTaskProofs).length).toBeGreaterThan(0);
  const before = calls(control, IPC.adeTaskProofs).length;
  await emitWailsEvent(page, IPC.proofChanged, { sessionId: 'P_login' });
  await expect
    .poll(() => calls(control, IPC.adeTaskProofs).length, { timeout: 15_000 })
    .toBeGreaterThan(before);
});

test('an archived task shows its proof with no attach or remove button', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_1') },
    proofsSnapshot(proofs()),
    diagramStep,
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-proof-entry'))).toHaveCount(1);
  await expect(page.locator(t('ade-proof-attach'))).toHaveCount(0);
  await expect(page.locator(t('ade-proof-remove'))).toHaveCount(0);
});
