import { expect, type Page } from '@playwright/test';
import { type CustomScript, customScriptSchema } from '@shared/domain/scripts';
import type { SecretStorageStatus } from '@shared/domain/secrets';
import type { ControlSnapshot } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsList'
  | 'customScriptsFile'
  | 'customScriptsSetSecrets'
  | 'scriptRunsNextFires'
  | 'scriptRunsPreview'
  | 'scriptRunsResolveDir'
  | 'scriptRunsStart'
>;

// P447: a secret param's saved value is write-only in the run file dialog and optional in the run dialog.
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
      collection: '',
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
    {
      channel: IPC.customScriptsList,
      response: { collections: [], scripts: [script(saved)], secretStorage: storage },
    },
    {
      channel: IPC.customScriptsFile,
      response: {
        file: 's1.yaml',
        id: 's1',
        path: '/kira/runs/s1.yaml',
        text: 'name: deploy\n',
        hash: 'h1',
        problem: null,
        lastGood: false,
        editable: true,
      },
    },
  ];
  const dialog = (page: Page) => page.locator('[data-testid="run-file-dialog"]');

  async function openEditor(page: Page): Promise<void> {
    await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
    await page.locator('[data-testid="script-s1"]').click({ button: 'right' });
    await page.locator('[data-testid="menu-item-edit-file"]').click();
    await expect(dialog(page)).toBeVisible();
    await dialog(page).locator('[data-testid="run-file-tab-yaml"]').click();
  }

  test('a saved secret is write-only: empty password field, Saved badge, Clear sends null', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({
      control: [...base(['token']), { channel: IPC.customScriptsSetSecrets, response: script([]) }],
    });
    await openEditor(page);
    const field = dialog(page).locator('[data-testid="run-secret-value-token"]');
    await expect(field).toHaveAttribute('type', 'password');
    await expect(field).toHaveValue('');
    await expect(dialog(page).locator('[data-testid="run-secret-saved-token"]')).toBeVisible();
    const eye = dialog(page).locator('[data-testid="run-secret-value-token-reveal"]');
    await expect(eye).toBeDisabled();
    await field.fill('typed');
    await expect(eye).toBeEnabled();
    await eye.click();
    await expect(field).toHaveAttribute('type', 'text');
    await eye.click();
    await expect(field).toHaveAttribute('type', 'password');
    await field.fill('');

    await dialog(page).locator('[data-testid="run-secret-clear-token"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSetSecrets)?.args)
      .toMatchObject({ id: 's1', values: { token: null } });
  });

  test('a typed secret goes out on Save and never reaches the snapshot', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [...base([]), { channel: IPC.customScriptsSetSecrets, response: script(['token']) }],
    });
    await openEditor(page);
    await dialog(page).locator('[data-testid="run-secret-value-token"]').fill('s3cr3t');
    await dialog(page).locator('[data-testid="run-secret-save-token"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSetSecrets)?.args)
      .toMatchObject({ id: 's1', values: { token: 's3cr3t' } });
    expect(JSON.stringify(script(['token']))).not.toContain('s3cr3t');
  });

  test('storage off disables the field and says why', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: base([], OFF) });
    await openEditor(page);
    await expect(dialog(page).locator('[data-testid="run-secret-value-token"]')).toBeDisabled();
    await expect(dialog(page).locator('[data-testid="run-secrets-off"]')).toBeVisible();
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
    const eye = page.locator('[data-testid="run-param-input-token-reveal"]');
    await expect(eye).toBeDisabled();
    await input.fill('typed');
    await eye.click();
    await expect(input).toHaveAttribute('type', 'text');
    await eye.click();
    await expect(input).toHaveAttribute('type', 'password');
    await input.fill('');
    await expect(page.locator('[data-testid="run-env-row"]')).toContainText('••••');
    await expect(page.locator('[data-testid="run-start"]')).toBeEnabled();
    await page.locator('[data-testid="run-start"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStart)?.args)
      .toMatchObject({ scriptId: 's1' });
  });
}
