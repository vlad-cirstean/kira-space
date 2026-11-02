import { expect, type Locator, type Page } from '@playwright/test';
import {
  type ScriptRunRestoreItem,
  scriptRunRestoreResultSchema,
  scriptRunRestoreStateSchema,
} from '@shared/domain/scriptRuns';
import type { ControlSnapshot } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsList'
  | 'scriptRunsList'
  | 'scriptRunsReadLog'
  | 'scriptRunsRestore'
  | 'scriptRunsRestoreDismiss'
  | 'scriptRunsRestoreState'
  | 'terminalOpen'
>;

export interface Options {
  /** Both apps' extra boot answers, e.g. Kira Space's window mode. */
  extraBase: readonly ControlSnapshot[];
  modeTab: (page: Page) => Locator;
}

// P441: the Runs banner after a restart and the shared restore dialog it opens. Backend half: termflow
// TestRunsRestore (both apps) stores the offer, the confirmed args and the result.
export function registerRestoreScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  const state = () =>
    contract('runs-restore', 'ScriptRunsService.RestoreState#pending', {
      schema: scriptRunRestoreStateSchema,
    });
  const args = () =>
    contract<{ items: unknown[] }>('runs-restore', 'args:ScriptRunsService.Restore');
  const result = () =>
    contract('runs-restore', 'ScriptRunsService.Restore', { schema: scriptRunRestoreResultSchema });
  const named = (name: string): ScriptRunRestoreItem => {
    const item = state().items.find((i) => i.scriptName === name);
    if (!item) throw new Error(`no restore item ${name}`);
    return item;
  };
  const TOKEN = 'tok-restore-1';

  // The two normal scripts the confirmed runs open terminals for.
  const script = (item: ScriptRunRestoreItem) => ({
    id: item.scriptId,
    kind: 'script',
    params: [],
    smart: null,
    name: item.scriptName,
    command: item.command,
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    color: item.color,
    collectionId: null,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  });

  const base = (...offers: unknown[]): ControlSnapshot[] => [
    ...options.extraBase,
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
    { channel: IPC.scriptRunsList, response: [] },
    { channel: IPC.scriptRunsReadLog, response: { chunks: [], truncated: false } },
    {
      channel: IPC.customScriptsList,
      response: { collections: [], scripts: [script(named('plain')), script(named('param'))] },
    },
    ...offers.map((response) => ({ channel: IPC.scriptRunsRestoreState, response })),
  ];

  // The Go result with the terminal token a spec can recognise.
  const started = () => ({
    items: result().items.map((o) =>
      o.started?.terminal
        ? { ...o, started: { ...o.started, terminal: { ...o.started.terminal, token: TOKEN } } }
        : o,
    ),
  });

  const banner = (page: Page) => page.locator('[data-testid="runs-restore-banner"]');
  const dialog = (page: Page) => page.locator('[data-testid="restore-dialog"]');
  const row = (page: Page, id: string) =>
    page.locator(`[data-testid="restore-row"][data-id="${id}"]`);
  const calls = (control: { log(): { channel: string; args?: unknown }[] }, channel: string) =>
    control.log().filter((e) => e.channel === channel);

  async function openDialog(page: Page): Promise<void> {
    await options.modeTab(page).click();
    await banner(page).locator('[data-testid="runs-restore-review"]').click();
    await expect(dialog(page)).toBeVisible();
  }

  test('contract: nothing opens at start; the banner counts the runs and Review lists them', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: base(state(), state()) });
    await expect(page.locator('[data-testid="mode-tab"]').first()).toBeVisible();
    await expect(dialog(page)).toHaveCount(0);
    await options.modeTab(page).click();
    await expect(banner(page)).toContainText('4 scripts were running when');
    await expect(dialog(page)).toHaveCount(0);

    await banner(page).locator('[data-testid="runs-restore-review"]').click();
    await expect(dialog(page)).toBeVisible();
    for (const name of ['plain', 'param', 'smart']) {
      const r = row(page, named(name).runId);
      await expect(r).toHaveAttribute('data-state', 'ready');
      await expect(r.locator('[data-slot="checkbox"]')).toHaveAttribute('data-state', 'checked');
    }
    await expect(row(page, named('plain').runId)).toContainText(named('plain').command);
    await expect(row(page, named('plain').runId)).toContainText(named('plain').cwd);
    await expect(row(page, named('smart').runId)).toContainText('Continues its conversation');
    await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('3 of 3 selected');

    await page.locator('[data-testid="restore-unavailable-toggle"]').click();
    const blocked = row(page, named('secret').runId);
    await expect(blocked).toHaveAttribute('data-state', 'unavailable');
    await expect(blocked).toContainText('Save the secret values in the script');
    expect(calls(control, IPC.scriptRunsRestore)).toHaveLength(0);
  });

  test('contract: deselecting one and confirming starts the rest and keeps the banner', async ({
    relaunch,
  }) => {
    const remaining = {
      items: state().items.filter((i) => ['param', 'secret'].includes(i.scriptName)),
    };
    const { window: page, control } = await relaunch({
      control: [
        ...base(state(), state(), remaining),
        { channel: IPC.scriptRunsRestore, response: started() },
      ],
    });
    await openDialog(page);

    await row(page, named('param').runId).locator('[data-slot="checkbox"]').click();
    await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('2 of 3 selected');
    await page.locator('[data-testid="restore-submit"]').click();

    await expect(dialog(page)).toHaveCount(0);
    expect(calls(control, IPC.scriptRunsRestore)[0]?.args).toEqual(args());
    // Two tabs open, the smart run's last; the terminal starts once its tab shows.
    const tabs = page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
    await expect(tabs).toHaveCount(2);
    await tabs.filter({ hasText: 'plain' }).click();
    await expect
      .poll(() => {
        const open = calls(control, IPC.terminalOpen)[0]?.args as
          | { scriptId?: string; scriptLaunchToken?: string }
          | undefined;
        return [open?.scriptId, open?.scriptLaunchToken];
      })
      .toEqual([named('plain').scriptId, TOKEN]);
    await expect(banner(page)).toContainText('2 scripts were running when');
  });

  test('Select none disables Restore, Select all brings it back, Escape keeps the banner', async ({
    relaunch,
  }) => {
    const { window: page } = await relaunch({ control: base(state(), state()) });
    await openDialog(page);
    const submit = page.locator('[data-testid="restore-submit"]');
    await expect(submit).toBeEnabled();

    await page.locator('[data-testid="restore-select-none"]').click();
    await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('0 of 3 selected');
    await expect(submit).toBeDisabled();
    await page.locator('[data-testid="restore-select-all"]').click();
    await expect(page.locator('[data-testid="restore-selection"]')).toHaveText('3 of 3 selected');
    await expect(submit).toBeEnabled();

    await page.keyboard.press('Escape');
    await expect(dialog(page)).toHaveCount(0);
    await expect(banner(page)).toBeVisible();
  });

  test('Don’t restore forgets the offer and the banner goes', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        ...base(state(), state(), { items: [] }),
        { channel: IPC.scriptRunsRestoreDismiss, response: null },
      ],
    });
    await openDialog(page);
    await page.locator('[data-testid="restore-dismiss"]').click();

    await expect(banner(page)).toHaveCount(0);
    expect(calls(control, IPC.scriptRunsRestoreDismiss)).toHaveLength(1);
    expect(calls(control, IPC.scriptRunsRestore)).toHaveLength(0);
  });

  test('a failed item keeps the dialog open with its row error', async ({ relaunch }) => {
    const plain = named('plain');
    const failed = {
      items: [
        {
          runId: plain.runId,
          started: null,
          code: 'E_CONFLICT',
          error: 'The script changed since the preview. Preview it again.',
        },
      ],
    };
    const { window: page } = await relaunch({
      control: [
        ...base(state(), state(), state()),
        { channel: IPC.scriptRunsRestore, response: failed },
      ],
    });
    await openDialog(page);
    await page.locator('[data-testid="restore-submit"]').click();

    await expect(row(page, plain.runId).locator('[data-testid="restore-row-error"]')).toContainText(
      'changed since the preview',
    );
    await expect(dialog(page)).toBeVisible();
  });
}
