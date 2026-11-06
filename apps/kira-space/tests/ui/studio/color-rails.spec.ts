import type { ConnectionSummary } from '@shared/domain/connection';
import { expect, test } from './fixtures';
import { modeTab } from './support/apiMode';
import { IPC } from './support/ipcChannels';

// P226: one colour mark across modules. The Studio connection tree rail and the script
// rail share one class string and geometry; apps/kira-space/tests/ui/color-rails.spec.ts pins
// the same literal for the Space rails.

const RAIL_CLASS = 'absolute inset-y-0 left-0 w-1 bg-conn-cyan';
const EMPTY_RAIL_CLASS = 'absolute inset-y-0 left-0 w-1';

const CONNECTION: ConnectionSummary = {
  file: 'fixture.yaml',
  id: 'conn-rail',
  name: 'Rail Conn',
  kind: 'postgres',
  color: 'cyan',
  mode: 'fields',
  readOnly: false,
  host: '127.0.0.1',
  port: 5432,
  database: 'testdb',
  username: 'testuser',
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
  mcpReview: { read: 'inherit', write: 'inherit', ddl: 'inherit', masked: 'inherit' },
  mcpMaskedMode: 'inherit',
  mcpStatementTimeoutSec: 10,
  mcpTables: null,
  maskReviewOn: false,
  sortOrder: 0,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

function script(id: string, color: string, sortOrder: number) {
  return {
    id,
    name: `Script ${id}`,
    command: 'npm run dev',
    workingDir: '/tmp/demo-repo',
    color,
    collection: '',
    sortOrder,
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  };
}

test('tree and script rails share one class, geometry and paint', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      { channel: IPC.connectionsList, response: [CONNECTION] },
      {
        channel: IPC.customScriptsList,
        response: { collections: [], scripts: [script('a', 'cyan', 0), script('b', 'none', 1)] },
      },
    ],
  });

  const treeRow = page
    .locator('[data-testid="tree-row"][data-kind="connection"]')
    .filter({ hasText: CONNECTION.name });
  const treeRail = treeRow.locator('[data-testid="tree-rail"]');
  await expect(treeRail).toHaveAttribute('class', RAIL_CLASS);
  const treeBox = await treeRail.boundingBox();
  const treeRowBox = await treeRow.boundingBox();
  const treePaint = await treeRail.evaluate((el) => getComputedStyle(el).backgroundColor);

  await modeTab(page, 'automations').click();
  const row = page.locator('[data-testid="script-a"]');
  const rail = row.locator('[data-testid="script-rail"]');
  await expect(rail).toHaveAttribute('class', RAIL_CLASS);
  const box = await rail.boundingBox();
  const rowBox = await row.boundingBox();
  const paint = await rail.evaluate((el) => getComputedStyle(el).backgroundColor);

  for (const [b, r] of [
    [treeBox, treeRowBox],
    [box, rowBox],
  ] as const) {
    expect(b?.width).toBe(4);
    expect(b?.x).toBe(r?.x);
    expect(b?.height).toBe(r?.height);
  }
  expect(paint).toBe(treePaint);

  const plain = page.locator('[data-testid="script-b"]');
  await expect(plain.locator('[data-testid="script-rail"]')).toHaveAttribute(
    'class',
    EMPTY_RAIL_CLASS,
  );
  await expect(plain.locator('.codicon-play')).toBeVisible();
});
