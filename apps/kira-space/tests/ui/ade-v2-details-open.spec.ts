import { expect, test } from './fixtures';
import { adeBoard, openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { openCard, pill } from './support/links';

// Details links open in the system browser: a button per link card and per timeline artifact, through
// LinkService.OpenExternal. Contract ade-details-open; backend half: flows/adeflow TestDetailsOpenExternal.

const t = (id: string) => `[data-testid="${id}"]`;
const S = 'ade-details-open';

interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  url: string;
  source: string;
}
interface Links {
  jira: unknown;
  hostUrl: string;
  hostLink: unknown;
}
interface OpenArgs {
  url: string;
}

const links = contract<Links>(S, 'Task#links');
const openArgs = (key: string) => contract<OpenArgs>(S, `args:LinkService.OpenExternal#${key}`);

const listed = (): ArtifactFx[] =>
  contract<{ artifacts: ArtifactFx[] }>(S, 'AdeTaskService.Artifacts').artifacts.map((a, i) => ({
    ...a,
    id: a.source === 'user' ? `a_${i}` : '',
  }));

function snapshots() {
  const board = adeBoard((b) => {
    Object.assign(b.tasks.find((x) => x.id === 'T_alerts') ?? {}, links);
  });
  return [
    { channel: IPC.adeTaskBoard, response: board },
    { channel: IPC.adeTaskArtifacts, response: { artifacts: listed(), gaps: [] } },
    { channel: IPC.linkOpenExternal, response: null },
    { channel: IPC.hostViewOpen, response: { key: 'hostview-1', opened: true } },
  ];
}

test('contract: each link card opens in the browser', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  const rows: [string, string][] = [
    ['PAY-9', 'jira'],
    ['api!8', 'host'],
    ['api!9', 'changeRequest'],
    ['Design notes', 'doc'],
  ];
  for (const [i, [text, key]] of rows.entries()) {
    const card = await openCard(page, text);
    await card.locator(t(S)).click();
    await page.keyboard.press('Escape');
    await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(i + 1);
    expect(calls(control, IPC.linkOpenExternal)[i]?.args).toEqual(openArgs(key));
  }
  expect(calls(control, IPC.hostViewOpen)).toHaveLength(0);
});

test('the change request title still opens the in-app view', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  const card = await openCard(page, 'api!8');
  await card.locator(t('ade-links-card-title')).click();
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  expect(calls(control, IPC.hostViewOpen)[0]?.args).toEqual({ url: links.hostUrl });
  expect(calls(control, IPC.linkOpenExternal)).toHaveLength(0);
});

test('a link without a URL has no open button', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await expect(pill(page, 'PAY-20')).toHaveCount(1);
  const card = await openCard(page, 'PAY-20');
  await expect(card.locator(t(S))).toHaveCount(0);
});

test('editing hides the button', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  const card = await openCard(page, 'Design notes');
  await expect(card.locator(t(S))).toHaveCount(1);
  await card.locator(t('ade-links-card-edit')).click();
  await expect(card.locator(t(S))).toHaveCount(0);
});

test('contract: a timeline artifact opens in the browser', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, snapshots());
  await openDetails(page);
  await page.locator(t('ade-details-timeline-toggle')).click();
  const entry = page.locator('[data-event="artifact"]', { hasText: 'Design notes' });
  await entry.locator(t(S)).click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual(openArgs('doc'));
});
