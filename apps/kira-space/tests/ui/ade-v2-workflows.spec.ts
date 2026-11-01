import type { Page } from '@playwright/test';
import { previewText } from './adePromptPreview';
import { expect, test } from './fixtures';
import { adeFixture, openAdvanced, openWorkflows, SCRIPTS, saved } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// The Workflows page: list, Graph and YAML editors, import and new.

interface WorkflowsFx {
  dir: string;
  workflows: { fileName: string; workflow: unknown; error: unknown; usedBy: number }[];
}

const t = (id: string) => `[data-testid="${id}"]`;

interface SavedStep {
  id: string;
  kind: string;
  skip: boolean;
  allowedTools: string[];
  review: { sendTo: string; implementer: string } | null;
  maxReturns: number;
  runsOn: string;
  results: { id: string; ok: boolean; next: string; loop: boolean; description: string }[];
}
interface SavedWorkflow {
  workflow: { steps: SavedStep[] };
}

const node = (page: Page, step: string) =>
  page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`);
const stepOf = (out: SavedWorkflow, id: string): SavedStep | undefined =>
  out.workflow.steps.find((x) => x.id === id);
const ids = (out: SavedWorkflow): string[] => out.workflow.steps.map((x) => x.id);
const menuItem = async (page: Page, step: string, item: string): Promise<void> => {
  await node(page, step).locator(t('ade-wf-step-menu')).click();
  await page.locator(t(item)).click();
};

async function openYaml(page: Page): Promise<void> {
  await page.locator(t('ade-wf-mode-yaml')).click();
  await expect(page.locator(t('ade-wf-yaml'))).toBeVisible();
}

test('an empty folder shows the explanation', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: { dir: '~/.kira-space/workflows', workflows: [] } },
  ]);
  await expect(page.locator(t('ade-wf-empty'))).toContainText(
    'No workflows yet. Workflows are YAML files in ~/.kira-space/workflows.',
  );
});

test('the list shows each file with its steps and flags a broken one', async ({ relaunch }) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  const { window: page } = await openWorkflows(relaunch);
  await expect(page.locator(t('ade-wf-row'))).toHaveCount(fx.workflows.length);
  const first = page.locator(t('ade-wf-row')).first();
  await expect(first.locator(t('ade-wf-row-name'))).toContainText('Standard feature');
  await expect(first.locator(t('ade-wf-row-steps'))).toContainText(
    'Spec › Plan from spec › Implement',
  );
  await expect(first.locator(t('ade-wf-row-used'))).toContainText('Used by');
  await expect(page.locator(t('ade-wf-row-error'))).toHaveCount(1);
  await page.locator(t('ade-wf-row')).last().click();
  await expect(page.locator(t('ade-wf-yaml'))).toBeVisible();
});

test('the graph saves on Save, never on its own, with ids unchanged', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await expect(page.locator(t('ade-wf-form'))).toBeVisible();
  const name = page.locator(t('ade-wf-form-name'));
  await name.fill('Standard feature v2');
  // Absence of an autosave: spans the longest debounce; no event to await.
  await page.waitForTimeout(1200);
  expect(calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(0);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const args = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as {
    fileName: string;
    workflow: { id: string; name: string; steps: { id: string }[] };
  };
  expect(args.fileName).toBe('standard.yaml');
  expect(args.workflow.name).toBe('Standard feature v2');
  expect(args.workflow.id).toBe('standard');
  expect(args.workflow.steps.map((s) => s.id)).toEqual([
    'spec',
    'plan',
    'impl',
    'tests',
    'ci',
    'pr',
    'review',
    'release',
  ]);
});

test('Runs on one repo picks a repo from the project picker and saves only <nick>', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'plan').click();
  await page.locator(t('ade-wf-step-scope-one')).click();
  await page.locator(t('ade-wf-step-scope-repo')).click();
  await page.locator(t('ade-wf-step-scope-repo-option-repo-mobile')).click();
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(stepOf(out, 'plan')?.runsOn).toBe('only mobile');
});

test('a stored repo nickname that no repo has shows the missing hint', async ({ relaunch }) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  const first = fx.workflows[0];
  const workflow = structuredClone(first?.workflow) as { steps: { id: string; runsOn: string }[] };
  const plan = workflow.steps.find((x) => x.id === 'plan');
  if (plan) plan.runsOn = 'only ghost';
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskWorkflows,
      response: { ...fx, workflows: [{ ...first, workflow }, ...fx.workflows.slice(1)] },
    },
  ]);
  await node(page, 'plan').click();
  await expect(page.locator(t('ade-wf-step-scope-missing'))).toContainText('ghost');
});

test('a new step gets a fresh id and Allowed tools becomes allowedTools', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  const added = await saved<SavedWorkflow>(page, control, 1);
  expect(added.workflow.steps.at(-1)?.id).toMatch(/^step-[0-9a-f]{8}$/);
  expect(added.workflow.steps.at(-1)?.kind).toBe('agent');

  await node(page, 'plan').click();
  await openAdvanced(page);
  await page.locator(t('ade-wf-step-tools')).fill('Read, Grep, Bash(git diff:*)');
  const tools = await saved<SavedWorkflow>(page, control, 2);
  expect(stepOf(tools, 'plan')?.allowedTools).toEqual(['Read', 'Grep', 'Bash(git diff:*)']);
});

test('switching the kind to Review confirms, then saves a review block with approved and rejected results', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'pr').click();
  await page.locator(t('ade-wf-step-kind-review')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-wf-step-review'))).toBeVisible();
  await expect(page.locator(t('ade-wf-step-prompt'))).toHaveCount(0);
  await expect(node(page, 'pr')).toHaveAttribute('data-kind', 'review');
  await page.locator(t('ade-wf-step-review-send-to')).selectOption('new');
  const args = await saved<SavedWorkflow>(page, control, 1);
  const step = stepOf(args, 'pr');
  expect(step?.kind).toBe('review');
  expect(step?.review).toEqual({ sendTo: 'new', implementer: '' });
  expect(step?.results.map((r) => [r.id, r.ok])).toEqual([
    ['approved', true],
    ['rejected', false],
  ]);
});

test('a kind switch on an empty step needs no confirmation', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-agent')).click();
  const fresh = page.locator(`${t('ade-wf-node')}[data-step-id^="step-"]`);
  await fresh.click();
  await page.locator(t('ade-wf-step-kind-script')).click();
  await expect(page.locator(t('confirm-dialog'))).toHaveCount(0);
  await expect(fresh).toHaveAttribute('data-kind', 'script');
  await expect(page.locator(t('ade-wf-step-command'))).toBeVisible();
});

test('the inspector groups a step into What it does, Where and when and How it ends', async ({
  relaunch,
}) => {
  const { window: page } = await openWorkflows(relaunch);
  await node(page, 'ci').click();
  for (const section of ['what', 'where', 'end'])
    await expect(page.locator(t(`ade-wf-step-section-${section}`))).toBeVisible();
  await expect(page.locator(t('ade-wf-step-tools'))).toBeHidden();
  await openAdvanced(page);
  await expect(page.locator(t('ade-wf-step-tools'))).toBeVisible();
  await node(page, 'spec').click();
  await expect(page.locator(t('ade-wf-step-section-where'))).toHaveCount(0);
  await expect(page.locator(t('ade-wf-step-session'))).toBeVisible();
});

test('the step menu moves a step up and down, and the first step cannot go up', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'ci').locator(t('ade-wf-step-menu')).click();
  await page.locator(t('ade-wf-step-up')).click();
  await node(page, 'spec').locator(t('ade-wf-step-menu')).click();
  await expect(page.locator(t('ade-wf-step-up'))).toHaveAttribute('data-disabled', '');
  await page.keyboard.press('Escape');
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(ids(out)).toEqual(['spec', 'plan', 'impl', 'ci', 'tests', 'pr', 'review', 'release']);
});

test('edges are green for ok results, red for not ok, and a loop is dashed', async ({
  relaunch,
}) => {
  const { window: page } = await openWorkflows(relaunch);
  await expect(
    page.locator(`${t('ade-wf-edge')}[data-tone="ok"][data-results="done"]`).first(),
  ).toBeVisible();
  const loop = page.locator(`${t('ade-wf-edge')}[data-loop="true"]`);
  await expect(loop).toHaveCount(3);
  for (const e of await loop.all()) {
    await expect(e).toHaveAttribute('data-tone', 'fail');
    await expect(e).toHaveAttribute('data-results', 'failed');
  }
});

test('the route select sets where a result goes and saves it in results', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'pr').click();
  const failed = page.locator(`${t('ade-wf-result')}[data-result="failed"]`);
  await failed.locator(t('ade-wf-result-route')).selectOption('plan');
  await expect(page.locator(t('ade-wf-step-max-returns'))).toHaveValue('3');
  await expect(page.locator(`${t('ade-wf-edge')}[data-loop="true"][data-tone="fail"]`)).toHaveCount(
    4,
  );
  const out = await saved<SavedWorkflow>(page, control, 1);
  const pr = stepOf(out, 'pr');
  expect(pr?.results.find((r) => r.id === 'failed')).toMatchObject({
    ok: false,
    next: 'plan',
    loop: true,
  });
});

test('contract: a result built in the editor saves as the backend takes it', async ({
  relaunch,
}) => {
  const sent = contract<{
    workflow: {
      steps: { id: string; maxReturns: number; results: Record<string, unknown>[] }[];
    };
  }>('ade-workflow-results', 'args:AdeTaskService.SaveWorkflow');
  const want = sent.workflow.steps
    .find((x) => x.id === 'review')
    ?.results.find((r) => r.id === 'changes');
  expect(want).toMatchObject({ next: 'impl', loop: true });
  const wantLimit = sent.workflow.steps.find((x) => x.id === 'review')?.maxReturns;
  expect(wantLimit).toBe(2);
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'tests').click();
  await page.locator(t('ade-wf-add-result')).click();
  const row = page.locator(t('ade-wf-result')).nth(2);
  await row.locator(t('ade-wf-result-id')).fill(String(want?.id));
  await row.locator(t('ade-wf-result-description')).fill(String(want?.description));
  if (want?.ok === false) await row.locator(t('ade-wf-result-ok')).click();
  await row.locator(t('ade-wf-result-route')).selectOption(String(want?.next));
  await page.locator(t('ade-wf-step-max-returns')).fill(String(wantLimit));
  const out = await saved<SavedWorkflow>(page, control, 1);
  const tests = stepOf(out, 'tests');
  expect(tests?.results.find((r) => r.id === 'changes')).toEqual(want);
  expect(tests?.maxReturns).toBe(wantLimit);
});

test('dragging a result dot to another step sets its route', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  const dot = node(page, 'pr').locator(
    `${t('ade-wf-node-result')}[data-result="failed"] .vue-flow__handle`,
  );
  const target = node(page, 'impl').locator('[data-handleid="in"]');
  await dot.dragTo(target);
  const out = await saved<SavedWorkflow>(page, control, 1);
  const pr = stepOf(out, 'pr');
  expect(pr?.results.find((r) => r.id === 'failed')).toMatchObject({ next: 'impl', loop: true });
});

test('a result added in the inspector gets its own connector', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'ci').click();
  await expect(node(page, 'ci').locator(t('ade-wf-node-result'))).toHaveCount(2);
  await page.locator(t('ade-wf-add-result')).click();
  await expect(node(page, 'ci').locator(t('ade-wf-node-result'))).toHaveCount(3);
  const row = page.locator(t('ade-wf-result')).nth(2);
  await row.locator(t('ade-wf-result-id')).fill('flaky');
  await row.locator(t('ade-wf-result-route')).selectOption('end');
  const out = await saved<SavedWorkflow>(page, control, 1);
  const ci = stepOf(out, 'ci');
  expect(ci?.results.map((r) => [r.id, r.next])).toEqual([
    ['done', 'next'],
    ['failed', 'impl'],
    ['flaky', 'end'],
  ]);
});

test('Delete removes the selected step after asking', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'ci').click();
  await page.locator(t('ade-wf-graph')).press('Meta+Backspace');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(node(page, 'ci')).toHaveCount(0);
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(ids(out)).toEqual(['spec', 'plan', 'impl', 'tests', 'pr', 'review', 'release']);
});

test('steps add from the graph button and from the step menu, by kind', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await expect(page.locator(t('ade-wf-strip-row'))).toHaveCount(0);
  await page.locator(t('ade-wf-add-step')).click();
  await page.locator(t('ade-wf-add-step-manual')).click();
  await node(page, 'ci').locator(t('ade-wf-step-menu')).click();
  await page.locator(t('ade-wf-step-add-after')).click();
  await page.locator(t('ade-wf-step-add-after-script')).click();
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(out.workflow.steps).toHaveLength(10);
  expect(ids(out).slice(0, 5)).toEqual(['spec', 'plan', 'impl', 'tests', 'ci']);
  expect(out.workflow.steps[5]?.kind).toBe('script');
  expect(out.workflow.steps.at(-1)?.kind).toBe('manual');
  expect(out.workflow.steps.at(-1)?.results).toEqual([]);
});

test('the step menu makes a step the start and removes one after asking', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'tests').locator(t('ade-wf-step-menu')).click();
  await page.locator(t('ade-wf-step-start')).click();
  await expect(node(page, 'tests')).toHaveAttribute('data-start', 'true');
  await node(page, 'ci').locator(t('ade-wf-step-menu')).click();
  await page.locator(t('ade-wf-step-remove')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(node(page, 'ci')).toHaveCount(0);
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(ids(out)[0]).toBe('tests');
  expect(ids(out)).not.toContain('ci');
});

test('Max returns is a step field: agent steps take 1 to 10, review steps 0', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await node(page, 'ci').click();
  const field = page.locator(t('ade-wf-step-max-returns'));
  await expect(field).toHaveValue('3');
  await expect(field).toHaveAttribute('min', '1');
  await field.fill('4');
  await expect(node(page, 'ci').locator(t('ade-wf-node-returns'))).toHaveText('↩ ≤4');
  await node(page, 'pr').click();
  await page.locator(t('ade-wf-step-kind-review')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(field).toHaveValue('0');
  await expect(field).toHaveAttribute('min', '0');
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(stepOf(out, 'ci')?.maxReturns).toBe(4);
  expect(stepOf(out, 'pr')?.maxReturns).toBe(0);
});

test('a refused save shows the error and a Switch to YAML link', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskSaveWorkflow,
      error: { code: 'invalid', message: 'workflow file has comments; edit it in YAML' },
    },
  ]);
  await page.locator(t('ade-wf-form-name')).fill('Renamed');
  await page.locator(t('ade-wf-save')).click();
  await expect(page.locator(t('ade-wf-save-error'))).toContainText('edit it in YAML');
  await page.locator(t('ade-wf-switch-yaml')).click();
  await expect(page.locator(t('ade-wf-yaml'))).toBeVisible();
});

test('YAML mode validates while typing and saves the text on Save', async ({ relaunch }) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  const { window: page, control } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskValidateWorkflowYaml,
      response: { workflow: fx.workflows[0]?.workflow, error: null },
    },
  ]);
  await openYaml(page);
  const area = page.locator(t('ade-wf-yaml'));
  await area.fill(`${await area.inputValue()}\n# note\n`);
  await expect(page.locator(t('ade-wf-yaml-msg'))).toContainText(
    '✓ Valid, the form and the plan use this file',
  );
  expect(calls(control, IPC.adeTaskSaveWorkflowYaml)).toHaveLength(0);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflowYaml)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSaveWorkflowYaml)[0]?.args).toMatchObject({
    fileName: 'standard.yaml',
  });
});

