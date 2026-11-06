import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { emitPrompts, routed } from '@workbench/testing/ui/prompts';
import {
  controlSnapshots as redisControl,
  portSnapshots as redisPorts,
} from '../ipc/redis/redis.fixture';
import { expect, studioTest, test } from './fixtures';
import { modeTab } from './studio/support/apiMode';
import { collectionRow } from './studio/support/apiRows';
import { primaryChord } from './studio/support/chord';
import { installClipboardSpy, lastClipboardWrite } from './studio/support/clipboard';
import {
  connectAndExpand,
  connectionCreateArgs,
  createConnection,
  openConsoleFromMenu,
} from './studio/support/connect';
import { acceptConfirm, cancelConfirm } from './studio/support/dialogs';
import { gridCell } from './studio/support/grid';
import {
  mongoConnectionSummary,
  WIDGETS_PATH,
  widgetsFixture,
} from './studio/support/mongoFixture';
import {
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './studio/support/postgresFixture';
import { openRedisBrowse, REDIS_DB0 } from './studio/support/redisFixture';
import { expandRow, findRow, openBrowseLeaf } from './studio/support/tree';
import { FIXED_NOW, openPlan, openWorkflows } from './support/adeV2';
import { gitStreamParams } from './support/gitStreamMock';
import { BASE_RESULTS, PORT_CONTROL, PORT_REPO, SHA_A, SHA_B } from './support/gitUiPortFixtures';
import { calls } from './support/ipcCalls';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { ControlSnapshot } from './support/types';

// P267 Stream B: unified primitives and behaviours in the ADE, the Repos panel and the status bar.

const t = (id: string) => `[data-testid="${id}"]`;

test('All sessions filter is the shared segmented secondary tabs', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page.locator(t('ade-tab-needs')).click();
  await page.locator(t('ade-needs-all')).click();
  const tabs = page
    .locator('[data-slot="secondary-tabs"]')
    .filter({ has: page.locator(t('ade-all-running')) });
  await expect(tabs).toBeVisible();
  await expect(tabs.locator(t('ade-all-stopped'))).toBeVisible();
});

test('the notes toolbar toggles expose aria-pressed', async ({ relaunch }) => {
  const { window: page } = await openPlan(relaunch);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_notif"] [data-testid="ade-card-head"]')
    .click();
  await page.locator(t('ade-panel-tab-details')).click();
  await page.locator(t('ade-note-add')).click();
  await page.locator(`${t('ade-note-composer')} .ProseMirror`).click();
  const bold = page.locator(t('ade-notes-toolbar-btn')).first();
  await expect(bold).toHaveAttribute('aria-pressed', 'false');
  await bold.click();
  await expect(bold).toHaveAttribute('aria-pressed', 'true');
});

interface SavedWorkflow {
  workflow: { steps: { id: string }[] };
}

// tree.delete is Cmd+Backspace on a Mac userAgent, which Playwright's WebKit always reports.
test('workflow graph: Delete removes the selected step, but edits text inside a field', async ({
  relaunch,
}) => {
  const { window: page, control } = await openWorkflows(relaunch);
  const step = page.locator(`${t('ade-wf-node')}[data-step-id="plan"]`);
  await step.click();

  await page.locator(t('ade-wf-step-advanced')).click();
  await page.locator(t('ade-wf-step-tools')).fill('Read, Grep');
  await page.locator(t('ade-wf-step-tools')).press('Meta+Backspace');
  await expect(page.locator(t('confirm-dialog-confirm'))).toHaveCount(0);
  await expect(step).toBeVisible();

  await step.click();
  await step.press('Meta+Backspace');
  await page.locator(t('confirm-dialog-confirm')).click();
  await expect(step).toHaveCount(0);
  await page.locator(t('ade-wf-save')).click();
  await expect.poll(() => calls(control, IPC.adeTaskSaveWorkflow)).toHaveLength(1);
  const saved = calls(control, IPC.adeTaskSaveWorkflow)[0]?.args as SavedWorkflow;
  expect(saved.workflow.steps.map((x) => x.id)).not.toContain('plan');
});

