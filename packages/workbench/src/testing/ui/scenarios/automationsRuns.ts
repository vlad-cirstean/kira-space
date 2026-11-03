import { expect, type Locator, type Page } from '@playwright/test';
import { scriptRunSchema } from '@shared/domain/scriptRuns';
import { customScriptSchema } from '@shared/domain/scripts';
import { type ControlSnapshot, emitWailsEvent } from '../mockRuntime';
import type { ScenarioEnv } from './env';

type Env = ScenarioEnv<
  | 'customScriptsList'
  | 'scriptRunsChanged'
  | 'scriptRunsList'
  | 'scriptRunsResolveDir'
  | 'scriptRunsStop'
  | 'terminalOpen'
>;

export interface Options {
  extraBase: readonly ControlSnapshot[];
  modeTab: (page: Page) => Locator;
  /** The home directory the legacy-home notice shows for a script resolving to `dirPath`. */
}

export function registerRunsScenarios(env: Env, options: Options): void {
  const { test, contract, ipc: IPC } = env;

  // P242 Part 1: Automations runs on the mocked bridge. A run's push event is emitted by hand; the
  // terminal id is the client-generated tab id, read back from the terminalOpen call.

  const SCRIPT = {
    id: 'script-1',
    kind: 'script',
    params: [],
    smart: null,
    name: 'Build all',
    command: 'make build',
    workingDir: '',
    dirMode: 'kira',
    useAdeDir: false,
    color: 'blue',
    collection: '',
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };

  const DIR = {
    path: '/kira/automations/script-1',
    mode: 'kira',
    base: '/kira',
    blocker: '',
    branch: '',
    pending: false,
  };

  const BASE: ControlSnapshot[] = [
    ...options.extraBase,
    { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
    { channel: IPC.scriptRunsResolveDir, response: DIR },
    { channel: IPC.customScriptsList, response: { collections: [], scripts: [SCRIPT] } },
  ];

  function run(terminalId: string, state: string, extra: Record<string, unknown> = {}) {
    const finished = state !== 'running';
    return {
      id: 'run-1',
      scriptId: SCRIPT.id,
      scriptName: SCRIPT.name,
      color: SCRIPT.color,
      kind: 'script',
      trigger: 'terminal',
      state,
      terminalId,
      cwd: DIR.path,
      command: SCRIPT.command,
      outcome: finished
        ? { status: state, reason: 'finished', source: 'exit', reported: false }
        : null,
      createdAt: 1_000,
      startedAt: 1_000,
      finishedAt: finished ? 4_000 : null,
      taskId: '',
      taskTitle: '',
      branchId: '',
      branchLabel: '',
      ...extra,
    };
  }

  async function startRun(page: Page, control: { log(): { channel: string; args?: unknown }[] }) {
    await page.locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-play"]`).click();
    let terminalId = '';
    await expect
      .poll(() => {
        const e = control.log().find((x) => x.channel === IPC.terminalOpen);
        terminalId = (e?.args as { terminalId?: string } | undefined)?.terminalId ?? '';
        return terminalId;
      })
      .not.toBe('');
    return terminalId;
  }

  const modeTab = options.modeTab;

  async function installClipboardSpy(page: Page): Promise<void> {
    await page.evaluate(() => {
      const w = window as unknown as { __clipboard: string[] };
      w.__clipboard = [];
      navigator.clipboard.writeText = (text: string) => {
        w.__clipboard.push(text);
        return Promise.resolve();
      };
    });
  }

  test('the module is named Runs', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: [] });
    await expect(modeTab(page)).toHaveAccessibleName('Runs');
    await modeTab(page).click();
    await expect(page.locator('[data-testid="automations-panel"]')).toContainText('Runs');
    await expect(page.locator('[data-testid="automations-panel"]')).toContainText('No scripts');
    await expect(page.locator('[data-testid="automations-add"]')).toBeVisible();
    await expect(page.locator('[data-testid="automations-start"]')).toContainText(
      'Run a script from the panel',
    );
  });

  test('a run shows a spinner, a tab badge and the status bar count, then its result', async ({
    relaunch,
  }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await modeTab(page).click();
    const terminalId = await startRun(page, control);
    const open = control.log().find((e) => e.channel === IPC.terminalOpen);
    expect((open?.args as { scriptId?: string } | undefined)?.scriptId).toBe(SCRIPT.id);

    await emitWailsEvent(page, IPC.scriptRunsChanged, run(terminalId, 'running'));
    await expect(
      page.locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-running"]`),
    ).toBeVisible();
    await expect(page.locator('[data-testid="status-runs"]')).toContainText('1 running');
    await expect(page.locator('[data-testid="run-row"][data-state="running"]')).toHaveCount(1);
    await expect(
      page.locator('[data-testid="script-run-strip"] [data-testid="run-status"]'),
    ).toHaveText('Running');

    await emitWailsEvent(page, IPC.scriptRunsChanged, run(terminalId, 'done'));
    await expect(page.locator('[data-testid="status-runs"]')).toHaveCount(0);
    await expect(page.locator('[data-testid="run-row"]')).toHaveCount(0);
    await expect(page.locator('[data-testid="script-run-outcome"]')).toBeVisible();
    await expect(page.locator('[data-testid="tab-badge"]')).toHaveCount(1);
  });

  test('a failed run shows its reason and Copy for agent copies it', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await modeTab(page).click();
    const terminalId = await startRun(page, control);
    await installClipboardSpy(page);
    await emitWailsEvent(
      page,
      IPC.scriptRunsChanged,
      run(terminalId, 'failed', {
        outcome: {
          status: 'failed',
          reason: 'exited with status 3',
          source: 'exit',
          reported: false,
          exitCode: 3,
        },
      }),
    );
    const outcome = page.locator('[data-testid="script-run-outcome"]');
    await expect(outcome.locator('[data-testid="run-outcome-reason"]')).toHaveText(
      'exited with status 3',
    );
    await outcome.locator('[data-testid="run-copy"]').click();
    await expect
      .poll(() =>
        page.evaluate(
          () => (window as unknown as { __clipboard: string[] }).__clipboard.at(-1) ?? '',
        ),
      )
      .toContain('exited with status 3');
    const copied = await page.evaluate(
      () => (window as unknown as { __clipboard: string[] }).__clipboard.at(-1) ?? '',
    );
    expect(copied).toContain('Script: Build all');
    expect(copied).toContain('Exit code: 3');
  });

  test('Run again opens a second terminal for the same script', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({ control: BASE });
    await modeTab(page).click();
    const terminalId = await startRun(page, control);
    await emitWailsEvent(page, IPC.scriptRunsChanged, run(terminalId, 'failed'));
    await page.locator('[data-testid="script-run-outcome"] [data-testid="run-again"]').click();
    await expect
      .poll(() => control.log().filter((e) => e.channel === IPC.terminalOpen).length)
      .toBe(2);
  });

  test('Stop on a running row stops that run', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [...BASE, { channel: IPC.scriptRunsStop, response: null }],
    });
    await modeTab(page).click();
    const terminalId = await startRun(page, control);
    await emitWailsEvent(page, IPC.scriptRunsChanged, run(terminalId, 'running'));
    await page.locator('[data-testid="run-row"] [data-testid="run-stop"]').click();
    await expect
      .poll(() => control.log().find((e) => e.channel === IPC.scriptRunsStop)?.args)
      .toEqual({ id: 'run-1' });
  });

  test('a blocked folder shows the reason and opens no terminal', async ({ relaunch }) => {
    const blocked = {
      ...DIR,
      blocker: '/kira/automations/script-1 is not a real folder: remove it',
    };
    const { window: page, control } = await relaunch({
      control: [
        ...BASE.filter((s) => s.channel !== IPC.scriptRunsResolveDir),
        { channel: IPC.scriptRunsResolveDir, response: blocked },
      ],
    });
    await modeTab(page).click();
    await page.locator(`[data-testid="script-${SCRIPT.id}"] [data-testid="script-play"]`).click();
    await expect(page.locator('[data-testid="script-error"]')).toHaveText(blocked.blocker);
    expect(control.log().some((e) => e.channel === IPC.terminalOpen)).toBe(false);
  });

  // Contract automations. Backend half: termflow TestScriptRunLifecycle, TestScriptRunNonZeroExit and
  // TestScriptRunStopAndClose store the script and its finished run for each outcome.
  const CONTRACT_OUTCOMES = [
    { key: 'done', label: 'Succeeded', reason: undefined },
    { key: 'failed', label: 'Failed', reason: 'exited with status 3' },
    { key: 'cancelled', label: 'Cancelled', reason: 'stopped by you' },
  ] as const;

  for (const outcome of CONTRACT_OUTCOMES) {
    test(`contract: a script run ends ${outcome.label}`, async ({ relaunch }) => {
      const script = contract('automations', `CustomScriptsService.Create#${outcome.key}`, {
        schema: customScriptSchema,
      });
      const finished = contract('automations', `ScriptRunsService.Get#${outcome.key}`, {
        schema: scriptRunSchema,
      });
      const { window: page, control } = await relaunch({
        control: [
          { channel: IPC.terminalOpen, response: { shell: '/bin/zsh' } },
          {
            channel: IPC.scriptRunsResolveDir,
            response: { ...DIR, path: finished.cwd },
          },
          { channel: IPC.customScriptsList, response: { collections: [], scripts: [script] } },
        ],
      });
      await modeTab(page).click();
      await page.locator(`[data-testid="script-${script.id}"] [data-testid="script-play"]`).click();
      let terminalId = '';
      await expect
        .poll(() => {
          const e = control.log().find((x) => x.channel === IPC.terminalOpen);
          terminalId = (e?.args as { terminalId?: string } | undefined)?.terminalId ?? '';
          return terminalId;
        })
        .not.toBe('');

      const times = { createdAt: 1_000, startedAt: 1_000 };
      await emitWailsEvent(page, IPC.scriptRunsChanged, {
        ...finished,
        ...times,
        terminalId,
        state: 'running',
        outcome: null,
        finishedAt: null,
      });
      await expect(page.locator('[data-testid="status-runs"]')).toContainText('1 running');
      await emitWailsEvent(page, IPC.scriptRunsChanged, {
        ...finished,
        ...times,
        terminalId,
        finishedAt: 4_000,
      });
      await expect(page.locator('[data-testid="status-runs"]')).toHaveCount(0);
      await expect(
        page.locator('[data-testid="script-run-strip"] [data-testid="run-status"]'),
      ).toHaveText(outcome.label);
      if (outcome.reason) {
        await expect(
          page.locator('[data-testid="script-run-outcome"] [data-testid="run-outcome-reason"]'),
        ).toHaveText(outcome.reason);
      }
    });
  }
}
