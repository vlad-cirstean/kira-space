import { expect, type Page } from '@playwright/test';
import { scriptFileViewSchema, scriptsSnapshotSchema } from '@shared/domain/scripts';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  'configFilesOpen' | 'customScriptsFile' | 'customScriptsList' | 'customScriptsSaveFile'
>;

export interface Options {
  extraBase: readonly { channel: string; response?: unknown }[];
  modeTab: (page: Page) => ReturnType<Page['locator']>;
}

// A run is a YAML file. Backend half: termflow TestRunsConfigFile (Studio and Space) writes the
// runs-file contract: a valid file, one edited to an error on line 3, and the fixed text.
export function registerFileScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  const snapshot = (key: string) =>
    contract('runs-file', `snapshot#${key}`, { schema: scriptsSnapshotSchema });
  const file = (key: string) =>
    contract('runs-file', `file#${key}`, { schema: scriptFileViewSchema });

  const dialog = (page: Page) => page.locator('[data-testid="run-file-dialog"]');
  const base = (list: unknown, view?: unknown) => [
    ...options.extraBase,
    { channel: IPC.customScriptsList, response: list },
    ...(view ? [{ channel: IPC.customScriptsFile, response: view }] : []),
  ];

  async function openPanel(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  test('contract: a valid run file is a row under its collection', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: base(snapshot('valid')) });
    await openPanel(page);
    await expect(page.locator('[data-testid="script-group"][data-name="Ops"]')).toBeVisible();
    const row = page.locator('[data-testid="script-deploy"]');
    await expect(row).toContainText('Deploy');
    await expect(row.locator('[data-testid="script-row-problem"]')).toHaveCount(0);
  });

  test('contract: a file with an error keeps its row, marked, on the last good version', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: base(snapshot('bad')) });
    await openPanel(page);
    const row = page.locator('[data-testid="script-deploy"]');
    await expect(row).toContainText('Ship');
    await expect(row.locator('[data-testid="script-row-problem"]')).toBeVisible();
  });

  test('contract: Edit file shows the line error; Save with the fixed text clears it', async ({
    relaunch,
  }) => {
    const bad = file('bad');
    const edited = file('edited');
    const { window: page, control } = await relaunch({
      control: [
        ...base(snapshot('bad'), bad),
        { channel: IPC.customScriptsSaveFile, response: edited },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="script-deploy"]').dblclick();
    await expect(dialog(page)).toBeVisible();
    await expect(dialog(page).locator('[data-testid="config-file-status"]')).toContainText(
      'Line 3',
    );
    await expect(dialog(page).locator('[data-testid="run-file-last-good"]')).toBeVisible();
    await expect(dialog(page).locator('[data-testid="run-file-text"]')).toHaveValue(bad.text);

    await dialog(page).locator('[data-testid="run-file-text"]').fill(edited.text);
    await dialog(page).locator('[data-testid="run-file-save"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSaveFile)?.args)
      .toEqual({ file: 'deploy.yaml', text: edited.text, baseHash: bad.hash });
    await expect(dialog(page).locator('[data-testid="config-file-status"]')).toContainText('Valid');
    await expect(dialog(page).locator('[data-testid="run-file-last-good"]')).toHaveCount(0);
  });

  test('a save over a file changed elsewhere offers Reload and Overwrite', async ({ relaunch }) => {
    const edited = file('edited');
    const { window: page } = await relaunch({
      control: [
        ...base(snapshot('valid'), edited),
        {
          channel: IPC.customScriptsSaveFile,
          error: { code: 'E_CONFLICT', message: 'The file changed since it was read.' },
        },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="script-deploy"]').dblclick();
    await dialog(page).locator('[data-testid="run-file-text"]').fill(`${edited.text}# note\n`);
    await dialog(page).locator('[data-testid="run-file-save"]').click();
    await expect(dialog(page).locator('[data-testid="run-file-save-error"]')).toBeVisible();
    await expect(dialog(page).locator('[data-testid="run-file-reload"]')).toBeVisible();
    await expect(dialog(page).locator('[data-testid="run-file-overwrite"]')).toBeVisible();
  });

  test('Open sends the runs folder and the file name', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...base(snapshot('valid'), file('edited')),
        { channel: IPC.configFilesOpen, response: null },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="script-deploy"]').dblclick();
    await dialog(page).locator('[data-testid="config-file-open"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.configFilesOpen)?.args)
      .toEqual({ folder: 'runs', file: 'deploy.yaml' });
  });

  test('a file with no valid version is listed as a broken row that opens', async ({
    relaunch,
  }) => {
    const valid = snapshot('valid');
    const list = {
      ...valid,
      collections: [],
      scripts: [],
      problems: [
        { file: 'Bad Name.yaml', id: '', line: 0, message: 'Invalid file name.', lastGood: false },
      ],
    };
    const view = {
      ...file('bad'),
      file: 'Bad Name.yaml',
      id: '',
      lastGood: false,
      problem: { line: 0, message: 'Invalid file name.' },
    };
    const { window: page } = await relaunch({ control: base(list, view) });
    await openPanel(page);
    const row = page.locator('[data-testid="script-broken-Bad Name.yaml"]');
    await expect(row).toBeVisible();
    await row.dblclick();
    await expect(dialog(page).locator('[data-testid="config-file-status"]')).toContainText(
      'Invalid file name.',
    );
  });
}
