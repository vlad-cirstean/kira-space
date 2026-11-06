import type { Locator, Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import {
  connectMongo,
  connectRedis,
  createConnection,
  openConsoleFromMenu,
} from './support/connect';
import { mongoCreateArgs, postgresCreateArgs, redisCreateArgs } from './support/connectionArgs';
import { typeInto } from './support/editor';
import { editorText } from './support/editorText';
import { IPC } from './support/ipcChannels';
import {
  connectAndExpandControl as mongoConnectAndExpandControl,
  mongoConnectionSummary,
} from './support/mongoFixture';
import {
  APP_PATH,
  DB_PATH,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import {
  connectControl as redisConnectControl,
  redisConnectionSummary,
} from './support/redisFixture';
import { connectionRow, expandRow, openRowMenu } from './support/tree';

// P13 §6.1: no scenario here runs a statement, so no execute() port snapshot is needed at all —
// Format is pure renderer work, and the boot/connect control snapshots each fixture already
// exports are the whole setup.

async function connectAndExpandPostgres(page: Page, name: string, _color: string): Promise<void> {
  await createConnection(page, 'postgres', name);

  const connRow = connectionRow(page);
  await expect(connRow).toBeVisible();
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-connect"]');
  await expect(connRow.locator('.status-dot')).toHaveAttribute('data-status', 'connected', {
    timeout: 10_000,
  });
  await expandRow(page, '');
  await expandRow(page, DB_PATH);
  await expandRow(page, APP_PATH);
}

/** `editorText()`, not `.innerText()` — Monaco virtualises `.view-lines` (only the currently
 *  rendered subset of a large document is ever a real DOM child), so reading the model's own
 *  value through `MonacoHost.vue`'s debug-hook attribute is the only way to get
 *  the *whole* document back, not just whatever happens to be on screen. */
async function consoleText(view: Locator): Promise<string> {
  return editorText(view);
}

test('Query console — Format reformats a Postgres statement in place', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-format-pg';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format DB', 'green');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format DB', 'green'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format DB', 'green');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'SELECT a,b FROM t WHERE a=1 AND b IN (SELECT x FROM y)');

  const before = Date.now();
  await page.click('[data-testid="console-format"]');
  // The emitted chunk's own cost (the ~180ms cold import + first format) lands here, on
  // the first press of the whole test run — a coarse in-process measurement, logged rather than
  // asserted on (docs/PERF.md's own rule for this timing tier). Matches the reformatted shape
  // specifically (a lone "SELECT" first line) — the *unformatted* source also starts with the
  // substring "SELECT", so a bare prefix match would pass before Format's own async work lands.
  await expect.poll(() => consoleText(view)).toMatch(/^SELECT\n/);
  console.log(`P13 §6.2 first Format press (cold import + format), ms: ${Date.now() - before}`);

  const text = await consoleText(view);
  const lines = text.split('\n');
  expect(lines[0]).toBe('SELECT');
  expect(lines).toContain('  a,');
  expect(lines).toContain('  b');
  expect(lines).toContain('FROM');
});

test('Query console — Format on unparseable SQL leaves the text untouched', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-pg-err';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format Err DB', 'blue');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Err DB', 'blue'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Err DB', 'blue');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  const broken = 'select from where (((';
  await typeInto(view, page, broken);
  await page.click('[data-testid="console-format"]');

  const strip = view.locator('[data-testid="console-format-error"]');
  await expect(strip).toBeVisible();
  // F5's own guard: the library's raw parse dump (a multi-thousand-character nearley
  // expectation list) must never reach the UI — only its first line is fit to show.
  const stripText = (await strip.innerText()).trim();
  expect(stripText.split('\n')).toHaveLength(1);
  expect(await consoleText(view)).toBe(broken);

  // D9: the strip clears on the very next edit, so it never outlives the text that caused it.
  // Clicking the Format button (a real DOM button, unlike CodeMirror's own focus behaviour) moves
  // focus off the editor — re-click before typing so the keystroke actually lands in the document.
  await view.locator('.view-lines').click();
  await page.keyboard.type('x');
  await expect(strip).toHaveCount(0);
});

// P19 T12/D13 (reopening P13 §3): one statement the grammar rejects no longer takes the whole
// press down with it — it's emitted verbatim, in place, and a warn strip (not err — the press
// still did something useful) names which one.
test('Query console — one unparseable statement no longer blocks the rest (P19 D13)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-partial';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format Partial DB', 'cyan');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Partial DB', 'cyan'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Partial DB', 'cyan');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'select a,b from t;\n\\dt');
  await page.click('[data-testid="console-format"]');

  await expect(view.locator('[data-testid="console-format-error"]')).toHaveCount(0);
  const warning = view.locator('[data-testid="console-format-warning"]');
  await expect(warning).toBeVisible();
  await expect(warning).toContainText('Formatted 1 of 2 statements');
  await expect(warning).toContainText('Statement 2');

  const text = await consoleText(view);
  expect(text).toContain('\\dt'); // the broken statement, verbatim
  const lines = text.split('\n');
  expect(lines[0]).toBe('select'); // the first statement still reformatted (keywordCase: preserve)
});

