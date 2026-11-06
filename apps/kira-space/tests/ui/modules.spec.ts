import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Module switching is multi-step, cross-store state (the mode store, the workspace
// store, and the tabs store's own per-workspace filtering all shift together) — the one thing this
// new surface has no other coverage for. Mirrors Kira Studio's own
// mode-switch.spec.ts, trimmed to the three properties genuinely new here: boot honours a
// persisted mode, a click eventually (never synchronously) reaches windowsSetMode, and a full
// round trip across all three modules leaves the right tabs showing in each.

function modeTab(page: Page, mode: 'git' | 'automations' | 'ade') {
  return page.locator(`[data-testid="mode-tab"][data-mode="${mode}"]`);
}

test('the title bar lists Agents, Runs, Memory, then Git', async ({ relaunch }) => {
  const { window: page } = await relaunch();
  const modes = await page
    .locator('[data-testid="mode-tab"]')
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-mode')));
  expect(modes).toEqual(['ade', 'automations', 'memory', 'git']);
  await expect(modeTab(page, 'git')).toHaveClass(/is-active/);
  const tabs = page.locator('[data-testid="mode-tab"]');
  const names = ['Agents', 'Runs', 'Memory', 'Git'];
  for (const [i, name] of names.entries()) {
    await expect(tabs.nth(i)).toHaveAccessibleName(name);
    expect((await tabs.nth(i).innerText()).trim()).toBe('');
  }
  await modeTab(page, 'ade').hover();
  await expect(page.locator('[data-slot="tooltip-content"]').first()).toContainText('Agents');
});

test('a window boots into whatever mode windowsEnsure answers with, with no Git panel mounted alongside it', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [{ channel: IPC.windowsEnsure, response: { mode: 'automations' } }],
  });

  await expect(modeTab(page, 'automations')).toHaveClass(/is-active/);
  await expect(modeTab(page, 'git')).not.toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="automations-panel"]')).toBeVisible();
  await expect(page.locator('[data-testid="automations-start"]')).toBeVisible();
  // The Git module's own left panel never mounts alongside another module's (WorkbenchShell.vue's
  // own `<component :is="activeModePanel">` — one panel at a time, P128 §2.6).
  await expect(page.locator('[data-testid="git-panel-tab-repos"]')).toHaveCount(0);
});

test('clicking a mode tab schedules exactly one debounced windowsSetMode, never a synchronous one (P22 D12/F20, shared)', async ({
  relaunch,
}) => {
  // state/mode.ts's own re-export of packages/workbench/src/state/createModeStore.ts's own
  // constant — restated here rather than imported across the web/node tsconfig boundary, same
  // convention tests/ui/studio/mode-switch.spec.ts already follows.
  const MODE_WRITE_DEBOUNCE_MS = 150;
  const { window: page, control } = await relaunch({ control: [] });
  // Playwright's fake clock, paused — installed after boot (relaunch() already waited on the
  // status bar), the same precaution tests/ui/studio/support/clock.ts's own
  // installFakeTimers documents: a bare install() alone still ticks on real time until paused.
  await page.clock.install();
  await page.clock.pauseAt(Date.now() + 10_000);

  const setModeCalls = () => control.log().filter((e) => e.channel === IPC.windowsSetMode);

  await modeTab(page, 'automations').click();
  // Nothing yet — the click itself must not fire a synchronous IPC (F20's own invariant, shared
  // with the Studio modules since P128 §2.3).
  expect(setModeCalls()).toHaveLength(0);
  await page.clock.runFor(MODE_WRITE_DEBOUNCE_MS - 1);
  expect(setModeCalls()).toHaveLength(0);
  await page.clock.runFor(2);
  await expect.poll(() => setModeCalls().length).toBe(1);
  expect(setModeCalls()[0]?.args).toMatchObject({ mode: 'automations' });
});

const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

function repoRow(page: Page) {
  return page.locator(`[data-testid="repo-row"][data-repo-id="${REPO.id}"]`);
}

function tab(page: Page, kind?: string) {
  return kind
    ? page.locator(`[data-testid="tab-strip-wrapper"] [data-testid="tab"][data-tab-kind="${kind}"]`)
    : page.locator('[data-testid="tab-strip-wrapper"] [data-testid="tab"]');
}

const TERMINAL_OPEN_OK: ControlSnapshot = {
  channel: IPC.terminalOpen,
  response: { shell: '/bin/zsh' },
};

test('round trip: a repo\'s tabs hide behind Terminal, a Terminal-module tab opens and closes out of view in Git, and Agents shows no "+"', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
      {
        channel: IPC.codeWorkspaceListFiles,
        args: { id: REPO.id },
        response: { paths: [], status: {}, truncated: false },
      },
      TERMINAL_OPEN_OK,
    ],
  });

  // Git is active by default (this app's own boot mode, mockRuntime.ts's own WILDCARD_DEFAULTS) —
  // opening the repo shows its pinned graph tab, alone.
  await repoRow(page).click();
  await expect(tab(page)).toHaveCount(1);
  await expect(tab(page, 'repo-graph')).toHaveCount(1);

  // Switching to Terminal hides the repo's own tab strip entirely — visibleWorkspace()
  // (state/workspace.ts) now reads the mode store's own `'terminal'` id, not the still-active repo
  // workspace underneath it.
  await modeTab(page, 'automations').click();
  await expect(modeTab(page, 'automations')).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="automations-start"]')).toBeVisible();
  await expect(tab(page)).toHaveCount(0);

  // Opening one there (automations-start's own "New terminal") is the only tab visible while
  // Terminal stays active.
  await page.click('[data-testid="automations-start-new"]');
  await expect(tab(page, 'terminal')).toHaveCount(1);
  await expect(tab(page)).toHaveCount(1);

  // Switching back to Git brings the repo's own tab (and only it) back — the Terminal-module tab
  // still exists (createTabsStore never dropped it), but is out of view: git's own workspace never
  // reaches a module key (state/workspace.ts's own header comment).
  await modeTab(page, 'git').click();
  await expect(modeTab(page, 'git')).toHaveClass(/is-active/);
  await expect(tab(page)).toHaveCount(1);
  await expect(tab(page, 'repo-graph')).toHaveCount(1);
  await expect(tab(page, 'terminal')).toHaveCount(0);

  // Agents (P129 Part 3 §0.12/§0.13: a `layout: 'full'` module) renders its own full-area view in
  // `#main` — no tab strip at all (never an empty "+" slot within one), and no project panel.
  await modeTab(page, 'ade').click();
  await expect(modeTab(page, 'ade')).toHaveClass(/is-active/);
  await expect(page.locator('[data-testid="ade-view"]')).toBeVisible();
  await expect(page.locator('[data-testid="tab-strip"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="project-panel"]')).toHaveCount(0);
});
