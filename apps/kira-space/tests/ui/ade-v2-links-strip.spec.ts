import type { Page } from '@playwright/test';
import type { ContractJson } from '@workbench/testing/ui/contract';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import { expect, test } from './fixtures';
import {
  type AdeBoardFx,
  adeBoard,
  adeFixture,
  FIXED_NOW,
  openDetails,
  openPlan,
  openTask,
} from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { addLine, line, openCard, openIn, pill } from './support/links';
import type { ControlSnapshot } from './support/types';

// The link strip: category lines of pills, the click-open card, keyboard, loading, and the same strip
// in the handoff dialogs and the Run dialog. UI half of contracts ade-details-links and ade-handover;
// backend halves: flows/adeflow TestDetailsLinks and TestStageHandover.

const t = (id: string) => `[data-testid="${id}"]`;

interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  ref: string;
  url: string;
  status: string;
  repo: string;
  source: string;
  sessionId: string;
  createdAt: number;
}

// Eight links: the Jira slot PAY-130, then PAY-10 (detected), PAY-12, api!9, feat/x, Design spec, Runbook, wiki.example/x.
const listed = (): ArtifactFx[] =>
  contract<{ artifacts: ArtifactFx[] }>(
    'ade-details-links',
    'AdeTaskService.Artifacts',
  ).artifacts.map((a, i) => ({ ...a, id: a.source === 'user' ? `a_${i}` : '' }));

const DETECTED_BRANCH: ArtifactFx = {
  id: '',
  kind: 'branch',
  title: 'feat/api-work',
  ref: 'feat/api-work',
  url: 'https://forge.test/acme/api/-/tree/feat/api-work',
  status: 'pushed',
  repo: 'api',
  source: 'git',
  sessionId: '',
  createdAt: 0,
};

const DEPLOYMENT: ArtifactFx = {
  ...DETECTED_BRANCH,
  kind: 'deployment',
  title: 'staging',
  ref: 'abc1234',
  url: '',
  status: 'deployed',
};

const open = (key: string): ControlSnapshot[] => [
  { channel: IPC.linkOpenExternal, response: null },
  { channel: IPC.hostViewOpen, response: { key, opened: true } },
];

const base = (artifacts: ArtifactFx[] = listed()): ControlSnapshot[] => [
  { channel: IPC.adeTaskArtifacts, response: { artifacts, gaps: [] } },
  {
    channel: IPC.adeTaskAddLink,
    response: contract('ade-details-links', 'AdeTaskService.AddLink'),
  },
  { channel: IPC.adeTaskRemoveArtifact, response: null },
  ...open('hostview-1'),
];

const focusedTestid = (page: Page) =>
  page.evaluate(() => document.activeElement?.getAttribute('data-testid') ?? '');

test('eight links take three lines and under 104px', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  const strip = page.locator(t('ade-links'));
  await expect(strip.locator(t('ade-links-line'))).toHaveCount(3);
  await expect(strip.locator(t('ade-links-pill'))).toHaveCount(8);
  const panel = await page.locator(t('ade-details-tab')).boundingBox();
  expect(panel?.width ?? 0).toBeGreaterThanOrEqual(340);
  const box = await strip.boundingBox();
  expect(box?.height ?? 0).toBeLessThanOrEqual(104);
});

test('nine misc links show seven and a more pill that expands the line', async ({ relaunch }) => {
  const docs: ArtifactFx[] = Array.from({ length: 9 }, (_, i) => ({
    ...DETECTED_BRANCH,
    id: `d_${i}`,
    kind: 'doc',
    title: `Doc ${i + 1}`,
    ref: '',
    url: `https://wiki.example/doc-${i + 1}`,
    status: '',
    repo: '',
    source: 'user',
  }));
  const { window: page } = await openPlan(relaunch, base(docs));
  await openDetails(page);
  const misc = line(page, 'misc');
  await expect(misc.locator(t('ade-links-pill'))).toHaveCount(7);
  await expect(misc.locator(t('ade-links-more'))).toHaveText('2 more');
  await misc.locator(t('ade-links-more')).click();
  await expect(misc.locator(t('ade-links-pill'))).toHaveCount(9);
  await expect(misc.locator(t('ade-links-more'))).toHaveCount(0);
});

test('pills are one tab stop; arrows, Home, End and Up/Down move focus', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  await page.locator(t('ade-links-add')).focus();
  await page.keyboard.press('Tab');
  await expect(pill(page, 'PAY-130')).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.locator(`${t('ade-links-pill')}:focus`)).toHaveCount(0);
  await pill(page, 'PAY-130').focus();
  await page.keyboard.press('ArrowRight');
  await expect(pill(page, 'PAY-10')).toBeFocused();
  await page.keyboard.press('End');
  await expect(pill(page, 'wiki.example/x')).toBeFocused();
  await page.keyboard.press('Home');
  await expect(pill(page, 'PAY-130')).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(pill(line(page, 'changeRequest'), 'api!9')).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(pill(line(page, 'misc'), 'feat/x')).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(pill(line(page, 'changeRequest'), 'api!9')).toBeFocused();
});

