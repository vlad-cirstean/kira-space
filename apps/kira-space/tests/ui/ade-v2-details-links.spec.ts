import { expect, test } from './fixtures';
import { openDetails, openPlan } from './support/adeV2';
import { contract } from './support/contract';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { line, openCard, pill } from './support/links';
import type { ControlSnapshot } from './support/types';

// Details links: the strip's add popover files typed or pasted text, a pill's card edits in place and re-files
// a stored link's kind. Contract ade-details-links; backend half: flows/adeflow TestDetailsLinks.

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

const paste = (page: import('@playwright/test').Page) => page.locator(t('ade-links-paste'));
const openAdd = async (page: import('@playwright/test').Page) => {
  await page.locator(t('ade-links-add')).click();
  await expect(paste(page)).toBeVisible();
};
const pasteEvent = (
  page: import('@playwright/test').Page,
  testid: string,
  text: string,
  has = '',
) =>
  page.evaluate(
    ([id, value, within]) => {
      const els = [...document.querySelectorAll(`[data-testid="${id}"]`)];
      const el = els.find((e) => e.textContent?.includes(within ?? '')) ?? els[0];
      const data = new DataTransfer();
      data.setData('text', value ?? '');
      el?.dispatchEvent(
        new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }),
      );
    },
    [testid, text, has],
  );

test('contract: stored pills carry their kind', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch, base());
  await openDetails(page);
  const kinds = await page
    .locator(`${t('ade-links-pill')}[data-origin="stored"]`)
    .evaluateAll((els) => els.map((e) => e.getAttribute('data-kind')));
  expect(kinds.sort()).toEqual(
    listed()
      .filter((a) => a.id)
      .map((a) => a.kind)
      .sort(),
  );
  await expect(pill(page, 'feat/x')).toHaveAttribute('data-kind', 'branch');
  const card = await openCard(page, 'feat/x');
  await expect(card.locator(`${t('ade-links-card-kind-branch')}`)).toHaveAttribute(
    'data-state',
    'on',
  );
  await page.keyboard.press('Escape');
  const detected = await openCard(page, 'PAY-10');
  await expect(detected.locator(t('ade-links-card-kind'))).toHaveCount(0);
});

const GROUP_NAME: Record<string, string> = {
  issue: 'Issues',
  changeRequest: 'Change requests',
  misc: 'Misc',
};
const categoryOf = (kind: string) => (kind === 'issue' || kind === 'changeRequest' ? kind : 'misc');

test('links group under Issues, Change requests and Misc; the block stays compact', async ({
  relaunch,
}) => {
  const issueFx = contract('ade-issue-tracker', 'IssueTrackerService.TaskIssue');
  const { window: page } = await openPlan(relaunch, [
    ...base(),
    { channel: IPC.adeTaskIssue, response: issueFx },
  ]);
  await openDetails(page);
  await expect(
    page
      .locator(t('ade-links-line'))
      .evaluateAll((els) => els.map((e) => e.getAttribute('data-category'))),
  ).resolves.toEqual(['issue', 'changeRequest', 'misc']);
  for (const id of ['issue', 'changeRequest', 'misc']) {
    await expect(line(page, id).locator(t('ade-links-category'))).toHaveAttribute(
      'aria-label',
      new RegExp(`^${GROUP_NAME[id]}`),
    );
  }
  // Jira slot plus listed artifacts, minus detected branches.
  await expect(
    line(page, 'issue').locator(t('ade-links-pill'), { hasText: 'PAY-130' }),
  ).toHaveCount(1);
  for (const a of listed().filter((x) => x.id || x.kind !== 'branch')) {
    const found = line(page, categoryOf(a.kind)).locator(t('ade-links-pill'), {
      hasText: a.title || a.url,
    });
    expect(await found.count(), a.title).toBeGreaterThan(0);
  }
  const box = await page.locator(t('ade-links')).boundingBox();
  expect(box?.height ?? Infinity).toBeLessThanOrEqual(104);
  const jira = await openCard(page, 'PAY-130');
  await expect(jira.locator(t('ade-task-issue'))).toHaveCount(1);
});

test('a line without links is hidden', async ({ relaunch }) => {
  const onlyMisc = listed().filter((a) => a.id && categoryOf(a.kind) === 'misc');
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.adeTaskArtifacts, response: { artifacts: onlyMisc, gaps: [] } },
  ]);
  await openDetails(page);
  // The task's own Jira slot keeps Issues; no stored change request, so no Change requests line.
  await expect(
    page
      .locator(t('ade-links-line'))
      .evaluateAll((els) => els.map((e) => e.getAttribute('data-category'))),
  ).resolves.toEqual(['issue', 'misc']);
  await expect(page.locator(t('ade-links-empty'))).toHaveCount(0);
});

