import type { Page } from '@playwright/test';
import { codeEditorText, replaceCodeEditorText } from '@workbench/testing/ui/codeEditor';
import { expect, test } from './fixtures';
import { openWorkflows } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The step library: the Workflows page tabs, the Steps tab, and library steps in the workflow editor.
// The payloads come from the Go flow test (contract ade-step-library).

const t = (id: string) => `[data-testid="${id}"]`;

interface LibStep {
  id: string;
  name: string;
  timeout: string;
  prompt: string;
  promptExtra?: string;
  use?: string;
  results: { id: string; next: string }[];
}
interface Listing {
  steps: { fileName: string; hash: string; usedBy: string[]; step: LibStep }[];
  workflows: { fileName: string; workflow: { steps: LibStep[] } }[];
  [key: string]: unknown;
}

const listing = contract<Listing>('ade-step-library', 'AdeTaskService.Workflows#steps');
const withLibrary: ControlSnapshot[] = [{ channel: IPC.adeTaskWorkflows, response: listing }];
const unused: ControlSnapshot[] = [
  {
    channel: IPC.adeTaskWorkflows,
    response: { ...listing, steps: listing.steps.map((s) => ({ ...s, usedBy: [] })) },
  },
];

const node = (page: Page, step: string) =>
  page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`);

async function openSteps(page: Page): Promise<void> {
  await page.locator(t('ade-wf-tab-steps')).click();
  await page.locator(t('ade-steps-tab')).waitFor();
}

test('the tabs read Statuses, Steps, Workflows; Workflows opens first and a switch asks about unsaved edits', async ({
  relaunch,
}) => {
  const { window: page } = await openWorkflows(relaunch, withLibrary);
  await expect(page.locator(`${t('ade-wf-tabs')} button`)).toHaveText([
    'Statuses',
    'Steps',
    'Workflows',
  ]);
  await expect(page.locator(t('ade-wf-tab-workflows'))).toHaveAttribute('data-state', 'on');
  await page.locator(t('ade-wf-form-name')).fill('Edited');
  await page.locator(t('ade-wf-tab-steps')).click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('ade-wf-form-name'))).toHaveValue('Edited');
  await page.locator(t('ade-wf-tab-steps')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-steps-tab'))).toBeVisible();
  await page.locator(t('ade-wf-tab-statuses')).click();
  await expect(page.locator(t('ade-status-list'))).toBeVisible();
});

test('the Steps list shows each library step with its kind and users', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  const row = page.locator(t('ade-step-row'));
  await expect(row).toHaveCount(1);
  await expect(row.locator(t('ade-step-row-name'))).toHaveText('Implement');
  await expect(row.locator(t('ade-chip'))).toHaveText('Agent step');
  await expect(row.locator(t('ade-step-row-used'))).toHaveText('Used by 1 workflow');
});

test('a used step cannot be deleted and lists the workflows that use it', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  await expect(page.locator(t('ade-step-delete'))).toBeDisabled();
  await expect(page.locator(t('ade-step-used-by'))).toContainText('Library flow');
  expect(calls(control, IPC.adeTaskDeleteStep)).toHaveLength(0);
});

test('an unused step deletes after asking', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, unused);
  await openSteps(page);
  await page.locator(t('ade-step-delete')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDeleteStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDeleteStep)[0]?.args).toEqual({ fileName: 'implement.yaml' });
});

test('the form saves the library step whole, with the file hash', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  await expect(page.locator(t('ade-wf-step-skip'))).toHaveCount(0);
  await page.locator(t('ade-wf-step-name')).fill('Implement v2');
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveStep)).toHaveLength(1);
  const args = calls(control, IPC.adeTaskSaveStep)[0]?.args as {
    fileName: string;
    baseHash: string;
    step: LibStep;
  };
  expect(args.fileName).toBe('implement.yaml');
  expect(args.baseHash).toBe(listing.steps[0]?.hash);
  expect(args.step).toMatchObject({ id: 'implement', name: 'Implement v2' });
});

test('library result routes offer next, end and stop only', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  const routes = page.locator(t('ade-wf-result-route'));
  await expect(routes.first().locator('option')).toHaveText(['Next step', 'End']);
  await expect(routes.nth(1).locator('option')).toHaveText(['Next step', 'End', 'Stop']);
});

test('YAML mode validates the text and saves it', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  await page.locator(t('ade-step-mode-yaml')).click();
  const area = page.locator(t('ade-step-yaml'));
  await expect(area).toBeVisible();
  await replaceCodeEditorText(page, area, `${await codeEditorText(area)}skip: true\n`);
  await expect(page.locator(t('ade-step-yaml-msg'))).toContainText(
    'skip is set in a workflow, not in a library step',
  );
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveStepYaml)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSaveStepYaml)[0]?.args).toMatchObject({
    fileName: 'implement.yaml',
    baseHash: listing.steps[0]?.hash,
  });
});

test('New step creates a library step of the picked kind', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await openSteps(page);
  await page.locator(t('ade-step-new')).click();
  await page.locator(t('ade-step-new-script')).click();
  await expect.poll(() => calls(control, IPC.adeTaskNewStep)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskNewStep)[0]?.args).toEqual({
    name: 'New step',
    kind: 'script',
    from: null,
  });
});

test('the add menu offers library steps and adds a reference with a fresh id', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-library-implement')).click();
  const added = page.locator(`${t('ade-wf-node')}[data-step-id^="step-"]`);
  await expect(added).toHaveCount(1);
  await expect(added.locator(t('ade-wf-node-library'))).toBeVisible();
  await expect(page.locator(t('ade-wf-step-library'))).toContainText('Implement');
  await expect(page.locator(t('ade-wf-step-overrides'))).toHaveCount(0);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as {
    workflow: { steps: LibStep[] };
  };
  expect(sent.workflow.steps.at(-1)).toMatchObject({ use: 'implement', name: 'Implement' });
});

test('a reference marks its overrides, Reset restores the library value, Extra prompt saves', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await node(page, 'implement').click();
  const overridden = page.locator(t('ade-wf-step-overridden'));
  await expect(overridden).toHaveText(['timeout', 'results']);
  await expect(page.locator(t('ade-wf-step-kind-agent'))).toBeDisabled();
  await expect(page.locator(t('ade-wf-step-extra-prompt'))).toHaveValue(
    'Also update CHANGELOG.md.',
  );
  await page.locator(t('ade-wf-step-reset-timeout')).click();
  await expect(overridden).toHaveText(['results']);
  await page.locator(t('ade-wf-step-extra-prompt')).fill('Run the linter too.');
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as {
    workflow: { steps: LibStep[] };
  };
  expect(sent.workflow.steps[0]).toMatchObject({
    use: 'implement',
    timeout: '1m',
    promptExtra: 'Run the linter too.',
  });
});

test('Edit in the library opens the Steps tab on that step', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, withLibrary);
  await node(page, 'implement').click();
  await page.locator(t('ade-wf-step-edit-library')).click();
  await expect(page.locator(t('ade-steps-tab'))).toBeVisible();
  await expect(page.locator(t('ade-step-row'))).toHaveAttribute('data-file', 'implement.yaml');
});

test('Detach turns a reference into an inline step with the extra prompt folded in', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await node(page, 'implement').click();
  await page.locator(t('ade-wf-step-detach')).click();
  await expect(page.locator(t('ade-wf-step-library'))).toHaveCount(0);
  await expect(page.locator(t('ade-wf-step-save-library'))).toBeVisible();
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as {
    workflow: { steps: LibStep[] };
  };
  const step = sent.workflow.steps[0];
  expect(step?.use).toBeUndefined();
  expect(step?.prompt).toBe('Implement the task in {repo}.\n\nAlso update CHANGELOG.md.');
});

test('Save to library sends the inline step to NewStep and references the new file', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch, withLibrary);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  await page.locator(t('ade-wf-step-save-library')).click();
  await expect.poll(() => calls(control, IPC.adeTaskNewStep)).toHaveLength(1);
  const args = calls(control, IPC.adeTaskNewStep)[0]?.args as {
    name: string;
    kind: string;
    from: LibStep;
  };
  expect(args).toMatchObject({ name: 'New step', kind: 'agent' });
  expect(args.from.id).toMatch(/^step-[0-9a-f]{8}$/);
  await expect(page.locator(t('ade-wf-step-library'))).toBeVisible();
});
