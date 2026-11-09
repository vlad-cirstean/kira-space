import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { adeFixture, openWorkflows, saved } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// Readable file and step names: a Form save of a step or workflow follows the renamed file, and a
// step added in the graph takes its id from its name until the first save. The payloads come from the
// Go flow test (contract ade-readable-names).

const t = (id: string) => `[data-testid="${id}"]`;

interface Listing {
  steps: { fileName: string; hash: string }[];
  workflows: { fileName: string }[];
  [key: string]: unknown;
}
interface SavedWorkflow {
  fileName: string;
  workflow: {
    steps: { id: string; name: string; results: { id: string; next: string }[] }[];
  };
}

const before = contract<Listing>('ade-readable-names', 'AdeTaskService.Workflows#before');
const renamedList = contract<Listing>('ade-readable-names', 'AdeTaskService.Workflows#renamed');
const renamedStep = contract<{ fileName: string }>(
  'ade-readable-names',
  'AdeTaskService.SaveStep#renamed',
);
const renamedWorkflow = contract<{ fileName: string; workflow: { name: string } }>(
  'ade-readable-names',
  'AdeTaskService.SaveWorkflow#renamed',
);

const node = (page: Page, step: string) =>
  page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`);

test('saving a step under a new name follows the renamed file', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: before },
    { channel: IPC.adeTaskSaveStep, response: renamedStep },
  ]);
  await page.locator(t('ade-wf-tab-steps')).click();
  await page.locator(`${t('ade-step-row')}[data-file="new-step.yaml"]`).click();
  await expect(page.locator(t('ade-step-file'))).toContainText('new-step.yaml');
  await page.locator(t('ade-wf-step-name')).fill('Lint repo');
  control.respond(IPC.adeTaskWorkflows, renamedList);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSaveStep)[0]?.args).toMatchObject({
    fileName: 'new-step.yaml',
    step: { id: 'new-step', name: 'Lint repo' },
  });
  await expect(page.locator(t('ade-step-file'))).toContainText('lint-repo.yaml');
  await expect(page.locator(t('ade-wf-step-name'))).toHaveValue('Lint repo');
  await expect(
    page.locator(`${t('ade-step-row')}[data-file="lint-repo.yaml"] ${t('ade-step-row-name')}`),
  ).toHaveText('Lint repo');
});

test('a new step is named by its kind and its id follows the name until it is saved', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  await expect(node(page, 'agent-step')).toBeVisible();
  await expect(page.locator(t('ade-wf-step-name'))).toHaveValue('Agent step');

  // A route to the step follows its id when the name changes it.
  await node(page, 'pr').click();
  const failed = page.locator(`${t('ade-wf-result')}[data-result="failed"]`);
  await failed.locator(t('ade-wf-result-route')).selectOption('agent-step');
  await node(page, 'agent-step').click();
  await page.locator(t('ade-wf-step-name')).fill('Lint');
  await expect(node(page, 'lint')).toBeVisible();
  await expect(node(page, 'agent-step')).toHaveCount(0);
  await expect(page.locator(t('ade-wf-step-name'))).toHaveValue('Lint');

  const first = await saved<SavedWorkflow>(page, control, 1);
  const ids = first.workflow.steps.map((s) => s.id);
  expect(ids.at(-1)).toBe('lint');
  expect(ids.some((id) => id.startsWith('stp_') || id === 'agent-step')).toBe(false);
  expect(first.workflow.steps.find((s) => s.id === 'pr')?.results).toContainEqual(
    expect.objectContaining({ id: 'failed', next: 'lint' }),
  );

  // Saved: a later rename keeps the id.
  await node(page, 'lint').click();
  await page.locator(t('ade-wf-step-name')).fill('Lint again');
  const second = await saved<SavedWorkflow>(page, control, 2);
  expect(second.workflow.steps.at(-1)).toMatchObject({ id: 'lint', name: 'Lint again' });
});

test('saving a workflow under a new name stays on the renamed file', async ({ relaunch }) => {
  const fx = adeFixture<{ workflows: unknown[] }>('workflows');
  const { window: page, control } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskSaveWorkflow, response: renamedWorkflow },
  ]);
  const name = page.locator(t('ade-wf-form-name'));
  await name.fill('Release');
  control.respond(IPC.adeTaskWorkflows, {
    ...fx,
    workflows: [renamedWorkflow, ...fx.workflows.slice(1)],
  });
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  await expect(page.locator(t('ade-wf-form-file'))).toContainText('release.yaml');
  await expect(page.locator(t('ade-wf-form-name'))).toHaveValue(renamedWorkflow.workflow.name);
});
