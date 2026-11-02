import { expect } from '@playwright/test';
import {
  calls,
  issueControl,
  issueFallback,
  issueGone,
  issueOk,
  issueOmitted,
  issueTarget,
  issueUnauthenticated,
  t,
  hostViewTest as test,
} from './support/hostView';
import { IPC } from './support/ipcChannels';

// UI half of issueview_test.go: the Go flow writes tests/contract/issue-view.json (target and issue
// result per case); this spec renders them in the host view window.

const LINK = { channel: IPC.linkOpenExternal, response: null };
const OPEN = { channel: IPC.hostViewOpen, response: { key: 'hostview-x', opened: true } };

test('issue renders header, description, comments and sidebar', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueOk, [LINK]) });
  await expect(page.locator(t('host-view-title'))).toHaveText('Export report as CSV');
  await expect(page.locator(t('host-view-ref'))).toHaveText('PROJ-12 Story');
  await expect(page.locator(t('host-view-state'))).toHaveText('In Progress');
  await expect(page.locator(t('issue-meta'))).toContainText('Rita Reporter reported');

  const body = page.locator(t('issue-description')).locator(t('host-view-markdown'));
  await expect(body.locator('h2')).toHaveText('Goal');
  await expect(body.locator('script')).toHaveCount(0);
  await expect(body).toContainText('<script>alert(1)</script>');
  await expect(body.locator('[data-kira-image]')).toHaveText('mockup.png');
  const link = body.locator('a', { hasText: 'the spec' });
  await expect(link).toHaveAttribute('href', 'https://example.com/spec');
  await link.click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({
    url: 'https://example.com/spec',
  });

  const comments = page.locator(t('issue-comment'));
  await expect(comments).toHaveCount(3);
  await expect(comments.nth(0)).toContainText('Dana Dev');
  await expect(comments.nth(0)).toContainText('Started on the exporter.');
  await expect(comments.nth(1).locator(t('issue-comment-edited'))).toBeVisible();
  await expect(comments.nth(0).locator(t('issue-comment-edited'))).toHaveCount(0);
  await expect(page.locator(t('issue-comments-truncated'))).toContainText('Open on Jira');

  await expect(page.locator(t('issue-side-assignee'))).toHaveText('Unassigned');
  await expect(page.locator(t('issue-side-reporter'))).toContainText('Rita Reporter');
  await expect(page.locator(t('issue-side-priority'))).toHaveText('High');
  await expect(page.locator(t('issue-side-label'))).toHaveText(['backend', 'export']);
  await expect(page.locator(t('issue-side-component'))).toHaveText(['Reports', 'API']);
  await expect(page.locator(t('issue-side-parent'))).toContainText('PROJ-1 Reporting epic');
  await expect(page.locator(t('issue-side-pull-requests-count'))).toHaveText('2 pull requests');
});

test('description renders chips, callouts and attachments', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueOk, [LINK]) });
  const body = page.locator(t('issue-description')).locator(t('host-view-markdown'));
  await expect(body.locator('.markdown-alert-note')).toContainText('Export runs nightly.');
  await expect(body.locator('.markdown-alert-note .markdown-alert-title')).toHaveText('Note');
  await expect(body.locator('span', { hasText: '@Dana Dev' })).toHaveCount(1);
  await expect(body.locator('a', { hasText: '@Dana Dev' })).toHaveCount(0);
  await expect(body.locator('span', { hasText: 'IN REVIEW' })).toHaveCount(1);
  await expect(body.locator('.kira-prose-scroll table')).toHaveCount(1);

  await body.locator('[data-kira-image]').click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({
    url: 'https://acme.atlassian.net/rest/api/3/attachment/content/10100',
  });

  const tiles = page.locator(t('issue-attachment'));
  await expect(tiles).toHaveCount(2);
  await expect(tiles.nth(0)).toContainText('spec.pdf');
  await expect(tiles.nth(1)).toContainText('mockup.png');
  await tiles.nth(1).click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(2);
  expect(calls(control, IPC.linkOpenExternal)[1]?.args).toEqual({
    url: 'https://acme.atlassian.net/rest/api/3/attachment/content/10100',
  });
});

test('the parent opens its own view', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueOk, [OPEN]) });
  await page.locator(t('issue-side-parent-open')).click();
  await expect.poll(() => calls(control, IPC.hostViewOpen)).toHaveLength(1);
  expect(calls(control, IPC.hostViewOpen)[0]?.args).toEqual({ url: 'PROJ-1' });
});

test('Open on Jira and Copy link use the browse URL', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueOk, [LINK]) });
  await page.evaluate(() => {
    const w = window as unknown as { __clipboard: string[] };
    w.__clipboard = [];
    navigator.clipboard.writeText = async (text: string) => {
      w.__clipboard.push(text);
    };
  });
  await page.locator(t('host-view-open-host')).click();
  await expect.poll(() => calls(control, IPC.linkOpenExternal)).toHaveLength(1);
  expect(calls(control, IPC.linkOpenExternal)[0]?.args).toEqual({ url: issueTarget.url });
  await page.locator(t('host-view-copy')).click();
  await expect
    .poll(() =>
      page.evaluate(() => (window as unknown as { __clipboard: string[] }).__clipboard[0]),
    )
    .toBe(issueTarget.url);
});

test('comments from the list and omitted comments show their notes', async ({ relaunch }) => {
  const fallback = await relaunch({ control: issueControl(issueFallback) });
  const comments = fallback.window.locator(t('issue-comment'));
  await expect(comments).toHaveCount(2);
  await expect(comments.nth(1)).toContainText('Newest from the list.');
  await fallback.window.close();

  const { window: page } = await relaunch({ control: issueControl(issueOmitted) });
  await expect(page.locator(t('issue-comments-omitted'))).toContainText('too large');
  await expect(page.locator(t('issue-comment'))).toHaveCount(0);
});

test('unauthenticated shows the reason and Retry asks again', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueUnauthenticated) });
  await expect(page.locator(t('host-view-failure-reason'))).toContainText('Jira');
  expect(calls(control, IPC.hostViewIssue)).toHaveLength(1);
  await page.locator(t('host-view-retry')).click();
  await expect.poll(() => calls(control, IPC.hostViewIssue)).toHaveLength(2);
});

test('gone shows the not-found reason', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: issueControl(issueGone) });
  await expect(page.locator(t('host-view-failure-reason'))).toContainText('Jira');
});

test('Refresh fetches the issue again', async ({ relaunch }) => {
  const { window: page, control } = await relaunch({ control: issueControl(issueOk) });
  await expect(page.locator(t('host-view-title'))).toBeVisible();
  expect(calls(control, IPC.hostViewIssue)).toHaveLength(1);
  await page.locator(t('host-view-refresh')).click();
  await expect.poll(() => calls(control, IPC.hostViewIssue)).toHaveLength(2);
});