test('a YAML save refused for a changed file offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskSaveWorkflowYaml,
      error: { code: 'E_CONFLICT', message: 'the workflow changed since it was loaded' },
    },
  ]);
  await openYaml(page);
  const area = page.locator(t('ade-wf-yaml'));
  await area.fill(`${await area.inputValue()}\n# note\n`);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflowYaml)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSaveWorkflowYaml)[0]?.args).toMatchObject({
    fileName: 'standard.yaml',
    baseHash: 'hash-standard.yaml',
  });
  await expect(page.locator(t('ade-wf-save-error'))).toContainText('changed since it was loaded');

  await page.locator(t('ade-wf-overwrite')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflowYaml)).toHaveLength(2);

  await page.locator(t('ade-wf-reload')).click();
  await expect(page.locator(t('ade-wf-save-error'))).toHaveCount(0);
  await expect(page.locator(t('ade-wf-save'))).toBeDisabled();
});

test('an invalid YAML shows its line and keeps the last valid version', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskValidateWorkflowYaml,
      response: {
        workflow: null,
        error: { line: 7, message: 'kind must be agent, script, review or manual' },
      },
    },
  ]);
  await openYaml(page);
  await page.locator(t('ade-wf-yaml')).fill('id: x\n');
  await expect(page.locator(t('ade-wf-yaml-msg'))).toContainText(
    '✕ Line 7: kind must be agent, script, review or manual (the last valid version stays in use)',
  );
});

