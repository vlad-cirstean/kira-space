import type { ApprovalItem } from '@shared/domain/approvals';
import { contract } from '../ui/support/contract';
import { expect, test } from './fixtures';

// P637: the phone's agent requests on Need You. Contract mobile-approvals; backend half:
// flows/mobileflow TestPhoneApprovals (approvals_test.go).

const t = (id: string) => `[data-testid="${id}"]`;

const pending = () =>
  contract<ApprovalItem[]>('mobile-approvals', 'http:GET /api/approvals#pending');
const taskItem = (): ApprovalItem => {
  const it = pending().find((i) => i.kind === 'task.create');
  if (!it) throw new Error('contract has no task.create item');
  return it;
};
const branchItem = (): ApprovalItem => {
  const it = pending().find((i) => i.kind === 'branch.request');
  if (!it) throw new Error('contract has no branch.request item');
  return it;
};

const detailsItem = (): ApprovalItem => {
  const [it] = contract<ApprovalItem[]>('mobile-approvals', 'http:GET /api/approvals#detailsEdit');
  if (!it) throw new Error('contract has no details.edit item');
  return it;
};

const answers = (server: { state: { requests: { method: string; path: string }[] } }) =>
  server.state.requests.filter(
    (r) => r.method === 'POST' && r.path === '/api/approvals/answer',
  ) as {
    body: string;
    key: string;
  }[];
const gets = (server: { state: { requests: { method: string; path: string }[] } }) =>
  server.state.requests.filter((r) => r.method === 'GET' && r.path === '/api/approvals').length;

// `app` resets the server on first use, so a test pairs after requesting it, before opening.
function pair(
  server: { state: { auth: string; permissions: unknown; agentInputGlobal: boolean } },
  agentInput = true,
): void {
  server.state.auth = 'ok';
  server.state.permissions = { write: false, agentInput };
  server.state.agentInputGlobal = agentInput;
}

test('contract: pending requests render on Need You with the brief and count', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = taskItem();
  server.state.replies.approvals = [it];
  await app();
  const card = page.locator(t(`approval-item-${it.id}`));
  await expect(card).toBeVisible();
  await expect(card.locator(t('approval-operation'))).toHaveText('Create task');
  await expect(card.locator(t('approval-target-name'))).toHaveText('API login');
  await expect(card.locator(t('ade-task-create-brief'))).toContainText(
    'Wire the login form to the API.',
  );
  await expect(card.locator(t('ade-task-create-brief'))).toContainText(
    'Hand back the endpoint list.',
  );
  await expect(card.locator(t('ade-task-create-workflow'))).toContainText('Flow');
  await expect(page.locator(t('phone-approvals-count'))).toHaveText('1');
  await expect(page.locator(t('tab-needs-badge'))).toHaveText('5');
});

test('contract: Approve sends the answer once with a key, then the push removes the card', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = taskItem();
  const kept = branchItem();
  server.state.replies.approvals = [it, kept];
  await app();
  await expect(page.locator(t('phone-approval'))).toHaveCount(2);
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => answers(server).length).toBe(1);
  const sent = answers(server)[0];
  expect(JSON.parse(sent?.body ?? '{}')).toEqual({ ids: [it.id], approve: true });
  expect(sent?.key).toMatch(/^[0-9a-f-]{36}$/);
  server.emit(
    'kira:approvals:changed',
    contract('mobile-approvals', 'sse:kira:approvals:changed#afterApprove'),
  );
  await expect(page.locator(t(`approval-item-${it.id}`))).toHaveCount(0);
  await expect(page.locator(t(`approval-item-${kept.id}`))).toBeVisible();
  await expect(page.locator(t('phone-approvals-count'))).toHaveText('1');
});

test('contract: the last answered request leaves no approvals section', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = taskItem();
  server.state.replies.approvals = [it];
  await app();
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => answers(server).length).toBe(1);
  server.emit('kira:approvals:changed', []);
  await expect(page.locator(t('phone-approvals'))).toHaveCount(0);
  await expect(page.locator(t('tab-needs-badge'))).toHaveText('4');
});

test('contract: Deny sends approve false', async ({ app, page, server }) => {
  pair(server);
  const it = taskItem();
  server.state.replies.approvals = [it];
  await app();
  await page.locator(t(`approval-deny-${it.id}`)).click();
  await expect.poll(() => answers(server).length).toBe(1);
  expect(JSON.parse(answers(server)[0]?.body ?? '{}')).toEqual({ ids: [it.id], approve: false });
});

