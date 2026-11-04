import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  type AdeBoardFx,
  adeBoard,
  adeFixture,
  openBillTask,
  openPlan,
  openSessions,
} from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// The agent sandbox: run-line badge and popover, the step inspector section.
// Contract ade-sandbox; backend half: adeflow TestSandbox* (sandbox_test.go). The task, branch and
// run ids are the fixture board's; every other field is the backend's.

const t = (id: string) => `[data-testid="${id}"]`;
const RUN_ID = 'r_T_bill_impl_b_billdash';
const PROFILE = '(version 1)\n(allow default)';
type Wire = Record<string, unknown>;

function sandboxControl(key: string, run: Wire = {}): ControlSnapshot[] {
  const wire = contract<Wire>('ade-sandbox', `AdeTaskService.Run#${key}`);
  const detail = contract<{ denials: Wire[] } & Wire>(
    'ade-sandbox',
    `AdeTaskService.RunSandbox#${key}`,
  );
  return [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b: AdeBoardFx) => {
        const bill = b.tasks.find((x) => x.id === 'T_bill');
        if (!bill) throw new Error('fixture task missing');
        bill.runs = bill.runs.map((r) =>
          r.id === RUN_ID
            ? {
                ...r,
                sandbox: wire.sandbox,
                state: wire.state,
                outcome: wire.outcome && {
                  ...(wire.outcome as Wire),
                  report: { tried: 'git branch x' },
                },
                sessionId: 'sess-1',
                ...run,
              }
            : r,
        );
      }),
    },
    {
      channel: IPC.adeTaskRunSandbox,
      response: {
        ...detail,
        profile: detail.access === 'off' ? '' : PROFILE,
        denials: detail.denials.map((d) => ({ ...d, at: 1_790_036_400_000 })),
      },
    },
  ];
}

const line = (page: Page) =>
  page
    .locator(`${t('ade-step-block')}[data-step-id="impl"]`)
    .locator(`${t('ade-run-line')}[data-branch-id="b_billdash"]`);

test('contract: the run line shows the policy in the popover', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('policy'));
  await openBillTask(page);
  const badge = line(page).locator(t('ade-run-sandbox'));
  await expect(badge).toHaveText('Write worktree');
  await expect(badge).toHaveAttribute('data-denials', '0');
  await badge.click();
  const pop = page.locator(t('ade-run-sandbox-popover'));
  await expect(pop.locator(t('ade-run-sandbox-enforced'))).toHaveText('Enforced by macOS');
  await expect(pop.locator(t('ade-run-sandbox-write'))).toContainText('/api-sb');
  await expect(pop.locator(t('ade-run-sandbox-network'))).toHaveText('All hosts');
  await expect(pop.locator(t('ade-run-sandbox-denial'))).toHaveCount(0);
});

test('contract: a script run shows its read-only sandbox', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('script'));
  await openBillTask(page);
  const badge = line(page).locator(t('ade-run-sandbox'));
  await expect(badge).toHaveText('Read only');
  await expect(badge).toHaveAttribute('data-enforced', 'true');
});

test('contract: denials show as a count, a row and the profile', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('denied'));
  await openBillTask(page);
  const badge = line(page).locator(t('ade-run-sandbox'));
  await expect(badge).toHaveText('2 blocked');
  await badge.click();
  const row = page.locator(t('ade-run-sandbox-denial'));
  await expect(row).toHaveCount(1);
  await expect(row).toContainText('file-write-create');
  await expect(row).toContainText('/etc/kira-denied');
  await expect(row).toContainText('git');
  await expect(page.locator(t('ade-run-sandbox-popover'))).toContainText('1 of 2 shown');
  await page.locator(t('ade-run-sandbox-profile')).click();
  await expect(page.locator(t('ade-run-sandbox-profile-text'))).toContainText('(allow default)');
});

test('contract: a blocked failure shows the reason and the chip in its outcome', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('denied'));
  await openBillTask(page);
  const outcome = page.locator(t('ade-run-outcome'));
  await expect(outcome.locator(t('ade-outcome-reason'))).toContainText(
    'Sandbox blocked file-write-create /etc/kira-denied.',
  );
  await expect(outcome.locator(t('ade-run-sandbox'))).toHaveText('2 blocked');
});

test('Take over warns that it leaves the sandbox', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('denied'));
  await openBillTask(page);
  await line(page).locator(t('ade-run-takeover')).hover();
  await expect(page.locator('[data-slot="tooltip-content"]')).toContainText(
    'Takes over without the sandbox.',
  );
});

test('contract: an OS that cannot enforce shows Not enforced', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, sandboxControl('notEnforced'));
  await openBillTask(page);
  const badge = line(page).locator(t('ade-run-sandbox'));
  await expect(badge).toHaveText('Not enforced');
  await expect(badge).toHaveAttribute('data-enforced', 'false');
});

interface WorkflowsFx {
  workflows: { workflow: { steps: Wire[] } }[];
  sandbox: { enforced: boolean; reason: string };
}

async function openStep(page: Page, step: string): Promise<void> {
  await page.locator(t('ade-tab-workflows')).click();
  await page.locator(t('ade-workflows')).waitFor();
  await page.locator(`${t('ade-wf-node')}[data-step-id="${step}"]`).click();
  await expect(page.locator(t('ade-wf-step-sandbox'))).toBeVisible();
}

interface Saved {
  workflow: { steps: { id: string; sandbox: Wire }[] };
}

async function saved(page: Page, control: Parameters<typeof calls>[0]): Promise<Saved> {
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  return calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as Saved;
}

