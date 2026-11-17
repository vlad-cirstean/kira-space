import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { apiTest, expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { emitWailsEvent } from '../support/mockRuntime';
import { expectPanelFind } from '../support/panelFind';
import { modeTab } from './support/apiMode';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import { openDocker, setupDocker } from './support/dockerFixture';
import { gridScroller } from './support/grid';
import { openKubernetes, setupKubernetes } from './support/kubernetesFixture';
import {
  BIG_ROWS_PATH,
  bigRowsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow } from './support/tree';

// P665: Cmd/Ctrl+F in the Connections, API, Docker and Kubernetes panels. UI-only, no backend half.

const t = (id: string) => `[data-testid="${id}"]`;
const CONNECTION_ID = 'conn-panel-find';
const FIXTURE = bigRowsFixture(CONNECTION_ID);

const NOW = '2026-01-01T00:00:00.000Z';
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Find DB', 'green'),
    response: postgresConnectionSummary(CONNECTION_ID, 'Find DB', 'green'),
  },
  ...FIXTURE.control,
];
const PORT: PortSnapshot[] = [...FIXTURE.port];

test('Studio Connections: Cmd+F reveals and focuses the search', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Find DB', 'green');
  await expect(page.locator(t('tree-search'))).toHaveCount(0);
  await expectPanelFind(page, page.locator(t('toggle-search')), page.locator(t('tree-search')));
});

apiTest('API Collections: Cmd+F reveals and focuses the search', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.collectionsList,
        response: {
          collections: [
            {
              id: 'col-1',
              name: 'Find API',
              file: 'find.yaml',
              sortOrder: 0,
              createdAt: NOW,
              updatedAt: NOW,
            },
          ],
          items: [],
        },
      },
    ],
  });
  await modeTab(page, 'api').click();
  await expect(page.locator(t('toggle-search'))).toBeVisible();
  await expectPanelFind(page, page.locator(t('toggle-search')), page.locator(t('tree-search')));
});

test('Docker: Cmd+F reveals and focuses the search', async ({ relaunch }) => {
  const { page } = await setupDocker(relaunch);
  await openDocker(page);
  await expectPanelFind(page, page.locator(t('toggle-search')), page.locator(t('tree-search')));
});

test('Kubernetes: Cmd+F reveals and focuses the search', async ({ relaunch }) => {
  const { page } = await setupKubernetes(relaunch);
  await openKubernetes(page);
  await expectPanelFind(page, page.locator(t('toggle-search')), page.locator(t('tree-search')));
});

test('menu Find: a focused grid keeps its own find, a focused panel takes the panel search', async ({
  relaunch,
}) => {
  test.setTimeout(90_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await connectAndExpand(page, 'Find DB', 'green');
  await (await findRow(page, BIG_ROWS_PATH)).dblclick();
  const gridFind = page.locator(t('toolbar-search'));
  await expect(gridFind).toBeVisible();
  await expect(gridFind).toHaveAttribute('aria-pressed', 'false');

  // Focus in the grid, not the tree row the double click left it on.
  await gridScroller(page).click();
  await emitWailsEvent(page, IPC.viewFind, undefined);
  await expect(gridFind).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator(t('tree-search'))).toHaveCount(0);

  await page.locator(t('toggle-search')).focus();
  await emitWailsEvent(page, IPC.viewFind, undefined);
  await expect(page.locator(t('tree-search'))).toBeFocused();
  await expect(gridFind).toHaveAttribute('aria-pressed', 'true');
});