test('the review code event opens the review; a window keydown does not', async ({ relaunch }) => {
  const OPEN: ControlSnapshot = { channel: IPC.adeTaskOpenReviewWindow, response: true };
  const { window: page, control } = await openPlan(relaunch, [OPEN]);
  await page
    .locator('[data-testid="ade-card"][data-task-id="T_deps"] [data-testid="ade-card-head"]')
    .click();
  await page.keyboard.press('Control+Shift+R');
  await page.keyboard.press('Meta+Shift+R');
  expect(calls(control, IPC.adeTaskOpenReviewWindow)).toHaveLength(0);

  await emitWailsEvent(page, IPC.reviewCode, null);
  await expect.poll(() => calls(control, IPC.adeTaskOpenReviewWindow)).toHaveLength(1);
});

test('the Claude usage status item is one control-sm row high', async ({ relaunch }) => {
  const HOUR = 3_600_000;
  const snap = {
    state: 'ok',
    source: 'session',
    fiveHour: { usedPercent: 23, resetsAt: FIXED_NOW + 2 * HOUR },
    sevenDay: { usedPercent: 41, resetsAt: FIXED_NOW + 50 * HOUR },
    updatedAt: FIXED_NOW - 5 * 60_000,
  };
  const { window: page } = await openPlan(relaunch, [
    { channel: IPC.agentUsageGet, response: snap },
  ]);
  const box = await page.locator(t('agent-usage-status')).boundingBox();
  expect(Math.round(box?.height ?? 0)).toBe(18);
});

test('an outside click keeps a dirty git credential dialog open', async ({ relaunch }) => {
  const { window } = await relaunch({
    control: [{ channel: IPC.gitCredentialProvide, response: true }],
  });
  const prompt = {
    requestId: 'req-1',
    source: 'ADE board',
    repoLabel: 'demo-repo',
    prompt: "Password for 'https://example.com':",
    masked: true,
  };
  await emitWailsEvent(window, IPC.gitCredential, [prompt]);
  await emitPrompts(window, [routed('git-credential', 'req-1')]);
  const dialog = window.locator(t('git-credential-dialog'));
  await expect(dialog).toBeVisible();

  await window.fill(t('git-credential-input'), 'hunter2');
  await window.mouse.click(2, 2);
  await expect(dialog).toBeVisible();
});

// Repos panel fixtures, as list-keyboard.spec.ts.
const REPO = {
  id: 'repo-1',
  name: 'demo-repo',
  root: '/tmp/demo-repo',
  repoId: '/tmp/demo-repo',
  sortOrder: 1,
  color: 'none',
  createdAt: '2026-01-01T00:00:00.000Z',
};
const README = [
  '# Top',
  '',
  '[jump](#bottom)',
  '',
  ...Array.from({ length: 80 }, (_, i) => `Paragraph ${i}\n`),
  '## Bottom',
  '',
  'The end.',
  '',
  // Trailing content lets the heading reach the pane top.
  ...Array.from({ length: 80 }, (_, i) => `Tail ${i}\n`),
].join('\n');

function readFile(path: string, text: string, language: string): ControlSnapshot {
  return {
    channel: IPC.codeWorkspaceReadFile,
    args: { id: REPO.id, path },
    response: {
      kind: 'found',
      text,
      bytes: text.length,
      limitBytes: 8 * 1024 * 1024,
      language,
    },
  };
}

const REPO_CONTROL: ControlSnapshot[] = [
  { channel: IPC.codeWorkspaceListRepos, response: [REPO] },
  {
    channel: IPC.codeWorkspaceListFiles,
    args: { id: REPO.id },
    response: { paths: ['README.md', 'dir/b.ts'], status: {}, truncated: false },
  },
  readFile('README.md', README, 'markdown'),
  readFile('dir/b.ts', 'export const b = 1;\n', 'typescript'),
];