test('contract: with agent input off the card shows without Approve or Deny', async ({
  app,
  page,
  server,
}) => {
  const it = taskItem();
  server.state.replies.approvals = [it];
  pair(server, false);
  await app();
  await expect(page.locator(t(`approval-item-${it.id}`))).toBeVisible();
  await expect(page.locator(t(`approval-approve-${it.id}`))).toHaveCount(0);
  await expect(page.locator(t(`approval-deny-${it.id}`))).toHaveCount(0);
  await expect(page.locator(`${t('phone-approvals')} ${t('permission-hint')}`)).toContainText(
    'Answering agent requests is off for this phone.',
  );
});

test('contract: a refused answer keeps the card and shows the error', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = taskItem();
  server.state.replies.approvals = [it];
  server.state.replies.approvalAnswer = contract(
    'mobile-approvals',
    'http:POST /api/approvals/answer#notPhoneKind',
  ).map((r: { id: string }) => ({ ...r, id: it.id }));
  await app();
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect(
    page.locator(t(`approval-item-${it.id}`)).locator(t('approval-error')),
  ).toContainText('no longer exists');
  await expect(page.locator(t(`approval-item-${it.id}`))).toBeVisible();
});

test('contract: an expired device returns to the pairing screen on Approve', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = taskItem();
  server.state.replies.approvals = [it];
  server.state.replies.expired = contract(
    'mobile-approvals',
    'http:POST /api/approvals/answer#expired',
  );
  await app();
  await expect(page.locator(t(`approval-item-${it.id}`))).toBeVisible();
  server.state.auth = 'expired';
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect(page.locator(t('pair-screen'))).toBeVisible();
  await expect(page.locator(t('pair-notice'))).toContainText('expired');
});

test('contract: a branch request shows the holder without a path and answers with its options', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = branchItem();
  server.state.replies.approvals = [it];
  await app();
  const holder = page.locator(t('ade-branch-request-holder-c1i1'));
  await expect(holder).toContainText('feat-x');
  await expect(page.locator(t(`approval-item-${it.id}`))).not.toContainText('wt/');
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => answers(server).length).toBe(1);
  expect(JSON.parse(answers(server)[0]?.body ?? '{}')).toEqual({
    ids: [it.id],
    approve: true,
    options: { [it.id]: { itemIds: ['c1i1'], remoteItemIds: [], shownItemIds: ['c1i1'] } },
  });
});

test('contract: a details edit shows its cause and files and answers with the paths', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = detailsItem();
  const { items } = it.payload as { items: { path: string }[] };
  server.state.replies.approvals = [it];
  await app();
  const body = page.locator(t('ade-details-edit-body'));
  await expect(body.locator(t('ade-details-edit-reason'))).toHaveCount(0);
  await expect(body.locator(t('ade-details-edit-file'))).toHaveCount(items.length);
  await expect(body.locator(t('ade-details-edit-file'))).toContainText('Another agent’s note');
  await page.locator(t(`approval-approve-${it.id}`)).click();
  await expect.poll(() => answers(server).length).toBe(1);
  expect(JSON.parse(answers(server)[0]?.body ?? '{}')).toEqual({
    ids: [it.id],
    approve: true,
    options: { [it.id]: { approved: items.map((f) => f.path) } },
  });
});

test('an unreadable kind keeps Deny, hides Approve and falls back to the origin text', async ({
  app,
  page,
  server,
}) => {
  pair(server);
  const it = detailsItem();
  // details.edit's requester parses the payload; this one no longer matches, so the kind throws.
  server.state.replies.approvals = [
    { ...it, payload: { broken: true }, origin: { client: 'Claude Code' } },
  ];
  await app();
  const row = page.locator(t('phone-approval'));
  await expect(row.locator(t('approval-origin'))).toHaveText('Claude Code');
  await expect(row.locator(t(`approval-deny-${it.id}`))).toBeVisible();
  await expect(row.locator(t(`approval-approve-${it.id}`))).toHaveCount(0);
});

test('a reconnect refetches the approvals', async ({ app, page, server }) => {
  pair(server);
  server.state.replies.approvals = [taskItem()];
  await app();
  await expect(page.locator(t('connection-dot'))).toHaveAttribute('aria-label', 'Connection live');
  const before = gets(server);
  for (const stream of server.state.streams) stream.destroy();
  await expect.poll(() => gets(server)).toBeGreaterThan(before);
});