test('pasting into the add popover files at once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await openAdd(page);
  await paste(page).focus();
  await pasteEvent(page, 'ade-links-paste', 'https://wiki.example/pasted');
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({
    taskId: 'T_alerts',
    text: 'https://wiki.example/pasted',
  });
});

test('pasting on a focused pill files at once', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await pill(page, 'feat/x').focus();
  await pasteEvent(page, 'ade-links-pill', 'https://wiki.example/strip', 'feat/x');
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({
    taskId: 'T_alerts',
    text: 'https://wiki.example/strip',
  });
});

test('a title typed before a pasted link stays; Enter sends both', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await openAdd(page);
  await paste(page).fill('Design notes ');
  await paste(page).pressSequentially('https://docs.google.com/document/d/abc/edit');
  expect(calls(control, IPC.adeTaskAddLink)).toHaveLength(0);
  await expect(paste(page)).toHaveValue(addArgs.text);
  await paste(page).press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskAddLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskAddLink)[0]?.args).toEqual({ ...addArgs, taskId: 'T_alerts' });
});

test('Escape closes the add popover without a call; reopening starts empty', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  await openAdd(page);
  await paste(page).fill('half a thought');
  await paste(page).press('Escape');
  await expect(page.locator(t('ade-links-add-popover'))).toHaveCount(0);
  expect(calls(control, IPC.adeTaskAddLink)).toHaveLength(0);
  await openAdd(page);
  await expect(paste(page)).toHaveValue('');
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
  await openAdd(page);
  await paste(page).fill('just some words');
  await paste(page).press('Enter');
  await expect(page.locator(t('ade-links-paste-error'))).toContainText('Not a link');
  await expect(paste(page)).toHaveValue('just some words');
});

test('an already-shown link answers with a quiet note', async ({ relaunch }) => {
  const { window: page } = await openPlan(
    relaunch,
    base({ response: { filed: 'known', kind: 'issue', artifact: null } }),
  );
  await openDetails(page);
  await openAdd(page);
  await paste(page).fill('PAY-130');
  await paste(page).press('Enter');
  await expect(page.locator(t('ade-links-paste-known'))).toBeVisible();
  await expect(paste(page)).toHaveValue('');
});

test('contract: editing a stored link sends its text as typed; Escape cancels; F2 opens', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const runbook = stored('Runbook');

  const card = await openCard(page, 'Runbook');
  await card.locator(t('ade-links-card-edit')).click();
  const input = page.locator(t('ade-links-edit-input'));
  await expect(input).toHaveValue(`${runbook.title} ${runbook.url}`);
  await input.fill(editArgs.text);
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskEditLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskEditLink)[0]?.args).toEqual({
    ...editArgs,
    taskId: 'T_alerts',
    id: runbook.id,
  });

  await pill(page, 'Runbook').focus();
  await page.keyboard.press('F2');
  await expect(input).toBeVisible();
  await input.fill('discarded');
  await input.press('Escape');
  await expect(input).toHaveCount(0);
  expect(calls(control, IPC.adeTaskEditLink)).toHaveLength(1);
  await page.keyboard.press('Escape');
  await expect(page.locator(t('ade-links-card'))).toHaveCount(0);

  const again = await openCard(page, 'Runbook');
  await again.locator(t('ade-links-card-edit')).click();
  await input.fill('');
  await input.press('Enter');
  await expect.poll(() => calls(control, IPC.adeTaskRemoveArtifact)).toHaveLength(1);
});

test('the card re-files a stored link; detected links have no kind picker or edit', async ({
  relaunch,
}) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const link = stored('wiki.example/x');
  const card = await openCard(page, 'wiki.example/x');
  await card.locator(t('ade-links-card-kind-doc')).click();
  await expect.poll(() => calls(control, IPC.adeTaskEditLink)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskEditLink)[0]?.args).toEqual({
    taskId: 'T_alerts',
    slot: 'stored',
    id: link.id,
    text: '',
    kind: 'doc',
  });
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');

  await expect(pill(page, 'PAY-10')).toHaveAttribute('data-origin', 'detected');
  const detected = await openCard(page, 'PAY-10');
  await expect(detected.locator(t('ade-links-card-kind'))).toHaveCount(0);
  await expect(detected.locator(t('ade-links-card-edit'))).toHaveCount(0);
});

test('removing the Jira link clears the slot', async ({ relaunch }) => {
  const { window: page, control } = await openPlan(relaunch, base());
  await openDetails(page);
  const card = await openCard(page, 'PAY-130');
  await card.locator(t('ade-links-card-remove')).click();
  await expect.poll(() => calls(control, IPC.adeTaskUpdateTask)).toHaveLength(1);
  expect(calls(control, IPC.adeTaskUpdateTask)[0]?.args).toMatchObject({
    taskId: 'T_alerts',
    patch: { clearJira: true },
  });
});
