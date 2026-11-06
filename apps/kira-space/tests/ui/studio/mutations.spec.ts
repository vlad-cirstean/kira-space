import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, studioTest as test } from '../fixtures';
import { IPC } from '../support/ipcChannels';
import { connectionCreateArgs, createConnection } from './support/connect';
import { diffEditorText, editorText } from './support/editorText';
import { cellText, gridCell, gutterCell } from './support/grid';
import {
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, expandRow, findRow, openRowMenu } from './support/tree';

const RO_CONNECTION_ID = 'conn-mutations-ro';
const RO_CONNECTION_SUMMARY = {
  ...postgresConnectionSummary(RO_CONNECTION_ID, 'Mutations DB (RO)', 'red'),
  readOnly: true,
};
const RO_FIXTURE = compositePkConnectAndOpen(RO_CONNECTION_ID);

// Ported from tests/e2e/mutations.spec.ts, against real captures of app.composite_pk's
// preview/mutate/count responses (scripts/capture-postgres-tree.ts, including the real
// "duplicate key value violates unique constraint" error text — not invented). Every scenario
// here is single-session (no relaunch), so all of it ports; the one thing genuinely new to this
// tier is that `data:preview` and a failed `data:mutate` had no mock support at all before this —
// see tests/ipc/support/types.ts's PortSnapshot.error and the new 'preview' LogicalPortResponse
// kind.

const CONNECTION_ID = 'conn-mutations';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Mutations DB', 'green');
const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Mutations DB'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Mutations DB (RO)'),
    response: RO_CONNECTION_SUMMARY,
  },
  ...RO_FIXTURE.control,
];

const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  {
    op: DATA_OP.preview,
    payload: {
      connectionId: CONNECTION_ID,
      path: COMPOSITE_PK_PATH,
      ops: [{ kind: 'insert', values: { tenant_id: '9', entity_id: '1', name: 'inserted row' } }],
    },
    response: {
      kind: 'preview',
      statements: [
        `INSERT INTO "app"."composite_pk" ("tenant_id", "entity_id", "name") VALUES ('9', '1', 'inserted row')`,
      ],
    },
  },
  // The committed batch: delete (1,2), then update (1,1)'s name (buildPlan's own order —
  // deletes, then edits, then inserts).
  {
    op: DATA_OP.mutate,
    payload: {
      connectionId: CONNECTION_ID,
      path: COMPOSITE_PK_PATH,
      ops: [
        { kind: 'delete', key: { tenant_id: '1', entity_id: '2' } },
        {
          kind: 'update',
          key: { tenant_id: '1', entity_id: '1' },
          changes: { name: 'committed value' },
        },
      ],
    },
    response: { kind: 'mutate', affectedRows: 2 },
  },
  // reloadAfterMutation()'s own data.invalidate() call, before it reloads the page below —
  // must be answered or the whole async function throws and the reload read never runs.
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, scope: 'pages' },
    response: { kind: 'invalidate' },
  },
  // reloadAfterMutation()'s own reload, at the tab's current page/pageSize.
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: COMPOSITE_PK_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [
          ['1', '1', 'committed value'],
          ['2', '1', 'tenant 2 / entity 1'],
        ],
        position: {
          offset: 0,
          pageSize: 100,
          hasMore: false,
          nextToken: null,
          prevToken: null,
          strategy: 'keyset',
        },
        truncatedCells: 0,
      },
      source: 'server',
    },
  },
  // Count sequence (all share one (op, {filter, refresh}) key when refresh matches — see
  // mockStreamBrowser.js's own cursor replay): (1) the "establish a known count" click,
  // (2) reloadAfterMutation()'s own automatic re-count (still refresh:false — the local `stale`
  // flag flips only once *this* response lands), both refresh:false, replayed in that order;
  // (3) the user's own "click it through" recount, refresh:true.
  {
    op: DATA_OP.count,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, filter: null, refresh: false },
    response: { kind: 'count', value: 3, exact: true, stale: false, source: 'server' },
  },
  {
    op: DATA_OP.count,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, filter: null, refresh: false },
    response: { kind: 'count', value: 3, exact: true, stale: true, source: 'cache' },
  },
  {
    op: DATA_OP.count,
    payload: { connectionId: CONNECTION_ID, path: COMPOSITE_PK_PATH, filter: null, refresh: true },
    response: { kind: 'count', value: 2, exact: true, stale: false, source: 'server' },
  },
  // The duplicate-key insert: (1,1) already exists after the commit above.
  {
    op: DATA_OP.mutate,
    payload: {
      connectionId: CONNECTION_ID,
      path: COMPOSITE_PK_PATH,
      ops: [{ kind: 'insert', values: { tenant_id: '1', entity_id: '1', name: 'duplicate key' } }],
    },
    error: {
      code: 'E_QUERY',
      message: 'duplicate key value violates unique constraint "composite_pk_pkey"',
    },
  },
  ...RO_FIXTURE.port,
];

