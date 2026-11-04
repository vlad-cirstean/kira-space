import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openPlan, openTask } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import type { ControlSnapshot } from './support/types';

// Details links: one add line files typed or pasted text, rows edit in place, a stored row's kind is a
// menu. Contract ade-details-links; backend half: flows/adeflow TestDetailsLinks.

const t = (id: string) => `[data-testid="${id}"]`;

interface ArtifactFx {
  id: string;
  kind: string;
  title: string;
  url: string;
  source: string;
}
interface LinkArgs {
  taskId: string;
  text: string;
}
interface EditArgs {
  taskId: string;
  slot: string;
  id: string;
  text: string;
  kind: string;
}

const listed = (): ArtifactFx[] =>
  contract<{ artifacts: ArtifactFx[] }>(
    'ade-details-links',
    'AdeTaskService.Artifacts',
  ).artifacts.map((a, i) => ({ ...a, id: a.source === 'user' ? `a_${i}` : '' }));
const stored = (title: string): ArtifactFx => {
  const a = listed().find((x) => x.title === title);
  if (!a) throw new Error(`contract has no ${title}`);
  return a;
};
const addArgs = contract<LinkArgs>('ade-details-links', 'args:AdeTaskService.AddLink');
const editArgs = contract<EditArgs>('ade-details-links', 'args:AdeTaskService.EditLink');

const base = (addLink: Partial<ControlSnapshot> = {}): ControlSnapshot[] => [
  { channel: IPC.adeTaskArtifacts, response: { artifacts: listed(), gaps: [] } },
  {
    channel: IPC.adeTaskAddLink,
    response: contract('ade-details-links', 'AdeTaskService.AddLink'),
    ...addLink,
  },
  {
    channel: IPC.adeTaskEditLink,
    response: contract('ade-details-links', 'AdeTaskService.EditLink'),
  },
];

async function openDetails(page: Page): Promise<void> {
  await openTask(page, 'T_alerts');
  await page.locator(t('ade-panel-tab-details')).click();
  await expect(page.locator(t('ade-details-tab'))).toBeVisible();
}

const row = (page: Page, text: string) => page.locator(t('ade-details-link'), { hasText: text });

test('contract: stored rows show under their kind', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  const kinds = await page
    .locator(t('ade-details-link-kind'))
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));
  expect(kinds.sort()).toEqual(
    listed()
      .filter((a) => a.id)
      .map((a) => a.kind)
      .sort(),
  );
  await expect(row(page, 'feat/x').locator(t('ade-details-link-kind'))).toHaveAttribute(
    'data-kind',
    'branch',
  );
  await expect(row(page, 'PAY-10').locator(t('ade-details-link-kind'))).toHaveCount(0);
});

test('pasting into the empty add line files at once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const paste = page.locator(t('ade-details-paste'));
  await paste.focus();
  await page.evaluate(() => {
    const el = document.querySelector('[data-testid="ade-details-paste"]');
    const data = new DataTransfer();
    data.setData('text', 'https://wiki.example/pasted');
    el?.dispatchEvent(
      new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }),
    );
  });
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({
    taskId: 'T_alerts',
    text: 'https://wiki.example/pasted',
  });
  await expect(paste).toHaveValue('');
});

test('a title typed before a pasted link stays; Enter sends both', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const paste = page.locator(t('ade-details-paste'));
  await paste.fill('Design notes ');
  await paste.pressSequentially('https://docs.google.com/document/d/abc/edit');
  expect(calls(control, IPC.adeTaskAddLink)).toHaveLength(0);
  await expect(paste).toHaveValue(addArgs.text);
  await paste.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({ ...addArgs, taskId: 'T_alerts' });
  await expect(paste).toHaveValue('');
});

test('Escape clears the add line without a call', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const paste = page.locator(t('ade-details-paste'));
  await paste.fill('half a thought');
  await paste.press('Escape');
  await expect(paste).toHaveValue('');
  expect(calls(control, IPC.adeTaskAddLink)).toHaveLength(0);
});

test('a refused line shows the error and keeps the text', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    base({
      response: undefined,
      error: { code: 'E_BAD_REQUEST', message: 'Not a link or issue key.' },
    }),
  );
  await openDetails(page);
  const paste = page.locator(t('ade-details-paste'));
  await paste.fill('just some words');
  await paste.press('Enter');
  await expect(page.locator(t('ade-details-paste-error'))).toContainText('Not a link');
  await expect(paste).toHaveValue('just some words');
});

test('an already-shown link answers with a quiet note', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    base({ response: { filed: 'known', kind: 'issue', artifact: null } }),
  );
  await openDetails(page);
  const paste = page.locator(t('ade-details-paste'));
  await paste.fill('PAY-130');
  await paste.press('Enter');
  await expect(page.locator(t('ade-details-paste-known'))).toBeVisible();
  await expect(paste).toHaveValue('');
});

test('contract: editing a stored row sends its text as typed; Escape cancels; F2 opens', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const runbook = stored('Runbook');
  const target = row(page, 'Runbook');

  await target.dblclick();
  const input = page.locator(t('ade-details-link-input'));
  await expect(input).toHaveValue(`${runbook.title} ${runbook.url}`);
  await input.fill(editArgs.text);
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskEditLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskEditLink)[0]?.args).toEqual({
    ...editArgs,
    taskId: 'T_alerts',
    id: runbook.id,
  });

  await target.locator(t('ade-details-link-title')).focus();
  await page.keyboard.press('F2');
  await expect(input).toBeVisible();
  await input.fill('discarded');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  expect(calls(control, IPC.adeTaskEditLink)).toHaveLength(1);

  await target.locator(t('ade-details-link-edit')).click();
  await input.fill('');
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveArtifact)).toHaveLength(1);
});

test('the kind menu re-files a stored row; detected rows have no menu or edit', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const link = stored('wiki.example/x');
  await row(page, 'wiki.example/x').locator(t('ade-details-link-kind')).click();
  await page.locator(`${t('ade-details-link-kind-item')}[data-kind="doc"]`).click();
  await expect.poll(() => calls(control, IPC.adeTaskEditLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskEditLink)[0]?.args).toEqual({
    taskId: 'T_alerts',
    slot: 'stored',
    id: link.id,
    text: '',
    kind: 'doc',
  });

  const detected = row(page, 'PAY-10');
  await expect(detected).toHaveAttribute('data-origin', 'detected');
  await expect(detected.locator(t('ade-details-link-kind'))).toHaveCount(0);
  await expect(detected.locator(t('ade-details-link-edit'))).toHaveCount(0);
});

test('removing the Jira link clears the slot', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await row(page, 'PAY-130').locator(t('ade-details-link-remove')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_alerts',
    patch: { clearJira: true },
  });
});