test('line 0 shows the message alone', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskValidateWorkflowYaml,
      response: {
        workflow: null,
        error: { line: 0, message: 'step 4: kind must be agent, script, review or manual' },
      },
    },
  ]);
  await openYaml(page);
  await page.locator(t('ade-wf-yaml')).fill('id: x\n');
  await expect(page.locator(t('ade-wf-yaml-msg'))).toContainText(
    '✕ step 4: kind must be agent, script, review or manual',
  );
  await expect(page.locator(t('ade-wf-yaml-msg'))).not.toContainText('✕ ✕');
});

test('Copy YAML confirms the copy', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch);
  const copy = page.locator(t('ade-wf-copy'));
  await expect(copy).toHaveText('Copy YAML');
  await copy.click();
  await expect(copy).toHaveText('Copied');
});

test('the config file bar shows a valid file, Open and Reveal name the folder and file', async ({
  relaunch,
}) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  const { window: page, control } = await openWorkflows(relaunch);
  await expect(page.locator(t('config-file-status'))).toContainText('✓ Valid');
  await page.locator(t('config-file-open')).click();
  await page.locator(t('config-file-reveal')).click();
  const file = (fx.workflows[0] as { fileName: string }).fileName;
  await expect.poll(() => calls(control, IPC.configFilesOpen)).toHaveLength(1);
  await expect.poll(() => calls(control, IPC.configFilesReveal)).toHaveLength(1);
  expect(calls(control, IPC.configFilesOpen)[0]?.args).toEqual({ folder: 'workflows', file });
  expect(calls(control, IPC.configFilesReveal)[0]?.args).toEqual({ folder: 'workflows', file });
});