async function editCell(
  page: import('@playwright/test').Page,
  row: number,
  column: string,
  value: string,
) {
  await gridCell(page, row, column).dblclick();
  const input = page.locator('[data-testid="grid-cell-input"]');
  await expect(input).toBeVisible();
  await input.fill(value);
  await input.press('Enter');
}

test('mutations — edit, add, delete, preview, commit, discard, read-only guard', async ({
  relaunch,
}) => {
  // Seven scenarios and two connection setups in one test: ~13s unloaded, but it ran into the 60s
  // default on a CPU-starved runner (parallel suites).
  test.setTimeout(180_000);
  const { window: page, stream } = await relaunch({ control: CONTROL, stream: PORT });

  await createConnection(page, 'postgres', 'Mutations DB');

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');

  const compositeRow = await findRow(page, COMPOSITE_PK_PATH);
  await compositeRow.dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="name"]')).toBeVisible();

  // --- scenario 1: editing a cell stages it (tinted, not yet sent) ------------------------
  const originalName = await cellText(page, 0, 'name');
  await editCell(page, 0, 'name', 'edited via UI');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe('edited via UI');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toBeVisible();
  await expect(page.locator('[data-testid="toolbar-discard-changes"]')).toBeVisible();

  // --- scenario 1b: preview diffs a staged edit --------------------------------------------
  await page.click('[data-testid="toolbar-preview-command"]');
  const diffPanel = page.locator('[data-testid="preview-command-panel"]');
  await expect(diffPanel).toBeVisible();
  await expect(diffPanel.locator('[data-testid="preview-command-view-changes"]')).toHaveAttribute(
    'data-state',
    'on',
  );
  const diffItems = diffPanel.locator('[data-testid="pending-diff-item"]');
  await expect(diffItems).toHaveCount(1);
  await expect(diffItems).toHaveAttribute('data-column', 'name');
  expect(await diffEditorText(diffPanel.locator('[data-testid="pending-diff"]'))).toEqual({
    original: originalName,
    modified: 'edited via UI',
  });
  const diffHost = diffPanel.locator('[data-testid="pending-diff"]');
  await expect(
    diffHost.locator('.editor.modified .view-line').filter({ hasText: 'edited via UI' }),
  ).toBeVisible();
  const diffBox = await diffHost.locator('.monaco-diff-editor').boundingBox();
  expect(diffBox?.height ?? 0).toBeGreaterThan(200);
  await diffPanel.locator('[data-testid="preview-command-view-sql"]').click();
  await expect(diffPanel.locator('[data-testid="pending-diff"]')).toHaveCount(0);
  await page.click('[data-testid="preview-command-close"]');
  await expect(diffPanel).toHaveCount(0);

  // --- scenario 2: discard reverts every pending change -------------------------------------
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
  expect(await cellText(page, 0, 'name')).toBe(originalName);

  // --- scenario 3: add row appends an always-editable insert row ---------------------------
  await page.click('[data-testid="toolbar-add-row"]');
  const insertRow = page.locator('[data-testid="grid-row-insert"]');
  await expect(insertRow).toHaveCount(1);
  const insertId = await insertRow.getAttribute('data-insert-id');
  expect(insertId).not.toBeNull();
  const insertInputs = insertRow.locator('[data-testid="grid-cell-insert"] input');
  await insertInputs.nth(0).fill('9');
  await insertInputs.nth(1).fill('1');
  await insertInputs.nth(2).fill('inserted row');

  // --- scenario 4: preview command shows the exact SQL for pending changes -----------------
  await page.click('[data-testid="toolbar-preview-command"]');
  const previewPanel = page.locator('[data-testid="preview-command-panel"]');
  await expect(previewPanel).toBeVisible();
  await expect(previewPanel.locator('.monaco-host')).toBeVisible({
    timeout: 10_000,
  });
  const previewText = await editorText(previewPanel);
  expect(previewText).toContain('INSERT INTO');
  expect(previewText).toContain('composite_pk');
  await page.click('[data-testid="preview-command-close"]');
  await expect(previewPanel).toHaveCount(0);

  // Discard the insert before moving to the delete/commit scenarios below, so they start clean.
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(insertRow).toHaveCount(0);

  // --- scenario 5: delete-row marks a row struck-through, non-committed until commit -------
  // Page-row-scoped (support/grid.ts's own gutterCell), not a raw DOM-order `.nth(1)`:
  // SlickGrid recycles row DOM nodes as it renders, so their physical DOM order does not
  // track visual/page-row order the way the incumbent's own always-append-in-order rows did.
  await gutterCell(page, 1).click();
  await page.click('[data-testid="toolbar-delete-row"]');
  const deletedRow = page.locator('[data-testid="grid-row"][data-row="1"]');
  await expect(deletedRow).toHaveClass(/pending-delete/);

  // --- scenario 6: commit sends the batch and reloads the tab ------------------------------
  const countButton = page.locator('[data-testid="toolbar-count"]');
  const countTip = page.locator('[data-slot="tooltip-content"]').first();
  // Neither click() nor fill()/press() moves the real pointer off this button once it has already
  // been hovered, so hover() alone can land with no actual movement and fire no new mouseenter.
  // Move away first, every time, to guarantee a real one.
  async function hoverCount(): Promise<void> {
    await page.mouse.move(0, 0);
    await countButton.hover();
  }
  await countButton.click();
  await hoverCount();
  await expect(countTip).toContainText(/\d[\d,]* rows in total/, { timeout: 10_000 });
  const countBeforeCommit = (await countTip.textContent())?.match(/[\d,]+ rows in total/)?.[0];
  expect(countBeforeCommit).toBeTruthy();

  const deletedRowName = await cellText(page, 1, 'name');
  await editCell(page, 0, 'name', 'committed value');
  await page.click('[data-testid="toolbar-commit-changes"]');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
  await expect(gridCell(page, 0, 'name')).not.toHaveClass(/pending-edit/);
  await expect(gridCell(page, 0, 'name')).toHaveText('committed value', { timeout: 10_000 });
  const remainingNames = await page
    .locator('[data-testid="grid-cell"][data-column="name"]')
    .allInnerTexts();
  expect(remainingNames).not.toContain(deletedRowName);

  // Go mints the op id for a count and a mutate: the renderer sends none.
  const minted = (await stream.ops()).filter(
    (o) => o.op === DATA_OP.count || o.op === DATA_OP.mutate,
  );
  expect(minted.length).toBeGreaterThan(0);
  expect(minted.every((o) => o.opId === undefined)).toBe(true);

  // §7/F21/D18: a local mutation greys the count instead of blanking it.
  await hoverCount();
  await expect(countTip).toContainText('Count is stale. Click to refresh', { timeout: 10_000 });
  await expect(countTip).toContainText(`${countBeforeCommit}. Count is stale. Click to refresh`);

  // Clicking it through produces a real recount and clears the stale mark.
  await countButton.click();
  await hoverCount();
  await expect(countTip).not.toContainText('stale', { timeout: 10_000 });
  const countAfterCommit = (await countTip.textContent())?.match(/[\d,]+ rows in total/)?.[0];
  expect(countAfterCommit).not.toBe(countBeforeCommit);

  // --- P43 F5/D7: a failed commit reports the server's own error, not an unhandled rejection —
  // (1, 1) is the surviving row from scenario 6's own edit, so a fresh insert of the same
  // composite key violates the PK. -----------------------------------------------------------
  await page.click('[data-testid="toolbar-add-row"]');
  const dupInsertRow = page.locator('[data-testid="grid-row-insert"]');
  await expect(dupInsertRow).toHaveCount(1);
  const dupInsertInputs = dupInsertRow.locator('[data-testid="grid-cell-insert"] input');
  await dupInsertInputs.nth(0).fill('1');
  await dupInsertInputs.nth(1).fill('1');
  await dupInsertInputs.nth(2).fill('duplicate key');
  await page.click('[data-testid="toolbar-commit-changes"]');
  const actionError = page.locator('[data-testid="data-action-error"]');
  await expect(actionError).toBeVisible();
  await expect(actionError).toContainText(/duplicate key|unique/i);
  await expect(dupInsertRow).toHaveCount(1); // the staged insert survives the failure
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(dupInsertRow).toHaveCount(0);
  await expect(actionError).toHaveCount(0);

  // --- scenario 7: a read-only connection disables every mutation button ------------------
  const firstConnRow = connectionRow(page, 'Mutations DB');
  await expect(firstConnRow).toHaveCount(1);
  await firstConnRow.locator('[data-testid="tree-twisty"]').click();

  await createConnection(page, 'postgres', 'Mutations DB (RO)');

  const roConnRow = connectionRow(page, 'Mutations DB (RO)');
  await expect(roConnRow).toBeVisible();
  await roConnRow.click({ button: 'right' });
  await page.click('[data-testid="menu-item-connect"]');
  await expect(roConnRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await roConnRow.locator('[data-testid="tree-twisty"]').click();
  await expandRow(page, 'database:kira_test');
  await expandRow(page, 'database:kira_test/schema:app');
  const roCompositeRow = await findRow(page, COMPOSITE_PK_PATH);
  await roCompositeRow.dblclick();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="name"]')).toBeVisible();

  for (const testId of ['toolbar-add-row', 'toolbar-delete-row']) {
    await expect(page.locator(`[data-testid="${testId}"]`)).toBeDisabled();
  }
  await expect(page.locator('[data-testid="toolbar-preview-command"]')).toHaveCount(0);
});