test('Enter opens in the app, Cmd+Enter in the browser; Space opens the card, Esc closes it', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const runbook = listed().find((a) => a.title === 'Runbook');
  const mr = listed().find((a) => a.title === 'api!9');
  await pill(page, 'Runbook').focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url: runbook?.url });

  await pill(page, 'api!9').focus();
  await page.keyboard.press('Enter');
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(1);

  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(2);
  expect(calls(control, IPC.linkOpenExternal)[1]?.args).toEqual({ url: mr?.url });
  expect(calls(control, IPC.hostViewOpen)).toHaveLength(1);

  await pill(page, 'Runbook').focus();
  await page.keyboard.press(' ');
  const card = page.locator(t('ade-links-card'));
  await expect(card).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(card).toHaveCount(0);
  await expect(pill(page, 'Runbook')).toBeFocused();
  await page.keyboard.press('Alt+ArrowDown');
  await expect(card).toBeVisible();
  await page.keyboard.press('Escape');
  // A stray click from Space would add calls.
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(2);
  expect(calls(control, IPC.hostViewOpen)).toHaveLength(1);
});

test('a click opens in the app, Cmd-click in the browser, per kind', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const urlOf = (title: string) => listed().find((a) => a.title === title)?.url ?? '';
  const table: { text: string; channel: 'view' | 'browser'; url: string; external: boolean }[] = [
    {
      text: 'PAY-130',
      channel: 'view',
      url: 'https://acme.atlassian.net/browse/PAY-130',
      external: true,
    },
    { text: 'PAY-12', channel: 'view', url: urlOf('PAY-12'), external: true },
    { text: 'PAY-10', channel: 'view', url: 'PAY-10', external: false },
    { text: 'api!9', channel: 'view', url: urlOf('api!9'), external: true },
    { text: 'feat/x', channel: 'browser', url: urlOf('feat/x'), external: true },
    { text: 'Design spec', channel: 'browser', url: urlOf('Design spec'), external: true },
    { text: 'wiki.example/x', channel: 'browser', url: urlOf('wiki.example/x'), external: true },
  ];
  for (const row of table) {
    const view = calls(control, IPC.hostViewOpen).length;
    const browser = calls(control, IPC.linkOpenExternal).length;
    await openIn(page, row.text);
    const channel = row.channel === 'view' ? IPC.hostViewOpen : IPC.linkOpenExternal;
    await expect
      .poll(() => calls(control, channel).length)
      .toBe((row.channel === 'view' ? view : browser) + 1);
    expect(calls(control, channel).at(-1)?.args).toEqual({ url: row.url });
    expect(
      calls(control, row.channel === 'view' ? IPC.linkOpenExternal : IPC.hostViewOpen),
    ).toHaveLength(row.channel === 'view' ? browser : view);
    await expect(page.locator(t('ade-links-card'))).toHaveCount(0);
  }
  for (const row of table.filter((r) => r.external)) {
    const view = calls(control, IPC.hostViewOpen).length;
    const browser = calls(control, IPC.linkOpenExternal).length;
    await openIn(page, row.text, { external: true });
    await expect.poll(() => calls(control, IPC.linkOpenExternal).length).toBe(browser + 1);
    expect(calls(control, IPC.linkOpenExternal).at(-1)?.args).toEqual({
      url: row.url,
    });
    expect(calls(control, IPC.hostViewOpen)).toHaveLength(view);
  }
  const before = calls(control, IPC.linkOpenExternal).length;
  await openIn(page, 'PAY-10', { external: true });
  await expect(page.locator(t('ade-links-card'))).toBeVisible();
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(before);
});

test('a link with nothing to open shows its card', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base([...listed(), DEPLOYMENT]));
  await openDetails(page);
  await openIn(page, 'staging');
  await expect(page.locator(t('ade-links-card'))).toBeVisible();
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(0);
  expect(calls(control, IPC.hostViewOpen)).toHaveLength(0);
});

test('a refused view falls back: browser with a url, the card without', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    ...base(),
    { channel: IPC.hostViewOpen, error: { code: 'E_BAD_REQUEST', message: 'Refused.' } },
  ]);
  await openDetails(page);
  const pay12 = listed().find((a) => a.title === 'PAY-12');
  await openIn(page, 'PAY-12');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url: pay12?.url });
  await openIn(page, 'PAY-10');
  await expect(page.locator(t('ade-links-card'))).toBeVisible();
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(1);
});

