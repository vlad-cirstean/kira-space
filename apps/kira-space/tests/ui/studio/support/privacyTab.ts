import type { ConnectionSummary } from '@shared/domain/connection';
import type { ConnectionConfigView, ConnectionFileDetail } from '@shared/domain/connectionConfig';
import type { MaskReviewListing } from '@shared/domain/maskReview';
import { expect, type studioTest as test } from '../../fixtures';
import { contract } from '../../support/contract';
import { IPC } from '../../support/ipcChannels';
import { connectionRow } from './tree';

// Opens the connection dialog's Privacy tab on a mocked sqlite connection, for the Privacy tab's UI
// and visual specs. The review listing is the privacy-masking contract.

const FILE = 'local.yaml';
const dlg = '[data-testid="connection-file-dialog"]';

const list = () => contract<ConnectionSummary[]>('connection-files', 'ConnectionsService.List');
const view = () =>
  contract<ConnectionConfigView>('connection-files', 'ConnectionConfigService.View');
const listing = () =>
  contract<MaskReviewListing>('privacy-masking', 'MaskRulesService.ReviewColumns');

export type Relaunch = Parameters<Parameters<typeof test>[2]>[0]['relaunch'];

export interface Draft {
  reviewed?: string[];
  masks?: { table?: string; column: string; kind: string }[];
  mcp?: boolean;
}

function draftControl(d: Draft) {
  const mcp = d.mcp ?? true;
  const lines = ['name: Local', 'kind: sqlite', ...(mcp ? ['mcp:', '  enabled: true'] : [])];
  const value: Record<string, unknown> = { name: 'Local', kind: 'sqlite' };
  if (mcp) value.mcp = { enabled: true };
  if (d.reviewed) {
    lines.push('mask_review:', '  columns:', ...d.reviewed.map((x) => `    - ${x}`));
    value.mask_review = { columns: d.reviewed };
  }
  if (d.masks) {
    lines.push(
      'masks:',
      ...d.masks.flatMap((m) => [
        `  - column: ${m.column}`,
        ...(m.table ? [`    table: ${m.table}`] : []),
        `    kind: ${m.kind}`,
      ]),
    );
    value.masks = d.masks.map(({ table, column, kind }) => ({ table, column, kind }));
  }
  const text = `${lines.join('\n')}\n`;
  const base = contract<ConnectionFileDetail>('connection-form', 'file#read');
  const detail = { ...base, file: FILE, id: list()[0]?.id ?? '', text };
  return [
    { channel: IPC.connectionsList, response: list() },
    { channel: IPC.connectionConfigView, response: view() },
    { channel: IPC.connectionConfigFile, response: detail },
    { channel: IPC.connectionConfigForm, response: { problem: null, text, value } },
    { channel: IPC.maskReviewColumns, response: listing() },
    { channel: IPC.maskReviewPending, response: { count: 0 } },
  ];
}

export async function openPrivacyTab(relaunch: Relaunch, d: Draft = {}) {
  const { window: page } = await relaunch({ control: draftControl(d) });
  await (await connectionRow(page, 'Local')).click({ button: 'right' });
  await page.click('[data-testid="menu-item-edit"]');
  await expect(page.locator(dlg)).toBeVisible();
  await page.locator('[data-testid="connection-tab-privacy"]').click();
  return page;
}

export type Page = Awaited<ReturnType<typeof openPrivacyTab>>;