test('a broken workflow file shows its error with the line in the config file bar', async ({
  relaunch,
}) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  const first = fx.workflows[0];
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskWorkflows,
      response: {
        ...fx,
        workflows: [
          { ...first, error: { line: 7, message: 'kind must be agent, script, review or manual' } },
          ...fx.workflows.slice(1),
        ],
      },
    },
  ]);
  await expect(page.locator(t('config-file-status'))).toContainText(
    '✕ Line 7: kind must be agent, script, review or manual',
  );
});

test('Import sends the typed path and opens YAML mode', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-import')).click();
  await page.locator(t('ade-wf-import-path')).fill('/tmp/shared/release.yaml');
  await page.locator(t('ade-wf-import-go')).click();
  await expect.poll(() => calls(control, IPC.adeTaskImportWorkflow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskImportWorkflow)[0]?.args).toEqual({
    path: '/tmp/shared/release.yaml',
  });
  await expect(page.locator(t('ade-wf-yaml'))).toBeVisible();
});

test('+ New creates a workflow and opens its graph', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  await page.locator(t('ade-wf-new')).click();
  await expect.poll(() => calls(control, IPC.adeTaskNewWorkflow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskNewWorkflow)[0]?.args).toEqual({ name: 'New workflow' });
  await expect(page.locator(t('ade-wf-form'))).toBeVisible();
});