test('right-click opens the card and opens nothing', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await openCard(page, 'Runbook');
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(0);
  expect(calls(control, IPC.hostViewOpen)).toHaveLength(0);
  await expect(pill(page, 'Runbook')).toHaveAttribute('aria-expanded', 'true');
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-links-card'))).toHaveCount(0);
});

test('every pill points at the hint that names the modifier', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  const hint = page.locator(t('ade-links-hint'));
  await expect(hint).toContainText('Cmd+Enter');
  const id = await hint.getAttribute('id');
  const pills = page.locator(t('ade-links-pill'));
  const n = await pills.count();
  for (let i = 0; i < n; i++)
    await expect(pills.nth(i)).toHaveAttribute('aria-describedby', id ?? '');
});

test('F2 opens the editor, Esc cancels it; Delete removes a stored link, not a detected one', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await pill(page, 'Runbook').focus();
  await page.keyboard.press('F2');
  const input = page.locator(t('ade-links-edit-input'));
  await expect(input).toBeVisible();
  await input.fill('discarded');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  await expect(page.locator(t('ade-links-card'))).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-links-card'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskEditLink)).toHaveLength(0);

  await pill(page, 'PAY-10').focus();
  await page.keyboard.press('Delete');
  await pill(page, 'Runbook').focus();
  await page.keyboard.press('Delete');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveArtifact)).toHaveLength(1);
  const runbook = listed().find((a) => a.title === 'Runbook');
  expect(calls(control, IPC.adeTaskRemoveArtifact)[0]?.args).toEqual({
    taskId: 'T_alerts',
    id: runbook?.id,
  });
});

test('+ on a focused pill opens the add popover', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  await pill(page, 'Runbook').focus();
  await page.keyboard.press('+');
  await expect(page.locator(t('ade-links-add-popover'))).toBeVisible();
  await expect(page.locator(t('ade-links-paste'))).toBeFocused();
});

test('loading shows three skeleton pills, then the links', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: listed(), gaps: [] }, hold: true },
  ]);
  await openDetails(page);
  await expect(page.locator(t('ade-links-skeleton'))).toHaveCount(3);
  control.release(IPC.adeTaskArtifacts);
  await expect(page.locator(t('ade-links-pill'))).toHaveCount(8);
  await expect(page.locator(t('ade-links-skeleton'))).toHaveCount(0);
});

// Handoff dialogs: the strip in add mode, on every move and the pass.

const dialogLinks = (page: Page) =>
  page.locator(t('ade-stage-dialog')).locator(t('ade-stage-links'));

async function noEditInCard(page: Page, text: string): Promise<void> {
  await openCard(page, text);
  await expect(page.locator(t('ade-links-card'))).toBeVisible();
  await expect(page.locator(t('ade-links-card-remove'))).toHaveCount(0);
  await expect(page.locator(t('ade-links-card-edit'))).toHaveCount(0);
  await page.keyboard.press('Escape');
}

test('a stage move shows the strip in add mode; + adds a link; cards have no edit or remove', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base([...listed(), DETECTED_BRANCH]));
  await openTask(page, 'T_auth');
  await page.locator(t('ade-step-back')).click();
  const links = dialogLinks(page);
  await expect(links.locator(t('ade-links'))).toHaveAttribute('data-mode', 'add');
  await expect(pill(links, 'Runbook')).toBeVisible();
  await expect(
    links.locator(`${t('ade-links-pill')}[data-origin="detected"][data-kind="branch"]`),
  ).toHaveCount(0);
  await noEditInCard(page, 'Runbook');
  await openIn(page, 'Runbook');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();

  await addLine(page, 'https://wiki.example/handoff');
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({
    taskId: 'T_auth',
    text: 'https://wiki.example/handoff',
  });
});

test('moving to Done adds the detected branches; another target drops them', async ({
  relaunch,
}) => {
  const { window: page } = await openPlan(relaunch, base([...listed(), DETECTED_BRANCH]));
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-step-pick')).click();
  await page.locator(t('ade-step-option-done')).click();
  const dialog = page.locator(t('ade-stage-dialog'));
  const branch = dialogLinks(page).locator(
    `${t('ade-links-pill')}[data-origin="detected"][data-kind="branch"]`,
  );
  await expect(branch).toHaveCount(1);
  const other = await dialog
    .locator(`${t('ade-stage-target')} option`)
    .evaluateAll((els) => els.map((e) => (e as HTMLOptionElement).value).find((v) => v !== 'done'));
  await dialog.locator(t('ade-stage-target')).selectOption(other ?? '');
  await expect(branch).toHaveCount(0);
});

type Json = ContractJson;
interface BoardFx {
  tasks: Json[];
  statuses: Json[];
  repos: Json[];
  [key: string]: unknown;
}

const ORCH = 'T_orch';
const CHILD = 'T_child';

