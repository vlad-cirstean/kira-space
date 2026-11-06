import type { Locator, Page } from '@playwright/test';
import type { ConnectionSummary } from '@shared/domain/connection';
import type { ObjectMeta } from '@shared/domain/tree';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ControlSnapshot, PortSnapshot } from '../../ipc/support/types';
import { expect, test } from './fixtures';
import { primaryChord } from './support/chord';
import { installClipboardShim } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import {
  cellNavButton,
  cellText,
  clickCellNav,
  fkPreview,
  gridCell,
  gutterCell,
  headerCell,
  nullMarker,
} from './support/grid';
import { IPC } from './support/ipcChannels';
import {
  APP_PATH,
  COMPOSITE_PK_COLUMNS,
  COMPOSITE_PK_PATH,
  compositePkConnectAndOpen,
  ORDER_ITEMS_PATH,
  orderItemsFixture,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { connectionRow, findRow, openRowMenu, settleFrames } from './support/tree';

// Ported from tests/e2e/interaction.spec.ts (P57 D16), against real captures of the
// regions -> customers -> orders -> order_items <- products FK graph and app.composite_pk's own
// filter/sort/projection reads (scripts/capture-postgres-tree.ts) — every scenario that exercises
// real Vue component behaviour (right-click menus, selection accumulation, real keyboard-driven
// copy/paste/duplicate, PK/FK cell navigation) ports against that data. Three things do not:
//
// 1. **The entire "D10: Operations panel" scenario is dropped.** `state/ops.ts`'s `hydrateOps()`
//    fetches `opsRecent()` exactly once at boot and thereafter relies solely on a live
//    `control.onOpUpdate` push (a real Wails `Events.On` subscription) for every status change —
//    a new op appearing, a running op finishing or being cancelled. `tests/ui/support/mockRuntime.ts`
//    intercepts only the `Call` RPC endpoint; it has no `Events.On` mechanism at all (already noted,
//    narrowly, in `connections.spec.ts`'s own header comment for `connectionsChanged`). So an
//    `op-row` for a statement run *during this test* can never appear in the DOM here, regardless of
//    what the console itself does — this is a hard, structural gap, not a fixture-design problem.
//    The op menu's own actions (reveal-tab/copy-command/copy-error/re-run/cancel) and the
//    `window.kira.opsRecent()` polling the original used to verify cancellation have no coverage
//    surface left to port onto.
// 2. **The entire "D11/D12: native-menu keyboard shortcuts" scenario is dropped** — Command
//    Palette, Window ▸ Next/Previous/Close Tab, View ▸ Find/Refresh/Run Statement/Run All. Unlike
//    `console.spec.ts`'s Undo/Redo (which keeps its keyboard half because CodeMirror's own
//    `history()`/`historyKeymap` is a real, independent, renderer-owned keydown handler), every one
//    of these is a native-menu accelerator binding (`shared/domain/shortcuts.ts`) — its accelerator is owned *exclusively* by the native menu
//    (`main/menu.ts` pre-cutover, the Go-side Wails menu post-cutover) and "never a local keydown
//    handler". Confirmed by reading `App.vue`: every one of `onCommandPalette`/`onTabNext`/
//    `onTabPrev`/`onTabClose`/`onViewFind`/`onViewRefresh`/`onViewRun`/`onViewRunAll` is wired to a
//    `control.onXxx` push-event listener with no matching `matchesShortcut`/`shortcutFor` call site
//    anywhere in the renderer. There is no `ElectronApplication` in this tier (the same reason
//    `console.spec.ts` already gives), and — unlike undo/redo — there is also no independent
//    keyboard path *or* UI button surviving underneath: the Command Palette itself has no reachable
//    affordance besides that same broken push event (no button opens it), so even the commands it
//    would otherwise reach (`runCommand('view.find')` et al.) are unreachable from here. This is the
//    same root cause as (1) above, not a second, unrelated gap — worth a line in `CLAUDE.md` since it
//    will recur for any future port touching a `global: true` shortcut or a live `control.onXxx`
//    push. The two view-level behaviours this would have exercised a second way (Run Statement/Run
//    All, grid refresh) are already covered by `console.spec.ts` and the toolbar-button paths below,
//    so nothing about the underlying *feature* goes untested — only the menu-triggering mechanism.
// 3. **`window.kira`-based helpers have no replacement** (confirmed P57 finding, `CLAUDE.md`):
//    `createConnection()`'s raw `window.kira.connectionsCreate(...)` call becomes a real
//    dialog-driven creation (`connectAndExpand`, matching `console.spec.ts`'s own helper); `getOps()`
//    (`window.kira.opsRecent`) has nothing to port onto given (1) above.
//
// One environment-only addition: `navigator.clipboard.readText()` throws `NotAllowedError` under
// Playwright's WebKit automation even with `clipboard-read` granted (`context.grantPermissions`
// recognises the permission name but the browser still refuses an ungestured read) — WebKit has no
// story here the way Chromium's CDP-backed permission grant does. `installClipboardShim` replaces
// `navigator.clipboard` with a plain in-memory backing store via `page.addInitScript` + one reload
// before the scenario starts; `copyText()`/`DataGrid.vue`'s own `onPaste()` call the exact same
// `navigator.clipboard.writeText`/`readText` interface either way, so this changes nothing about
// what's under test, only which implementation answers it.
//
// A second, more surprising environment finding, worth its own line in `CLAUDE.md`:
// **Playwright's bundled WebKit reports `navigator.userAgent` as `Macintosh` unconditionally**,
// confirmed by direct experiment (`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)
// AppleWebKit/605.1.15 …`) regardless of the actual host OS this sandbox runs on (Linux).
// `renderer/shortcuts/keys.ts`'s own `isMac` is `navigator.userAgent.includes('Mac')` — evaluated
// *inside the page*, not the Node test-runner process — so the original e2e file's own
// `process.platform === 'darwin'` check (correct under real Electron, where the renderer's UA
// honestly reflects the host) silently picks the *wrong* branch here: the app believes it is
// running on macOS even though `process.platform` in this very test file reports `'linux'`.
// Concretely, `grid.deleteRows`'s mac-only override (`Cmd+Backspace`, not the plain `Delete`
// every other platform uses) is the one this bit hardest — a keyboard 'Delete' press matched no
// shortcut at all, silently. `DUPLICATE_KEY`/`COPY_KEY`/`DELETE_KEY` below are therefore derived
// from the *page's own* `navigator.userAgent` after `relaunch()`, not from `process.platform` —
// the only way to guarantee the test and the app under test can never disagree about which chord
// is live. `TOGGLE_MODIFIER` (Control vs. Meta for a gutter toggle-click) is a different mechanism
// entirely — a real OS-level input-translation quirk on macOS hardware, not a UA string read by
// app code — so it stays keyed off the real host platform, unaffected by this.

const COMPOSITE_PATH = COMPOSITE_PK_PATH;
const EMPLOYEES_PATH = `${APP_PATH}/table:employees`;
const ORDERS_PATH = `${APP_PATH}/table:orders`;
const CUSTOMERS_PATH = `${APP_PATH}/table:customers`;
const PRODUCTS_PATH = `${APP_PATH}/table:products`;

const CONNECTION_ID = 'conn-interaction';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Interaction DB', 'green');
const DUPLICATE_SUMMARY: ConnectionSummary = {
  ...CONNECTION_SUMMARY,
  id: 'conn-interaction-copy',
  name: 'Interaction DB copy',
  sortOrder: 1,
  createdAt: '2026-01-01T00:00:01.000Z',
  updatedAt: '2026-01-01T00:00:01.000Z',
};

const FIXTURE = compositePkConnectAndOpen(CONNECTION_ID);
const ORDER_ITEMS_FIXTURE = orderItemsFixture(CONNECTION_ID);

// Real captures (scripts/capture-postgres-tree.ts) of the regions -> customers -> orders ->
// order_items <- products FK graph's own describe() output — see
// packages/db-fixtures/fixtures/0001_seed.sql's own comment for why this shape exists. Transcribed
// verbatim from the capture tool's output, not hand-written.
const ORDERS_META: ObjectMeta = {
  path: ORDERS_PATH,
  kind: 'table',
  name: 'orders',
  qualifiedName: 'app.orders',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: "nextval('app.orders_id_seq'::regclass)",
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'customer_id',
      position: 2,
      dataType: 'integer',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'ordered_at',
      position: 3,
      dataType: 'timestamp with time zone',
      nullable: false,
      defaultExpr: 'now()',
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [
    {
      name: 'orders_customer_id_fkey',
      columns: ['customer_id'],
      referencedPath: CUSTOMERS_PATH,
      referencedColumns: ['id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  referencedBy: [
    {
      name: 'order_items_order_id_fkey',
      columns: ['id'],
      referencedPath: ORDER_ITEMS_PATH,
      referencedColumns: ['order_id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  indexes: [{ name: 'orders_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' }],
  rowEstimate: null,
  comment: null,
};

const CUSTOMERS_META: ObjectMeta = {
  path: CUSTOMERS_PATH,
  kind: 'table',
  name: 'customers',
  qualifiedName: 'app.customers',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: "nextval('app.customers_id_seq'::regclass)",
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'name',
      position: 2,
      dataType: 'text',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'region_id',
      position: 3,
      dataType: 'integer',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  foreignKeys: [
    {
      name: 'customers_region_id_fkey',
      columns: ['region_id'],
      referencedPath: `${APP_PATH}/table:regions`,
      referencedColumns: ['id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  referencedBy: [
    {
      name: 'orders_customer_id_fkey',
      columns: ['id'],
      referencedPath: ORDERS_PATH,
      referencedColumns: ['customer_id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  indexes: [
    { name: 'customers_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' },
  ],
  rowEstimate: null,
  comment: null,
};

const EMPLOYEES_META: ObjectMeta = {
  path: EMPLOYEES_PATH,
  kind: 'table',
  name: 'employees',
  qualifiedName: 'app.employees',
  columns: [
    {
      name: 'id',
      position: 1,
      dataType: 'integer',
      nullable: false,
      defaultExpr: "nextval('app.employees_id_seq'::regclass)",
      isPrimaryKey: true,
      comment: null,
    },
    {
      name: 'name',
      position: 2,
      dataType: 'text',
      nullable: false,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
    {
      name: 'manager_id',
      position: 3,
      dataType: 'integer',
      nullable: true,
      defaultExpr: null,
      isPrimaryKey: false,
      comment: null,
    },
  ],
  primaryKey: ['id'],
  // D17: employees is its own referencedBy target (a self-referencing FK) — manager_id -> id.
  foreignKeys: [
    {
      name: 'employees_manager_id_fkey',
      columns: ['manager_id'],
      referencedPath: EMPLOYEES_PATH,
      referencedColumns: ['id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  referencedBy: [
    {
      name: 'employees_manager_id_fkey',
      columns: ['id'],
      referencedPath: EMPLOYEES_PATH,
      referencedColumns: ['manager_id'],
      onDelete: 'NO ACTION',
      onUpdate: 'NO ACTION',
    },
  ],
  indexes: [
    { name: 'employees_pkey', columns: ['id'], unique: true, primary: true, method: 'btree' },
  ],
  rowEstimate: null,
  comment: null,
};

const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Interaction DB', 'green'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
  ...ORDER_ITEMS_FIXTURE.control,
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: ORDERS_PATH, refresh: false, tabId: null },
    response: { meta: ORDERS_META, source: 'server' },
  },
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: CUSTOMERS_PATH, refresh: false, tabId: null },
    response: { meta: CUSTOMERS_META, source: 'server' },
  },
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: EMPLOYEES_PATH, refresh: false, tabId: null },
    response: { meta: EMPLOYEES_META, source: 'server' },
  },
  // P21 tree shortcuts: F2 rename opens the edit dialog, Ctrl/Cmd+D duplicates. No
  // connectionsReveal snapshot here any more — P14 D1 stopped fetching the secret just to open
  // the dialog, so a stale mock for that call would be dead weight, not real coverage.
  { channel: IPC.connectionsDuplicate, args: { id: CONNECTION_ID }, response: DUPLICATE_SUMMARY },
];

function readPayload(
  path: string,
  extra?: Partial<Record<'filter' | 'sort' | 'projection', unknown>>,
) {
  return {
    connectionId: CONNECTION_ID,
    path,
    projection: extra?.projection ?? null,
    filter: extra?.filter ?? null,
    sort: extra?.sort ?? null,
    pageSize: 100,
    cursor: { mode: 'offset' as const, offset: 0 },
  };
}

// The FK preview popover's own tab-free fetch — same shape as readPayload() above but
// always pageSize 10 (fkPreview.ts's own fetchReferencedRow, MAX_PAGE_SIZE's smallest), so it is a
// distinct mock entry from the pageSize-100 read the same filter serves when "Open in new tab" or
// a real tab open follows it.
function previewPayload(path: string, filter: string) {
  return {
    connectionId: CONNECTION_ID,
    path,
    projection: null,
    filter,
    sort: null,
    pageSize: 10,
    cursor: { mode: 'offset' as const, offset: 0 },
  };
}

const PORT: PortSnapshot[] = [
  ...FIXTURE.port,
  ...ORDER_ITEMS_FIXTURE.port,
  // --- D4: "Filter by this value"'s "= value" and "IS NULL" branches, real Postgres captures ---
  {
    op: DATA_OP.read,
    payload: readPayload(COMPOSITE_PATH, { filter: `"name" = 'tenant 1 / entity 1'` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [['1', '1', 'tenant 1 / entity 1']],
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
  {
    op: DATA_OP.read,
    payload: readPayload(COMPOSITE_PATH, { filter: `"name" IS NULL` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [],
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
  // --- D7: header menu's Sort asc/desc — real server-side ORDER BY, real row order ------------
  {
    op: DATA_OP.read,
    payload: readPayload(COMPOSITE_PATH, {
      sort: { kind: 'structured', terms: [{ column: 'entity_id', direction: 'asc' }] },
    }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [
          ['1', '1', 'tenant 1 / entity 1'],
          ['2', '1', 'tenant 2 / entity 1'],
          ['1', '2', 'tenant 1 / entity 2'],
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
  {
    op: DATA_OP.read,
    payload: readPayload(COMPOSITE_PATH, {
      sort: { kind: 'structured', terms: [{ column: 'entity_id', direction: 'desc' }] },
    }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: COMPOSITE_PK_COLUMNS,
        rows: [
          ['1', '2', 'tenant 1 / entity 2'],
          ['2', '1', 'tenant 2 / entity 1'],
          ['1', '1', 'tenant 1 / entity 1'],
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
  // --- D8: header menu's Hide column — a real narrowed projection round trip -------------------
  {
    op: DATA_OP.read,
    payload: readPayload(COMPOSITE_PATH, { projection: ['tenant_id', 'name'] }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          COMPOSITE_PK_COLUMNS[0] as (typeof COMPOSITE_PK_COLUMNS)[number],
          COMPOSITE_PK_COLUMNS[2] as (typeof COMPOSITE_PK_COLUMNS)[number],
        ],
        rows: [
          ['1', 'tenant 1 / entity 1'],
          ['1', 'tenant 1 / entity 2'],
          ['2', 'tenant 2 / entity 1'],
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
  // --- P7 D1/D6/D7: PK/FK cell nav — orders <-> customers, employees self-FK, order_items ------
  {
    op: DATA_OP.read,
    payload: readPayload(ORDERS_PATH),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'customer_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'ordered_at',
            dataType: 'timestamp with time zone',
            typeClass: 'temporal',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [
          ['1', '1', '2026-08-30 18:04:36.22154+00'],
          ['2', '2', '2026-08-30 18:04:36.22154+00'],
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
  {
    op: DATA_OP.read,
    payload: readPayload(ORDERS_PATH, { filter: `customer_id = 1` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'customer_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'ordered_at',
            dataType: 'timestamp with time zone',
            typeClass: 'temporal',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', '1', '2026-08-30 18:04:36.22154+00']],
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
  {
    op: DATA_OP.read,
    payload: readPayload(ORDERS_PATH, { filter: `id = 1` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'customer_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'ordered_at',
            dataType: 'timestamp with time zone',
            typeClass: 'temporal',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', '1', '2026-08-30 18:04:36.22154+00']],
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
  {
    op: DATA_OP.read,
    payload: readPayload(CUSTOMERS_PATH, { filter: `id = 1` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'region_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: true,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', 'Acme Co', '1']],
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
  {
    op: DATA_OP.read,
    payload: readPayload(EMPLOYEES_PATH),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'manager_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: true,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [
          ['1', 'Ada', null],
          ['2', 'Grace', '1'],
          ['3', 'Alan', '1'],
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
  {
    op: DATA_OP.read,
    payload: readPayload(EMPLOYEES_PATH, { filter: `manager_id = 1` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'manager_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: true,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [
          ['2', 'Grace', '1'],
          ['3', 'Alan', '1'],
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
  {
    op: DATA_OP.read,
    payload: readPayload(PRODUCTS_PATH, { filter: `id = 1` }),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'price',
            dataType: 'numeric(10,2)',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', 'Widget', '9.99']],
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
  // --- P67 §4.1/§7: the FK preview popover's own pageSize-10 fetch — a distinct mock entry from
  // the pageSize-100 reads above, one per case §7 exercises ---------------------------------------
  {
    op: DATA_OP.read,
    payload: previewPayload(CUSTOMERS_PATH, `id = 1`),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'region_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: true,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', 'Acme Co', '1']],
        position: {
          offset: 0,
          pageSize: 10,
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
  {
    op: DATA_OP.read,
    payload: previewPayload(PRODUCTS_PATH, `id = 1`),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'price',
            dataType: 'numeric(10,2)',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', 'Widget', '9.99']],
        position: {
          offset: 0,
          pageSize: 10,
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
  {
    op: DATA_OP.read,
    payload: previewPayload(ORDERS_PATH, `id = 1`),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'customer_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'ordered_at',
            dataType: 'timestamp with time zone',
            typeClass: 'temporal',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [['1', '1', '2026-08-30 18:04:36.22154+00']],
        position: {
          offset: 0,
          pageSize: 10,
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
  // P67 §7 case 4: a "no matching row" preview — orders row 1's customer_id (2) filtered against
  // customers comes back empty (an orphaned FK value, or the row deleted since the page loaded).
  {
    op: DATA_OP.read,
    payload: previewPayload(CUSTOMERS_PATH, `id = 2`),
    response: {
      kind: 'read',
      page: {
        kind: 'tabular',
        columns: [
          {
            name: 'id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: false,
            isPrimaryKey: true,
            generated: false,
          },
          {
            name: 'name',
            dataType: 'text',
            typeClass: 'text',
            nullable: false,
            isPrimaryKey: false,
            generated: false,
          },
          {
            name: 'region_id',
            dataType: 'integer',
            typeClass: 'number',
            nullable: true,
            isPrimaryKey: false,
            generated: false,
          },
        ],
        rows: [],
        position: {
          offset: 0,
          pageSize: 10,
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
];

async function menuItemIds(page: Page): Promise<string[]> {
  const menu = page.locator('[data-testid="context-menu"]');
  return menu
    .locator(
      '[data-testid^="menu-item-"]:not([data-testid$="-shortcut"]), [data-slot="dropdown-menu-separator"]',
    )
    .evaluateAll((els) =>
      els.map((el) =>
        el.getAttribute('data-slot') === 'dropdown-menu-separator'
          ? '--separator--'
          : (el.getAttribute('data-testid') ?? '').replace('menu-item-', ''),
      ),
    );
}

// Mirrors tests/ui/support/tree.ts's openRowMenu: Playwright's own actionability check can trigger
// an internal scroll-into-view whose 'scroll' event lands asynchronously right after the click
// opens a fresh menu, closing it before the next assertion sees it. Draining any pending scroll
// first avoids the race — this is for grid cell/row/header targets, not tree rows, so it stays a
// local helper rather than moving into support/tree.ts.
async function rightClick(locator: Locator): Promise<void> {
  const page = locator.page();
  const menu = page.locator('[data-testid="context-menu"]');
  for (let attempt = 0; attempt < 4; attempt++) {
    // SlickGrid can re-render the target row between the scroll and the click under load; a
    // detached element throws instead of retrying, so treat it as a failed attempt.
    try {
      await locator.scrollIntoViewIfNeeded();
      await settleFrames(page);
      await locator.click({ button: 'right' });
      await expect(menu).toBeVisible();
      await settleFrames(page);
      if (await menu.isVisible()) return;
    } catch (error) {
      if (attempt === 3) throw error;
    }
  }
  await expect(menu).toBeVisible();
}

async function openSubmenu(page: Page, triggerId: string): Promise<void> {
  await page.locator(`[data-testid="menu-item-${triggerId}"]`).hover();
  await expect(page.locator('[data-testid="context-submenu"]')).toBeVisible();
}

async function clipboardText(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText());
}

async function discardChanges(page: Page): Promise<void> {
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(page.locator('[data-testid="toolbar-commit-changes"]')).toHaveCount(0);
}

test('interaction completeness — grid menus, selection, copy/paste, shortcuts', async ({
  relaunch,
}) => {
  test.setTimeout(240_000);
  const { window: page } = await relaunch({ control: CONTROL, stream: PORT });
  await installClipboardShim(page);

  // See the file header comment: Playwright's WebKit always reports a Mac userAgent, so these
  // three chords are derived from the page's own `isMac` reading, not `process.platform`.
  const isMac = await page.evaluate(() => navigator.userAgent.includes('Mac'));
  const DUPLICATE_KEY = isMac ? 'Meta+d' : 'Control+d';
  const COPY_KEY = isMac ? 'Meta+c' : 'Control+c';
  const DELETE_KEY = isMac ? 'Meta+Backspace' : 'Delete';

  await page.click('[data-testid="toggle-operations-panel"]');
  await expect(page.locator('[data-testid="operations-panel"]')).toBeVisible();

  // The cell editor panel now only shows once a cell is actually selected (no more manual
  // toggle) — with no grid open yet, nothing is selected, so it starts hidden on its own.
  await expect(page.locator('[data-testid="cell-editor"]')).toHaveCount(0);

  await connectAndExpand(page, 'Interaction DB', 'green');

  const compositeRow = await findRow(page, COMPOSITE_PATH);
  await compositeRow.dblclick();
  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'name')).toBeVisible();

  // =============================================================================================
  // Cell context menu — Copy / Copy with header / Copy as JSON / Edit / Set NULL /
  // Filter by this value.
  // =============================================================================================
  const row0Name = await cellText(page, 0, 'name');
  expect(row0Name).toBe('tenant 1 / entity 1');

  await rightClick(gridCell(page, 0, 'name'));
  expect(await menuItemIds(page)).toEqual([
    'copy',
    'copy-with-header',
    'copy-as-json',
    'paste',
    '--separator--',
    'edit',
    'set-null',
    'delete-row',
    'filter-by-value',
  ]);

  // P21: shortcut hints — the grid's already-working Cmd/Ctrl+C/Enter print their key next to
  // the menu label.
  await expect(page.locator('[data-testid="menu-item-copy-shortcut"]')).toHaveText(
    /^(⌘C|Ctrl\+C)$/,
  );
  await expect(page.locator('[data-testid="menu-item-edit-shortcut"]')).toHaveText(/^(⏎|Enter)$/);

  await page.click('[data-testid="menu-item-copy"]');
  expect(await clipboardText(page)).toBe(row0Name);

  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-copy-with-header"]');
  expect(await clipboardText(page)).toBe(`name\n${row0Name}`);

  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-copy-as-json"]');
  expect(await clipboardText(page)).toBe(JSON.stringify(row0Name));

  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-edit"]');
  const cellInput = page.locator('[data-testid="grid-cell-input"]');
  await expect(cellInput).toBeVisible();
  await expect(cellInput).toHaveValue(row0Name);
  await cellInput.press('Escape');
  await expect(cellInput).toHaveCount(0);

  // Filter by this value replaces (D5) the WHERE box's effect — the "= value" branch, dialect
  // quoted with Postgres double quotes.
  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-filter-by-value"]');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  expect(await cellText(page, 0, 'name')).toBe(row0Name);

  await page.fill('[data-testid="filter-where-input"]', '');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3, { timeout: 10_000 });

  // Set NULL stages an actual SQL NULL, never a string — the inline <input> can't express this.
  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-set-null"]');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await expect(nullMarker(gridCell(page, 0, 'name'))).toHaveText('NULL');

  // Filter by this value on the now-null cell exercises the "IS NULL" branch — no fixture row
  // has a real NULL name, so 0 rows matching proves the generated clause is IS NULL, not = ''.
  await rightClick(gridCell(page, 0, 'name'));
  await page.click('[data-testid="menu-item-filter-by-value"]');
  await expect(page.locator('.no-rows')).toBeVisible({ timeout: 10_000 });

  // Staged changes are keyed by primary key (P174): the filter reload above kept the staged NULL,
  // and clearing the filter shows it again on its row. Revert it to restore the real value.
  await page.fill('[data-testid="filter-where-input"]', '');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3, { timeout: 10_000 });
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await rightClick(gutterCell(page, 0));
  await page.click('[data-testid="menu-item-revert-row"]');
  await expect(gridCell(page, 0, 'name')).not.toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe(row0Name);

  // =============================================================================================
  // Row selection accumulation (Shift range, Ctrl toggle) and the row context menu's
  // selection convention — right-clicking inside the current selection acts on all of it,
  // right-clicking outside it replaces the selection with just that row first.
  // =============================================================================================
  await gutterCell(page, 0).click();
  await gutterCell(page, 2).click({ modifiers: ['Shift'] }); // rows [0,1,2]
  await rightClick(gutterCell(page, 1)); // inside the selection -> acts on all 3
  await openSubmenu(page, 'copy-rows');
  await page.click('[data-testid="menu-item-copy-rows-tsv"]');
  const allRowsTsv = await clipboardText(page);
  expect(allRowsTsv.split('\n')).toHaveLength(3);
  expect(allRowsTsv).toContain('tenant 1 / entity 1');
  expect(allRowsTsv).toContain('tenant 1 / entity 2');
  expect(allRowsTsv).toContain('tenant 2 / entity 1');

  await gutterCell(page, 0).click(); // plain click -> replaces with [0]
  await rightClick(gutterCell(page, 2)); // outside [0] -> replaces with [2] alone
  await openSubmenu(page, 'copy-rows');
  await page.click('[data-testid="menu-item-copy-rows-csv"]');
  const row2Csv = await clipboardText(page);
  expect(row2Csv.trim()).toBe('2,1,tenant 2 / entity 1');

  await gutterCell(page, 0).click(); // [0]
  // Control+click (Meta on macOS): a literal Control+click, unlike a keyboard chord, becomes a
  // contextmenu event at the OS level on macOS regardless of the target's own click handler
  // (DataGrid.vue's onGutterClick toggle-select needs a real click event) — this repo's own
  // platform-aware TOGGLE_MODIFIER exists for exactly that reason, but this sandbox always runs
  // Linux/webkit, where Control+click is a real click, so 'Control' is used unconditionally here.
  await gutterCell(page, 2).click({ modifiers: ['Control'] }); // toggles row 2 in -> [0,2] disjoint
  await rightClick(gutterCell(page, 0)); // inside [0,2] -> acts on both
  await openSubmenu(page, 'copy-rows');
  await page.click('[data-testid="menu-item-copy-rows-json"]');
  const disjointJson = JSON.parse(await clipboardText(page)) as Array<Record<string, unknown>>;
  expect(disjointJson).toHaveLength(2);
  expect(disjointJson.map((r) => r.name).sort()).toEqual(
    ['tenant 1 / entity 1', 'tenant 2 / entity 1'].sort(),
  );

  // =============================================================================================
  // The same [0,1,2] row range as above, but built by a real mousedown -> mouseenter ->
  // mouseup drag across the gutter instead of shift-click. DataGrid.vue's onGutterMouseDown/
  // onGutterMouseEnter used to close over rowVm.row via a template call expression; they now read
  // it back off the row's own data-row attribute instead — a wrong attribute name here would fail
  // silently (no range ever forms) rather than throwing, so this is real coverage for that
  // rebinding, not just the click-based row selection already exercised above.
  // =============================================================================================
  await gutterCell(page, 0).hover();
  await page.mouse.down();
  await gutterCell(page, 1).hover();
  await gutterCell(page, 2).hover();
  await page.mouse.up();
  await rightClick(gutterCell(page, 1)); // inside the dragged-out [0,1,2] range -> acts on all 3
  await openSubmenu(page, 'copy-rows');
  await page.click('[data-testid="menu-item-copy-rows-tsv"]');
  const draggedRowsTsv = await clipboardText(page);
  expect(draggedRowsTsv.split('\n')).toHaveLength(3);
  expect(draggedRowsTsv).toContain('tenant 1 / entity 1');
  expect(draggedRowsTsv).toContain('tenant 1 / entity 2');
  expect(draggedRowsTsv).toContain('tenant 2 / entity 1');

  // Same drag gesture over grid cells instead of the gutter (SlickHybridSelectionModel's own
  // dragToSelect) — a same-column, multi-row drag keeps the resulting `kira-cell-selected` count
  // unambiguous: exactly one cell per row. `kira-cell-selected`, not the incumbent's own bare
  // `.selected`.
  await gridCell(page, 0, 'name').hover();
  await page.mouse.down();
  await gridCell(page, 1, 'name').hover();
  await gridCell(page, 2, 'name').hover();
  await page.mouse.up();
  await expect(page.locator('[data-testid="grid-cell"].kira-cell-selected')).toHaveCount(3);
  await expect(gridCell(page, 0, 'name')).toHaveClass(/kira-cell-selected/);
  await expect(gridCell(page, 1, 'name')).toHaveClass(/kira-cell-selected/);
  await expect(gridCell(page, 2, 'name')).toHaveClass(/kira-cell-selected/);

  // D6: Duplicate row(s) — non-PK columns copied, PK columns left blank.
  await gutterCell(page, 0).click();
  await rightClick(gutterCell(page, 0));
  await page.click('[data-testid="menu-item-duplicate-row"]');
  const insertRow = page.locator('[data-testid="grid-row-insert"]');
  await expect(insertRow).toHaveCount(1);
  const insertInputs = insertRow.locator('[data-testid="grid-cell-insert"] input');
  await expect(insertInputs.nth(0)).toHaveValue(''); // tenant_id (PK) left blank
  await expect(insertInputs.nth(1)).toHaveValue(''); // entity_id (PK) left blank
  await expect(insertInputs.nth(2)).toHaveValue('tenant 1 / entity 1'); // name copied
  await discardChanges(page);
  await expect(insertRow).toHaveCount(0);

  // Delete row(s) marks the acted-on rows struck-through, uncommitted.
  await gutterCell(page, 1).click();
  await rightClick(gutterCell(page, 1));
  await page.click('[data-testid="menu-item-delete-row"]');
  await expect(page.locator('[data-testid="grid-row"][data-row="1"]')).toHaveClass(
    /pending-delete/,
  );
  await discardChanges(page);

  // Revert row(s) un-stages a single row's pending edit, without a tab-wide Discard.
  const row0OriginalName = await cellText(page, 0, 'name');
  await gridCell(page, 0, 'name').dblclick();
  await page.locator('[data-testid="grid-cell-input"]').fill('reverted via row menu');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await rightClick(gutterCell(page, 0));
  await page.click('[data-testid="menu-item-revert-row"]');
  await expect(gridCell(page, 0, 'name')).not.toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe(row0OriginalName);

  // =============================================================================================
  // Header context menu — Sort asc/desc/Clear sort, Hide column/Show all columns,
  // Copy column name/values. The menu adds "Mark column as PII" as a trailing submenu.
  // =============================================================================================
  await rightClick(headerCell(page, 'entity_id'));
  expect(await menuItemIds(page)).toEqual([
    'sort-asc',
    'sort-desc',
    'clear-sort',
    '--separator--',
    'hide-column',
    'show-all-columns',
    '--separator--',
    'copy-column-name',
    'copy-column-values',
    '--separator--',
    'mark-pii',
  ]);
  await page.click('[data-testid="menu-item-sort-asc"]');
  await expect(headerCell(page, 'entity_id')).toHaveAttribute('data-sort', 'asc');

  await rightClick(headerCell(page, 'entity_id'));
  await page.click('[data-testid="menu-item-sort-desc"]');
  await expect(headerCell(page, 'entity_id')).toHaveAttribute('data-sort', 'desc');

  await rightClick(headerCell(page, 'entity_id'));
  await page.click('[data-testid="menu-item-clear-sort"]');
  // P22 Pass B — `.slick-sort-indicator` (F8) is always present on every sortable header (its own
  // modifier classes, not its own presence, say whether a column is actively sorted); the
  // incumbent's bare `.sort-indicator` never matched anything under this engine either way.
  await expect(page.locator('.slick-sort-indicator-asc, .slick-sort-indicator-desc')).toHaveCount(
    0,
  );

  await rightClick(headerCell(page, 'name'));
  await page.click('[data-testid="menu-item-copy-column-name"]');
  expect(await clipboardText(page)).toBe('name');

  await rightClick(headerCell(page, 'name'));
  await page.click('[data-testid="menu-item-copy-column-values"]');
  const columnValues = (await clipboardText(page)).split('\n');
  expect(columnValues).toEqual(
    expect.arrayContaining(['tenant 1 / entity 1', 'tenant 1 / entity 2', 'tenant 2 / entity 1']),
  );

  await rightClick(headerCell(page, 'entity_id'));
  await page.click('[data-testid="menu-item-hide-column"]');
  await expect(headerCell(page, 'entity_id')).toHaveCount(0);

  await rightClick(headerCell(page, 'tenant_id'));
  await page.click('[data-testid="menu-item-show-all-columns"]');
  await expect(headerCell(page, 'entity_id')).toBeVisible();

  // =============================================================================================
  // Local grid copy/paste via Ctrl+C/Ctrl+V — anchor-and-fill, rows beyond the loaded
  // page become new pending inserts. The grid binds the platform primary modifier (grid.paste).
  // =============================================================================================
  await page.evaluate(() => navigator.clipboard.writeText('typed via paste'));
  await gridCell(page, 0, 'name').click();
  await grid.focus();
  await page.keyboard.press(await primaryChord(page, 'v'));
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe('typed via paste');
  await discardChanges(page);

  // Two TSV rows starting at the last existing row: row 2 becomes a staged edit, the row past
  // the loaded page becomes a new pending insert.
  await page.evaluate(() =>
    navigator.clipboard.writeText('2\t1\tedited last row\n3\t3\tbrand new row'),
  );
  await gridCell(page, 2, 'tenant_id').click();
  await grid.focus();
  await page.keyboard.press(await primaryChord(page, 'v'));
  await expect(gridCell(page, 2, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 2, 'name')).toBe('edited last row');
  const pastedInsertRow = page.locator('[data-testid="grid-row-insert"]');
  await expect(pastedInsertRow).toHaveCount(1);
  const pastedInsertInputs = pastedInsertRow.locator('[data-testid="grid-cell-insert"] input');
  await expect(pastedInsertInputs.nth(0)).toHaveValue('3');
  await expect(pastedInsertInputs.nth(1)).toHaveValue('3');
  await expect(pastedInsertInputs.nth(2)).toHaveValue('brand new row');

  // Regression: pasting again onto that same staged insert row (selected via its gutter,
  // a 'row'-kind selection) must update the row already staged there, not append a second one.
  // The gutter lives in the frozen LEFT pane's own clone of this row, a
  // different DOM subtree than `pastedInsertRow` (the right pane's clone, the only one carrying
  // `data-testid="grid-row-insert"`) — `gutterCell`'s own page-row-scoped locator reaches it
  // regardless of which pane it's in. SlickGrid also marks an insert row's own identity
  // on the row (`kira-row-inserted`), not a per-cell `.gutter-cell.inserted` combo like the
  // incumbent did.
  const pastedRow = Number(await pastedInsertRow.getAttribute('data-row'));
  await page.evaluate(() => navigator.clipboard.writeText('3\t3\tupdated brand new row'));
  await gutterCell(page, pastedRow).click();
  await grid.focus();
  await page.keyboard.press(await primaryChord(page, 'v'));
  await expect(pastedInsertRow).toHaveCount(1);
  await expect(pastedInsertInputs.nth(0)).toHaveValue('3');
  await expect(pastedInsertInputs.nth(1)).toHaveValue('3');
  await expect(pastedInsertInputs.nth(2)).toHaveValue('updated brand new row');
  await discardChanges(page);

  // =============================================================================================
  // The grid cell's new Paste row — an existing Cmd/Ctrl+V handler.
  // =============================================================================================
  await page.evaluate(() => navigator.clipboard.writeText('typed via paste menu'));
  await rightClick(gridCell(page, 0, 'name'));
  await expect(page.locator('[data-testid="menu-item-paste-shortcut"]')).toHaveText(
    /^(⌘V|Ctrl\+V)$/,
  );
  await page.click('[data-testid="menu-item-paste"]');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe('typed via paste menu');
  await discardChanges(page);

  // =============================================================================================
  // Ctrl/Cmd+V inside the open cell editor is the browser's own paste, not the grid's
  // spread-down paste. A multi-line clipboard must not stage rows below, and the editor must hold
  // a line break exactly.
  // =============================================================================================
  const editorBefore = await cellText(page, 1, 'name');
  await page.evaluate(() => navigator.clipboard.writeText('line one\nline two'));
  await gridCell(page, 0, 'name').dblclick();
  const editorInput = page.locator('[data-testid="grid-cell-input"]');
  await expect(editorInput).toBeVisible();
  await page.keyboard.press('ControlOrMeta+V');
  await expect(editorInput).toBeVisible();
  expect(await cellText(page, 1, 'name')).toBe(editorBefore);
  await expect(gridCell(page, 1, 'name')).not.toHaveClass(/pending-edit/);
  await editorInput.fill('a\nb');
  await page.keyboard.press('Enter');
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  await gridCell(page, 0, 'name').dblclick();
  await expect(editorInput).toHaveValue('a\nb');
  await page.keyboard.press('Escape');
  await discardChanges(page);

  // =============================================================================================
  // The row menu's new Duplicate/Delete shortcuts — dispatched through the exact same
  // rowMenu() builder a right-click uses (runMenuShortcut), so a real keypress produces the same
  // pending-change state the menu item does.
  // =============================================================================================
  await rightClick(gutterCell(page, 1));
  await expect(page.locator('[data-testid="menu-item-delete-row-shortcut"]')).toHaveText(
    /^(Delete|⌘⌫)$/,
  );
  await expect(page.locator('[data-testid="menu-item-duplicate-row-shortcut"]')).toHaveText(
    /^(Ctrl\+D|⌘D)$/,
  );
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  await gutterCell(page, 1).click();
  await grid.focus();
  await page.keyboard.press(DELETE_KEY);
  await expect(page.locator('[data-testid="grid-row"][data-row="1"]')).toHaveClass(
    /pending-delete/,
  );
  await discardChanges(page);

  await gutterCell(page, 0).click();
  await grid.focus();
  await page.keyboard.press(DUPLICATE_KEY);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  await discardChanges(page);

  // =============================================================================================
  // A tab-scoped shortcut prints on the tab menu even though the key acts on the
  // *active* tab, not the one right-clicked.
  // =============================================================================================
  await page.locator('[data-testid="tab"]').first().click({ button: 'right' });
  await expect(page.locator('[data-testid="menu-item-close-shortcut"]')).toHaveText(
    /^(⌘W|Ctrl\+W)$/,
  );
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  // =============================================================================================
  // The project tree's own local keyboard shortcuts — Copy name, F2 rename, Ctrl/Cmd+D
  // duplicate — dispatch through menuForRow(), the same builder a right-click uses. The
  // guard: the tree's per-row Refresh never prints a shortcut, since F5 is the *active tab's*
  // refresh, a different command on a different object.
  // =============================================================================================
  await openRowMenu(page, '');
  await expect(page.locator('[data-testid="menu-item-refresh-shortcut"]')).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  const connRow = connectionRow(page, 'Interaction DB');
  await expect(connRow).toHaveCount(1);
  await connRow.click();
  // P110 I2-14: `.tree-row`'s literal `selected` class is gone -- `cn()` now folds selected into
  // `bg-select` via a `stateClass` computed, so `aria-selected` is the stable selected-state check.
  await expect(connRow).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press(COPY_KEY);
  expect(await clipboardText(page)).toBe('Interaction DB');

  // Rename moved into the connection's file: F2 opens nothing.
  await connRow.click();
  await page.keyboard.press('F2');
  await expect(page.locator('[data-testid="connection-file-dialog"]')).toHaveCount(0);

  // Ctrl/Cmd+D last — it's the one assertion here that leaves an extra connection row behind
  // (the duplicate), which would otherwise make `connectionRow`'s own filter ambiguous for
  // anything after it.
  const connectionCountBefore = await page
    .locator('[data-testid="tree-row"][data-kind="connection"]')
    .count();
  await connRow.click();
  await page.keyboard.press(DUPLICATE_KEY);
  await expect(page.locator('[data-testid="tree-row"][data-kind="connection"]')).toHaveCount(
    connectionCountBefore + 1,
  );

  // =============================================================================================
  // An outbound FK-cell click now opens a read-only preview popover, anchored at the click,
  // instead of jumping straight there — SPEC's own description of what clicking an FK cell does
  // ("shows the referenced row"). "Open in new tab" inside it is the unchanged jump
  // (navigateForeignKey), one click further in. A PK cell ("Referenced by")
  // is untouched — it names *tables*, plural, with no single record to preview.
  // =============================================================================================
  const ordersRow = await findRow(page, ORDERS_PATH);
  await ordersRow.dblclick();
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'customer_id')).toBeVisible();
  expect(await cellText(page, 0, 'customer_id')).toBe('1');

  // P67 §7 case 1: the popover opens and shows the referenced record, with no new tab.
  let tabCount = await page.locator('[data-testid="tab"]').count();
  await clickCellNav(page, 0, 'customer_id');
  await expect(fkPreview(page)).toBeVisible();
  await expect(fkPreview(page)).toContainText('customers');
  await expect(fkPreview(page)).toContainText('Acme Co');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount);

  // P67 §7 case 2: "Open in new tab" is P7's own unchanged jump, one click further in — this
  // replaces the three stale assertions that used to expect `clickCellNav` alone to open a tab.
  await fkPreview(page).locator('[data-testid="fk-preview-open"]').click();
  await expect(fkPreview(page)).toHaveCount(0);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount + 1);
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'name')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  expect(await cellText(page, 0, 'name')).toBe('Acme Co');

  // customers.id is referenced by exactly one table (orders.customer_id) — a "pk"-kind button,
  // single candidate, direct nav to the filtered referencing rows. The single
  // button only lives in a cell once it's hovered or selected (never a pure-CSS always-in-the-
  // DOM affordance the way the incumbent's own per-cell button was), so this needs its own
  // explicit selection first, same as `clickCellNav`'s own first step just below.
  await gridCell(page, 0, 'id').click();
  await expect(cellNavButton(page, 0, 'id')).toHaveAttribute('data-nav-kind', 'pk');
  tabCount = await page.locator('[data-testid="tab"]').count();
  await cellNavButton(page, 0, 'id').click();
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount + 1);
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'customer_id')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  expect(await cellText(page, 0, 'customer_id')).toBe('1');

  // =============================================================================================
  // Self-referencing FK — employees.manager_id -> employees.id. Ada (id 1) has no manager
  // (NULL): her manager_id cell renders no nav button at all (a missing/NULL source value
  // means there's no row to jump to). Her id cell's "Referenced by" is a single candidate
  // (employees itself) that opens a *new* tab on the same table, filtered to her direct reports.
  // =============================================================================================
  const employeesRow = await findRow(page, EMPLOYEES_PATH);
  await employeesRow.dblclick();
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'manager_id')).toBeVisible();
  expect(await cellText(page, 0, 'name')).toBe('Ada');
  await expect(nullMarker(gridCell(page, 0, 'manager_id'))).toHaveText('NULL');
  await gridCell(page, 0, 'manager_id').click();
  await expect(cellNavButton(page, 0, 'manager_id')).toHaveCount(0);
  // P67 §7 case 5: NULL source value still no-ops — no button to click, and no popover either.
  await expect(fkPreview(page)).toHaveCount(0);

  tabCount = await page.locator('[data-testid="tab"]').count();
  await clickCellNav(page, 0, 'id');
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount + 1);
  await expect(grid).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(2, { timeout: 10_000 });
  const reportNames = [await cellText(page, 0, 'name'), await cellText(page, 1, 'name')].sort();
  expect(reportNames).toEqual(['Alan', 'Grace']);

  // Row 0 here (either direct report — both have manager_id 1) has a nav button on its
  // manager_id cell, and the right-click cell menu offers the same navigation as a "Go to
  // referenced row" item (the button and the menu share one function, so they can never disagree
  // about what's navigable).
  await rightClick(gridCell(page, 0, 'manager_id'));
  const fkMenuIds = await menuItemIds(page);
  expect(fkMenuIds.some((id) => id.startsWith('go-to-referenced-'))).toBe(true);
  // P67 §7 case 6 / P89: "Edit referenced row" — editReferencedRow's only entry point (the
  // popover dropped its own, P89 §2).
  expect(fkMenuIds.some((id) => id.startsWith('edit-referenced-'))).toBe(true);
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="context-menu"]')).toHaveCount(0);

  // =============================================================================================
  // A row with two independent outbound FKs (order_items.order_id -> orders,
  // order_items.product_id -> products) gets two independent nav buttons, each targeting its own
  // table — the two never share or overwrite each other's target.
  // =============================================================================================
  const orderItemsRow = await findRow(page, ORDER_ITEMS_PATH);
  await orderItemsRow.dblclick();
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'order_id')).toBeVisible();
  await expect(headerCell(page, 'product_id')).toBeVisible();
  expect(await cellText(page, 0, 'order_id')).toBe('1');
  expect(await cellText(page, 0, 'product_id')).toBe('1');

  // The cell menu's "Edit referenced row" item (the popover dropped its own
  // "Edit this record") opens a new tab on the referenced table, lands the caret on its
  // first non-primary-key column ('name' — 'id' is products' own primary key) and enters edit
  // mode; typing into the dock stages into that NEW tab's own pending set — the *existing* edit
  // surface, never a second one.
  tabCount = await page.locator('[data-testid="tab"]').count();
  await rightClick(gridCell(page, 0, 'product_id'));
  const editMenuIds = await menuItemIds(page);
  const editItemId = editMenuIds.find((id) => id.startsWith('edit-referenced-'));
  expect(editItemId).toBeDefined();
  await page.click(`[data-testid="menu-item-${editItemId}"]`);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount + 1);
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'price')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  expect(await cellText(page, 0, 'name')).toBe('Widget');

  const cellEditorPanel = page.locator('[data-testid="cell-editor-panel"]');
  await expect(cellEditorPanel).toBeVisible();
  await expect(cellEditorPanel).toHaveAttribute('data-cell-key', /:0:name$/);

  // Real host key handling (this container is Linux), not the page's own `navigator.userAgent`
  // (WebKit reports 'Macintosh' unconditionally here, per this file's own header note) — selecting
  // all text inside the Monaco editor is the browser's native binding, not an app-level shortcut
  // gated by isMac. Mirrors cell-editor.spec.ts's own identical SELECT_ALL constant.
  const SELECT_ALL = process.platform === 'darwin' ? 'Meta+A' : 'Control+A';
  await cellEditorPanel.locator('.view-lines').click();
  await page.keyboard.press(SELECT_ALL);
  await page.keyboard.type('"Deluxe Widget"');
  await page.keyboard.press(await primaryChord(page, 'Enter'));
  await expect(gridCell(page, 0, 'name')).toHaveClass(/pending-edit/);
  expect(await cellText(page, 0, 'name')).toBe('"Deluxe Widget"');

  // P67 §7 case 2 (again): order_id's own "Open in new tab" — the last of the three stale
  // clickCellNav-then-expect-a-tab assertions this phase replaces.
  const orderItemsRowAgain = await findRow(page, ORDER_ITEMS_PATH);
  await orderItemsRowAgain.dblclick();
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'order_id')).toBeVisible();
  tabCount = await page.locator('[data-testid="tab"]').count();
  await clickCellNav(page, 0, 'order_id');
  await expect(fkPreview(page)).toBeVisible();
  await fkPreview(page).locator('[data-testid="fk-preview-open"]').click();
  await expect(fkPreview(page)).toHaveCount(0);
  await expect(page.locator('[data-testid="tab"]')).toHaveCount(tabCount + 1);
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'ordered_at')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1, { timeout: 10_000 });
  expect(await cellText(page, 0, 'customer_id')).toBe('1');

  // =============================================================================================
  // No matching row. orders row 1's customer_id (2) filtered against customers
  // comes back empty (an orphaned FK value, or the row deleted since the page loaded) — "Open in
  // new tab" stays offered.
  // =============================================================================================
  const ordersRowAgain = await findRow(page, ORDERS_PATH);
  await ordersRowAgain.dblclick();
  await expect(grid).toBeVisible();
  await expect(headerCell(page, 'customer_id')).toBeVisible();
  expect(await cellText(page, 1, 'customer_id')).toBe('2');

  await clickCellNav(page, 1, 'customer_id');
  await expect(fkPreview(page)).toBeVisible();
  await expect(fkPreview(page).locator('[data-testid="fk-preview-empty"]')).toBeVisible();
  await expect(fkPreview(page).locator('[data-testid="fk-preview-open"]')).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(fkPreview(page)).toHaveCount(0);
});
