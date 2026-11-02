import { expect, type Locator, type Page } from '@playwright/test';
import { customScriptSchema } from '@shared/domain/scripts';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsChanged'
  | 'customScriptsCreate'
  | 'customScriptsList'
  | 'customScriptsUpdate'
  | 'scriptRunsMcpServers'
  | 'scriptRunsNextFires'
  | 'scriptRunsPreview'
  | 'scriptRunsResolveDir'
>;

export type Ctx = {
  COMMON: ControlSnapshot[];
  dialog: (page: Page) => Locator;
  command: (page: Page) => Locator;
  suggestions: (page: Page) => Locator;
  openNew: (page: Page, item: 'new-script' | 'new-smart-script') => Promise<void>;
  typeBody: (page: Page, text: string) => Promise<void>;
};

export function registerEditorScenarios(env: Env, extra?: (ctx: Ctx) => void): void {
  const { test, contract, ipc: IPC } = env;

  // P254: the three-tab script editor on the mocked bridge. Backend half: termflow
  // TestScriptEditorSave stores the contract script-editor fixtures.

  const DIR = {
    path: '/kira/automations/x',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };
  const FIRES = [1_900_000_000_000, 1_900_086_400_000, 1_900_172_800_000];

  const COMMON: ControlSnapshot[] = [
    { channel: IPC.scriptRunsResolveDir, response: DIR },
    { channel: IPC.scriptRunsNextFires, response: FIRES },
    { channel: IPC.scriptRunsMcpServers, response: { source: 'agent', servers: [] } },
  ];

  const dialog = (page: Page) => page.locator('[data-testid="script-dialog"]');
  const command = (page: Page) => dialog(page).locator('[data-testid="script-dialog-command"]');
  const suggestions = (page: Page) => page.locator('.autocomplete-suggestions');

  async function openPanel(page: Page): Promise<void> {
    await page.locator('[data-testid="mode-tab"][data-mode="automations"]').click();
    await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  }

  async function openNew(page: Page, item: 'new-script' | 'new-smart-script'): Promise<void> {
    await openPanel(page);
    await page.locator('[data-testid="automations-add"]').click();
    await page.locator(`[data-testid="menu-item-${item}"]`).click();
    await expect(dialog(page)).toBeVisible();
  }

  async function addParam(
    page: Page,
    i: number,
    name: string,
    opts: { def?: string; secret?: boolean } = {},
  ): Promise<void> {
    await dialog(page).locator('[data-testid="script-dialog-tab-params"]').click();
    await dialog(page).locator('[data-testid="param-add"]').click();
    await dialog(page).locator(`[data-testid="param-name-${i}"]`).fill(name);
    if (opts.def) await dialog(page).locator(`[data-testid="param-default-${i}"]`).fill(opts.def);
    if (opts.secret) await dialog(page).locator(`[data-testid="param-secret-${i}"]`).click();
    await dialog(page).locator('[data-testid="script-dialog-tab-script"]').click();
  }

  // Types into the body from a clean slate, key by key, so the completion popup sees real input.
  async function typeBody(page: Page, text: string): Promise<void> {
    const body = command(page);
    await body.fill('');
    await body.click();
    await body.pressSequentially(text);
  }

  test('contract: save a smart script from the three tabs; the run dialog resolves its param', async ({
    relaunch,
  }) => {
    const script = contract('script-editor', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const created = contract<{ fields: Record<string, unknown> & { schedule: { cron: string } } }>(
      'script-editor',
      'args:CustomScriptsService.Create',
    );
    const preview = contract<{ prompt: { var: string; value: string }[] }>(
      'script-editor',
      'ScriptRunsService.Preview',
    );
    const collection = {
      id: script.collectionId as string,
      name: 'Ops',
      sortOrder: 0,
      createdAt: script.createdAt,
      updatedAt: script.updatedAt,
    };
    const { window: page, control } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [collection], scripts: [] } },
        { channel: IPC.customScriptsCreate, response: script },
      ],
    });
    await openNew(page, 'new-smart-script');
    const d = dialog(page);

    await d.locator('[data-testid="script-dialog-name"]').fill('triage');
    const order = await d
      .locator(
        '[data-testid="script-dialog-name"], [data-testid="script-dialog-collection"], [data-testid="script-dialog-command"]',
      )
      .evaluateAll((els) => els.map((e) => e.getAttribute('data-testid')));
    expect(order).toEqual([
      'script-dialog-name',
      'script-dialog-collection',
      'script-dialog-command',
    ]);
    await d.locator('[data-testid="script-dialog-collection"]').selectOption(collection.id);

    await d.locator('[data-testid="script-dialog-tab-params"]').click();
    await d.locator('[data-testid="param-add"]').click();
    await d.locator('[data-testid="param-name-0"]').fill('topic');
    await d.locator('[data-testid="param-default-0"]').fill('auth');

    await d.locator('[data-testid="script-dialog-tab-script"]').click();
    await command(page).click();
    await command(page).pressSequentially('Look at {to');
    await expect(suggestions(page)).toContainText('topic');
    await command(page).press('Tab');
    await expect(command(page)).toHaveValue('Look at {topic}');
    const used = preview.prompt.find((p) => p.var);
    if (!used) throw new Error('script-editor preview has no variable');
    await expect(
      d.locator(`[data-testid="script-dialog-uses"] [data-var="${used.var}"]`),
    ).toContainText(used.value);

    await d.locator('[data-testid="script-dialog-tab-schedule"]').click();
    await d.locator('[data-testid="script-schedule"]').click();
    await d.locator('[data-testid="schedule-cron"]').fill(created.fields.schedule.cron);
    await d.locator('[data-testid="script-dialog-save"]').click();

    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsCreate)?.args)
      .toMatchObject({
        fields: {
          name: created.fields.name,
          kind: created.fields.kind,
          command: created.fields.command,
          collectionId: created.fields.collectionId,
          params: created.fields.params,
          schedule: { cron: created.fields.schedule.cron, enabled: true, confirm: true },
        },
      });
  });

  test('contract: the run dialog shows the saved param resolved', async ({ relaunch }) => {
    const script = contract('script-editor', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const preview = contract<{ prompt: { var: string; value: string }[] }>(
      'script-editor',
      'ScriptRunsService.Preview',
    );
    const { window: page } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
        { channel: IPC.scriptRunsPreview, response: preview },
      ],
    });
    await openPanel(page);
    await page.locator(`[data-testid="script-${script.id}"] [data-testid="script-play"]`).click();
    await expect(
      page.locator(
        '[data-testid="run-dialog"] [data-testid="run-prompt-text"] [data-testid="var-chip"][data-var="topic"]',
      ),
    ).toContainText(preview.prompt.find((p) => p.var === 'topic')?.value ?? '');
  });

  test('prompt completion: a bare { lists params, a secret is never offered, Enter is a newline', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
      ],
    });
    await openNew(page, 'new-smart-script');
    await addParam(page, 0, 'topic', { def: 'auth' });
    await addParam(page, 1, 'level', { def: 'low' });
    await addParam(page, 2, 'token', { secret: true });

    await typeBody(page, 'Fix {');
    await expect(suggestions(page)).toContainText('topic');
    await expect(suggestions(page)).toContainText('level');
    await expect(suggestions(page)).not.toContainText('token');

    await command(page).press('Enter');
    await expect(suggestions(page)).toHaveCount(0);
    await expect(command(page)).toHaveValue('Fix {\n');

    await typeBody(page, 'Fix {to');
    await command(page).press('Tab');
    await expect(command(page)).toHaveValue('Fix {topic}');
  });

  test('command completion: $KIRA_P suggests every param, a secret included', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
      ],
    });
    await openNew(page, 'new-script');
    await addParam(page, 0, 'topic', { def: 'auth' });
    await addParam(page, 1, 'token', { secret: true });

    await typeBody(page, 'echo $KIRA_P');
    await expect(suggestions(page)).toContainText('KIRA_PARAM_TOPIC');
    await expect(suggestions(page)).toContainText('KIRA_PARAM_TOKEN');
    await command(page).press('Tab');
    await expect(command(page)).toHaveValue('echo $KIRA_PARAM_TOPIC');
  });

  test('tabs flag their errors with a badge and Save waits', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [] } },
      ],
    });
    await openNew(page, 'new-script');
    const d = dialog(page);
    const badge = (tab: string) => d.locator(`[data-testid="script-dialog-tab-${tab}-errors"]`);
    const save = d.locator('[data-testid="script-dialog-save"]');
    const saveWaits = async (): Promise<void> => {
      await save.click();
      await expect(d).toBeVisible();
      expect(control.log().filter((e) => e.channel === IPC.customScriptsCreate)).toHaveLength(0);
    };

    await d.locator('[data-testid="script-dialog-name"]').fill('build');
    await command(page).fill('make');
    await expect(save).toBeEnabled();
    await d.locator('[data-testid="script-dialog-name"]').fill('');
    await expect(badge('script')).toHaveText('1');
    await saveWaits();
    await d.locator('[data-testid="script-dialog-name"]').fill('build');
    await expect(badge('script')).toHaveCount(0);

    await d.locator('[data-testid="script-dialog-tab-params"]').click();
    await d.locator('[data-testid="param-add"]').click();
    await d.locator('[data-testid="param-name-0"]').fill('Bad');
    await expect(badge('params')).toHaveText('1');
    await saveWaits();
    await d.locator('[data-testid="param-name-0"]').fill('good');
    await expect(badge('params')).toHaveCount(0);

    await d.locator('[data-testid="script-dialog-tab-schedule"]').click();
    await d.locator('[data-testid="script-schedule"]').click();
    await d.locator('[data-testid="schedule-cron"]').fill('');
    await expect(badge('schedule')).toHaveText('1');
    await saveWaits();
    await d.locator('[data-testid="schedule-cron"]').fill('0 9 * * 1-5');
    await expect(badge('schedule')).toHaveCount(0);
    await expect(save).toBeEnabled();
  });

  test('creating offers New script and New smart script once; Edit schedule opens the Schedule tab', async ({
    relaunch,
  }) => {
    const script = contract('script-editor', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const { window: page } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
      ],
    });
    await openPanel(page);
    await page.locator('[data-testid="automations-add"]').click();
    await expect(page.locator('[data-testid="menu-item-new-script"]')).toBeVisible();
    await expect(page.locator('[data-testid="menu-item-new-smart-script"]')).toBeVisible();
    await expect(page.locator('[data-testid="menu-item-new-recurring"]')).toHaveCount(0);
    await page.keyboard.press('Escape');

    await page.locator(`[data-testid="script-${script.id}"]`).click({ button: 'right' });
    await expect(page.locator('[data-testid="menu-item-toggle-schedule"]')).toHaveCount(0);
    await page.locator('[data-testid="menu-item-edit-schedule"]').click();
    await expect(dialog(page).locator('[data-testid="schedule-fields"]')).toBeVisible();
    await expect(
      dialog(page).locator('[data-testid="script-dialog-tab-schedule"]'),
    ).toHaveAttribute('data-state', 'on');
  });

  test('contract: Edit schedule turns the schedule off on Save', async ({ relaunch }) => {
    const script = contract('script-editor', 'CustomScriptsService.Create', {
      schema: customScriptSchema,
    });
    const sent = contract<{
      fields: { schedule: { cron: string; enabled: boolean; confirm: boolean } };
    }>('script-editor', 'args:CustomScriptsService.Update#schedule-off');
    const updated = contract('script-editor', 'CustomScriptsService.Update#schedule-off', {
      schema: customScriptSchema,
    });
    const { window: page, control } = await relaunch({
      control: [
        ...COMMON,
        { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
        { channel: IPC.customScriptsUpdate, response: updated },
      ],
    });
    await openPanel(page);
    const row = page.locator(`[data-testid="script-${script.id}"]`);
    await expect(row.locator('[data-testid="script-next"]')).toContainText('Next');

    await row.click({ button: 'right' });
    await page.locator('[data-testid="menu-item-edit-schedule"]').click();
    await dialog(page).locator('[data-testid="schedule-enabled"]').click();
    await dialog(page).locator('[data-testid="script-dialog-save"]').click();

    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.customScriptsUpdate)?.args)
      .toMatchObject({ id: script.id, fields: { schedule: sent.fields.schedule } });
    await emitWailsEvent(page, IPC.customScriptsChanged, { collections: [], scripts: [updated] });
    await expect(row.locator('[data-testid="script-next"]')).toHaveText('Off');
  });

  extra?.({ COMMON, dialog, command, suggestions, openNew, typeBody });
}