function orchestratorBoard() {
  const fx = contract<BoardFx>('ade-orchestrator', 'AdeTaskService.Board#orchestrator');
  const base = adeFixture<BoardFx>('board');
  return adeBoard((board) => {
    const [orch, child] = fx.tasks as [Json, Json];
    orch.id = ORCH;
    orch.workflowId = 'orch';
    orch.workflow = { ...orch.workflow, id: 'orch' };
    orch.workflow.steps[0].id = 'plan';
    for (const r of orch.runs) Object.assign(r, { id: 'run_orch', taskId: ORCH, stepId: 'plan' });
    child.id = CHILD;
    child.parentId = ORCH;
    child.handoverIn = { ...child.handoverIn, taskId: CHILD, fromTaskId: ORCH };
    for (const task of [orch, child]) {
      task.createdAt = FIXED_NOW - 864e5;
      task.branchIds = [];
    }
    orch.entryStepId = 'done';
    orch.statusId = 'done';
    orch.runs = [];
    board.tasks = [orch, child] as typeof board.tasks;
    board.statuses = base.statuses;
    board.repos = base.repos;
    board.branches = [];
    board.history = [];
    board.plan = {
      backlog: [],
      day: { [ORCH]: '2026-09-22', [CHILD]: '2026-09-22' },
      order: [ORCH, CHILD],
      queuedAfter: {},
      unpushed: {},
    };
  });
}

test('the orchestrator pass dialog shows the strip', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: orchestratorBoard() },
    ...base(),
  ]);
  const orch = page.locator(`${t('ade-card')}[data-task-id="${ORCH}"]`);
  await orch.locator(t('ade-task-action-nextPass')).click();
  const links = dialogLinks(page);
  await expect(links.locator(t('ade-links'))).toHaveAttribute('data-mode', 'add');
  await expect(page.locator(t('ade-stage-pass-target'))).toBeVisible();
  await expect(pill(links, 'Runbook')).toBeVisible();
});

// The routed handover prompt: same dialog, same strip.

interface HandoverFx {
  [key: string]: unknown;
}

test('the routed handover prompt shows the strip', async ({ relaunch }) => {
  const board = contract<{ tasks: { handoverDrafts: HandoverFx[] }[] }>(
    'ade-handover',
    'AdeTaskService.Board#draft',
  );
  const draft = board.tasks[0]?.handoverDrafts[0];
  const withDraft = (b: AdeBoardFx): void => {
    const alerts = b.tasks.find((x) => x.id === 'T_alerts');
    if (!alerts) throw new Error('T_alerts missing');
    alerts.handoverDrafts = [
      {
        ...draft,
        id: 'h_draft',
        taskId: 'T_alerts',
        fromStepId: 'spec',
        fromStepName: 'Spec',
        updatedAt: 1,
      },
    ] as typeof alerts.handoverDrafts;
  };
  const entry = routed('handover', 'T_alerts');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskBoard, response: adeBoard(withDraft) },
    { channel: IPC.promptsList, response: [entry] },
    ...base(),
  ]);
  await emitPrompts(page, [entry]);
  await expect(page.locator(t('ade-stage-dialog'))).toBeVisible();
  const links = dialogLinks(page);
  await expect(links.locator(t('ade-links'))).toHaveAttribute('data-mode', 'add');
  await expect(pill(links, 'Runbook')).toBeVisible();
});

test('the Run dialog shows a read-only strip and keeps its own focus', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    {
      channel: IPC.adeTaskBoard,
      response: adeBoard((b) => {
        const alerts = b.tasks.find((x) => x.id === 'T_alerts');
        if (!alerts) throw new Error('T_alerts missing');
        alerts.entryStepId = 'plan';
        alerts.workflowId = 'standard';
        alerts.runs = [];
      }),
    },
    ...base(),
  ]);
  await page
    .locator(`${t('ade-task')}[data-task-id="T_alerts"] ${t('ade-task-action-run')}`)
    .click();
  const dialog = page.locator(t('ade-run-dialog'));
  const links = dialog.locator(t('ade-run-links'));
  await expect(links.locator(t('ade-links'))).toHaveAttribute('data-mode', 'view');
  await expect(pill(links, 'Runbook')).toBeVisible();
  await expect(links.locator(t('ade-links-add'))).toHaveCount(0);
  await expect.poll(() => focusedTestid(page)).toMatch(/^ade-run-(message|branch)$/);
  await openIn(links, 'api!9');
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  await openIn(links, 'Runbook', { external: true });
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  await expect(dialog).toBeVisible();
  await openCard(links, 'Runbook');
  await expect(page.locator(t('ade-links-card-remove'))).toHaveCount(0);
  await expect(page.locator(t('ade-links-card-edit'))).toHaveCount(0);
  await expect(page.locator(t('ade-origin-session'))).toHaveCount(0);
});