// P19 T12/D13/F19: keywordCase: 'preserve' (P13 D4) means Format only ever changes whitespace —
// pressing it on an already-indented document changes nothing, and without this nothing
// distinguished "already formatted" from "the button is dead".
test('Query console — pressing Format twice shows "Already formatted" the second time (P19 D13)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-twice';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format Twice DB', 'magenta');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Twice DB', 'magenta'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Twice DB', 'magenta');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'select a,b from t');
  await page.click('[data-testid="console-format"]');
  // Format applies off the main thread; a second press before it lands re-formats the unformatted
  // text and never reports "Already formatted".
  await expect.poll(() => consoleText(view)).toMatch(/^select\n/);
  await expect(view.locator('[data-testid="console-format-note"]')).toHaveCount(0);

  await page.click('[data-testid="console-format"]');
  const note = view.locator('[data-testid="console-format-note"]');
  await expect(note).toBeVisible();
  // P22b D13: reworded to name the reason (indentation-only, ClickHouse's own identifier
  // hazard) rather than assert a bare null result.
  await expect(note).toContainText(
    'Already formatted. Format only changes indentation; keywords keep the case you typed',
  );
});

// splitSqlStatements' pushIfNonEmpty slices up to, not through, a statement's own ';'
// — a plain join(';\n\n') used to delete the document's LAST semicolon on every Format press.
// The reported symptom: a user writes `select * from t;`, presses Format, and watches
// their semicolon disappear.
test("Query console — Format keeps the document's last semicolon (P22b D12)", async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-semicolon';
  const CONNECTION_SUMMARY = postgresConnectionSummary(
    CONNECTION_ID,
    'Format Semicolon DB',
    'magenta',
  );
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Semicolon DB', 'magenta'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Semicolon DB', 'magenta');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'select 1;');
  await page.click('[data-testid="console-format"]');
  const text = await consoleText(view);
  expect(text.trim().endsWith(';')).toBe(true);
});

test('Query console — Format reformats a Mongo aggregate pipeline', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-format-mongo';
  const CONNECTION_SUMMARY = mongoConnectionSummary(CONNECTION_ID, 'Format Mongo', 'green');
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: mongoCreateArgs('Format Mongo'),
      response: CONNECTION_SUMMARY,
    },
    ...mongoConnectAndExpandControl(CONNECTION_ID),
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectMongo(page, 'Format Mongo', 'green');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-open-console"]');
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'db.widgets.aggregate([{$match:{a:1}},{$group:{_id:"$a"}}])');
  await page.click('[data-testid="console-format"]');

  const lines = (await consoleText(view)).split('\n');
  expect(lines[0]).toBe('db.widgets.aggregate([');
  expect(lines.some((l) => l.trim() === '"$match": {')).toBe(true);
  expect(lines.some((l) => l.trim() === '"$group": {')).toBe(true);
});

test('Query console — Redis has no Format button (D6)', async ({ relaunch }) => {
  const CONNECTION_ID = 'conn-console-format-redis';
  const CONNECTION_SUMMARY = redisConnectionSummary(CONNECTION_ID, 'Format Redis', 'red');
  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: redisCreateArgs('Format Redis'),
      response: CONNECTION_SUMMARY,
    },
    ...redisConnectControl(CONNECTION_ID),
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectRedis(page, 'Format Redis', 'red');
  await openRowMenu(page, '');
  await page.click('[data-testid="menu-item-open-console"]');
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  // A Redis statement is a flat token list with nothing to reformat — the toolbar mounts
  // (Run is there) and the button is deliberately absent, not the whole view missing. This is
  // the assertion most likely to be quietly "fixed" later by someone who assumes the missing
  // button is an oversight rather than a deliberate decision.
  await expect(view.locator('[data-testid="console-run-statement"]')).toBeVisible();
  await expect(view.locator('[data-testid="console-format"]')).toHaveCount(0);
});

