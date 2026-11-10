import { expect, test } from './fixtures';
import { adeBoard, adeFixture, openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';

// A GitHub PR link opens in the in-app view whatever tab of the PR it names, and one the view
// refuses opens in the browser. Contract github-links-in-app; backend half: flows/githostflow
// TestGitHubLinksInApp.

const t = (id: string) => `[data-testid="${id}"]`;
const S = 'github-links-in-app';
const NUMBERS = [12, 13, 14, 15];

interface ArtifactFx {
  kind: string;
  url: string;
  source: string;
}
interface Links {
  hostUrl: string;
  hostLink: unknown;
}

const links = contract<Links>(S, 'Task#links');
const openArgs = (n: number) => contract<{ url: string }>(S, `args:HostViewService.Open#${n}`);

const listed = (): (ArtifactFx & { id: string })[] =>
  contract<{ artifacts: ArtifactFx[] }>(S, 'AdeTaskService.Artifacts').artifacts.map((a, i) => ({
    ...a,
    id: a.source === 'user' ? `a_${i}` : '',
  }));

function snapshots(open: Record<string, unknown>) {
  const board = adeBoard((b) => {
    Object.assign(b.tasks.find((x) => x.id === 'T_alerts') ?? {}, links);
  });
  return [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskArtifacts, response: { artifacts: listed(), gaps: [] } },
    { channel: IPC.linkOpenExternal, response: null },
    { channel: IPC.hostViewOpen, ...open },
  ];
}

const titles = (page: import('@playwright/test').Page) => page.locator(t('ade-details-link-title'));

test('contract: a GitHub PR link title opens the in-app view', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(
    relaunch,
    snapshots({ response: { key: 'hostview-1', opened: true } }),
  );
  await openDetails(page);
  for (const [i, n] of NUMBERS.entries()) {
    await titles(page)
      .filter({ hasText: `#${n}` })
      .click();
    await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(i + 1);
    expect(calls(control, IPC.hostViewOpen)[i]?.args).toEqual(openArgs(n));
  }
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(0);
});

test('a link the view refuses opens in the browser', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(
    relaunch,
    snapshots({ error: { code: 'E_BAD_REQUEST', message: 'This view opens a GitHub PR link.' } }),
  );
  await openDetails(page);
  await titles(page)
    .filter({ hasText: `#${NUMBERS[0]}` })
    .click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url: links.hostUrl });
});

test('the branch panel View button falls back to the browser', async ({ relaunch }) => {
  const url = 'https://github.com/acme/web-app/pull/7/changes';
  const fx = adeFixture<{ branches: Record<string, unknown> }>('change-requests');
  fx.branches.b_auth = { number: 7, title: 'Fix login', state: 'Open', url };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskChangeRequests, response: fx },
    { channel: IPC.linkOpenExternal, response: null },
    { channel: IPC.hostViewOpen, error: { code: 'E_BAD_REQUEST', message: 'Refused.' } },
  ]);
  await page.locator(`${t('ade-card')}[data-task-id="T_auth"] ${t('ade-card-head')}`).click();
  await page.locator(t('ade-panel-tab-main')).click();
  await page.locator(`${t('ade-task-branch')}[data-branch-id="b_auth"]`).click();
  await page.locator(t('ade-pr-view')).click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url });
});
