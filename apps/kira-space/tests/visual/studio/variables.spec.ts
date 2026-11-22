import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../../ui/fixtures';
import { openHttpMode } from '../../ui/studio/support/apiMode';
import { openVariablesTab } from '../../ui/studio/support/apiRows';
import { IPC } from '../../ui/support/ipcChannels';

// The Variables tab at rest (grid, narrow single column) and the request's variables popover.

const NOW = '2026-01-01T00:00:00Z';
const COL = 'col-1';
const STAGING = 'env-staging';
const LOCAL = 'env-local';

const row = (scope: string, ownerId: string, name: string, over: Record<string, unknown> = {}) => ({
  id: `${ownerId}-${name}`,
  scope,
  ownerId,
  name,
  value: '',
  isSecret: false,
  secretSet: false,
  sortOrder: 0,
  description: '',
  ...over,
});
const env = (id: string, name: string, order: number, isActive: boolean, color: string) => ({
  id,
  name,
  sortOrder: order,
  isActive,
  color,
  description: '',
  file: `${name.toLowerCase()}.environment.yaml`,
});

const ITEM = {
  id: 'item-1',
  collectionId: COL,
  parentId: null,
  kind: 'request',
  name: 'List orders',
  sortOrder: 0,
  method: 'GET',
  url: 'https://{{base_url}}/orders?page={{page_size}}&key={{missing_key}}',
  createdAt: NOW,
  updatedAt: NOW,
};

const CONTROL: ControlSnapshot[] = [
  {
    channel: IPC.collectionsList,
    response: {
      collections: [
        {
          id: COL,
          name: 'Orders API',
          file: 'orders-api.collection.yaml',
          sortOrder: 0,
          createdAt: NOW,
          updatedAt: NOW,
        },
      ],
      items: [ITEM],
    },
  },
  {
    channel: IPC.collectionsGetRequest,
    response: {
      method: 'GET',
      url: ITEM.url,
      headers: [],
      bodyMode: 'none',
      body: '',
      code: '',
      codeLanguage: 'json',
      urlEncoded: [],
      formData: [],
      binaryFile: null,
    },
  },
  {
    channel: IPC.variablesListEnvironments,
    response: [env(STAGING, 'Staging', 0, true, 'green'), env(LOCAL, 'Local', 1, false, 'blue')],
  },
  {
    channel: IPC.variablesList,
    args: { scope: 'collection', ownerId: COL },
    response: [
      row('collection', COL, 'base_url', { value: 'api.example.com', description: 'API root' }),
      row('collection', COL, 'page_size', { value: '20' }),
    ],
  },
  {
    channel: IPC.variablesList,
    args: { scope: 'environment', ownerId: STAGING },
    response: [
      row('environment', STAGING, 'base_url', { value: 'staging.example.com' }),
      row('environment', STAGING, 'token', { isSecret: true, secretSet: true }),
    ],
  },
  {
    channel: IPC.variablesList,
    args: { scope: 'environment', ownerId: LOCAL },
    response: [
      row('environment', LOCAL, 'base_url', { value: 'localhost:8080' }),
      row('environment', LOCAL, 'token', { isSecret: true }),
    ],
  },
];

test('Variables tab at rest', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await openVariablesTab(page);
  await expect(page.locator('[data-testid="matrix-row-token"]')).toBeVisible();
  await expect(page.locator('[data-testid="variables-matrix"]')).toHaveScreenshot(
    'variables-matrix.png',
  );
});

test('Variables tab narrow shows one column', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await openVariablesTab(page);
  await page.setViewportSize({ width: 900, height: 600 });
  await expect(page.locator('[data-testid="matrix-env-picker"]')).toBeVisible();
  await expect(page.locator('[data-testid="variables-matrix"]')).toHaveScreenshot(
    'variables-matrix-narrow.png',
  );
});

test('Variables tab with 20 environments', async ({ relaunch }) => {
  const envs = Array.from({ length: 20 }, (_, i) =>
    env(
      `env-${i}`,
      `Environment ${String(i + 1).padStart(2, '0')}`,
      i,
      i === 0,
      i % 2 ? 'blue' : 'green',
    ),
  );
  const { window: page } = await relaunch({
    control: [
      ...CONTROL.filter(
        (c) => c.channel !== IPC.variablesListEnvironments && c.channel !== IPC.variablesList,
      ),
      { channel: IPC.variablesListEnvironments, response: envs },
      {
        channel: IPC.variablesList,
        args: { scope: 'collection', ownerId: COL },
        response: [
          row('collection', COL, 'base_url', { value: 'api.example.com' }),
          row('collection', COL, 'page_size', { value: '20' }),
        ],
      },
      ...envs.map((e, i) => ({
        channel: IPC.variablesList,
        args: { scope: 'environment', ownerId: e.id },
        response: [
          row('environment', e.id, 'base_url', { value: `env${i + 1}.example.com` }),
          row('environment', e.id, 'token', { isSecret: true, secretSet: i % 3 !== 0 }),
        ],
      })),
    ],
  });
  await openHttpMode(page);
  await openVariablesTab(page);
  await expect(page.locator('[data-testid="matrix-row-token"]')).toBeVisible();
  // Scrolled to the middle, so the pinned Name column sits over scrolled environment columns.
  await page.locator('[data-testid="variables-matrix-scroll"]').evaluate((el) => {
    el.scrollLeft = (el.scrollWidth - el.clientWidth) / 2;
  });
  await expect(page.locator('[data-testid="variables-matrix"]')).toHaveScreenshot(
    'variables-matrix-20-envs.png',
  );
});

test('request variables popover', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL });
  await openHttpMode(page);
  await page
    .locator(`[data-testid="collection-row"][data-id="${COL}"]`)
    .locator('[data-testid="tree-twisty"]')
    .click();
  await page.locator('[data-testid="collection-row"][data-id="item-1"]').dblclick();
  await page.click('[data-testid="http-unresolved-chip"]');
  await expect(page.locator('[data-testid="request-var-missing_key"]')).toBeVisible();
  await expect(page.locator('[data-testid="request-variables"]')).toHaveScreenshot(
    'request-variables.png',
  );
});
