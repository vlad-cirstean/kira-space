import { pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from './fixtures';
import { apiRequestItem, dbQueryItem } from './support/approvalItems';
import { armFirstFrame, firstFrame, framesLate } from './support/firstFrame';

// The approvals dialog opens with the whole statement and body in its first frame: no empty editor
// that grows later. Backend half: dbmcpflow TestApiRequestBodies.

test('a db.query statement is in the first frame, all lines', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await armFirstFrame(page, 'approvals-dialog', 'db-mcp-approval-statement');
  await pushApprovals(page, [dbQueryItem('db.query:1', 3)]);
  const r = await firstFrame(page);
  expect(framesLate(r)).toBe(0);
  for (const n of [1, 2, 3])
    expect(r.textAtDialogFrame).toContain(`select ${n} from shop_t${n - 1}`);
});

test('an api.request body is in the first frame, all lines', async ({ relaunch }) => {
  const { window: page } = await relaunch({});
  await armFirstFrame(page, 'approvals-dialog', 'api-approval-body-text');
  await pushApprovals(page, [apiRequestItem('api.request:1', 3)]);
  const r = await firstFrame(page);
  expect(framesLate(r)).toBe(0);
  expect(r.textAtDialogFrame).toContain('"k0": 0');
  expect(r.textAtDialogFrame).toContain('{');
  expect(r.textAtDialogFrame).toContain('}');
});