test('Discard restores the saved form, and leaving with edits asks first', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  const name = page.locator(t('ade-wf-form-name'));
  const saved = await name.inputValue();
  await expect(page.locator(t('ade-wf-save'))).toBeDisabled();
  await name.fill('Edited');
  await expect(page.locator(t('ade-wf-save'))).toBeEnabled();
  await page.locator(t('ade-wf-discard')).click();
  await expect(name).toHaveValue(saved);
  await expect(page.locator(t('ade-wf-save'))).toBeDisabled();

  await name.fill('Edited again');
  await page.locator(t('ade-tab-plan')).click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(page.locator(t('ade-workflows'))).toBeVisible();
  await expect(name).toHaveValue('Edited again');
  await page.locator(t('ade-tab-plan')).click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(page.locator(t('ade-plan'))).toBeVisible();
  expect(calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(0);
});

test('Cmd+S saves the open editor', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch);
  const name = page.locator(t('ade-wf-form-name'));
  await name.fill('Via shortcut');
  await name.press('Meta+s');
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
});

test('the step menu skips a step, and the last runnable step cannot be skipped', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  const skipItem = page.locator(t('ade-wf-step-skip'));
  const skip = (id: string): Promise<void> => menuItem(page, id, 'ade-wf-step-skip');
  await skip('impl');
  await expect(node(page, 'impl')).toHaveAttribute('data-skipped', 'true');
  const out = await saved<SavedWorkflow>(page, control, 1);
  expect(out.workflow.steps.filter((x) => x.skip).map((x) => x.id)).toEqual(['impl']);

  for (const id of ['spec', 'plan', 'tests', 'ci', 'pr', 'review']) await skip(id);
  await node(page, 'release').locator(t('ade-wf-step-menu')).click();
  await expect(skipItem).toHaveAttribute('data-disabled', '');
});

