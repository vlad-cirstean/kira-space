import { answeredOk } from '@workbench/testing/ui/approvals';
import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { detailsEditItem, seedApprovals } from './support/approvals';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { pill } from './support/links';
import type { ControlSnapshot } from './support/types';

// Task files: notes, the agent's link and draft, its edits that wait for the user, and the disk banner.
// Contract ade-details-files; backend half: flows/adeflow TestDetailsFiles (detailsfiles_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const TASK = 'T_alerts';
const SCENARIO = 'ade-details-files';

interface ChainFx {
  id: string;
  taskId: string;
  body: string;
  source: string;
  state: string;
}
interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  source: string;
}
interface DiskFx {
  path: string;
  state: string;
  pending: boolean;
  diff: string;
}
interface ResolveArgs {
  taskId: string;
  paths: string[];
}

function chain(key: string): ChainFx[] {
  const fx = contract<{ handovers: ChainFx[] }>(SCENARIO, `AdeTaskService.Handovers#${key}`);
  return fx.handovers.map((h, i) => ({ ...h, id: `h_${i}`, taskId: TASK }));
}

const chainSnapshot = (key: string): ControlSnapshot => ({
  channel: IPC.adeTaskHandovers,
  response: { handovers: chain(key) },
});

const diskChanges = () => contract<DiskFx[]>(SCENARIO, 'AdeTaskService.DetailsDiskChanges');
const resolveArgs = (key: string): ResolveArgs => ({
  ...contract<ResolveArgs>(SCENARIO, `args:AdeTaskService.${key}`),
  taskId: TASK,
});

test('contract: the chain shows the note and the agent draft; Add note sends the contract args', async ({
  relaunch,
}) => {
  const args = {
    ...contract<{ taskId: string; body: string }>(SCENARIO, 'args:AdeTaskService.AddNote'),
    taskId: TASK,
  };
  const { window: page, control } = await openPlan(relaunch, [
    chainSnapshot('draft'),
    { channel: IPC.adeTaskAddNote, response: chain('draft')[0] },
  ]);
  await openDetails(page, TASK);
  const entries = page.locator(`${t('ade-task-chain')} [data-author]`);
  await expect(entries).toHaveCount(2);
  await expect(entries.nth(0)).toContainText('Form wired; validation missing.');
  await expect(entries.nth(1)).toContainText(args.body);
  await page.locator(t('ade-note-add')).click();
  await page.locator(`${t('ade-note-composer')} .ProseMirror`).click();
  await page.keyboard.type(args.body);
  await page.keyboard.press('Meta+Enter');
  await expect.poll(() => calls(control, IPC.adeTaskAddNote)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddNote)[0]?.args).toEqual(args);
});

test('contract: the link an agent filed in links.yaml shows as a pill', async ({ relaunch }) => {
  const listed = contract<{ artifacts: ArtifactFx[] }>(SCENARIO, 'AdeTaskService.Artifacts#link');
  const agent = listed.artifacts.find((a) => a.source === 'agent');
  if (!agent) throw new Error('contract has no agent link');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: listed.artifacts, gaps: [] } },
  ]);
  await openDetails(page, TASK);
  await expect(pill(page, agent.title)).toBeVisible();
});

test('contract: an edit of another agent content that waits shows its cause and files; Approve sends the paths', async ({
  relaunch,
}) => {
  const it = detailsEditItem(SCENARIO);
  const { items } = it.payload as { items: { path: string }[] };
  const paths = items.map((f) => f.path);
  const { window: page, control } = await openPlan(relaunch, [
    ...seedApprovals([it]),
    { channel: IPC.approvalsAnswer, response: answeredOk([it.id]) },
  ]);
  const dialog = page.locator(t('approvals-dialog'));
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(t('ade-details-edit-reason'))).toHaveCount(0);
  await expect(dialog.locator(t('ade-details-edit-file'))).toHaveCount(paths.length);
  await expect(dialog.locator(t('ade-details-edit-file'))).toContainText('Another agent’s note');
  await dialog.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => calls(control, IPC.approvalsAnswer)).toHaveLength(1);
  expect(calls(control, IPC.approvalsAnswer)[0]?.args).toEqual({
    ids: [it.id],
    approve: true,
    options: { [it.id]: { approved: paths } },
  });
});

test('contract: a hand edit on disk shows the banner; Accept sends the contract args', async ({
  relaunch,
}) => {
  const [change] = diskChanges();
  if (!change) throw new Error('contract has no disk change');
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskDetailsDiskChanges, response: diskChanges() },
  ]);
  await openDetails(page, TASK);
  await expect(page.locator(t('ade-details-disk-banner'))).toBeVisible();
  await page.locator(t('ade-details-disk-review')).click();
  const file = page.locator(t('ade-details-disk-file'));
  await expect(file).toContainText(change.path);
  await expect(file).toContainText(change.diff.split('\n')[0] ?? '');
  await file.locator(t('ade-details-disk-accept')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDetailsAcceptDisk)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDetailsAcceptDisk)[0]?.args).toEqual(
    resolveArgs('DetailsAcceptDisk'),
  );
});

test('contract: Discard all sends the contract args with no paths', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskDetailsDiskChanges, response: diskChanges() },
  ]);
  await openDetails(page, TASK);
  await page.locator(t('ade-details-disk-review')).click();
  await page.locator(t('ade-details-disk-discard-all')).click();
  await expect.poll(() => calls(control, IPC.adeTaskDetailsDiscardDisk)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskDetailsDiscardDisk)[0]?.args).toEqual(
    resolveArgs('DetailsDiscardDisk'),
  );
});

test('with no change on disk there is no banner', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, []);
  await openDetails(page, TASK);
  await expect(page.locator(t('ade-details-disk-banner'))).toHaveCount(0);
  await expect(page.locator(t('ade-details-disk'))).toBeVisible();
});
