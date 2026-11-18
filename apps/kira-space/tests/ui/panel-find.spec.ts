import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';
import { openReviewListing } from './support/gitUiPortFixtures';
import { IPC } from './support/ipcChannels';
import { withMemory } from './support/memoryNav';
import { emitWailsEvent } from './support/mockRuntime';
import { switchModule } from './support/moduleRoundTrip';
import { expectPanelFind, pressFind } from './support/panelFind';
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

const GIT_CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: { paths: ['a.ts', 'b.ts', 'c.ts'], status: {}, truncated: false },
  },
  { channel: IPC.codeWorkspaceStartSearch, response: { searchId: 'search-1' } },
];

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

const memory = {
  id: 'm1',
  lineageId: 'm1',
  version: 1,
  fact: 'Fact number 1.',
  kind: 'fact',
  reason: 'Added for the find spec.',
  keywords: [],
  origin: 'user',
  sourceTag: null,
  scope: 'personal',
  project: null,
  sessionId: null,
  status: 'current',
  historical: false,
  supersedesId: null,
  supersededById: null,
  createdAt: '2026-03-01T10:00:00.000Z',
  supersededAt: null,
  versions: 1,
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

test('Memory: Cmd+F from a result row refocuses the always-shown search', async ({ relaunch }) => {
  const { window: page } = await relaunch({
    control: withMemory([{ channel: IPC.memoryRecent, response: [memory] }]),
  });
  await switchModule(page, 'memory');
  await expectPanelFind(page, page.locator(t('memory-row-m1')), page.locator(t('memory-search')));
});

async function openFiles(page: Page): Promise<void> {
  await page.locator(`${t('repo-row')}[data-repo-id="${REPO.id}"]`).click();
  await page.locator(t('git-panel-tab-files')).click();
}

test('Git Repos tab: Cmd+F focuses the repo filter', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await expectPanelFind(
    page,
    page.locator(`${t('repo-row')}[data-repo-id="${REPO.id}"]`),
    page.locator(t('tree-search')),
  );
});

test('Git Files tab: Cmd+F focuses the file filter', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await openFiles(page);
  await expectPanelFind(
    page,
    page.locator(t('repo-tree-row')).first(),
    page.locator(t('tree-search')),
  );
});

test('Git Files > Search: innermost panel wins, outer filter stays hidden', async ({
  relaunch,
}) => {
  const { window: page } = await relaunch({ control: GIT_CONTROL });
  await openFiles(page);
  await page.locator(t('repo-view-search')).click();
  const query = page.locator(t('repo-search-query'));
  await query.fill('const');
  await query.press('Enter');
  await emitWailsEvent(page, IPC.codeSearch, {
    searchId: 'search-1',
    seq: 0,
    files: [
      {
        path: 'a.ts',
        matches: [
          {
            line: 1,
            column: 8,
            endColumn: 13,
            preview: 'export const a = 1;',
            previewMatchStart: 7,
            previewMatchEnd: 12,
            truncatedStart: false,
            truncatedEnd: false,
          },
        ],
        truncated: false,
      },
    ],
    done: true,
  });
  const row = page.locator(t('repo-search-file-row')).first();
  await expect(row).toBeVisible();
  await row.focus();
  await pressFind(page);
  await expect(query).toBeFocused();
  await expect(page.locator(t('tree-search'))).toHaveCount(0);
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
