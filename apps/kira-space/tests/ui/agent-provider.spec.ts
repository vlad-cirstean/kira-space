import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import {
  adeFixture,
  openAdvanced,
  openPlan,
  openTask,
  openWorkflows,
  saved,
} from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';

// A second agent provider: the names, models and optional roles come from AgentService.Info.
// Backend half: harnessflow TestSecondProviderRegistered, which writes the fake provider's Info.

const fake = contract<{ headlessLabel: string; models: { id: string }[]; defaultModel: string }>(
  'agent',
  'AgentService.Info#fake',
);
const info = [{ channel: IPC.agentInfo, response: fake }];

async function openSettings(page: Page, section: string): Promise<void> {
  await emitWailsEvent(page, IPC.openSettings, undefined);
  await page.locator(`[data-testid="settings-section-${section}"]`).click();
}

test('the Memory pane lists the provider models and its default', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: withMemory(info) });
  await openSettings(page, 'Memory');
  const select = page.locator('[data-testid="settings-memory-agent-model"]');
  await expect(select).toHaveValue(fake.defaultModel);
  const options = await select
    .locator('option')
    .evaluateAll((els) => els.map((e) => (e as HTMLOptionElement).value));
  expect(options).toEqual(fake.models.map((m) => m.id));
});

test('a provider without hooks or usage hides those settings', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: info });
  await openSettings(page, 'Agent');
  await expect(page.locator('[data-testid="settings-agent-notify"]')).toBeVisible();
  await expect(page.locator('[data-testid="settings-agent-notify-run-ended"]')).toBeVisible();
  await expect(page.locator('[data-testid="settings-agent-notify-finished"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="settings-agent-notify-needs-input"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="settings-agent-usage"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="agent-usage-status"]')).toHaveCount(0);
});

test('headless session badges carry the provider label', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, info);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_bill"] [data-testid="ade-card-head"]')
    .click();
  await page.locator('[data-testid="ade-panel-tab-sessions"]').click();
  const badges = await page.locator('[data-testid="ade-session-badge"]').allTextContents();
  expect(badges).toContain(fake.headlessLabel);
  expect(badges).not.toContain('claude -p');
});

// OpenCode next to claude: the picker in Settings, the plus menu and the step editor.
// Backend half: harnessflow TestOpenCodeProvider, which writes the list.

const both = contract<{ id: string }[]>('agent', 'AgentService.List');
const claudeOnly = contract<{ id: string }[]>('agent', 'AgentService.List#no-opencode');
const bothList = [{ channel: IPC.agentList, response: both }];
const t = (id: string) => `[data-testid="${id}"]`;

test('the Agent pane lists every provider, disables a missing one and saves the pick', async ({
  relaunch,
}) => {
  const { window: page, control } = await relaunch({ control: bothList });
  await openSettings(page, 'Agent');
  const select = page.locator(t('settings-agent-provider'));
  await expect(select).toHaveValue('claude');
  const options = await select
    .locator('option')
    .evaluateAll((els) => els.map((e) => (e as HTMLOptionElement).value));
  expect(options).toEqual(both.map((e) => e.id));
  await select.selectOption('opencode');
  await page.locator(t('settings-save')).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch: { agent: { provider: 'opencode' } } });
});

test('the hermetic switch is on by default, saves off and resets', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: bothList });
  await openSettings(page, 'Agent');
  const sw = page.locator(t('settings-agent-hermetic'));
  await expect(sw).toHaveAttribute('aria-checked', 'true');
  await expect(page.getByText('Agents Kira starts skip your own agent config')).toBeVisible();
  await sw.click();
  await expect(sw).toHaveAttribute('aria-checked', 'false');
  await page.locator(t('settings-reset-agent-hermetic')).click();
  await expect(sw).toHaveAttribute('aria-checked', 'true');
  await sw.click();
  await page.locator(t('settings-save')).click();
  await expect
    .poll(() => control.log().find((e) => e.channel === IPC.settingsSet)?.args)
    .toEqual({ patch: { agent: { hermetic: false } } });
});

test('a provider whose CLI is missing is disabled and says so', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.agentList, response: claudeOnly }],
  });
  await openSettings(page, 'Agent');
  const missing = page.locator(`${t('settings-agent-provider')} option[value="opencode"]`);
  await expect(missing).toBeDisabled();
  await expect(missing).toHaveText('OpenCode (not found)');
});

test('the plus menu offers each installed agent and sends the provider', async ({ relaunch }) => {
  const launch = {
    ...contract<Record<string, unknown>>('ade-adhoc', 'AdeTaskService.StartAdHoc'),
    ...adeFixture<Record<string, unknown>>('launch'),
  };
  const { window: page, control } = await openPlan(relaunch, [
    ...bothList,
    { channel: IPC.adeTaskStartAdHoc, response: launch },
  ]);
  await openTask(page, 'T_cart');
  await page.locator(t('ade-panel-tab-sessions')).click();
  await page.locator(`${t('ade-session-bar-row')} ${t('ade-task-launch')}`).click();
  await expect(page.locator(t('menu-item-ade-task-launch-agent-claude-b_cart'))).toBeInViewport();
  await page.locator(t('menu-item-ade-task-launch-agent-opencode-b_cart')).click();
  await expect.poll(() => calls(control, IPC.adeTaskStartAdHoc)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskStartAdHoc)[0]?.args).toEqual({
    branchId: 'b_cart',
    provider: 'opencode',
  });
});

test('the step editor sets the step agent and saves it', async ({ relaunch }) => {
  const { window: page, control } = await openWorkflows(relaunch, bothList);
  await page.locator(`${t('ade-wf-node')}[data-step-id="plan"]`).click();
  await openAdvanced(page);
  const select = page.locator(t('ade-wf-step-agent'));
  await expect(select).toHaveValue('');
  await select.selectOption('opencode');
  const out = await saved<{ workflow: { steps: { id: string; agent: string }[] } }>(
    page,
    control,
    1,
  );
  expect(out.workflow.steps.find((x) => x.id === 'plan')?.agent).toBe('opencode');
});