async function openFiles(page: Page): Promise<void> {
  await page.locator('[data-testid="repo-row"][data-repo-id="repo-1"]').click();
  await page.locator(t('git-panel-tab-files')).click();
}

test('F2 on a repo row starts the inline rename', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  const row = page.locator('[data-testid="repo-row"][data-repo-id="repo-1"]');
  await row.focus();
  await page.keyboard.press('F2');
  const input = page.locator(t('repo-rename-input'));
  await expect(input).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(input).toHaveCount(0);
});

test('file tree folder expansion survives a reload', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  await openFiles(page);
  const row = (path: string) => page.locator(`[data-testid="repo-tree-row"][data-path="${path}"]`);
  await row('dir').click();
  if (!(await row('dir/b.ts').isVisible())) await page.keyboard.press('ArrowRight');
  await expect(row('dir/b.ts')).toBeVisible();
  // Persisting the expansion is debounced and has no observable end before the reload.
  await page.waitForTimeout(500);

  await page.reload();
  await openFiles(page);
  await expect(row('dir/b.ts')).toBeVisible();
});

test('a markdown heading link scrolls its target to the pane top', async ({ relaunch }) => {
  const { window: page } = await relaunch({ control: REPO_CONTROL });
  await openFiles(page);
  await page.locator('[data-testid="repo-tree-row"][data-path="README.md"]').click();
  await page.locator(t('repo-file-view-reading')).click();
  const pane = page.locator(t('repo-file-markdown'));
  await expect(pane).toBeVisible();
  const heading = pane.locator('#bottom');
  await pane.locator('a[href="#bottom"]').click();
  await expect
    .poll(async () => {
      const top = (await pane.boundingBox())?.y ?? 0;
      const at = (await heading.boundingBox())?.y ?? Number.POSITIVE_INFINITY;
      return Math.abs(at - top);
    })
    .toBeLessThan(48);
});

test('a review diff that fails to load shows an Alert with Retry, and Retry reloads', async ({
  relaunch,
}) => {
  const branch = 'feature/example';
  const { window: page } = await relaunch({
    control: [
      ...PORT_CONTROL,
      {
        channel: IPC.tabsList,
        response: [
          {
            id: 'restored-review-diff',
            kind: 'repo-diff',
            connectionId: null,
            path: 'a.ts',
            order: 0,
            active: true,
            workspaceId: PORT_REPO.id,
            state: {
              left: SHA_B,
              right: SHA_A,
              leftLabel: 'main',
              rightLabel: branch,
              review: { branch, branchTip: SHA_A, leftLabel: 'main' },
            },
          },
        ],
      },
    ],
    gitStream: {
      repoId: PORT_REPO.repoId,
      extraResults: {
        ...BASE_RESULTS,
        'file.read': { kind: 'found', content: 'one\ntwo\nthree\n' },
        'review.snapshot': { kind: 'absent', text: null, reviewedAtSha: null },
        'review.resolveBase': {
          __sequence: [
            { __error: { code: 'E_INTERNAL', message: 'index.lock exists' } },
            {
              branch,
              base: 'main',
              reason: 'defaultBranch',
              range: { kind: 'ready', commitCount: 1 },
              candidates: [],
            },
          ],
        },
        'review.fileDiff': {
          path: 'a.ts',
          deltaSource: 'noSnapshot',
          body: { kind: 'text', hunks: [] },
          reviewedRanges: [],
          lineCount: 3,
          reviewedAtSha: null,
        },
        'review.comment.list': { at: SHA_A, comments: [] },
      },
      graphStreamChunks: [],
    },
  });
  const alert = page.locator('[data-slot="alert"]');
  await expect(alert).toBeVisible();
  const retry = alert.locator('button', { hasText: 'Retry' });
  await expect(retry).toBeVisible();
  expect(await gitStreamParams(page, 'review.resolveBase')).toHaveLength(1);

  await retry.click();
  await expect(alert).toHaveCount(0);
  expect((await gitStreamParams(page, 'review.resolveBase')).length).toBeGreaterThanOrEqual(2);
});