test('the workflow editor has no Kira Space tools switch', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch);
  await expect(page.locator(t('ade-wf-form-name'))).toBeVisible();
  await expect(page.locator(t('ade-wf-form-space'))).toHaveCount(0);
});

test('leaving Agents with unsaved edits asks first', async ({ relaunch }) => {
  const { window: page } = await openWorkflows(relaunch);
  const name = page.locator(t('ade-wf-form-name'));
  await name.fill('Edited');
  const terminal = page.locator('[data-testid="mode-tab"][data-mode="automations"]');
  await terminal.click();
  await expect(page.locator(t('confirm-dialog'))).toBeVisible();
  await page.locator(t('confirm-dialog-cancel')).click();
  await expect(name).toHaveValue('Edited');
  await terminal.click();
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(terminal).toHaveClass(/is-active/);
});

test('a save refused for a changed file offers Reload and Overwrite', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, [
    {
      channel: IPC.adeTaskSaveWorkflow,
      error: { code: 'E_CONFLICT', message: 'the workflow changed since it was loaded' },
    },
  ]);
  await page.locator(t('ade-wf-form-name')).fill('Renamed');
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskSaveWorkflow)[0]?.args).toMatchObject({
    fileName: 'standard.yaml',
    baseHash: 'hash-standard.yaml',
  });
  await expect(page.locator(t('ade-wf-save-error'))).toContainText('changed since it was loaded');
  await expect(page.locator(t('ade-wf-switch-yaml'))).toHaveCount(0);

  await page.locator(t('ade-wf-overwrite')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(2);

  await page.locator(t('ade-wf-reload')).click();
  await expect(page.locator(t('ade-wf-save-error'))).toHaveCount(0);
  await expect(page.locator(t('ade-wf-save'))).toBeDisabled();
});

test('contract: the step inspector shows the finish instruction rendered by the backend', async ({
  relaunch,
}) => {
  const { window: page } = await openWorkflows(relaunch, [
    {
      channel: IPC.llmPromptsPreview,
      response: contract('ade-run', 'LlmPromptsService.Preview#finish'),
    },
  ]);
  await node(page, 'plan').click();
  await expect(page.locator(t('ade-wf-finish-note'))).toContainText(previewText('finish'));
});

