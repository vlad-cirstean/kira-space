import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { archivedTaskFx, FIXED_NOW, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// Proof as an artifact chain in the task details panel. Contracts ade-proof-chain and proof-files;
// backend halves: flows/adeflow TestProofChain (proofchain_test.go) and flows/proofflow
// TestProofFiles. A proof is a folder proofs/<slug>/ of the task; the contract masks ids and times,
// so the specs fill them in.

const t = (id: string) => `[data-testid="${id}"]`;
const HOUR = 3_600_000;

interface ProofFx {
  proofId: string;
  name: string;
  slug: string;
  closedAt: number;
  sessionId: string;
  steps: { id: string; ordinal: number; view: string }[];
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
    steps: p.steps.map((st, i) => ({ ...st, id: `s${i + 1}` })),
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
async function openChain(page: Page, taskId = 'T_alerts'): Promise<void> {
  await openTask(page, taskId);
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-task-chain'))).toBeVisible();
}

// Proofs start collapsed: the steps render only once the toggle opens them.
async function expandProof(page: Page, slug = 'login-checks'): Promise<void> {
  const entry = page.locator(`${t('ade-proof-entry')}[data-slug="${slug}"]`);
  await entry.locator(t('ade-proof-toggle')).click();
  await expect(entry.locator(t('ade-proof-toggle'))).toHaveAttribute('aria-expanded', 'true');
}

const kinds = (page: Page) =>
  page
    .locator(`${t('ade-task-chain')} [data-kind]`)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));

test('Details order: links, then the task chain with the Timeline toggle in its header', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  const order = await page
    .locator(`${t('ade-details-tab')} > :is(${t('ade-links')}, ${t('ade-task-chain')})`)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid')));
  expect(order).toEqual(['ade-links', 'ade-task-chain']);
  await expect(
    page.locator(`${t('ade-task-chain')} ${t('ade-details-timeline-toggle')}`),
  ).toBeVisible();
});

test('contract: the chain mixes a note and a proof newest first', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    notesSnapshot('T_alerts', FIXED_NOW - HOUR),
    diagramStep,
  ]);
  await openChain(page);
  await expect.poll(() => kinds(page)).toEqual(['handover', 'proof']);
  await expandProof(page);
  const entry = page.locator(t('ade-proof-entry'));
  await expect(entry.locator(t('ade-proof-name'))).toHaveText('Login checks');
  await expect(entry.locator(t('ade-proof-path'))).toHaveText('proofs/login-checks');
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(2);
  await expect(entry.locator(`${t('ade-proof-step')}[data-view="table"]`)).toContainText(
    'Login checks rows',
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
  await expect.poll(() => kinds(page)).toEqual(['proof', 'handover', 'proof']);
  await expect(page.locator(t('ade-proof-name')).first()).toHaveText('Login checks');
  await expect(page.locator(t('ade-proof-name')).last()).toHaveText('Perf checks');
});

test('contract: the diagram step draws inline', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  await expandProof(page);
  const diagram = page.locator(`${t('ade-proof-diagram')} ${t('proof-diagram')}`);
  await expect(diagram).toHaveAttribute('data-diagram-type', 'flowchart');
  await expect(diagram.locator('svg text', { hasText: 'Check' }).first()).toBeVisible();
});

test('the diagram step draws at full height', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  await expandProof(page);
  const scroller = page.locator(`${t('ade-proof-diagram')} ${t('proof-diagram')} > div`);
  await expect(scroller.locator('svg')).toBeVisible();
  const fit = await scroller.evaluate((el) => ({
    clipped: el.scrollHeight > el.clientHeight + 1,
    overflowY: getComputedStyle(el).overflowY,
    svg: el.querySelector('svg')?.getBoundingClientRect().height ?? 0,
    height: el.getBoundingClientRect().height,
  }));
  expect(fit.clipped).toBe(false);
  expect(fit.height).toBeGreaterThanOrEqual(fit.svg);
  await expect(page.locator(t('ade-proof-diagram'))).not.toHaveCSS('overflow', 'hidden');
});

test('summary and step text select', async ({ relaunch }) => {
  const long = 'A long step description that must wrap instead of ending in an ellipsis '.repeat(6);
  const list = proofs().map((p) => ({
    ...p,
    summary: 'Summary of the login checks',
    steps: p.steps.map((st) => (st.view === 'table' ? { ...st, description: long } : st)),
  }));
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(list), diagramStep]);
  await openChain(page);
  await expandProof(page);
  const entry = page.locator(t('ade-proof-entry'));
  const step = entry.locator(`${t('ade-proof-step')}[data-view="table"]`);
  const description = step.locator('span.select-text').first();
  for (const target of [entry.locator('p.select-text'), description]) {
    await target.click({ clickCount: 3 });
    const text = ((await target.textContent()) ?? '').trim();
    expect(await page.evaluate(() => window.getSelection()?.toString() ?? '')).toContain(text);
  }
  const wrap = await description.evaluate((el) => ({
    ellipsis: getComputedStyle(el).textOverflow === 'ellipsis',
    clipped: el.scrollWidth > el.clientWidth + 1,
  }));
  expect(wrap).toEqual({ ellipsis: false, clipped: false });
});