studioTest.describe('databases module', () => {
  // P267 Stream A: UI leftover unification. Written, not run in the authoring session.

  const MENU = '[data-testid="context-menu"]';
  const CONFIRM = '[data-testid="confirm-dialog"]';
  const NOW = '2026-01-01T00:00:00.000Z';

  // --- dirty guards -----------------------------------------------------------------------------

  const GRID_ID = 'conn-ul-grid';
  const GRID_FIXTURE = compositePkConnectAndOpen(GRID_ID);
  const GRID_CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: connectionCreateArgs('Leftover DB', 'green'),
      response: postgresConnectionSummary(GRID_ID, 'Leftover DB', 'green'),
    },
    ...GRID_FIXTURE.control,
  ];

  async function openGridWithStagedEdit(page: Page): Promise<void> {
    await connectAndExpand(page, 'Leftover DB', 'green');
    await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
    await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
    await gridCell(page, 0, 'name').dblclick();
    await page.locator('[data-testid="grid-cell-input"]').fill('staged value');
    await page.keyboard.press('Enter');
    await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  }

  studioTest(
    'closing a data tab with a staged edit asks to discard; Cancel keeps the tab and the edit',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: GRID_CONTROL, stream: GRID_FIXTURE.port });
      await openGridWithStagedEdit(page);

      await page
        .locator('[data-testid="tab"]')
        .first()
        .locator('[data-testid="tab-close"]')
        .click();
      await expect(page.locator(CONFIRM)).toBeVisible();
      await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
      await cancelConfirm(page);
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
      await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);

      await page
        .locator('[data-testid="tab"]')
        .first()
        .locator('[data-testid="tab-close"]')
        .click();
      await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
      await acceptConfirm(page);
      await expect(page.locator('[data-testid="tab"]')).toHaveCount(0);
    },
  );

  studioTest('Close others over a dirty tab asks once', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: GRID_CONTROL, stream: GRID_FIXTURE.port });
    await openGridWithStagedEdit(page);
    await openConsoleFromMenu(page, '');
    await expect(page.locator('[data-testid="tab"]')).toHaveCount(2);

    await page.locator('[data-testid="tab"]').nth(1).click({ button: 'right' });
    await page.click('[data-testid="menu-item-close-others"]');
    await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
    await acceptConfirm(page);
    await expect(page.locator(CONFIRM)).toHaveCount(0);
    await expect(page.locator('[data-testid="tab"]')).toHaveCount(1);
  });

  // --- connection dialog ------------------------------------------------------------------------

  async function openNewPostgresDialog(page: Page): Promise<void> {
    await page.click('[data-testid="add-connection"]');
    await expect(page.locator('[data-testid="new-connection-dialog"]')).toBeVisible();
    await page.click('[data-testid="connection-kind-postgres"]');
  }

  studioTest(
    'new connection dialog with a typed name: outside click keeps it open; Esc and X ask to discard; Cancel closes without asking',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({
        control: [{ channel: IPC.connectionsList, response: [] }],
      });
      const dialog = page.locator('[data-testid="new-connection-dialog"]');
      await openNewPostgresDialog(page);
      await page.fill('[data-testid="new-connection-name"]', 'Draft DB');

      await page.mouse.click(4, 4);
      await expect(dialog).toBeVisible();
      await expect(page.locator(CONFIRM)).toHaveCount(0);

      await page.keyboard.press('Escape');
      await expect(page.locator(CONFIRM)).toBeVisible();
      await expect(page.locator('[data-testid="confirm-dialog-confirm"]')).toHaveText('Discard');
      await cancelConfirm(page);
      await expect(dialog).toBeVisible();

      await page.click('[data-testid="new-connection-close"]');
      await expect(page.locator(CONFIRM)).toBeVisible();
      await cancelConfirm(page);
      await expect(dialog).toBeVisible();

      await page.click('[data-testid="new-connection-cancel"]');
      await expect(dialog).toHaveCount(0);
      await expect(page.locator(CONFIRM)).toHaveCount(0);
    },
  );

  studioTest(
    'new connection dialog: Create stays off until the connection has a name',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({
        control: [{ channel: IPC.connectionsList, response: [] }],
      });
      await openNewPostgresDialog(page);

      const create = page.locator('[data-testid="new-connection-create"]');
      await expect(create).toBeDisabled();
      await page.fill('[data-testid="new-connection-name"]', 'Named DB');
      await expect(create).toBeEnabled();
      await page.fill('[data-testid="new-connection-name"]', '   ');
      await expect(create).toBeDisabled();
    },
  );

  studioTest(
    'creating a connection: Create is aria-busy while the create call is pending',
    async ({ relaunch }) => {
      const { window: page, control } = await relaunch({
        control: [
          { channel: IPC.connectionsList, response: [] },
          {
            channel: IPC.connectionsCreate,
            args: connectionCreateArgs('Busy DB'),
            response: postgresConnectionSummary('conn-ul-busy', 'Busy DB', 'green'),
            hold: true,
          },
        ],
      });
      await openNewPostgresDialog(page);
      await page.fill('[data-testid="new-connection-name"]', 'Busy DB');

      const create = page.locator('[data-testid="new-connection-create"]');
      await create.click();
      await expect(create).toHaveAttribute('aria-busy', 'true');
      control.release(IPC.connectionsCreate);
      await expect(page.locator('[data-testid="new-connection-dialog"]')).toHaveCount(0);
    },
  );

  // --- shortcuts --------------------------------------------------------------------------------

  const COLLECTION_TREE = {
    collections: [
      { id: 'col-1', name: 'Orders API', sortOrder: 0, createdAt: NOW, updatedAt: NOW },
    ],
    items: [
      {
        id: 'item-health',
        collectionId: 'col-1',
        parentId: null,
        kind: 'request',
        name: 'Health check',
        sortOrder: 0,
        method: 'GET',
        url: 'https://api.example.com/healthz',
        createdAt: NOW,
        updatedAt: NOW,
      },
    ],
  };
  const HEALTH_REQUEST = {
    method: 'GET',
    url: 'https://api.example.com/healthz',
    headers: [],
    bodyMode: 'none',
    body: '',
    code: '',
    codeLanguage: 'json',
    urlEncoded: [],
    formData: [],
    binaryFile: null,
  };

  studioTest('Ctrl/Cmd+S in an HTTP request tab calls the save mock', async ({ relaunch }) => {
    const { window: page, control } = await relaunch({
      control: [
        { channel: IPC.collectionsList, response: COLLECTION_TREE },
        { channel: IPC.collectionsGetRequest, response: HEALTH_REQUEST },
        {
          channel: IPC.collectionsSaveRequest,
          response: COLLECTION_TREE.items[0],
        },
      ],
    });
    await modeTab(page, 'api').click();
    await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
    await collectionRow(page, 'item-health').dblclick();
    await expect(page.locator('[data-testid="http-request-view"]')).toBeVisible();

    await page.fill('[data-testid="http-url"]', 'https://api.example.com/healthz?x=1');
    await expect(page.locator('[data-testid="http-dirty"]')).toBeVisible();
    await page.locator('[data-testid="http-url"]').press(await primaryChord(page, 'S'));
    await expect(page.locator('[data-testid="http-dirty"]')).toHaveCount(0);
    expect(control.log().filter((e) => e.channel === IPC.collectionsSaveRequest)).toHaveLength(1);
  });

  const MONGO_ID = 'conn-ul-doc';
  const MONGO_FIXTURE = widgetsFixture(MONGO_ID);

  studioTest('Ctrl/Cmd+S in a document edit saves it', async ({ relaunch }) => {
    const { window: page, stream } = await relaunch({
      control: [
        { channel: IPC.connectionsList, response: [] },
        {
          channel: IPC.connectionsCreate,
          response: mongoConnectionSummary(MONGO_ID, 'Mongo UL', 'red'),
        },
        ...MONGO_FIXTURE.control,
      ],
      stream: MONGO_FIXTURE.port,
    });
    await createConnection(page, 'mongodb', 'Mongo UL');
    await expandRow(page, '');
    await expandRow(page, 'database:kira_test');
    await (await findRow(page, WIDGETS_PATH)).dblclick();
    await expect(page.locator('[data-testid="document-view"]')).toBeVisible();

    await page.locator('[data-testid="document-edit"]').first().click();
    await page.locator('.view-lines').first().click();
    await page.keyboard.type(' ');
    await page.keyboard.press(await primaryChord(page, 'S'));
    await expect
      .poll(async () => (await stream.ops()).some((op) => op.op === DATA_OP.mutate))
      .toBe(true);
  });

  // --- redis browse / key-value -----------------------------------------------------------------

  const REDIS_USER_NS = `${REDIS_DB0}/namespace:user`;
  const REDIS_USER1_NS = `${REDIS_USER_NS}/namespace:1`;
  const REDIS_BIGHASH = `${REDIS_USER1_NS}/key:user%3A1%3Abighash`;

  studioTest(
    'right-click on a KeyValue row selects it before the menu shows',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
      await openRedisBrowse(page);
      const browse = page.locator('[data-testid="browse-view"]');
      await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
      await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
      await openBrowseLeaf(page, REDIS_BIGHASH);
      const view = page.locator(`[data-testid="keyvalue-view"][data-path="${REDIS_BIGHASH}"]`);
      const rows = view.locator('[data-testid="keyvalue-row"]');
      await expect(rows.nth(2)).toBeVisible({ timeout: 15_000 });

      // Records the row's aria-selected at the instant the menu node is inserted.
      await page.evaluate(
        ({ menu }) => {
          const w = window as unknown as { __selectedAtMenu: string | null };
          w.__selectedAtMenu = null;
          new MutationObserver(() => {
            if (w.__selectedAtMenu !== null || !document.querySelector(menu)) return;
            w.__selectedAtMenu =
              document
                .querySelectorAll('[data-testid="keyvalue-row"]')[2]
                ?.getAttribute('aria-selected') ?? 'missing';
          }).observe(document.body, { childList: true, subtree: true });
        },
        { menu: MENU },
      );
      await rows.nth(2).click({ button: 'right' });
      await expect(page.locator(MENU)).toBeVisible();
      await expect(rows.nth(2)).toHaveAttribute('aria-selected', 'true');
      expect(
        await page.evaluate(
          () => (window as unknown as { __selectedAtMenu: string }).__selectedAtMenu,
        ),
      ).toBe('true');
    },
  );

  studioTest('Browse: the tree.copyName chord copies the name', async ({ relaunch }) => {
    const { window: page } = await relaunch({ control: redisControl, stream: redisPorts });
    await openRedisBrowse(page);
    const browse = page.locator('[data-testid="browse-view"]');
    await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER_NS}"]`).dblclick();
    await browse.locator(`[data-testid="browse-row"][data-path="${REDIS_USER1_NS}"]`).dblclick();
    const keyRow = browse.locator(`[data-testid="browse-row"][data-path="${REDIS_BIGHASH}"]`);
    await expect(keyRow).toBeVisible();

    await installClipboardSpy(page);
    await keyRow.focus();
    await page.keyboard.press(await primaryChord(page, 'C'));
    await expect.poll(() => lastClipboardWrite(page)).toBe('user:1:bighash');
  });

  // --- InlineState, find bar --------------------------------------------------------------------

  function httpRequestTab(state: Record<string, unknown>): Record<string, unknown> {
    return {
      id: 'tab-ul-1',
      connectionId: null,
      path: 'request',
      kind: 'http-request',
      order: 0,
      active: true,
      state: {
        method: 'GET',
        url: 'https://api.example.com/users',
        headers: [],
        bodyMode: 'none',
        body: '',
        code: '',
        codeLanguage: 'json',
        urlEncoded: [],
        formData: [],
        binaryFile: null,
        itemId: null,
        name: '',
        requestPane: 'body',
        responsePane: 'body',
        responseView: 'pretty',
        requestPaneHeight: 0,
        ...state,
      },
    };
  }

  function httpResponseOf(body: string): Record<string, unknown> {
    return {
      status: 200,
      statusText: 'OK',
      proto: 'HTTP/1.1',
      headers: [{ name: 'Content-Type', value: 'application/json' }],
      body,
      bodyEncoding: 'utf8',
      bodyBytes: body.length,
      bodyTruncated: false,
      elapsedMs: 10,
      finalUrl: 'https://api.example.com/users',
      redirects: [],
    };
  }

  studioTest(
    'pane InlineState: response formatting is hidden at 100 ms and visible within 1 s',
    async ({ relaunch }) => {
      // Over INLINE_CHARS (65 536), so the format pass runs in the worker and stays pending.
      const big = JSON.stringify({
        rows: Array.from({ length: 200_000 }, (_, i) => ({ id: i, v: 'x' })),
      });
      const { window: page } = await relaunch({
        control: [
          { channel: IPC.tabsList, response: [httpRequestTab({})] },
          { channel: IPC.httpSend, response: httpResponseOf(big) },
        ],
      });
      await page.click('[data-testid="http-send"]');
      const pending = page.locator('[data-testid="http-response-formatting"]');
      // Behavior under test: the formatting indicator shows only after a delay.
      await page.waitForTimeout(100);
      await expect(pending).toHaveCount(0);
      await expect(pending).toBeVisible({ timeout: 1_000 });
    },
  );

  studioTest(
    'response find bar: count text, Enter and Shift+Enter move the current match',
    async ({ relaunch }) => {
      const body = '{"id":1,"name":"Ada","note":"Ada Lovelace"}';
      const { window: page } = await relaunch({
        control: [
          { channel: IPC.tabsList, response: [httpRequestTab({})] },
          { channel: IPC.httpSend, response: httpResponseOf(body) },
        ],
      });
      await page.click('[data-testid="http-send"]');
      await expect(page.locator('[data-testid="http-status"]')).toContainText('200');
      await page.click('[data-testid="http-find-toggle"]');
      await page.fill('[data-testid="http-find-input"]', 'Ada');
      const count = page.locator('[data-testid="http-find-count"]');
      await expect(count).toHaveText('Match 1 of 2');

      await page.locator('[data-testid="http-find-input"]').press('Enter');
      await expect(count).toHaveText('Match 2 of 2');
      await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(1);
      await page.locator('[data-testid="http-find-input"]').press('Shift+Enter');
      await expect(count).toHaveText('Match 1 of 2');
      await expect(page.locator('.kira-ed-find-match-current')).toHaveCount(1);
    },
  );

  // --- trees ------------------------------------------------------------------------------------

  studioTest('project tree search renders matches as highlighted marks', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [
        {
          channel: IPC.connectionsList,
          response: [
            postgresConnectionSummary('conn-ul-1', 'Alpha DB', 'blue'),
            postgresConnectionSummary('conn-ul-2', 'Bravo DB', 'cyan'),
          ],
        },
      ],
    });
    await page.click('[data-testid="toggle-search"]');
    await page.fill('[data-testid="tree-search"]', 'Alpha');
    const mark = page.locator('[data-slot="highlighted-text"] mark').first();
    await expect(mark).toHaveText('Alpha');
    await expect(mark).toHaveClass(/bg-search-match/);
  });

  studioTest('collections tree expansion survives a reload', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [{ channel: IPC.collectionsList, response: COLLECTION_TREE }],
    });
    await modeTab(page, 'api').click();
    await collectionRow(page, 'col-1').locator('[data-testid="tree-twisty"]').click();
    await expect(collectionRow(page, 'item-health')).toBeVisible();

    await page.reload();
    await modeTab(page, 'api').click();
    await expect(collectionRow(page, 'item-health')).toBeVisible();
  });

  // --- layering, cursor -------------------------------------------------------------------------

  studioTest('a tooltip layer sits above TextPromptDialog', async ({ relaunch }) => {
    const { window: page } = await relaunch({
      control: [...GRID_CONTROL, { channel: IPC.queriesList, response: [] }],
      stream: GRID_FIXTURE.port,
    });
    await connectAndExpand(page, 'Leftover DB', 'green');
    await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
    await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
    await page.click('[data-testid="filter-history-button"]');
    await page.click('[data-testid="save-current-filter"]');
    const prompt = page.locator('[data-testid="text-prompt"]');
    await expect(prompt).toBeVisible();

    // Tooltip content cannot be hovered behind the scrim; probe the layer token it renders with.
    const [promptZ, tooltipZ] = await page.evaluate(() => {
      const scrim = document.querySelector('[data-testid="text-prompt"]') as HTMLElement;
      const probe = document.createElement('div');
      probe.style.cssText = 'position:fixed;z-index:var(--kira-z-tooltip)';
      document.body.appendChild(probe);
      const z = getComputedStyle(probe).zIndex;
      probe.remove();
      return [getComputedStyle(scrim).zIndex, z];
    });
    expect(Number(tooltipZ)).toBeGreaterThan(Number(promptZ));
  });

  studioTest(
    'tree rows and toolbar buttons use the default cursor; a link button uses pointer',
    async ({ relaunch }) => {
      const { window: page } = await relaunch({
        control: [
          {
            channel: IPC.connectionsList,
            response: [postgresConnectionSummary('conn-ul-1', 'Alpha DB', 'blue')],
          },
          { channel: IPC.tabsList, response: [httpRequestTab({ requestPane: 'settings' })] },
        ],
      });
      const cursor = (loc: Locator) => loc.evaluate((el) => getComputedStyle(el).cursor);
      await modeTab(page, 'studio').click();
      await expect
        .poll(() => cursor(page.locator('[data-testid="tree-row"]').first()))
        .toBe('default');
      await expect
        .poll(() => cursor(page.locator('[data-testid="add-connection"]')))
        .toBe('default');

      await modeTab(page, 'api').click();
      await page.click('[data-testid="http-request-pane-settings"]');
      await expect
        .poll(() => cursor(page.locator('[data-testid="http-settings-edit-global"]')))
        .toBe('pointer');
    },
  );

  // --- filter history ---------------------------------------------------------------------------

  studioTest(
    'filter history: pin toggles the row without reopening the menu',
    async ({ relaunch }) => {
      const SAVED = {
        id: 'q-1',
        connectionId: GRID_ID,
        name: 'Small ids',
        kind: 'filter',
        pinned: false,
        body: { where: 'id <= 5', orderBy: '' },
        createdAt: NOW,
        updatedAt: NOW,
      };
      const { window: page, control } = await relaunch({
        control: [
          ...GRID_CONTROL,
          { channel: IPC.queriesList, response: [SAVED] },
          { channel: IPC.queriesUpdate, response: { ...SAVED, pinned: true } },
        ],
        stream: GRID_FIXTURE.port,
      });
      await connectAndExpand(page, 'Leftover DB', 'green');
      await (await findRow(page, COMPOSITE_PK_PATH)).dblclick();
      await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();

      await page.click('[data-testid="filter-history-button"]');
      const panel = page.locator('[data-testid="filter-history"]');
      await expect(panel).toBeVisible();
      const pin = panel
        .locator('[data-testid="saved-entry"] [data-testid="saved-entry-pin"]')
        .first();
      await expect(pin).toHaveClass(/text-subtle/);

      await pin.click();
      // The mocked list stays unpinned after the update, so the toggle shows as the call it makes.
      await expect
        .poll(() => control.log().filter((e) => e.channel === IPC.queriesUpdate))
        .toHaveLength(1);
      expect(control.log().find((e) => e.channel === IPC.queriesUpdate)?.args).toEqual({
        id: 'q-1',
        name: null,
        pinned: true,
      });
      await expect(panel).toBeVisible();
    },
  );
});
