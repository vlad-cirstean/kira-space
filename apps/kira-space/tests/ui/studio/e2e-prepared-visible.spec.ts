import type { Locator } from '@playwright/test';
import type { ApprovalItem } from '@shared/domain/approvals';
import { liveItem, pushApprovals } from '@workbench/testing/ui/approvals';
import { expect, studioTest as test } from '../fixtures';
import { contract } from '../support/contract';

// Contract e2e-prepared-visible. Backend half: dbmcpflow TestE2EPreparedVisible, the same scenario. A prepare
// script sets arguments of a kube list, a kube logs read and an API request. Every key the script set is listed
// in the request box and marks its token in the request text, so no prepared argument is invisible.

const SC = 'e2e-prepared-visible';
const item = (key: string) =>
  contract<ApprovalItem[]>(SC, `ApprovalsService.List#${key}`)[0] as ApprovalItem;

interface Pair {
  name: string;
  value: string;
}
interface KubePayload {
  changed: string[];
  request: Record<string, string | number | boolean>;
}
interface ApiPayload {
  method: string;
  url: string;
  headers: Pair[];
  query: Pair[];
  body: { contentType: string; text: string } | null;
  script: { prepare: { changed: string[] } };
}

const pairs = (list: Pair[], sep: string) =>
  list.map((p) => `${p.name}${sep}${p.value}`).join(', ');

// The text a key's token shows in the request line; null for a part (headers, body) with no single token text.
function kubeToken(key: string, v: string | number | boolean): string {
  if (key === 'maxRows') return `# Up to ${v} rows`;
  if (key === 'previous') return v ? '--previous' : '--previous=false';
  if (key === 'timestamps') return v ? '--timestamps' : '--timestamps=false';
  return String(v);
}

async function expectMarked(box: Locator, key: string, text: string | null) {
  const mark = box.locator(`[data-prepared="${key}"]`).first();
  await expect(mark, `${key} is marked in the request`).toBeVisible();
  await expect(mark, `${key} carries the prepared mark`).toHaveClass(/-warn/);
  if (text !== null) await expect(mark).toContainText(text);
}

for (const key of ['list', 'logs'] as const) {
  test(`contract: every key the kube ${key} prepare set is listed and marked`, async ({
    relaunch,
  }) => {
    const it = item(key);
    const p = it.payload as unknown as KubePayload;
    const { window: page } = await relaunch({});
    await pushApprovals(page, [liveItem(it)]);
    const card = page.locator(`[data-testid="approval-item-${it.id}"]`);
    const box = card.locator('[data-testid="kube-approval-box-request"]');
    await expect(box.locator('[data-testid^="kube-approval-prepared-"]')).toHaveCount(
      p.changed.length,
    );
    for (const k of p.changed) {
      const v = p.request[k] as string | number | boolean;
      const shown = v === '' ? '""' : String(v);
      await expect(box.locator(`[data-testid="kube-approval-prepared-${k}"]`)).toContainText(shown);
      await expectMarked(box, k, kubeToken(k, v));
    }
  });
}

test('contract: every key the API prepare set is listed and marked', async ({ relaunch }) => {
  const it = item('api');
  const p = it.payload as unknown as ApiPayload;
  const changed = p.script.prepare.changed;
  const { window: page } = await relaunch({});
  await pushApprovals(page, [liveItem(it)]);
  const box = page.locator(
    `[data-testid="approval-item-${it.id}"] [data-testid="api-approval-box-request"]`,
  );
  const row: Record<string, string> = {
    method: p.method,
    url: p.url,
    headers: pairs(p.headers, ': '),
    query: pairs(p.query, '='),
    contentType: p.body?.contentType ?? '',
    body: p.body?.text ?? '',
  };
  await expect(box.locator('[data-testid^="api-approval-prepared-"]')).toHaveCount(changed.length);
  for (const k of changed) {
    await expect(box.locator(`[data-testid="api-approval-prepared-${k}"]`)).toContainText(
      row[k] as string,
    );
    // The body and the header rows draw their own text; the others show it in the request line.
    await expectMarked(box, k, k === 'method' ? p.method : null);
  }
  await expect(box.locator('[data-testid="api-approval-headers"]')).toContainText('Content-Type');
});