test('the proof name opens the proof window of its task and folder', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    diagramStep,
  ]);
  await openChain(page);
  await expect(page.locator(t('ade-proof-open')).locator(t('ade-proof-name'))).toHaveText(
    'Login checks',
  );
  await page.locator(t('ade-proof-open')).click();
  await expect.poll(() => calls(control, IPC.proofOpenWindow)).toHaveLength(1);
  expect(calls(control, IPC.proofOpenWindow)[0]?.args).toEqual({
    taskId: 'T_alerts',
    slug: 'login-checks',
  });
});

test('a refused proof window toasts the error', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    diagramStep,
    { channel: IPC.proofOpenWindow, error: { code: 'E_INTERNAL', message: 'proof is gone' } },
  ]);
  await openChain(page);
  await page.locator(t('ade-proof-open')).click();
  await expect(page.locator('[data-sonner-toast]')).toContainText('proof is gone');
});

test('a proof starts collapsed: title and summary only', async ({ relaunch }) => {
  const list = proofs().map((p) => ({ ...p, summary: 'Summary of the login checks' }));
  const { window: page, control } = await openPlan(relaunch, [proofsSnapshot(list), diagramStep]);
  await openChain(page);
  const entry = page.locator(t('ade-proof-entry'));
  await expect(entry.locator(t('ade-proof-name'))).toHaveText('Login checks');
  await expect(entry.locator('p')).toHaveText('Summary of the login checks');
  await expect(entry.locator(t('ade-proof-toggle'))).toHaveAttribute('aria-expanded', 'false');
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(0);
  await expect(entry.locator(t('ade-proof-diagram'))).toHaveCount(0);
  await expect(entry.locator(t('ade-proof-open'))).toHaveCount(1);
  expect(calls(control, IPC.proofStep)).toHaveLength(0);
});

test('an expanded proof stays expanded across a tab switch', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(proofs()), diagramStep]);
  await openChain(page);
  await expandProof(page);
  const entry = page.locator(t('ade-proof-entry'));
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(2);
  await page.locator(t('ade-panel-tab-main')).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(2);
  await expect(entry.locator(t('ade-proof-toggle'))).toHaveAttribute('aria-expanded', 'true');
});

test('contract: a proof with image and data steps lists every step by its id', async ({
  relaunch,
}) => {
  const fx = contract<ProofFx[]>('proof-files', 'AdeTaskService.TaskProofs');
  const list = fx.map((p) => ({
    ...p,
    proofId: 'P_checkout',
    sessionId: '',
    closedAt: FIXED_NOW - HOUR,
    linkedAt: FIXED_NOW - HOUR,
    steps: p.steps.map((st, i) => ({ ...st, id: `s${i + 1}` })),
  }));
  const { window: page } = await openPlan(relaunch, [proofsSnapshot(list), diagramStep]);
  await openChain(page);
  await expandProof(page, 'checkout-fix');
  const entry = page.locator(t('ade-proof-entry'));
  await expect(entry.locator(t('ade-proof-path'))).toHaveText('proofs/checkout-fix');
  await expect(entry.locator(t('ade-proof-step'))).toHaveCount(3);
  await expect(entry.locator(`${t('ade-proof-step')}[data-view="image"]`)).toHaveCount(1);
});

test('Delete asks first, then removes the proof folder; no attach entry point exists', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    proofsSnapshot(proofs()),
    diagramStep,
    { channel: IPC.proofDelete },
  ]);
  await openChain(page);
  await expect(page.locator(t('ade-proof-attach'))).toHaveCount(0);
  await page.locator(t('ade-proof-remove')).click();
  await expect(page.locator(t('confirm-dialog-message'))).toContainText('Login checks');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.proofDelete)).toHaveLength(1);
  expect(calls(control, IPC.proofDelete)[0]?.args).toEqual({ sessionId: 'P_login' });
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

test('an archived task shows its proof with no delete button', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx('H_1') },
    proofsSnapshot(proofs()),
    diagramStep,
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-proof-entry'))).toHaveCount(1);
  await expect(page.locator(t('ade-proof-remove'))).toHaveCount(0);
});
