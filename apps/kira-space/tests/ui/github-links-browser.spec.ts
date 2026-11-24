import { expect, test } from './fixtures';
import { adeFixture, openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { linkSnapshots, openIn } from './support/links';

// PR, merge request and Jira links open in the browser, whatever tab of the PR they name. Contract
// github-links-browser; backend half: flows/githostflow TestGitHubLinksOpenInBrowser. The branch row
// PR glyph has its own spec in ade-v2-row-actions.spec.ts.

const t = (id: string) => `[data-testid="${id}"]`;
const S = 'github-links-browser';
const PILLS: [text: string, name: string][] = [
  ['#13', 'pr13'],
  ['#14', 'pr14'],
  ['#15', 'pr15'],
  ['api!7', 'azure'],
  ['PAY-9', 'jira'],
];

interface ArtifactFx {
  kind: string;
  url: string;
  source: string;
}
interface Links {
  hostUrl: string;
  hostLink: unknown;
  jira: unknown;
}

const links = contract<Links>(S, 'Task#links');
const openArgs = (name: string) =>
  contract<{ url: string }>(S, `args:LinkService.OpenExternal#${name}`);

const snapshots = () =>
  linkSnapshots(
    links,
    contract<{ artifacts: ArtifactFx[] }>(S, 'AdeTaskService.Artifacts').artifacts,
  );

test('contract: a PR, merge request or Jira pill opens the link in the browser', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await openIn(page, '#12');
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual(openArgs('pr12'));
  for (const [i, [text, name]] of PILLS.entries()) {
    await openIn(page, text);
    await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(i + 2);
    expect(calls(control, IPC.linkOpenExternal)[i + 1]?.args).toEqual(openArgs(name));
  }
  await expect(page.locator(t('ade-links-card'))).toHaveCount(0);
  await expect(page.locator(t('ade-pr-view'))).toHaveCount(0);
  await expect(page.locator(t('ade-task-issue-view'))).toHaveCount(0);
});

test('the branch panel PR link opens the browser and marks its pending update seen', async ({
  relaunch,
}) => {
  const url = 'https://github.com/acme/web-app/pull/7/changes';
  const fx = adeFixture<{ branches: Record<string, unknown> }>('change-requests');
  fx.branches.b_auth = { number: 7, title: 'Fix login', state: 'Open', url };
  const crFx = contract<{ updates: Record<string, unknown>[] }>(
    'ade-tracked-updates',
    'AdeTaskService.TrackedUpdates#changed',
  ).updates.find((u) => u.kind === 'changeRequest');
  if (!crFx) throw new Error('contract lacks a change request update');
  const update = { ...crFx, taskId: 'T_auth', itemKey: 'cr:b_auth', branchId: 'b_auth' };
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskChangeRequests, response: fx },
    { channel: IPC.adeTaskTrackedUpdates, response: { updates: [update] } },
    { channel: IPC.adeTaskMarkTrackedSeen, hold: true },
    { channel: IPC.linkOpenExternal, response: null },
  ]);
  await page.locator(`${t('ade-card')}[data-task-id="T_auth"] ${t('ade-card-head')}`).click();
  await page.locator(t('ade-panel-tab-main')).click();
  await page.locator(`${t('ade-task-branch')}[data-branch-id="b_auth"]`).click();
  await expect(page.locator(t('ade-pr-view'))).toHaveCount(0);
  await page.locator(`${t('ade-branch-pr')} a`).click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url });
  await expect.poll(() => calls(control, IPC.adeTaskMarkTrackedSeen)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskMarkTrackedSeen)[0]?.args).toEqual({
    taskId: 'T_auth',
    itemKeys: ['cr:b_auth'],
  });
});
