import { expect, test } from './fixtures';
import { adeBoard, archivedTaskFx, openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Task artifacts: the Details tab links, manual add and remove, gap warnings, the Done dialog list and
// the read-only archived tab. Contract ade-artifacts; backend half: flows/adeflow TestTaskArtifacts
// (artifacts_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  url: string;
  ref: string;
  status: string;
  repo: string;
  source: string;
  createdAt: number;
}
interface ResultFx {
  artifacts: ArtifactFx[];
  gaps: { source: string; repo: string; reason?: string; count?: number }[];
}

const recorded = (key: string) => contract<ResultFx>('ade-artifacts', key);

/** Stored rows get distinct ids: the contract masks them. */
function withIds(res: ResultFx): ResultFx {
  return {
    ...res,
    artifacts: res.artifacts.map((a, i) => ({
      ...a,
      id: a.source === 'agent' || a.source === 'user' ? `a_${i}` : '',
    })),
  };
}

function snapshot(res: ResultFx): ControlSnapshot {
  return { channel: IPC.adeTaskArtifacts, response: res };
}

test('contract: the Details tab lists every link with kind and status', async ({ relaunch }) => {
  const res = withIds(recorded('AdeTaskService.Artifacts'));
  const { window: page, control } = await openPlan(relaunch, [snapshot(res)]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect.poll(() => calls(control, IPC.adeTaskArtifacts)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskArtifacts)[0]?.args).toEqual({ taskId: 'T_alerts' });

  const rows = page.locator(t('ade-details-link'));
  const nonBranch = res.artifacts.filter((a) => a.kind !== 'branch').length;
  // Jira PAY-130 is the task's own link; the tracker row KIRA-12 stays in Related.
  await expect(rows).toHaveCount(nonBranch + 1);
  const pr = page.locator(`${t('ade-details-main')} ${t('ade-details-link')}`, {
    hasText: 'Fix login',
  });
  await expect(pr).toContainText('Change request');
  await expect(pr).toContainText('Open');
  await expect(pr).toHaveAttribute('data-origin', 'detected');
  const deployment = rows.filter({ hasText: 'prod' });
  await expect(deployment).toContainText('Deployed');
  await expect(rows.filter({ hasText: 'KIRA-12' })).toHaveCount(1);

  const stored = res.artifacts.filter((a) => a.id).length;
  await expect(page.locator(`${t('ade-details-link')}[data-origin="stored"]`)).toHaveCount(stored);
  await expect(page.locator(t('ade-details-link-remove'))).toHaveCount(stored + 1);
  await expect(pr.locator(t('ade-details-link-remove'))).toHaveCount(0);
  const gap = page.locator(t('ade-artifact-gap'));
  await expect(gap).toHaveCount(1);
  await expect(gap).toContainText('The Jira command line tool isn’t installed');
});

test('contract: pasting a link sends AddArtifact with an empty title', async ({ relaunch }) => {
  const sentKeys = Object.keys(
    contract<Record<string, unknown>>('ade-artifacts', 'args:AdeTaskService.AddArtifact'),
  );
  // With the host slot filled, a pasted URL becomes an artifact instead of the host link.
  const withHost = adeBoard((b) => {
    const task = b.tasks.find((x) => x.id === 'T_alerts');
    Object.assign(task ?? {}, { hostUrl: 'https://forge.test/acme/api/-/issues/3' });
  });
  const { window: page, control } = await openPlan(relaunch, [
    snapshot(withIds(recorded('AdeTaskService.Artifacts'))),
    { channel: IPC.adeTaskBoard, response: withHost },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  const paste = page.locator(t('ade-details-paste'));
  await paste.fill('https://wiki.example/runbook');
  await paste.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskAddArtifact)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskAddArtifact)[0]?.args as Record<string, unknown>;
  expect(sent).toEqual({
    taskId: 'T_alerts',
    kind: 'link',
    title: '',
    url: 'https://wiki.example/runbook',
    ref: '',
  });
  expect(Object.keys(sent).sort()).toEqual([...sentKeys].sort());
  await expect(paste).toHaveValue('');
});

test('contract: remove on a stored row sends RemoveArtifact; detected rows have no remove', async ({
  relaunch,
}) => {
  const sentKeys = Object.keys(
    contract<Record<string, unknown>>('ade-artifacts', 'args:AdeTaskService.RemoveArtifact'),
  );
  const res = withIds(recorded('AdeTaskService.Artifacts'));
  const { window: page, control } = await openPlan(relaunch, [snapshot(res)]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  const doc = page.locator(t('ade-details-link')).filter({ hasText: 'Design notes' });
  await doc.locator(t('ade-details-link-remove')).click();
  await expect.poll(() => calls(control, IPC.adeTaskRemoveArtifact)).toHaveLength(1);
  const sent = calls(control, IPC.adeTaskRemoveArtifact)[0]?.args as Record<string, unknown>;
  const id = res.artifacts.find((a) => a.title === 'Design notes')?.id;
  expect(sent).toEqual({ taskId: 'T_alerts', id });
  expect(Object.keys(sent).sort()).toEqual([...sentKeys].sort());
});

test('contract: a failed source shows a warning above the list', async ({ relaunch }) => {
  const res = withIds(recorded('AdeTaskService.Artifacts#gap'));
  expect(res.gaps).toHaveLength(2);
  const { window: page } = await openPlan(relaunch, [snapshot(res)]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  const gaps = page.locator(t('ade-artifact-gap'));
  await expect(gaps).toHaveCount(2);
  const host = res.gaps.find((g) => !g.reason);
  await expect(gaps.first()).toContainText(`Couldn’t read ${host?.source} for ${host?.repo}`);
  await expect(gaps.nth(1)).toContainText('The Jira command line tool isn’t installed');
  await expect(gaps.nth(1)).toContainText('under CLI tools');
  await expect(page.locator(t('ade-details-link'))).not.toHaveCount(0);
});

test('an empty task shows the hint', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, [snapshot({ artifacts: [], gaps: [] })]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-empty')).first()).toBeVisible();
});

test('moving a task to Done lists its artifacts, read-only', async ({ relaunch }) => {
  const res = withIds(recorded('AdeTaskService.Artifacts'));
  const { window: page } = await openPlan(relaunch, [
    snapshot(res),
    { channel: IPC.adeTaskBoard, response: adeBoard(() => undefined) },
  ]);
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-step-pick')).click();
  await page.locator(t('ade-step-option-done')).click();
  const dialog = page.locator(t('ade-stage-dialog'));
  await expect(dialog).toBeVisible();
  const list = dialog.locator(t('ade-stage-artifacts'));
  await expect(list.locator(t('ade-artifact-row'))).toHaveCount(res.artifacts.length);
  await expect(list.locator(t('ade-artifact-remove'))).toHaveCount(0);
});

test('an archived task shows the links read-only', async ({ relaunch }) => {
  const res = withIds(recorded('AdeTaskService.Artifacts#archived'));
  const { window: page } = await openPlan(relaunch, [
    snapshot(res),
    { channel: IPC.adeTaskArchivedTask, response: archivedTaskFx() },
  ]);
  await page.locator(t('ade-archived-find')).click();
  await page.locator(`${t('ade-archived-find-item')}[data-task-id="H_1"]`).click();
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-link'))).not.toHaveCount(0);
  await expect(page.locator(t('ade-details-paste'))).toHaveCount(0);
  await expect(page.locator(t('ade-details-link-edit'))).toHaveCount(0);
  await expect(page.locator(t('ade-details-link-remove'))).toHaveCount(0);
});