test('the inspector edits access, paths, preset and network, and Save carries them', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch);
  await openStep(page, 'plan');
  await expect(page.locator(t('ade-wf-sandbox-off'))).toHaveCount(0);
  await page.locator(t('ade-wf-sandbox-read')).fill('~/notes\nrelative/path');
  await expect(page.locator(t('ade-wf-sandbox-read-error'))).toBeVisible();
  await page.locator(t('ade-wf-sandbox-read')).fill('~/notes');
  await expect(page.locator(t('ade-wf-sandbox-read-error'))).toHaveCount(0);
  await page.locator(t('ade-wf-sandbox-preset')).click();
  await page.locator(t('ade-wf-sandbox-preset-go')).click();
  await expect(page.locator(t('ade-wf-sandbox-allow'))).toHaveValue(
    '~/go\n~/Library/Caches/go-build',
  );
  await page.locator(t('ade-wf-sandbox-network')).click();
  const args = await saved(page, control);
  const step = args.workflow.steps.find((s) => s.id === 'plan');
  expect(step?.sandbox).toEqual({
    access: 'write',
    read: ['~/notes'],
    allow: ['~/go', '~/Library/Caches/go-build'],
    network: false,
  });
});

test('Read only drops the allow list and Off warns', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch);
  await openStep(page, 'plan');
  const node = page.locator(`${t('ade-wf-node')}[data-step-id="plan"]`);
  await page.locator(t('ade-wf-sandbox-access-read')).click();
  await expect(page.locator(t('ade-wf-sandbox-allow'))).toHaveCount(0);
  await expect(node.locator(t('ade-wf-node-sandbox'))).toHaveText('Read only');
  await page.locator(t('ade-wf-sandbox-access-off')).click();
  await expect(page.locator(t('ade-wf-sandbox-off'))).toContainText('full rights');
  await expect(node.locator(t('ade-wf-node-sandbox'))).toHaveText('No sandbox');
  const args = await saved(page, control);
  expect(args.workflow.steps.find((s) => s.id === 'plan')?.sandbox.access).toBe('off');
});

test('an OS that cannot enforce shows the notice in the inspector', async ({ relaunch }) => {
  const fx = adeFixture<WorkflowsFx>('workflows');
  fx.sandbox = {
    enforced: false,
    reason: 'The sandbox is not enforced on this OS. It runs on macOS only.',
  };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: fx },
  ]);
  await openStep(page, 'plan');
  await expect(page.locator(t('ade-wf-sandbox-notice'))).toContainText('runs on macOS only');
});

test('a script step has its sandbox section without opening Advanced', async ({ relaunch }) => {
  const fx = contract<WorkflowsFx>('ade-workflow-editor', 'AdeTaskService.Workflows#outside');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: fx },
  ]);
  await openStep(page, 'build');
  await expect(page.locator(t('ade-wf-step-advanced'))).toHaveAttribute('aria-expanded', 'false');
  await page.locator(t('ade-wf-sandbox-access-write')).click();
  await page.locator(t('ade-wf-sandbox-network')).click();
  await page.locator(t('ade-wf-sandbox-access-read')).click();
  const args = await saved(page, control);
  expect(args.workflow.steps.find((s) => s.id === 'build')?.sandbox).toEqual({
    access: 'read',
    read: [],
    allow: [],
    network: true,
  });
});

const sessionOf = (id: string) =>
  contract<{ sessions: Wire[] }>('ade-sandbox', `AdeTaskService.Sessions#${id}`).sessions[0];

test('contract: a sandboxed manual session shows its chip and policy', async ({ relaunch }) => {
  const fx = adeFixture<{ sessions: Wire[] }>('sessions');
  fx.sessions = fx.sessions.map((s) =>
    s.id === 'tk01' ? { ...s, sandbox: sessionOf('manual-sandbox')?.sandbox } : s,
  );
  const detail = contract<{ denials: Wire[] } & Wire>(
    'ade-sandbox',
    'AdeTaskService.SessionSandbox#manual',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskSessions, response: fx },
    {
      channel: IPC.adeTaskSessionSandbox,
      response: {
        ...detail,
        profile: PROFILE,
        denials: detail.denials.map((d) => ({ ...d, at: 1_790_036_400_000 })),
      },
    },
  ]);
  await openSessions(page, 'T_bill');
  const badge = page.locator(t('ade-session-sandbox-badge'));
  await expect(badge).toHaveAttribute('data-denials', '1');
  await badge.click();
  const pop = page.locator(t('ade-run-sandbox-popover'));
  await expect(pop.locator(t('ade-run-sandbox-denial'))).toContainText('/etc/kira-denied');
  expect(calls(control, IPC.adeTaskSessionSandbox)[0]?.args).toEqual({ sessionId: 'tk01' });
});

test('an unsandboxed manual session shows no chip', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await openSessions(page, 'T_bill');
  await expect(page.locator(t('ade-session-sandbox-badge'))).toHaveCount(0);
});

test('a manual step with a session has the sandbox section, off by default', async ({
  relaunch,
}) => {
  const fx = contract<WorkflowsFx>(
    'ade-workflow-editor',
    'AdeTaskService.Workflows#manual-sandbox',
  );
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskWorkflows, response: fx },
  ]);
  await openStep(page, 'free');
  await expect(page.locator(t('ade-wf-sandbox-off'))).toContainText('full rights');
  await page.locator(t('ade-wf-sandbox-access-write')).click();
  await expect(page.locator(t('ade-wf-sandbox-manual-limits'))).toContainText('push or fetch');
  await page.locator(t('ade-wf-step-session')).click();
  await expect(page.locator(t('ade-wf-step-section-sandbox'))).toHaveCount(0);
  const args = await saved(page, control);
  const step = args.workflow.steps.find((s) => s.id === 'free');
  expect(step?.sandbox).toMatchObject({ access: 'off' });
});