// P19 T11/D12: closes v1.1 P13's own OQ-2 — the caret used to jump to offset 0 on every format,
// which also silently re-pointed *Run statement* at the first statement regardless of where the
// user was actually working.
test('Query console — Format leaves the caret in the statement it was in, so Run statement still targets it (P19 D12)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-caret';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format Caret DB', 'amber');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Caret DB', 'amber'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page, stream } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Caret DB', 'amber');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  await typeInto(view, page, 'select 1 as a;\nselect 2 as b;\nselect 3 as c;');
  await expect(view.locator('.view-line')).toHaveCount(3);

  // Put the caret in the SECOND statement — typing left it at the very end (line 3); one Up
  // arrow from there lands in line 2, deterministically (a mouse click's target coordinate can
  // land ambiguously between two adjacent lines under font-metric rounding). Escape first closes
  // any completion popup still open from typing, which would otherwise steal the Up arrow to
  // navigate its own suggestion list instead of moving the real caret.
  await page.keyboard.press('Escape');
  await page.keyboard.press('ArrowUp');
  // ArrowUp alone preserves the goal column, which (typing left the caret at the very end of
  // line 3, the same column as line 2's own last character) lands exactly on the boundary
  // between statement 2 and statement 3 rather than inside statement 2 — Home moves it
  // unambiguously into the middle of line 2's own text instead.
  await page.keyboard.press('Home');
  await page.keyboard.press('ArrowRight');

  await page.click('[data-testid="console-format"]');
  // Formatting rewrote every offset in the document — this is only proof the press itself
  // completed, not that the caret landed anywhere in particular.
  await expect.poll(() => consoleText(view)).toContain('as c');

  await page.click('[data-testid="console-run-statement"]');
  const executed = (await stream.ops()).filter((o) => o.op === DATA_OP.execute).at(-1);
  const statements = (executed?.payload as { statements?: string[] } | undefined)?.statements;
  expect(statements).toHaveLength(1);
  expect(statements?.[0]).toContain('as b');
  expect(statements?.[0]).not.toContain('as a');
  expect(statements?.[0]).not.toContain('as c');
});

// CodeMirrorHost's old external-sync dispatch (used
// by Format's own setText(), and by loading a saved query) had no undo-boundary annotation, so it
// could merge into whatever undo group was still open — the ordinary case of typing a query then
// immediately clicking Format, right next to it. One Cmd+Z then wiped the WHOLE query back to
// empty instead of undoing just the reformat. MonacoHost.vue's own external-sync watcher now
// brackets every such write with a `pushStackElement()` pair for exactly this reason. No
// artificial delay anywhere in this test — that immediacy is exactly what triggered the bug.
test('Query console — Format immediately after typing, then undo, restores the typed text (not empty)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-undo';
  const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Format Undo DB', 'red');
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format Undo DB', 'red'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format Undo DB', 'red');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  const original = 'SELECT a,b FROM t WHERE a=1 AND b IN (SELECT x FROM y)';
  await typeInto(view, page, original);
  await page.click('[data-testid="console-format"]');
  // Matches the reformatted shape (a lone "SELECT" first line), not merely the "SELECT" prefix
  // the unformatted source already has — the same reason the first test in this file does.
  await expect.poll(() => consoleText(view)).toMatch(/^SELECT\n/);

  await view.locator('.view-lines').click();
  await page.keyboard.press('ControlOrMeta+z');
  await expect.poll(() => consoleText(view)).toBe(original);
});

// `localDoc` is a shallowRef, and Vue skips a dep trigger when an assigned value equals
// the ref's current one — so an external write (here, Format's own result) equal to the last
// value written to `localDoc` never reached Monaco, leaving the editor showing stale text while
// the store believed it had already advanced. Format-undo-format is the deterministic repro: the
// second Format's result is byte-identical to the first (which `localDoc` still holds), so it's
// exactly the write the old code silently dropped.
test('Query console — Format after an undo reaches the editor even when the result repeats a prior format (P89)', async ({
  relaunch,
}) => {
  const CONNECTION_ID = 'conn-console-format-after-undo';
  const CONNECTION_SUMMARY = postgresConnectionSummary(
    CONNECTION_ID,
    'Format After Undo DB',
    'grey',
  );
  const FIXTURE = orderItemsFixture(CONNECTION_ID);

  const CONTROL: ControlSnapshot[] = [
    { channel: IPC.connectionsList, response: [] },
    {
      channel: IPC.connectionsCreate,
      args: postgresCreateArgs('Format After Undo DB', 'grey'),
      response: CONNECTION_SUMMARY,
    },
    ...FIXTURE.control,
  ];

  const { window: page } = await relaunch({ control: CONTROL });
  await connectAndExpandPostgres(page, 'Format After Undo DB', 'grey');
  await openConsoleFromMenu(page, ORDER_ITEMS_PATH);
  const view = page.locator('[data-testid="console-view"]');
  await expect(view).toBeVisible();

  const original = 'SELECT a,b FROM t WHERE a=1';
  await typeInto(view, page, original);
  await page.click('[data-testid="console-format"]');
  await expect.poll(() => consoleText(view)).toMatch(/^SELECT\n/);

  await view.locator('.view-lines').click();
  await page.keyboard.press('ControlOrMeta+z');
  await expect.poll(() => consoleText(view)).toBe(original);

  // The bug: this used to leave the editor showing `original` (one line), since the reformatted
  // text is byte-identical to what `localDoc` already held from the first Format.
  await page.click('[data-testid="console-format"]');
  await expect.poll(() => consoleText(view)).toMatch(/^SELECT\n/);
  await expect(view.locator('[data-testid="console-format-note"]')).toHaveCount(0);
});
