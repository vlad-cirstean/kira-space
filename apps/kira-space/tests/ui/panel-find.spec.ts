import { expect, test } from './fixtures';
import { openReviewListing } from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import { switchModule } from './support/moduleRoundTrip';
import { expectPanelFind } from './support/panelFind';
import type { ControlSnapshot } from './support/types';

// P665: Cmd/Ctrl+F with focus in a left panel reveals and focuses that panel's search. UI-only: no
// backend half. The menu Find event and the palette entry route to the same panel search.

const t = (id: string) => `[data-testid="${id}"]`;

const REPO = {
  id: 'repo-find-1',
  name: 'find-repo',
  root: '/tmp/find-repo',
  repoId: '/tmp/find-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};

const GIT_CONTROL: ControlSnapshot[] = [{ channel: IPC.codeWorkspaceListRepos, response: [REPO] }];

const script = {
  id: 'script-1',
  kind: 'script',
  params: [],
  smart: null,
  name: 'Script one',
  command: 'true',
  workingDir: '/tmp/demo',
  dirMode: 'fixed',
  useAdeDir: false,
  color: 'green',
  collection: '',
  schedule: null,
  savedSecrets: [],
  sortOrder: 1,
  createdAt: '2026-01-01T00:00:00.000Z',
  updatedAt: '2026-01-01T00:00:00.000Z',
};

test('Automations: Cmd+F reveals and focuses the search', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: [
      {
        channel: IPC.customScriptsList,
        response: { collections: [], scripts: [script] },
      },
    ],
  });
  await switchModule(page, 'automations');
  await expect(page.locator(t('tree-search'))).toHaveCount(0);
  await expectPanelFind(page, page.locator(t('toggle-search')), page.locator(t('tree-search')));
});

test('Git Repos tab: Cmd+F focuses the repo filter', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await expectPanelFind(
    page,
    page.locator(`${t('repo-row')}[data-repo-id="${REPO.id}"]`),
    page.locator(t('tree-search')),
  );
});

test('Git Review: Cmd+F reveals and focuses the file filter', async ({ relaunch }) => {
  const page = await openReviewListing(relaunch);
  const filter = page.locator(t('review-file-filter'));
  await expectPanelFind(page, page.locator('[data-testid^="review-row-"]').first(), filter);
});

test('menu Find event focuses the panel search holding focus', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await page.locator(t('toggle-search')).focus();
  await emitWailsEvent(page, IPC.viewFind, undefined);
  await expect(page.locator(t('tree-search'))).toBeFocused();
});

test('palette Search panel command focuses the active panel search', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await emitWailsEvent(page, IPC.commandPalette, undefined);
  await expect(page.locator(t('command-palette-input'))).toBeVisible();
  await page.locator('[data-command-id="panel.find"]').click();
  await expect(page.locator(t('command-palette-input'))).toHaveCount(0);
  await expect(page.locator(t('tree-search'))).toBeFocused();
});
