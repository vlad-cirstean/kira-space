import { CAPTURE_LAUNCH, CAPTURE_NOW } from '@workbench/testing/screenshots/capture';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { openHttpMode } from '../../ui/studio/support/apiMode';
import { openVariablesTab } from '../../ui/studio/support/apiRows';
import { IPC } from '../../ui/support/ipcChannels';
import { shoot } from '../support/shoot';

// P734: the Api Variables tab, one grid of names by files. Run by `bun run screenshots`.

const uid = (n: number) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const COLLECTION = uid(1);
const STAGING = uid(2);
const PRODUCTION = uid(3);
const LOCAL = uid(4);
const NOW = '2026-09-20T09:40:00Z';

const env = (id: string, name: string, order: number, color: string, isActive: boolean) => ({
  id,
  name,
  sortOrder: order,
  isActive,
  color,
  description: '',
  file: `${name.toLowerCase()}.environment.yaml`,
});

function row(
  scope: 'collection' | 'environment',
  ownerId: string,
  name: string,
  order: number,
  over: Record<string, unknown> = {},
) {
  return {
    id: `${ownerId}-${name}`,
    scope,
    ownerId,
    name,
    value: '',
    isSecret: false,
    secretSet: false,
    sortOrder: order,
    description: '',
    ...over,
  };
}

const list = (ownerId: string, scope: string, rows: unknown[]): ControlSnapshot => ({
  channel: IPC.variablesList,
  args: { scope, ownerId },
  response: rows,
});

test('api variables tab', async ({ relaunch, consoleErrors }) => {
  const { window: page } = await relaunch({
    ...CAPTURE_LAUNCH,
    viewport: { width: 1200, height: 640 },
    control: [
      { channel: IPC.apiConfigView, response: { files: [], secrets: [] } },
      { channel: IPC.apiConfigAgentAccess, response: [] },
      { channel: IPC.repoConfigList, response: { dir: '~/.kira-space/repos', files: [] } },
      { channel: IPC.adeTaskRepos, response: { repos: [], folders: [] } },
      { channel: IPC.adeTaskSessions, response: { sessions: [] } },
      {
        channel: IPC.collectionsList,
        response: {
          collections: [
            {
              id: COLLECTION,
              name: 'Orders API',
              file: 'orders-api.collection.yaml',
              sortOrder: 0,
              createdAt: NOW,
              updatedAt: NOW,
            },
          ],
          items: [],
        },
      },
      {
        channel: IPC.variablesListEnvironments,
        response: [
          env(STAGING, 'Staging', 0, 'green', true),
          env(PRODUCTION, 'Production', 1, 'red', false),
          env(LOCAL, 'Local', 2, 'blue', false),
        ],
      },
      list(COLLECTION, 'collection', [
        row('collection', COLLECTION, 'page_size', 0, { value: '50' }),
      ]),
      list(STAGING, 'environment', [
        row('environment', STAGING, 'base_url', 0, { value: 'https://api.acme.example' }),
        row('environment', STAGING, 'token', 1, { isSecret: true, secretSet: true }),
        row('environment', STAGING, 'region', 2, { value: 'eu-west' }),
      ]),
      list(PRODUCTION, 'environment', [
        row('environment', PRODUCTION, 'base_url', 0, { value: 'https://api.acme.com' }),
        row('environment', PRODUCTION, 'token', 1, { isSecret: true }),
      ]),
      list(LOCAL, 'environment', [
        row('environment', LOCAL, 'base_url', 0, { value: 'http://localhost:8080' }),
        row('environment', LOCAL, 'token', 1, { isSecret: true, secretSet: true }),
      ]),
    ],
  });
  await openHttpMode(page);
  await openVariablesTab(page);
  await expect(page.locator('[data-testid="variables-matrix"]')).toBeVisible();
  await page.clock.setFixedTime(CAPTURE_NOW + 1000);
  await shoot(page, 'studio-api-variables', consoleErrors);
});
