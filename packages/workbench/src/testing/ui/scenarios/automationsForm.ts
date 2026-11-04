import { expect, type Page } from '@playwright/test';
import {
  runFormResultSchema,
  scriptFileViewSchema,
  scriptsSnapshotSchema,
} from '@shared/domain/scripts';
import { codeEditorText, expectHighlighted, replaceCodeEditorText } from '../codeEditor';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsFile'
  | 'customScriptsForm'
  | 'customScriptsList'
  | 'customScriptsSaveFile'
  | 'customScriptsSetSecrets'
>;

export interface Options {
  extraBase: readonly { channel: string; response?: unknown }[];
  modeTab: (page: Page) => ReturnType<Page['locator']>;
}

// A run file as a form and as YAML over the same draft. Backend half: termflow TestRunsForm
// (Studio and Space) writes the runs-form contract.
export function registerFormScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  const file = (key: string) =>
    contract('runs-form', `file#${key}`, { schema: scriptFileViewSchema });
  const form = (key: string) =>
    contract('runs-form', `form#${key}`, { schema: runFormResultSchema });
  const formArgs = (key: string) => contract('runs-form', `args:CustomScriptsService.Form#${key}`);
  const list = () => {
    const valid = contract('runs-file', 'snapshot#valid', { schema: scriptsSnapshotSchema });
    return {
      ...valid,
      scripts: valid.scripts.map((s) => ({ ...s, id: 'ship', name: 'Ship', collection: 'Ops' })),
    };
  };

  const dialog = (page: Page) => page.locator('[data-testid="run-file-dialog"]');
  const editor = (page: Page) => dialog(page).locator('[data-testid="run-file-text"]');

  function base(
    extra: readonly {
      channel: string;
      args?: unknown;
      response?: unknown;
      error?: { code: string; message: string };
    }[] = [],
  ) {
    const read = file('read');
    return [
      ...options.extraBase,
      { channel: IPC.customScriptsList, response: list() },
      { channel: IPC.customScriptsFile, response: read },
      { channel: IPC.customScriptsForm, args: formArgs('read'), response: form('read') },
      ...extra,
    ];
  }

  async function openDialog(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await page.locator('[data-testid="script-ship"]').dblclick();
    await expect(dialog(page)).toBeVisible();
  }

  test('contract: a form edit lands in the YAML tab with its comments', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: base([
        { channel: IPC.customScriptsForm, args: formArgs('rename'), response: form('renamed') },
      ]),
    });
    await openDialog(page);
    const name = dialog(page).locator('[data-testid="script-dialog-name"]');
    await expect(name).toHaveValue('Ship');
    await name.fill('Release');
    await dialog(page).locator('[data-testid="run-file-tab-yaml"]').click();
    await expect.poll(() => codeEditorText(editor(page))).toBe(form('renamed').text);
    expect(form('renamed').text).toContain('# Ship the build.');
    expect(form('renamed').text).toContain('# shown in the panel');
    await expectHighlighted(editor(page), 'yaml');
  });

  test('contract: a YAML edit shows in the form', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: base([
        { channel: IPC.customScriptsForm, args: formArgs('yaml'), response: form('yaml') },
      ]),
    });
    await openDialog(page);
    await dialog(page).locator('[data-testid="run-file-tab-yaml"]').click();
    await replaceCodeEditorText(page, editor(page), form('yaml').text);
    await dialog(page).locator('[data-testid="run-file-tab-script"]').click();
    await expect(dialog(page).locator('[data-testid="script-dialog-name"]')).toHaveValue('Rollout');
  });

  test('contract: bad YAML disables the form and names the line', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: base([
        { channel: IPC.customScriptsForm, args: formArgs('bad'), response: form('bad') },
      ]),
    });
    await openDialog(page);
    await dialog(page).locator('[data-testid="run-file-tab-yaml"]').click();
    await replaceCodeEditorText(page, editor(page), form('bad').text);
    await dialog(page).locator('[data-testid="run-file-tab-script"]').click();
    await expect(dialog(page).locator('[data-testid="run-form-problem"]')).toContainText(
      `Line ${form('bad').problem?.line}`,
    );
    await expect(dialog(page).locator('[data-testid="run-file-save"]')).toBeDisabled();
  });

  test('contract: Save sends the edited text and the loaded hash', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: base([
        { channel: IPC.customScriptsForm, args: formArgs('rename'), response: form('renamed') },
        { channel: IPC.customScriptsSaveFile, response: file('saved') },
      ]),
    });
    await openDialog(page);
    await dialog(page).locator('[data-testid="script-dialog-name"]').fill('Release');
    await dialog(page).locator('[data-testid="run-file-save"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSaveFile)?.args)
      .toEqual({ file: 'ship.yaml', text: form('renamed').text, baseHash: file('read').hash });
  });

  test('a save over a file changed elsewhere offers Reload and Overwrite', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: base([
        { channel: IPC.customScriptsForm, args: formArgs('rename'), response: form('renamed') },
        {
          channel: IPC.customScriptsSaveFile,
          error: { code: 'E_CONFLICT', message: 'The file changed since it was read.' },
        },
      ]),
    });
    await openDialog(page);
    await dialog(page).locator('[data-testid="script-dialog-name"]').fill('Release');
    await dialog(page).locator('[data-testid="run-file-save"]').click();
    await expect(dialog(page).locator('[data-testid="run-file-reload"]')).toBeVisible();
    await expect(dialog(page).locator('[data-testid="run-file-overwrite"]')).toBeVisible();
  });

  test('a typed secret param value goes through the write-only call, never the file', async ({
    relaunch,
  }) => {
    const saved = file('saved');
    const { window: page, control } = await relaunch({
      control: [
        ...options.extraBase,
        { channel: IPC.customScriptsList, response: list() },
        { channel: IPC.customScriptsFile, response: saved },
        {
          channel: IPC.customScriptsForm,
          args: { file: 'ship.yaml', text: saved.text, edits: [] },
          response: form('edited'),
        },
        { channel: IPC.customScriptsSetSecrets, response: list().scripts[0] },
      ],
    });
    await openDialog(page);
    await dialog(page).locator('[data-testid="run-file-tab-params"]').click();
    await dialog(page).locator('[data-testid="param-secret-value-0"]').fill('hunter2');
    await dialog(page).locator('[data-testid="run-file-save"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsSetSecrets)?.args)
      .toEqual({ id: 'ship', values: { token: 'hunter2' } });
    expect(control.log().some((e) => e.channel === IPC.customScriptsSaveFile)).toBe(false);
  });
}
