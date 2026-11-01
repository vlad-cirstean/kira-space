import { expect, type Page } from '@playwright/test';
import { type CustomScript, customScriptSchema } from '@shared/domain/scripts';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import type { ControlSnapshot } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsList'
  | 'customScriptsUpdate'
  | 'scriptRunsNextFires'
  | 'scriptRunsMcpServers'
  | 'scriptRunsPreview'
  | 'scriptRunsResolveDir'
  | 'scriptRunsStart'
>;

// P447: a secret param's saved value is write-only in the editor and optional in the run dialog.
// Backend half: termflow TestSecretParam* (both apps) assert the sealed storage and masked output.
export function registerSecretScenarios(env: Env): void {
  const { test, ipc: IPC } = env;

  const ON: SecretStorageStatus = {
    available: true,
    backend: 'keychain',
    insecureFallback: false,
    reason: null,
  };
  const OFF: SecretStorageStatus = {
    available: false,
    backend: 'unavailable',
    insecureFallback: false,
    reason: 'none',
  };
  const DIR = {
    path: '/kira/automations/x',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };
  const script = (saved: string[]): CustomScript =>
    customScriptSchema.parse({
      id: 's1',
      name: 'deploy',
      kind: 'script',
      command: 'deploy.sh',
      params: [
        { name: 'token', type: 'text', options: [], default: [], required: true, secret: true },
      ],
      savedSecrets: saved,
      smart: null,
      schedule: null,
      dirMode: 'kira',
      workingDir: '',
      color: 'blue',
      collectionId: null,
      useAdeDir: false,
      sortOrder: 0,
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    });
  const preview = (saved: boolean, missing: string[]) => ({
    kind: 'script',
    ade: null,
    allowedTools: [],
    blocker: '',
    body: '',
    command: 'deploy.sh',
    dir: DIR,
    env: [{ name: 'KIRA_PARAM_TOKEN', fromVar: 'token', secret: true, saved, value: '' }],
    hash: 'h',
    maxBudgetUsd: 0,
    mcpServers: [],
    missing,
    model: '',
    needs: { branches: [], tasks: [] },
    prompt: [],
    secretIssue: '',
    suffix: '',
    timeout: '',
    tools: [],
  });
  const base = (saved: string[], storage = ON): ControlSnapshot[] => [
    { channel: IPC.scriptRunsResolveDir, response: DIR },
    { channel: IPC.scriptRunsNextFires, response: [1_900_000_000_000] },
    { channel: IPC.scriptRunsMcpServers, response: [] },
    {
      channel: IPC.customScriptsList,
      response: { collections: [], scripts: [script(saved)], secretStorage: storage },
    },
  ];
  const dialog = (page: Page) => page.locator('[data-testid="script-dialog"]');

  async function openEditor(page: Page): Promise<void> {
    await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
    await page.locator('[data-testid="script-s1"]').click({ button: 'right' });
    await page.locator('[data-testid="menu-item-edit"]').click();
    await expect(dialog(page)).toBeVisible();
    await dialog(page).locator('[data-testid="script-dialog-tab-params"]').click();
  }

  test('a saved secret is write-only: empty password field, Saved badge, Clear sends null', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...base(['token']),
        { channel: IPC.customScriptsUpdate, response: script(['token']) },
      ],
    });
    await openEditor(page);
    const field = dialog(page).locator('[data-testid="param-secret-value-0"]');
    await expect(field).toHaveAttribute('type', 'password');
    await expect(field).toHaveValue('');
    await expect(dialog(page).locator('[data-testid="param-secret-saved-0"]')).toBeVisible();

    await dialog(page).locator('[data-testid="param-secret-clear-0"]').click();
    await expect(dialog(page).locator('[data-testid="param-secret-saved-0"]')).toHaveCount(0);
    await dialog(page).locator('[data-testid="script-dialog-save"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsUpdate)?.args)
      .toMatchObject({ id: 's1', secretValues: { token: null } });
  });

  test('a typed secret goes out on Update and never reaches the snapshot', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [...base([]), { channel: IPC.customScriptsUpdate, response: script(['token']) }],
    });
    await openEditor(page);
    await dialog(page).locator('[data-testid="param-secret-value-0"]').fill('s3cr3t');
    await dialog(page).locator('[data-testid="script-dialog-save"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsUpdate)?.args)
      .toMatchObject({ id: 's1', secretValues: { token: 's3cr3t' } });
    expect(JSON.stringify(script(['token']))).not.toContain('s3cr3t');
  });

  test('storage off disables the field and says why', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: base([], OFF) });
    await openEditor(page);
    await expect(dialog(page).locator('[data-testid="param-secret-value-0"]')).toBeDisabled();
    await expect(dialog(page).locator('[data-testid="param-secret-off-0"]')).toBeVisible();
  });

  test('the run dialog runs with a saved secret without typing', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...base(['token']),
        { channel: IPC.scriptRunsPreview, response: preview(true, []) },
        { channel: IPC.scriptRunsStart, response: { runId: 'r1', terminal: null } },
      ],
    });
    await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
    await page.locator('[data-testid="script-s1"] [data-testid="script-play"]').click();
    const input = page.locator('[data-testid="run-param-input-token"]');
    await expect(input).toHaveValue('');
    await expect(input).toHaveAttribute('placeholder', /Saved/);
    await expect(page.locator('[data-testid="run-env-row"]')).toContainText('••••');
    await expect(page.locator('[data-testid="run-start"]')).toBeEnabled();
    await page.locator('[data-testid="run-start"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStart)?.args)
      .toMatchObject({ scriptId: 's1' });
  });

  test('Run without asking unlocks once the required secret is saved', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: base(['token']) });
    await openEditor(page);
    await dialog(page).locator('[data-testid="script-dialog-tab-schedule"]').click();
    await dialog(page).locator('[data-testid="script-schedule"]').click();
    await expect(dialog(page).locator('[data-testid="schedule-quiet"]')).toBeEnabled();
    await expect(dialog(page).locator('[data-testid="schedule-quiet-why"]')).toHaveCount(0);
  });

  test('Run without asking stays locked while the required secret has no saved value', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: base([]) });
    await openEditor(page);
    await dialog(page).locator('[data-testid="script-dialog-tab-schedule"]').click();
    await dialog(page).locator('[data-testid="script-schedule"]').click();
    await expect(dialog(page).locator('[data-testid="schedule-quiet"]')).toBeDisabled();
    await expect(dialog(page).locator('[data-testid="schedule-quiet-why"]')).toBeVisible();
  });
}
