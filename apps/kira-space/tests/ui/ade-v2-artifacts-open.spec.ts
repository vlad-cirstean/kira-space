import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { line, pill } from './support/links';
import { emitWailsEvent } from './support/mockRuntime';

// Opening Details never waits on the host: Artifacts answers without the PR, a scoped push brings it,
// and a return to the tab keeps the links and restats only the disk. Contract ade-artifacts-open;
// backend half: flows/adeflow TestArtifactsOpen (artifactsopen_test.go).

const t = (id: string) => `[data-testid="${id}"]`;
const SCENARIO = 'ade-artifacts-open';

interface ResultFx {
  artifacts: { id: string; kind: string; title: string; source: string }[];
  gaps: { source: string; repo: string }[];
}

/** Stored rows get distinct ids: the contract masks them. */
function storedIds(res: ResultFx): ResultFx {
  return {
    ...res,
    artifacts: res.artifacts.map((a, i) => ({
      ...a,
      id: a.source === 'agent' || a.source === 'user' ? `a_${i}` : '',
    })),
  };
}

const cold = () => storedIds(contract<ResultFx>(SCENARIO, 'AdeTaskService.Artifacts#cold'));
const warm = () => storedIds(contract<ResultFx>(SCENARIO, 'AdeTaskService.Artifacts#warm'));

test('contract: Details shows the stored and notes links before the host answered, the PR joins on the push', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: cold() },
  ]);
  await openDetails(page);
  await expect(pill(page, 'Runbook')).toBeVisible();
  await expect(pill(page, 'notes.example/design')).toBeVisible();
  await expect(line(page, 'changeRequest')).toHaveCount(0);
  await expect(page.locator(t('ade-links-skeleton'))).toHaveCount(0);
  await expect(page.locator(t('ade-links-gap'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskArtifacts)).toHaveLength(1);

  const prFacts = calls(control, IPC.adeTaskChangeRequests).length;
  control.respond(IPC.adeTaskArtifacts, warm());
  await emitWailsEvent(
    page,
    IPC.adeTaskBoardChanged,
    contract(SCENARIO, 'event:kira:adetask:board#hostRefresh'),
  );
  await expect(pill(page, 'Fix login')).toHaveAttribute('data-kind', 'changeRequest');
  expect(calls(control, IPC.adeTaskArtifacts)).toHaveLength(2);
  await expect
    .poll(() => calls(control, IPC.adeTaskChangeRequests).length)
    .toBeGreaterThan(prFacts);
});

test('returning to Details keeps the links and restats the disk once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: warm() },
    { channel: IPC.adeTaskDetailsDiskChanges, response: [] },
  ]);
  await openDetails(page);
  await expect(pill(page, 'Fix login')).toBeVisible();
  await expect.poll(() => calls(control, IPC.adeTaskDetailsDiskChanges)).toHaveLength(1);
  const artifacts = calls(control, IPC.adeTaskArtifacts).length;

  await page.locator(t('ade-panel-tab-main')).click();
  await expect(page.locator(t('ade-main-tab'))).toBeVisible();
  await expect(page.locator(t('ade-details-tab'))).toBeHidden();
  await page.waitForTimeout(500);
  expect(calls(control, IPC.adeTaskDetailsDiskChanges)).toHaveLength(1);

  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-tab'))).toBeVisible();
  await expect(page.locator(t('ade-links-skeleton'))).toHaveCount(0);
  await expect(pill(page, 'Fix login')).toBeVisible();
  await expect.poll(() => calls(control, IPC.adeTaskDetailsDiskChanges)).toHaveLength(2);
  expect(calls(control, IPC.adeTaskArtifacts)).toHaveLength(artifacts);
});
