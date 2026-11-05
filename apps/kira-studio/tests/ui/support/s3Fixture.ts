import type { Page } from '@playwright/test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect } from '../fixtures';
import { IPC } from './ipcChannels';
import { connectionRow, expandRow, findRow, openRowMenu } from './tree';

// Hand-built, not a capture: no S3 IPC fixture exists, and what the stale-gate specs need is
// only the S3 capability set (internal/adapters/s3/caps.go) plus one bucket with a prefix and an
// object. The gates under test live in the renderer, so there is no backend half to split off.

export const S3_ID = 'test-s3';
export const BUCKET_PATH = 'bucket:media';
export const OBJECT_PATH = `${BUCKET_PATH}/object:a.txt`;

export const CAPS = {
  tabular: false,
  documents: false,
  keyValue: true,
  stream: false,
  keyBrowser: true,
  keyTypes: false,
  defaultPageKind: 'keyvalue' as const,
  sql: false,
  definition: false,
  describe: false,
  schemaColumns: false,
  projection: false,
  serverFilter: false,
  exactCount: true,
  pagination: 'token' as const,
  foreignKeys: false,
  canInsert: true,
  canUpdate: true,
  canDelete: true,
  writable: true,
  transactions: false,
  cancel: true,
  fileTransfer: true,
};

function summary(): ConnectionSummary {
  return {
    file: 'fixture.yaml',
    id: S3_ID,
    name: 'Test S3',
    kind: 's3',
    color: 'green',
    mode: 'fields',
    readOnly: false,
    host: '127.0.0.1',
    port: 4566,
    database: null,
    username: null,
    uri: null,
    options: {},
    preconnect: null,
    preconnectSidecar: false,
    preconnectDelaySec: 0,
    autoExplain: false,
    throttlePerSec: 0,
    mcpEnabled: false,
    mcpDescription: '',
    mcpReadMode: 'allow',
    mcpWriteMode: 'prompt',
    mcpDdlMode: 'deny',
    mcpAutoExplain: true,
    mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit' },
    mcpStatementTimeoutSec: 10,
    mcpTables: null,
    maskReviewOn: false,
    sortOrder: 0,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
}

function children(path: string, nodes: unknown[]): ControlSnapshot {
  return {
    channel: IPC.treeChildren,
    args: { connectionId: S3_ID, path, refresh: false },
    response: { nodes, source: 'server', truncated: false },
  };
}

export const s3Control: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [summary()] },
  { channel: IPC.connectionsStates, response: [] },
  {
    channel: IPC.connectionsConnect,
    args: { id: S3_ID },
    response: {
      connectionId: S3_ID,
      status: 'connected',
      serverVersion: 'LocalStack',
      error: null,
      since: 1735689600000,
      caps: CAPS,
    },
  },
  {
    channel: IPC.connectionsDisconnect,
    args: { id: S3_ID },
    response: {
      connectionId: S3_ID,
      status: 'disconnected',
      serverVersion: null,
      error: null,
      since: 1735689600000,
      caps: null,
    },
  },
  children('', [{ kind: 'bucket', name: 'media', path: BUCKET_PATH, hasChildren: true }]),
  children(BUCKET_PATH, [
    {
      kind: 'prefix',
      name: 'logs',
      path: `${BUCKET_PATH}/prefix:logs`,
      hasChildren: true,
    },
    { kind: 'object', name: 'a.txt', path: OBJECT_PATH, hasChildren: false },
  ]),
  children(`${BUCKET_PATH}/prefix:logs`, []),
  {
    channel: IPC.filesChooseOpen,
    response: {
      canceled: false,
      file: { path: '/tmp/report.csv', name: 'report.csv', size: 12 },
    },
  },
];

export async function openS3Browse(page: Page): Promise<void> {
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connectionRow(page).locator('.status-dot')).toHaveAttribute(
    'data-status',
    'connected',
    { timeout: 10_000 },
  );
  await expandRow(page, '');
  await (await findRow(page, BUCKET_PATH)).dblclick();
  await expect(page.locator('[data-testid="browse-view"]')).toHaveAttribute(
    'data-level',
    BUCKET_PATH,
  );
  await expect(page.locator('[data-testid="browse-row"]').first()).toBeVisible();
}