test('contract: the route select hides manual and skipped steps up to this one, keeping the current pick', async ({
  relaunch,
}) => {
  const refused = JSON.parse(
    contract<string>('ade-workflow-editor', 'AdeTaskService.SaveWorkflow#return-to-skipped'),
  ) as { code: string; message: string };
  const { window: page } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskSaveWorkflow, error: refused },
  ]);
  await menuItem(page, 'impl', 'ade-wf-step-skip');
  await menuItem(page, 'plan', 'ade-wf-step-skip');
  await node(page, 'tests').click();
  const route = page
    .locator(`${t('ade-wf-result')}[data-result="failed"]`)
    .locator(t('ade-wf-result-route'));
  const option = (value: string) => route.locator(`option[value="${value}"]`);
  await expect(route).toHaveValue('impl');
  await expect(option('impl')).toHaveCount(1);
  await expect(option('spec')).toHaveCount(0);
  await expect(option('plan')).toHaveCount(0);
  await expect(option('tests')).toHaveCount(1);
  await expect(option('review')).toHaveCount(1);
  await page.locator(t('ade-wf-save')).click();
  await expect(page.locator(t('ade-wf-save-error'))).toContainText(refused.message);
});

test('contract: a YAML return to a manual step shows the parser error', async ({ relaunch }) => {
  const refused = contract<{ workflow: null; error: { line: number; message: string } }>(
    'ade-workflow-editor',
    'AdeTaskService.ValidateWorkflowYaml#return-to-manual',
  );
  expect(refused.workflow).toBeNull();
  expect(refused.error.message).toContain('cannot return to a manual or skipped step');
  const { window: page } = await openWorkflows(relaunch, [
    { channel: IPC.adeTaskValidateWorkflowYaml, response: refused },
  ]);
  await openYaml(page);
  await page.locator(t('ade-wf-yaml')).fill('id: x\n');
  await expect(page.locator(t('ade-wf-yaml-msg'))).toHaveText(
    `✕ Line ${refused.error.line}: ${refused.error.message} (the last valid version stays in use).`,
  );
});

// Each edit leaves `review.implementer` naming a step that is gone, later, manual or smart; the
// editor sends '' and the backend accepts it (TestWorkflowEditorClearsStaleImplementer).
const STALE_IMPLEMENTER_EDITS: {
  name: string;
  apply: (page: Page) => Promise<void>;
}[] = [
  {
    name: 'removing the implementer',
    apply: async (page) => {
      await menuItem(page, 'impl', 'ade-wf-step-remove');
      await page.locator(t('confirm-dialog-confirm')).click();
    },
  },
  {
    name: 'moving the review above the implementer',
    apply: (page) => menuItem(page, 'review', 'ade-wf-step-up'),
  },
  {
    name: 'making the implementer manual',
    apply: async (page) => {
      await node(page, 'impl').click();
      await page.locator(t('ade-wf-step-kind-manual')).click();
      await page.locator(t('confirm-dialog-confirm')).click();
    },
  },
  {
    name: 'giving the implementer a smart script',
    apply: async (page) => {
      await node(page, 'impl').click();
      await page.locator(t('ade-wf-step-mode-smart')).click();
    },
  },
];

for (const edit of STALE_IMPLEMENTER_EDITS) {
  test(`contract: ${edit.name} clears the review implementer and the save goes through`, async ({
    relaunch,
  }) => {
    const { window: page, control } = await openWorkflows(relaunch, [
      {
        channel: IPC.adeTaskWorkflows,
        response: contract('ade-workflow-editor', 'AdeTaskService.Workflows'),
      },
      {
        channel: IPC.adeTaskSaveWorkflow,
        response: contract('ade-workflow-editor', 'AdeTaskService.SaveWorkflow#cleared'),
      },
      SCRIPTS,
    ]);
    const implementer = page.locator(t('ade-wf-step-review-implementer'));
    await node(page, 'review').click();
    await expect(implementer).toHaveValue('impl');
    await edit.apply(page);
    await node(page, 'review').click();
    await expect(implementer).toHaveValue('');
    const out = await saved<SavedWorkflow>(page, control, 1);
    expect(stepOf(out, 'review')?.review).toEqual({ sendTo: 'implementer', implementer: '' });
    await expect(page.locator(t('ade-wf-save-error'))).toHaveCount(0);
  });
}
