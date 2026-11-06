import type { Page } from '@playwright/test';
import { DATA_OP } from '@shared/protocol/dataOps';
import { boxCenter, sortableHover } from '@workbench/testing/ui/drag';
import type { ControlSnapshot, LogicalPage } from '../../ipc/support/types';
import { MD5_ROWS } from '../../support/seeds/md5Rows';
import { expect, test } from './fixtures';
import { installClipboardSpy, lastClipboardWrite } from './support/clipboard';
import { connectAndExpand, connectionCreateArgs } from './support/connect';
import {
  cellText,
  gridCell,
  gridRow,
  gridScroller,
  gutterCell,
  nullMarker,
  sortIndicators,
} from './support/grid';
import { IPC } from './support/ipcChannels';
import { emitWailsEvent } from './support/mockRuntime';
import type { StudioPortSnapshot } from './support/mockStream';
import {
  BIG_ROWS_COLUMNS,
  BIG_ROWS_PATH,
  bigRowsFixture,
  bigRowsRows,
  NULLS_AND_UNICODE_PAGE,
  NULLS_META,
  NULLS_PATH,
  postgresConnectionSummary,
} from './support/postgresFixture';
import { findRow, settleFrames } from './support/tree';

// Ported from tests/e2e/data-view.spec.ts (P57 D16), against a real captured app.big_rows
// (1,000,000 rows, id/hash=md5(id) — packages/db-fixtures/fixtures/0001_seed.sql) and app.nulls_and_unicode,
// via scripts/capture-postgres-tree.ts (extended this session with a cursor backreference —
// "after:<i>"/"before:<i>", resolved against an earlier read step's own real nextToken/prevToken —
// and a `cancelAfterMs` step that races a real `adapter:cancel` against an in-flight read to
// capture a genuine mid-flight-cancellation error, rather than inventing one).
//
// Dropped outright:
// - The final "relaunch, saved filter survives" scenario (original lines ~572-596): a real
//   cross-process persistence check. `tests/ui/fixtures.ts`'s own header comment already rules
//   this out for the whole tier — there is nothing to persist to.
// - `OpRecordLike`/`getOps()`/`window.kira.opsRecent()` throughout: `window.kira` no longer
//   exists post-M2/M3 (CLAUDE.md P57 finding), so this has no live call to make at all. Every
//   place the original used an op-log count to prove "a real round trip happened" or "zero round
//   trips happened" is replaced by `stream.ops()` (this tier's own `mockStream.ts` handle, which
//   records every `PortRequest` the UI actually sent, independent of the op log) filtered by
//   op — a strictly more direct signal for exactly the same claim, needing no dead escape hatch.
//   The one op-log fact with no `stream.ops()` equivalent — a cancelled op's *op-log status*
//   turning 'cancelled' — is replaced by the DOM-visible consequence a real cancellation has
//   either way: the previous page stays on screen and the Stop/Refresh buttons flip back
//   (`can-stop`/`can-refresh` are driven by `rt.opId`, which `applyLoadFailure`'s cancelled branch
//   always clears).
// - The server-side L3 page-cache hit/miss distinction (original's "cache: revisiting the exact
//   same cursor is a hit" section): real (this harness's own capture run *did* observe a genuine
//   `source: 'cache'` reply on a repeat within one container), but not something a client-side
//   mock can honestly reproduce — this tier's mock has no cache of its own, it just replays
//   whichever fixture snapshot matches, and a real read response's `source` field is never
//   rendered anywhere in the DOM for this view (unlike `definition.spec.ts`'s `data-source`
//   attribute) for a UI-level test to observe either way. What still ports: the *pagination is
//   correct and deterministic* half — next/prev/next round-trips to the exact same real captured
//   page content, and a refresh produces exactly one new `data:read` request (`stream.ops()`).
// - `navigator.clipboard.readText()`: Playwright's clipboard-permission grant is Chromium-only
//   (this tier's own `playwright.config.ts` runs WebKit, matching a real packaged build) — ported
//   as a `writeText` spy (`installClipboardSpy`/`lastClipboardWrite` below) instead, checking the
//   same "one clipboard line per visible row" claim without depending on a real OS clipboard.
//
// Everything else — pagination (offset and keyset), page sizes, count/refresh, filter-driven
// re-reads and their invalidation of a stale count, jump-to-page, the Columns menu and its
// projection wire-through (checked via the actual `data:read` request's own `projection` field,
// `stream.ops()` again — more direct than the original's op-log `command` SQL-text substring
// check), sort (structured and free-text), the filter history/saved-filters flow, the find
// toolbar and P24's filter-mode row hiding, the viewport-first search scan's ordering guarantees,
// a real mid-flight Stop, and the NULL-vs-empty-string distinction — all port faithfully.
//
// `queriesList`/`queriesHistoryList`/`queriesSave` have no capture path at all: they are Go/SQLite
// storage (`QueriesService`), never routed through the Postgres adapter
// `scripts/capture-postgres-tree.ts`'s harness wires up, so `SAVED_SMALL_IDS`/`HISTORY_SMALL_IDS`
// below are schema-accurate fixture data (`FilterHistoryEntry`/`SavedFilterQuery`), the same
// category as `postgresConnectionSummary()`'s own hand-set timestamps — not a captured response.

const CONNECTION_ID = 'conn-data-view';
const CONNECTION_SUMMARY = postgresConnectionSummary(CONNECTION_ID, 'Data View DB', 'green');
const FIXTURE = bigRowsFixture(CONNECTION_ID);

const SAVED_SMALL_IDS = {
  id: 'saved-small-ids',
  connectionId: CONNECTION_ID,
  path: BIG_ROWS_PATH,
  name: 'Small ids',
  pinned: false,
  createdAt: '2026-01-01T00:00:00.000Z',
  usedAt: null,
  kind: 'filter' as const,
  body: { where: 'id <= 5', orderBy: null },
};

const HISTORY_SMALL_IDS = {
  id: 'hist-small-ids',
  connectionId: CONNECTION_ID,
  path: BIG_ROWS_PATH,
  where: 'id <= 5',
  orderBy: null,
  usedAt: '2026-01-01T00:00:00.000Z',
};

// Real captures against a real 1,000,000-row app.big_rows (id, hash=md5(id)) — id/1000/10000-row
// pages (D/E below) are NOT inlined verbatim: their content is fully deterministic
// (packages/db-fixtures/support/postgres.ts's own seed, `md5(i::text)`), verified byte-for-byte
// against the
// real capture's first/last row (id=1000 -> a9b7ba70783b617e9998dc4dd82eb3c5, id=10000 ->
// b7a782741f667201b54880c925faec4b,
// id=999991 -> 0ef26b9d4469882962b1bd35ef7556f4, id=1000000 -> 8155bc545f84d9652f1012ef2bdfb6eb),
// so recomputing them at test time avoids a multi-thousand-row literal array for content nothing
// here inspects past a handful of specific ids and the first/last gutter number. Every other page
// below (10/100-row) is small enough to keep as the literal real capture.

// The real nextToken FIXTURE's own initial pageSize-100 read (BIG_ROWS_FIRST_PAGE) came back
// with — chaining a real opaque keyset token across requests the same way the renderer does
// (goNext()/goPrev() feed the previous response's own token straight back), not a guess at its
// encoding. Every later token below is threaded the same way, off the *previous* PAGE_*'s own
// `position.nextToken`/`prevToken` rather than hardcoded again.
const BIG_ROWS_PAGE1_NEXT_TOKEN = 'eyJ2IjoxLCJrIjpbIjEwMCJdLCJmIjoiNzY3Y2NlNDM1NDhmNjc5ZiJ9';

const PAGE_B: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['101', '38b3eff8baf56627478ec76a704e9b52'],
    ['102', 'ec8956637a99787bd197eacd77acce5e'],
    ['103', '6974ce5ac660610b44d9b9fed0ff9548'],
    ['104', 'c9e1074f5b3f9fc8ea15d152add07294'],
    ['105', '65b9eea6e1cc6bb9f0cd2a47751a186f'],
    ['106', 'f0935e4cd5920aa6c7c996a5ee53a70f'],
    ['107', 'a97da629b098b75c294dffdc3e463904'],
    ['108', 'a3c65c2974270fd093ee8a9bf8ae7d0b'],
    ['109', '2723d092b63885e0d7c260cc007e8b9d'],
    ['110', '5f93f983524def3dca464469d2cf9f3e'],
    ['111', '698d51a19d8a121ce581499d7b701668'],
    ['112', '7f6ffaa6bb0b408017b62254211691b5'],
    ['113', '73278a4a86960eeb576a8fd4c9ec6997'],
    ['114', '5fd0b37cd7dbbb00f97ba6ce92bf5add'],
    ['115', '2b44928ae11fb9384c4cf38708677c48'],
    ['116', 'c45147dee729311ef5b5c3003946c48f'],
    ['117', 'eb160de1de89d9058fcb0b968dbbbd68'],
    ['118', '5ef059938ba799aaa845e1c2e8a762bd'],
    ['119', '07e1cd7dca89a1678042477183b7ac3f'],
    ['120', 'da4fb5c6e93e74d3df8527599fa62642'],
    ['121', '4c56ff4ce4aaf9573aa5dff913df997a'],
    ['122', 'a0a080f42e6f13b3a2df133f073095dd'],
    ['123', '202cb962ac59075b964b07152d234b70'],
    ['124', 'c8ffe9a587b126f152ed3d89a146b445'],
    ['125', '3def184ad8f4755ff269862ea77393dd'],
    ['126', '069059b7ef840f0c74a814ec9237b6ec'],
    ['127', 'ec5decca5ed3d6b8079e2e7e7bacc9f2'],
    ['128', '76dc611d6ebaafc66cc0879c71b5db5c'],
    ['129', 'd1f491a404d6854880943e5c3cd9ca25'],
    ['130', '9b8619251a19057cff70779273e95aa6'],
    ['131', '1afa34a7f984eeabdbb0a7d494132ee5'],
    ['132', '65ded5353c5ee48d0b7d48c591b8f430'],
    ['133', '9fc3d7152ba9336a670e36d0ed79bc43'],
    ['134', '02522a2b2726fb0a03bb19f2d8d9524d'],
    ['135', '7f1de29e6da19d22b51c68001e7e0e54'],
    ['136', '42a0e188f5033bc65bf8d78622277c4e'],
    ['137', '3988c7f88ebcb58c6ce932b957b6f332'],
    ['138', '013d407166ec4fa56eb1e1f8cbe183b9'],
    ['139', 'e00da03b685a0dd18fb6a08af0923de0'],
    ['140', '1385974ed5904a438616ff7bdb3f7439'],
    ['141', '0f28b5d49b3020afeecd95b4009adf4c'],
    ['142', 'a8baa56554f96369ab93e4f3bb068c22'],
    ['143', '903ce9225fca3e988c2af215d4e544d3'],
    ['144', '0a09c8844ba8f0936c20bd791130d6b6'],
    ['145', '2b24d495052a8ce66358eb576b8912c8'],
    ['146', 'a5e00132373a7031000fd987a3c9f87b'],
    ['147', '8d5e957f297893487bd98fa830fa6413'],
    ['148', '47d1e990583c9c67424d369f3414728e'],
    ['149', 'f2217062e9a397a1dca429e7d70bc6ca'],
    ['150', '7ef605fc8dba5425d6965fbd4c8fbe1f'],
    ['151', 'a8f15eda80c50adb0e71943adc8015cf'],
    ['152', '37a749d808e46495a8da1e5352d03cae'],
    ['153', 'b3e3e393c77e35a4a3f3cbd1e429b5dc'],
    ['154', '1d7f7abc18fcb43975065399b0d1e48e'],
    ['155', '2a79ea27c279e471f4d180b08d62b00a'],
    ['156', '1c9ac0159c94d8d0cbedc973445af2da'],
    ['157', '6c4b761a28b734fe93831e3fb400ce87'],
    ['158', '06409663226af2f3114485aa4e0a23b4'],
    ['159', '140f6969d5213fd0ece03148e62e461e'],
    ['160', 'b73ce398c39f506af761d2277d853a92'],
    ['161', 'bd4c9ab730f5513206b999ec0d90d1fb'],
    ['162', '82aa4b0af34c2313a562076992e50aa3'],
    ['163', '0777d5c17d4066b82ab86dff8a46af6f'],
    ['164', 'fa7cdfad1a5aaf8370ebeda47a1ff1c3'],
    ['165', '9766527f2b5d3e95d4a733fcfb77bd7e'],
    ['166', '7e7757b1e12abcb736ab9a754ffb617a'],
    ['167', '5878a7ab84fb43402106c575658472fa'],
    ['168', '006f52e9102a8d3be2fe5614f42ba989'],
    ['169', '3636638817772e42b59d74cff571fbb3'],
    ['170', '149e9677a5989fd342ae44213df68868'],
    ['171', 'a4a042cf4fd6bfb47701cbc8a1653ada'],
    ['172', '1ff8a7b5dc7a7d1f0ed65aaa29c04b1e'],
    ['173', 'f7e6c85504ce6e82442c770f7c8606f0'],
    ['174', 'bf8229696f7a3bb4700cfddef19fa23f'],
    ['175', '82161242827b703e6acf9c726942a1e4'],
    ['176', '38af86134b65d0f10fe33d30dd76442e'],
    ['177', '96da2f590cd7246bbde0051047b0d6f7'],
    ['178', '8f85517967795eeef66c225f7883bdcb'],
    ['179', '8f53295a73878494e9bc8dd6c3c7104f'],
    ['180', '045117b0e0a11a242b9765e79cbf113f'],
    ['181', 'fc221309746013ac554571fbd180e1c8'],
    ['182', '4c5bde74a8f110656874902f07378009'],
    ['183', 'cedebb6e872f539bef8c3f919874e9d7'],
    ['184', '6cdd60ea0045eb7a6ec44c54d29ed402'],
    ['185', 'eecca5b6365d9607ee5a9d336962c534'],
    ['186', '9872ed9fc22fc182d371c3e9ed316094'],
    ['187', '31fefc0e570cb3860f2a6d4b38c6490d'],
    ['188', '9dcb88e0137649590b755372b040afad'],
    ['189', 'a2557a7b2e94197ff767970b67041697'],
    ['190', 'cfecdb276f634854f3ef915e2e980c31'],
    ['191', '0aa1883c6411f7873cb83dacb17b0afc'],
    ['192', '58a2fc6ed39fd083f55d4182bf88826d'],
    ['193', 'bd686fd640be98efaae0091fa301e613'],
    ['194', 'a597e50502f5ff68e3e25b9114205d4a'],
    ['195', '0336dcbab05b9d5ad24f4333c7658a0e'],
    ['196', '084b6fbb10729ed4da8c3d3f5a3ae7c9'],
    ['197', '85d8ce590ad8981ca2c8286f79f59954'],
    ['198', '0e65972dce68dad4d52d063967f0a705'],
    ['199', '84d9ee44e457ddef7f2c4f25dc8fa865'],
    ['200', '3644a684f98ea8fe223c713b77189a77'],
  ],
  position: {
    offset: null,
    pageSize: 100,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjIwMCJdLCJmIjoiNzY3Y2NlNDM1NDhmNjc5ZiJ9',
    prevToken: 'eyJ2IjoxLCJrIjpbIjEwMSJdLCJmIjoiNzY3Y2NlNDM1NDhmNjc5ZiJ9',
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_C: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: MD5_ROWS,
  position: {
    offset: null,
    pageSize: 100,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwMCJdLCJmIjoiNzY3Y2NlNDM1NDhmNjc5ZiJ9',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_D: LogicalPage = {
  kind: 'tabular' as const,
  columns: BIG_ROWS_COLUMNS,
  rows: bigRowsRows(1000, 1),
  position: {
    offset: 0,
    pageSize: 1000,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwMDAiXSwiZiI6IjIwMGE4OWM3NGQyMmNmNzAifQ',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_E: LogicalPage = {
  kind: 'tabular' as const,
  columns: BIG_ROWS_COLUMNS,
  rows: bigRowsRows(10000, 1),
  position: {
    offset: 0,
    pageSize: 10000,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwMDAwIl0sImYiOiI1YWIwNzAyNWZmMWNkZmE1In0',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_F: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['1', 'c4ca4238a0b923820dcc509a6f75849b'],
    ['2', 'c81e728d9d4c2f636f067f89cc14862c'],
    ['3', 'eccbc87e4b5ce2fe28308fd9f2a7baf3'],
    ['4', 'a87ff679a2f3e71d9181a67b7542122c'],
    ['5', 'e4da3b7fbbce2345d7772b0674a318d5'],
    ['6', '1679091c5a880faf6fb5e6087eb1b2dc'],
    ['7', '8f14e45fceea167a5a36dedd4bea2543'],
    ['8', 'c9f0f895fb98ab9159f51fd0297e236d'],
    ['9', '45c48cce2e2d7fbdea1afc51c7c6ad26'],
    ['10', 'd3d9446802a44259755d38e6d163e820'],
  ],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwIl0sImYiOiIyMmQ0ZmE1Y2YxYjUwMDIzIn0',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const COUNT_ALL = {
  payload: {
    opId: 'capture-count-7',
    tabId: null,
    connectionId: 'test-postgres',
    path: 'database:kira_test/schema:app/table:big_rows',
    filter: null,
    refresh: false,
  },
  value: 1000000,
  exact: true,
  at: 1788113423709,
  stale: false,
  source: 'server',
} as const;
const PAGE_H: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['11', '6512bd43d9caa6e02c990b0a82652dca'],
    ['12', 'c20ad4d76fe97759aa27a0c99bff6710'],
    ['13', 'c51ce410c124a10e0db5e4b97fc2af39'],
    ['14', 'aab3238922bcc25a6f606eb525ffdc56'],
    ['15', '9bf31c7ff062936a96d3c8bd1f8f2ff3'],
    ['16', 'c74d97b01eae257e44aa9d5bade97baf'],
    ['17', '70efdf2ec9b086079795c442636b55fb'],
    ['18', '6f4922f45568161a8cdf4ad2299f6d23'],
    ['19', '1f0e3dad99908345f7439f8ffabdffc4'],
    ['20', '98f13708210194c475687be6106a3b84'],
  ],
  position: {
    offset: null,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjIwIl0sImYiOiIyMmQ0ZmE1Y2YxYjUwMDIzIn0',
    prevToken: 'eyJ2IjoxLCJrIjpbIjExIl0sImYiOiIyMmQ0ZmE1Y2YxYjUwMDIzIn0',
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_I: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['11', '6512bd43d9caa6e02c990b0a82652dca'],
    ['12', 'c20ad4d76fe97759aa27a0c99bff6710'],
    ['13', 'c51ce410c124a10e0db5e4b97fc2af39'],
    ['14', 'aab3238922bcc25a6f606eb525ffdc56'],
    ['15', '9bf31c7ff062936a96d3c8bd1f8f2ff3'],
    ['16', 'c74d97b01eae257e44aa9d5bade97baf'],
    ['17', '70efdf2ec9b086079795c442636b55fb'],
    ['18', '6f4922f45568161a8cdf4ad2299f6d23'],
    ['19', '1f0e3dad99908345f7439f8ffabdffc4'],
    ['20', '98f13708210194c475687be6106a3b84'],
  ],
  position: {
    offset: 10,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjIwIl0sImYiOiIyMmQ0ZmE1Y2YxYjUwMDIzIn0',
    prevToken: 'eyJ2IjoxLCJrIjpbIjExIl0sImYiOiIyMmQ0ZmE1Y2YxYjUwMDIzIn0',
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_J: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['1', 'c4ca4238a0b923820dcc509a6f75849b'],
    ['2', 'c81e728d9d4c2f636f067f89cc14862c'],
    ['3', 'eccbc87e4b5ce2fe28308fd9f2a7baf3'],
    ['4', 'a87ff679a2f3e71d9181a67b7542122c'],
    ['5', 'e4da3b7fbbce2345d7772b0674a318d5'],
  ],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: false,
    nextToken: null,
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const COUNT_SMALL = {
  payload: {
    opId: 'capture-count-11',
    tabId: null,
    connectionId: 'test-postgres',
    path: 'database:kira_test/schema:app/table:big_rows',
    filter: 'id <= 5',
    refresh: false,
  },
  value: 5,
  exact: true,
  at: 1788113423724,
  stale: false,
  source: 'server',
} as const;
const PAGE_L: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['999991', '0ef26b9d4469882962b1bd35ef7556f4'],
    ['999992', 'cfd61ee8372d95d7217e1079c9c9b24f'],
    ['999993', '54259eb8b3dbd1ae7128ba33b451222d'],
    ['999994', '6ce6a3428657d7320506a95a8cc93747'],
    ['999995', '42f58798317292157b589727933614d8'],
    ['999996', '938d819b43f403e586fa498f55dbdcce'],
    ['999997', '9467a6b1a3f53c528087f9976ac934a3'],
    ['999998', '755af25720023b2f852105910b125ecc'],
    ['999999', '52c69e3a57331081823331c4e69d3f2e'],
    ['1000000', '8155bc545f84d9652f1012ef2bdfb6eb'],
  ],
  position: {
    offset: 999990,
    pageSize: 10,
    hasMore: false,
    nextToken: null,
    prevToken: 'eyJ2IjoxLCJrIjpbIjk5OTk5MSJdLCJmIjoiMjJkNGZhNWNmMWI1MDAyMyJ9',
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_N: LogicalPage = {
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
  ],
  rows: [['1'], ['2'], ['3'], ['4'], ['5'], ['6'], ['7'], ['8'], ['9'], ['10']],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwIl0sImYiOiI5OWYwOTc1ODRhYTkyODQxIn0',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_P: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['1', 'c4ca4238a0b923820dcc509a6f75849b'],
    ['2', 'c81e728d9d4c2f636f067f89cc14862c'],
    ['3', 'eccbc87e4b5ce2fe28308fd9f2a7baf3'],
    ['4', 'a87ff679a2f3e71d9181a67b7542122c'],
    ['5', 'e4da3b7fbbce2345d7772b0674a318d5'],
    ['6', '1679091c5a880faf6fb5e6087eb1b2dc'],
    ['7', '8f14e45fceea167a5a36dedd4bea2543'],
    ['8', 'c9f0f895fb98ab9159f51fd0297e236d'],
    ['9', '45c48cce2e2d7fbdea1afc51c7c6ad26'],
    ['10', 'd3d9446802a44259755d38e6d163e820'],
  ],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjEwIl0sImYiOiIwNzdkY2YxZDc5MWFhZDVkIn0',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_Q: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['1000000', '8155bc545f84d9652f1012ef2bdfb6eb'],
    ['999999', '52c69e3a57331081823331c4e69d3f2e'],
    ['999998', '755af25720023b2f852105910b125ecc'],
    ['999997', '9467a6b1a3f53c528087f9976ac934a3'],
    ['999996', '938d819b43f403e586fa498f55dbdcce'],
    ['999995', '42f58798317292157b589727933614d8'],
    ['999994', '6ce6a3428657d7320506a95a8cc93747'],
    ['999993', '54259eb8b3dbd1ae7128ba33b451222d'],
    ['999992', 'cfd61ee8372d95d7217e1079c9c9b24f'],
    ['999991', '0ef26b9d4469882962b1bd35ef7556f4'],
  ],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: true,
    nextToken: 'eyJ2IjoxLCJrIjpbIjk5OTk5MSJdLCJmIjoiYmZkNTZiODM0M2Y5ZGRjNyJ9',
    prevToken: null,
    strategy: 'keyset',
  },
  truncatedCells: 0,
};
const PAGE_S: LogicalPage = {
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
      name: 'hash',
      dataType: 'text',
      typeClass: 'text',
      nullable: false,
      isPrimaryKey: false,
      generated: false,
    },
  ],
  rows: [
    ['1', 'c4ca4238a0b923820dcc509a6f75849b'],
    ['2', 'c81e728d9d4c2f636f067f89cc14862c'],
    ['3', 'eccbc87e4b5ce2fe28308fd9f2a7baf3'],
    ['4', 'a87ff679a2f3e71d9181a67b7542122c'],
    ['5', 'e4da3b7fbbce2345d7772b0674a318d5'],
    ['6', '1679091c5a880faf6fb5e6087eb1b2dc'],
    ['7', '8f14e45fceea167a5a36dedd4bea2543'],
    ['8', 'c9f0f895fb98ab9159f51fd0297e236d'],
    ['9', '45c48cce2e2d7fbdea1afc51c7c6ad26'],
    ['10', 'd3d9446802a44259755d38e6d163e820'],
  ],
  position: {
    offset: 0,
    pageSize: 10,
    hasMore: true,
    nextToken: null,
    prevToken: null,
    strategy: 'offset',
  },
  truncatedCells: 0,
};
const INVALID_FILTER_ERROR = {
  message: 'syntax error at or near "sql"',
  code: 'E_QUERY',
};
const CANCEL_ERROR = {
  message: 'operation was cancelled',
  code: 'E_CANCELLED',
};
const CONTROL: ControlSnapshot[] = [
  { channel: IPC.connectionsList, response: [] },
  {
    channel: IPC.connectionsCreate,
    args: connectionCreateArgs('Data View DB', 'green'),
    response: CONNECTION_SUMMARY,
  },
  ...FIXTURE.control,
  {
    channel: IPC.treeDescribe,
    args: { connectionId: CONNECTION_ID, path: NULLS_PATH, refresh: false, tabId: null },
    response: { meta: NULLS_META, source: 'server' },
  },
  // The filter-history/saved-filters flow (queriesList's own two-call sequence: empty before the
  // save, carrying the one saved entry after — see the header comment on why these are
  // schema-accurate fixture data rather than a capture).
  {
    channel: IPC.queriesList,
    args: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH },
    response: [],
  },
  {
    channel: IPC.queriesList,
    args: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH },
    response: [SAVED_SMALL_IDS],
  },
  {
    channel: IPC.queriesHistoryList,
    args: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH, limit: 20 },
    response: [HISTORY_SMALL_IDS],
  },
  {
    channel: IPC.queriesSave,
    args: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      name: 'Small ids',
      body: { where: 'id <= 5', orderBy: null },
      pinned: false,
    },
    response: SAVED_SMALL_IDS,
  },
];

const PORT: StudioPortSnapshot[] = [
  ...FIXTURE.port, // pageSize 100, offset 0 (real capture, step 1 below)
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'after', token: BIG_ROWS_PAGE1_NEXT_TOKEN },
    },
    response: { kind: 'read', page: PAGE_B, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'before', token: PAGE_B.position.prevToken },
    },
    response: { kind: 'read', page: PAGE_C, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 1000,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_D, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10000,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_E, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_F, source: 'server' },
  },
  {
    op: DATA_OP.count,
    payload: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH, filter: null, refresh: false },
    response: {
      kind: 'count',
      value: COUNT_ALL.value,
      exact: COUNT_ALL.exact,
      stale: COUNT_ALL.stale,
      source: COUNT_ALL.source,
    },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10,
      cursor: { mode: 'after', token: PAGE_F.position.nextToken },
    },
    response: { kind: 'read', page: PAGE_H, source: 'server' },
  },
  // toolbar-refresh's own data.invalidate() call, before it reloads the page below — must be
  // answered or the whole async function throws and the reload read never runs (mutations.spec.ts's
  // own precedent finding).
  {
    op: DATA_OP.invalidate,
    payload: { connectionId: CONNECTION_ID, path: BIG_ROWS_PATH },
    response: { kind: 'invalidate' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 10 },
    },
    response: { kind: 'read', page: PAGE_I, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: 'id <= 5',
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_J, source: 'server' },
  },
  {
    op: DATA_OP.count,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      filter: 'id <= 5',
      refresh: false,
    },
    response: {
      kind: 'count',
      value: COUNT_SMALL.value,
      exact: COUNT_SMALL.exact,
      stale: COUNT_SMALL.stale,
      source: COUNT_SMALL.source,
    },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 999990 },
    },
    response: { kind: 'read', page: PAGE_L, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: ['id'],
      filter: null,
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_N, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: { kind: 'structured', terms: [{ column: 'id', direction: 'asc' }] },
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_P, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: { kind: 'structured', terms: [{ column: 'id', direction: 'desc' }] },
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_Q, source: 'server' },
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: null,
      sort: { kind: 'text', text: 'id ASC' },
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: PAGE_S, source: 'server' },
  },
  // A real Postgres syntax error (scripts/capture-postgres-tree.ts's own 'read' step now catches
  // one, the same way 'mutate'/'execute' already did).
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: 'not valid sql (((',
      sort: null,
      pageSize: 10,
      cursor: { mode: 'offset', offset: 0 },
    },
    error: INVALID_FILTER_ERROR,
  },
  // A real mid-flight cancellation (adapter:cancel raced against this read — see the header
  // comment and scripts/capture-postgres-tree.ts's own `cancelAfterMs` doc comment). `delayMs`
  // gives the test a real window to click Stop before this resolves, mirroring the pg_sleep(2)
  // the original spec used for the same reason.
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: BIG_ROWS_PATH,
      projection: null,
      filter: '(SELECT pg_sleep(2)) IS NULL OR id > 0',
      sort: null,
      pageSize: 10000,
      cursor: { mode: 'offset', offset: 0 },
    },
    error: CANCEL_ERROR,
    untilCancel: true,
  },
  {
    op: DATA_OP.read,
    payload: {
      connectionId: CONNECTION_ID,
      path: NULLS_PATH,
      projection: null,
      filter: null,
      sort: null,
      pageSize: 100,
      cursor: { mode: 'offset', offset: 0 },
    },
    response: { kind: 'read', page: NULLS_AND_UNICODE_PAGE, source: 'server' },
  },
];

// The grid is virtualized — only the scrolled-into-view + overscan rows exist in the DOM at any
// moment, so "N rows loaded" is asserted by scrolling to each end and reading the gutter, not by
// counting DOM nodes (ported from tests/e2e/data-view.spec.ts unchanged — real Electron hit the
// identical virtualization).
async function scrollGridToBottom(page: Page): Promise<void> {
  const grid = gridScroller(page);
  await grid.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await settleFrames(page);
}

async function firstGutterNumber(page: Page): Promise<string> {
  const grid = gridScroller(page);
  await grid.evaluate((el) => {
    el.scrollTop = 0;
  });
  // Page row 0, not the first DOM node: SlickGrid recycles row nodes, so under load the first node
  // still holds a stale far row until the post-scroll render lands.
  return (await gutterCell(page, 0).innerText()).trim();
}

async function lastGutterNumber(page: Page): Promise<string> {
  await scrollGridToBottom(page);
  // Highest page row, not the last DOM node (recycled nodes break DOM order, see above).
  return page.evaluate(() => {
    let best: { row: number; text: string } | null = null;
    for (const el of document.querySelectorAll(
      '[data-testid="data-grid"] .slick-row[data-row] [data-testid="grid-gutter-cell"]',
    )) {
      const row = Number(el.closest('.slick-row')?.getAttribute('data-row'));
      if (!best || row > best.row) best = { row, text: (el.textContent ?? '').trim() };
    }
    return best?.text ?? '';
  });
}

// Playwright's clipboard-permission grant (`browserContext.grantPermissions(['clipboard-read'])`)
// is Chromium-only — this tier runs WebKit (playwright.config.ts's `ui` project, matching what a
// real packaged build embeds), which has no such grant to make. Spying on `writeText` proves the
// same "one clipboard line per visible row" claim the original's `navigator.clipboard.readText()`
// did, without depending on a real OS clipboard round trip at all.
async function readOpsCount(
  stream: { ops(): Promise<{ op: string }[]> },
  op: string,
): Promise<number> {
  return (await stream.ops()).filter((o) => o.op === op).length;
}

test('data view — pagination, count, projection, sort, filter, search, stop, NULLs', async ({
  relaunch,
  consoleErrors,
}) => {
  test.setTimeout(120_000);
  const { window: page, stream, control } = await relaunch({ control: CONTROL, stream: PORT });
  await installClipboardSpy(page);
  await connectAndExpand(page, 'Data View DB', 'green');

  // --- open: 100 rows, gutter starts at 1, header shows column names ----------------------
  const bigRowsRowLoc = await findRow(page, BIG_ROWS_PATH);
  await bigRowsRowLoc.dblclick();

  const grid = page.locator('[data-testid="data-grid"]');
  await expect(grid).toBeVisible();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="id"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-header-cell"][data-column="hash"]')).toBeVisible();
  await expect.poll(() => firstGutterNumber(page), { timeout: 15_000 }).toBe('1');
  await expect.poll(() => lastGutterNumber(page)).toBe('100');

  // --- The pager is back in DataToolbar, at the toolbar's reading edge and immediately before the
  // page-size picker it pages through, rather than alone at the far right. Relative positions, not
  // pixel ones, so a token or spacing change still cannot break these. ----------------------------
  const pagerBox = await page.locator('[data-testid="pager"]').boundingBox();
  const pageSizeBox = await page.locator('[data-testid="page-size-picker"]').boundingBox();
  const searchToggleBox = await page.locator('[data-testid="toolbar-search"]').boundingBox();
  if (!pagerBox || !pageSizeBox || !searchToggleBox) {
    throw new Error('pager, page-size-picker or toolbar-search has no bounding box');
  }
  expect(pagerBox.x).toBeLessThan(pageSizeBox.x);
  expect(pagerBox.x).toBeLessThan(searchToggleBox.x);
  await expect(page.locator('[data-testid="pager-next"]')).toBeEnabled();

  // --- Dividers match the graph toolbar's (repo-graph-toolbar.spec.ts): 1x14, centred, transparent spacer.
  const dataBar = await page.locator('[data-testid="data-toolbar"]').boundingBox();
  if (!dataBar) throw new Error('data-toolbar has no bounding box');
  const separators = page.locator('[data-testid="data-toolbar"] [data-testid="toolbar-separator"]');
  expect(await separators.count()).toBeGreaterThanOrEqual(1);
  for (let i = 0; i < (await separators.count()); i++) {
    const box = await separators.nth(i).boundingBox();
    if (!box) throw new Error('separator has no bounding box');
    expect(box.width).toBe(1);
    expect(box.height).toBe(14);
    expect(Math.abs(box.y + box.height / 2 - (dataBar.y + dataBar.height / 2))).toBeLessThanOrEqual(
      0.5,
    );
  }

  // --- The assertion exists to prove nothing reserves width between the toolbar's right-most
  // control and its right edge (the reflow it fixed). The pager used to be that control; with it
  // moved back to the reading edge, RunState is — it still renders ahead of #toolbar-end, whose
  // only remaining content is the conditional pending-changes group. Every control the user reads
  // left-to-right now sits left of it. -----------------------
  const dataToolbarBox = await page.locator('[data-testid="data-toolbar"]').boundingBox();
  const runStateBox = await page.locator('[data-testid="run-state"]').first().boundingBox();
  if (!dataToolbarBox || !runStateBox) {
    throw new Error('data-toolbar or run-state has no bounding box');
  }
  // 16, not the 12: that bound was calibrated against the pager's last IconButton, whose
  // box hugs the toolbar edge, and RunState carries its own trailing padding. The gap measures 14.
  // Still tight enough to catch the ~76px unconditional reservation, which is the whole point
  // of the assertion.
  expect(
    dataToolbarBox.x + dataToolbarBox.width - (runStateBox.x + runStateBox.width),
  ).toBeLessThanOrEqual(16);
  expect(runStateBox.x).toBeGreaterThan(searchToggleBox.x);

  // --- P16 D2 (LAW-12): the run-state label is the toolbar's right-most item, so its text
  // changing between loads can never move a control that sits left of it. ----------------------
  const runStateLabel = page.locator('[data-testid="run-state-label"]').first();
  await expect(runStateLabel).toBeVisible();
  const leftEdges = () =>
    page.evaluate(() =>
      ['pager', 'page-size-picker', 'toolbar-search'].map(
        (id) => document.querySelector(`[data-testid="${id}"]`)?.getBoundingClientRect().x ?? -1,
      ),
    );
  const edgesAfterFirstLoad = await leftEdges();
  expect(edgesAfterFirstLoad.every((x) => x >= 0)).toBe(true);
  // A second, independently-timed load produces its own elapsed-time text.
  await page.click('[data-testid="toolbar-refresh"]');
  await expect.poll(leftEdges).toEqual(edgesAfterFirstLoad);

  // --- pagination: next/prev/first, page sizes ---------------------------------------------
  await page.click('[data-testid="pager-next"]');
  await expect.poll(() => firstGutterNumber(page)).toBe('101');

  await page.click('[data-testid="pager-prev"]');
  await expect.poll(() => firstGutterNumber(page)).toBe('1');

  await page.click('[data-testid="page-size-1000"]');
  await expect.poll(() => lastGutterNumber(page), { timeout: 15_000 }).toBe('1000');

  await page.click('[data-testid="page-size-10000"]');
  await expect.poll(() => lastGutterNumber(page), { timeout: 15_000 }).toBe('10000');

  await page.click('[data-testid="page-size-10"]');
  await expect.poll(() => lastGutterNumber(page), { timeout: 15_000 }).toBe('10');

  // --- count: Σ fills in "of N" pages; survives a page change; refresh recounts on demand ---
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
  await expect(countTip).toContainText(/1,000,000/, { timeout: 15_000 });
  await expect(page.locator('[data-testid="pager"]')).toContainText('of 100,000');

  await page.click('[data-testid="pager-next"]');
  await hoverCount();
  await expect(countTip).toContainText(/1,000,000/);

  const countsBeforeRefresh = await readOpsCount(stream, DATA_OP.count);
  await page.click('[data-testid="toolbar-refresh"]');
  // Refresh invalidates the count asynchronously; no DOM signal marks it done before the count click.
  await page.waitForTimeout(200);
  await countButton.click();
  await expect.poll(() => readOpsCount(stream, DATA_OP.count)).toBe(countsBeforeRefresh + 1);

  // --- P43 F7/D10: a filter change invalidates the count -----------------------------------
  await page.fill('[data-testid="filter-where-input"]', 'id <= 5');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await hoverCount();
  await expect(countTip).toContainText('Count all rows');
  await expect(page.locator('[data-testid="pager-last"]')).toBeDisabled();
  await countButton.click();
  await hoverCount();
  await expect(countTip).toContainText(/(?<!\d)5 rows in total/, { timeout: 15_000 });
  await expect(page.locator('[data-testid="pager-last"]')).toBeEnabled();

  await page.fill('[data-testid="filter-where-input"]', '');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await hoverCount();
  await expect(countTip).toContainText('Count all rows');
  await countButton.click();
  await hoverCount();
  await expect(countTip).toContainText(/1,000,000/, { timeout: 15_000 });

  // The page input only reacts to a native `change` event, which Enter does not fire on its own —
  // Tab moves focus away and blurs it, which does.
  await page.fill('[data-testid="pager-page-input"]', '100000');
  await page.press('[data-testid="pager-page-input"]', 'Tab');
  await expect.poll(() => firstGutterNumber(page), { timeout: 15_000 }).toBe('999991');
  await expect.poll(() => lastGutterNumber(page)).toBe('1000000');

  // --- P21 round 3 functional finding 12: a jump past the last known page must clamp to it,
  // not silently request an offset far beyond the end (page 99 of 3, in the finding's own
  // scenario), and the box must show the page it actually landed on rather than what was typed.
  await page.fill('[data-testid="pager-page-input"]', '999999999');
  await page.press('[data-testid="pager-page-input"]', 'Tab');
  await expect.poll(() => firstGutterNumber(page), { timeout: 15_000 }).toBe('999991');
  await expect.poll(() => lastGutterNumber(page)).toBe('1000000');
  await expect(page.locator('[data-testid="pager-page-input"]')).toHaveValue('100000');

  // A rejected jump (below the 1-page floor) must resync the box to the actual current page
  // instead of leaving it showing the rejected value forever — the grid itself must not move.
  await page.fill('[data-testid="pager-page-input"]', '0');
  await page.press('[data-testid="pager-page-input"]', 'Tab');
  await expect(page.locator('[data-testid="pager-page-input"]')).toHaveValue('100000');
  await expect.poll(() => firstGutterNumber(page)).toBe('999991');

  await page.click('[data-testid="pager-first"]');
  await expect.poll(() => firstGutterNumber(page)).toBe('1');

  // --- Columns button: opening/closing the menu without changing anything must not stamp a
  // "changed" indicator (P16's own bug, P31's dot-based version of the same guard). -------------
  await page.click('[data-testid="toolbar-columns"]');
  await expect(page.locator('[data-testid="columns-menu"]')).toBeVisible();
  await page.keyboard.press('Escape'); // P104: reka Popover has no backdrop element to click
  await expect(page.locator('[data-testid="toolbar-columns"]')).not.toHaveClass(/has-indicator/);

  // --- projection: half the columns, header shrinks, the request itself carries only those ----
  await page.click('[data-testid="toolbar-columns"]');
  await expect(page.locator('[data-testid="columns-menu"]')).toBeVisible();
  const columnItems = page.locator('[data-testid="columns-menu-item"]');
  await columnItems.nth(1).click(); // uncheck "hash", leaving only "id"
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="grid-header-cell"]')).toHaveCount(1, {
    timeout: 10_000,
  });
  const readsAfterProjection = (await stream.ops()).filter((o) => o.op === DATA_OP.read);
  const lastRead = readsAfterProjection[readsAfterProjection.length - 1] as
    | { payload: { projection: string[] | null } }
    | undefined;
  expect(lastRead?.payload.projection).toEqual(['id']);

  // Restore the full projection for the remaining scenarios.
  await page.click('[data-testid="toolbar-columns"]');
  await page.locator('[data-testid="columns-select-all"]').click();
  await page.keyboard.press('Escape');
  await expect(page.locator('[data-testid="grid-header-cell"]')).toHaveCount(2, {
    timeout: 10_000,
  });

  // --- column order: dragging a Columns menu row stages a custom order (indicator dot, order kept
  // on reopen); dragging back restores the natural order, which clears the dot (P140). -----------
  const columnsDot = page.locator('[data-testid="toolbar-columns"] span');
  const menuOrder = () =>
    page
      .locator('[data-testid="columns-menu-row"]')
      .evaluateAll((els) => els.map((el) => el.getAttribute('data-name')));
  const dragColumnRow = async (from: string, to: string) => {
    await page.click('[data-testid="toolbar-columns"]');
    const src = await page
      .locator(`[data-testid="columns-menu-row"][data-name="${from}"]`)
      .boundingBox();
    const dst = await page
      .locator(`[data-testid="columns-menu-row"][data-name="${to}"]`)
      .boundingBox();
    if (!src || !dst) throw new Error('columns menu row has no box');
    await sortableHover(page, boxCenter(src), {
      x: dst.x + dst.width / 2,
      y: dst.y + dst.height * 0.25,
    });
    await page.mouse.up();
    await page.keyboard.press('Escape');
    await expect(page.locator('[data-testid="columns-menu"]')).toHaveCount(0);
  };
  await expect(columnsDot).toHaveCount(0);
  await dragColumnRow('hash', 'id');
  await expect(columnsDot).toHaveCount(1);
  await page.click('[data-testid="toolbar-columns"]');
  expect(await menuOrder()).toEqual(['hash', 'id']);
  await page.keyboard.press('Escape');
  await dragColumnRow('hash', 'id');
  await expect(columnsDot).toHaveCount(0);

  // --- sort: header click asc -> desc -> none, then free-text ORDER BY wins -------------------
  // A left-click on a header's BODY now selects the column instead of sorting it; sorting moved to
  // a dedicated arrow control (`data-testid="grid-header-sort"`) on the right of each header cell.
  // This still exercises the identical asc -> desc -> none cycle, via the new control.
  const idSortArrow =
    '[data-testid="grid-header-cell"][data-column="id"] [data-testid="grid-header-sort"]';
  await page.click(idSortArrow); // -> asc (explicit)
  await expect
    .poll(() => page.locator('[data-testid="pager"]').getAttribute('data-pagination'))
    .toBe('keyset');
  await page.click(idSortArrow); // -> desc
  await expect.poll(() => firstGutterNumber(page)).toBe('1');
  await expect.poll(() => cellText(page, 0, 'id')).toBe('1000000');

  await page.click(idSortArrow); // -> none
  await expect(sortIndicators(page)).toHaveCount(0);
  await expect.poll(() => cellText(page, 0, 'id')).toBe('1'); // default order is still PK-ascending

  await page.fill('[data-testid="filter-orderby-input"]', 'id ASC');
  await page.press('[data-testid="filter-orderby-input"]', 'Enter');
  await expect.poll(() => cellText(page, 0, 'id')).toBe('1');
  await expect(sortIndicators(page)).toHaveCount(1);
  await page.fill('[data-testid="filter-orderby-input"]', '');
  await page.press('[data-testid="filter-orderby-input"]', 'Enter');

  // --- filter toolbar: valid/invalid WHERE, history, saved filters ---------------------------
  await page.fill('[data-testid="filter-where-input"]', 'id <= 5');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect.poll(() => lastGutterNumber(page), { timeout: 10_000 }).toBe('5');

  const readsBeforeBadFilter = await readOpsCount(stream, DATA_OP.read);
  await page.fill('[data-testid="filter-where-input"]', 'not valid sql (((');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="error-strip"]')).toBeVisible({ timeout: 10_000 });
  // The previous page is still on screen — the failed filter did not blank the grid.
  await expect.poll(() => firstGutterNumber(page)).toBe('1');
  await expect.poll(() => readOpsCount(stream, DATA_OP.read)).toBe(readsBeforeBadFilter + 1); // the failed attempt itself

  await page.fill('[data-testid="filter-where-input"]', 'id <= 5');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="error-strip"]')).toHaveCount(0);

  await page.click('[data-testid="filter-history-button"]');
  await expect(page.locator('[data-testid="filter-history"]')).toBeVisible();
  await expect(page.locator('[data-testid="history-entry"]').first()).toContainText('id <= 5');
  await page.click('[data-testid="save-current-filter"]');
  await page.fill('[data-testid="text-prompt-input"]', 'Small ids');
  await page.click('[data-testid="text-prompt-ok"]');
  await expect(page.locator('[data-testid="saved-entry"]').first()).toContainText('Small ids');
  await page.keyboard.press('Escape');
  await page.fill('[data-testid="filter-where-input"]', '');
  await page.press('[data-testid="filter-where-input"]', 'Enter');

  // --- search toolbar: match count, case, whole word, regex, prev/next, no new op rows -------
  const opsBeforeSearch = (await stream.ops()).length;
  await page.click('[data-testid="toolbar-search"]');
  const searchToolbar = page.locator('[data-testid="search-bar"]');
  await expect(searchToolbar).toBeVisible();
  await page.fill('[data-testid="search-input"]', '1');
  await expect(page.locator('[data-testid="search-count"]')).not.toContainText('No matches');
  await page.click('[data-testid="search-match-case"]');
  await page.click('[data-testid="search-whole-word"]');
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^1$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 1');
  await page.click('[data-testid="search-next"]');
  await page.click('[data-testid="search-prev"]');
  expect((await stream.ops()).length).toBe(opsBeforeSearch); // zero new PortRequests

  // --- Filter mode hides non-matching rows -------------------------------------
  // This page is still pageSize 10, unsorted (id-ascending), no WHERE — ids are plain digit
  // strings and `hash` (big_rows' other column) is always a 32-char md5 hex string that can never
  // equal one of them exactly, so an anchored regex alternation gives a deterministic,
  // non-contiguous 3-row subset with no dependency on hash's actual bytes. Regex mode (and the
  // still-open toolbar) carries over from the block above.
  const filterToggle = page.locator('[data-testid="search-filter-rows"]');
  const opsBeforeFilterSequence = (await stream.ops()).length;

  await page.fill('[data-testid="search-input"]', '^(2|5|9)$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3');
  await filterToggle.click();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3);
  // Row numbers stay real (D4): the gutter reads the rows' true, non-contiguous numbers rather
  // than renumbering 1..3, which is the affordance that says rows are hidden at all.
  await expect(page.locator('[data-testid="grid-gutter-cell"]')).toHaveText(['2', '5', '9']);
  // The match counter is unaffected by filtering (D6) — every match is inside a visible row.
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3');
  await expect(page.locator('[data-testid="search-scope"]')).toContainText(
    'Showing 3 of 10 loaded rows',
  );

  // Prev/next and the current-match highlight still work while filtering (D6).
  await page.click('[data-testid="search-next"]');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('2 of 3');
  await expect(page.locator('.search-match-current')).toBeVisible();
  // Contrast regression guard: `.search-match-current`'s bright `--kira-search-match-current`
  // background needs `color: var(--kira-bg)` alongside it (ConsoleResultGrid.vue's own
  // key-value branch carries the same pairing) or the inherited light-grey cell text sits at
  // ~1.45:1 contrast on exactly the cell the user just navigated to — effectively unreadable.
  const [matchColor, bgTokenColor] = await page.evaluate(() => {
    const el = document.querySelector('.search-match-current');
    const probe = document.createElement('div');
    probe.style.color = 'var(--kira-bg)';
    document.body.appendChild(probe);
    const resolved: [string | null, string] = [
      el ? getComputedStyle(el).color : null,
      getComputedStyle(probe).color,
    ];
    probe.remove();
    return resolved;
  });
  expect(matchColor).toBe(bgTokenColor);

  // Keyboard navigation stays inside the visible set (D11): id=2 is page row 1; ArrowDown must
  // land on the next *visible* row (id=5, page row 4), not page row 2 (id=3, hidden).
  await gridCell(page, 1, 'id').click();
  await page.keyboard.press('ArrowDown');
  // P22 Pass B — `kira-cell-selected` (§5 D4), not the incumbent's own bare `.selected`.
  await expect(
    page.locator('[data-testid="grid-row"] [data-testid="grid-cell"].kira-cell-selected'),
  ).toBeVisible();
  await expect(
    gridRow(page, 4).locator('[data-testid="grid-cell"].kira-cell-selected'),
  ).toHaveCount(1);

  // Copy column values follows the filter (D10): one clipboard line per visible row, not per
  // loaded row. `installClipboardSpy` (header comment) stands in for the original's real OS
  // clipboard read, which WebKit's Playwright automation has no permission grant for.
  await page.click('[data-testid="grid-header-cell"][data-column="id"]', { button: 'right' });
  await page.click('[data-testid="menu-item-copy-column-values"]');
  expect((await lastClipboardWrite(page)).split('\n')).toEqual(['2', '5', '9']);

  // Zero matches (D8): a query matching nothing shows the "No matching rows" empty state (not a
  // blank grid, and not "No rows" — LAW 15) with a working "Show all rows" action.
  await page.fill('[data-testid="search-input"]', '^nope-matches-nothing-here$');
  await expect(page.locator('[data-testid="grid-no-matching-rows"]')).toBeVisible();
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(0);
  await page.click('[data-testid="grid-show-all-rows"]');
  await expect(page.locator('[data-testid="grid-no-matching-rows"]')).toHaveCount(0);
  await expect(filterToggle).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(10);

  // Pending inserts survive the filter (D5), whatever the query matches — an unsaved row is work
  // in progress, not a search result, and can't be matched anyway (search never walks the pending
  // set). Purely client-side staging (pendingChanges.ts) — no network call either way.
  await page.click('[data-testid="toolbar-add-row"]');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  await filterToggle.click(); // query is still the zero-match string from above
  await expect(page.locator('[data-testid="grid-no-matching-rows"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(0);
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(1);
  await page.click('[data-testid="toolbar-discard-changes"]');
  await expect(page.locator('[data-testid="grid-row-insert"]')).toHaveCount(0);
  await filterToggle.click(); // back off

  // Empty query keeps the toggle lit and shows everything (D7) — clearing the field must not
  // empty the grid.
  await page.fill('[data-testid="search-input"]', '');
  await filterToggle.click();
  await expect(filterToggle).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(10);

  // Closing the toolbar unfilters (D7), and reopening starts with the toggle off.
  await page.fill('[data-testid="search-input"]', '^(2|5|9)$');
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(3);
  await page.click('[data-testid="search-close"]');
  await expect(searchToolbar).toHaveCount(0);
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(10);
  await page.click('[data-testid="toolbar-search"]');
  await expect(searchToolbar).toBeVisible();
  await expect(filterToggle).toHaveAttribute('aria-pressed', 'false');

  // Zero operations for the whole filter sequence (D13): the single assertion that proves the
  // filter never reached the server — a new PortRequest would mean it had.
  expect((await stream.ops()).length).toBe(opsBeforeFilterSequence);

  await page.click('[data-testid="search-close"]');
  await expect(searchToolbar).toHaveCount(0);

  // --- stop: a filtered read cancelled mid-flight, previous page stays on screen -------------
  await page.click('[data-testid="page-size-10000"]');
  await expect.poll(() => lastGutterNumber(page), { timeout: 15_000 }).toBe('10000');

  // --- Viewport-first search — the rows on screen highlight before the full scan
  // finishes, and the filter toggle waits for a *completed* scan before it hides anything.
  // runChunkedScan's priority window lands in its own animation frame before the ordinary
  // ascending pass starts, so ".search-match" reliably appears first — but exactly how far ahead
  // of "search-count" settling is a real race (tens of ms), not a guarantee, so this only asserts
  // the ordering it can: the match highlights, then (below) the count eventually reaches its final
  // value. --------------------------------------------------------------------------------------
  await scrollGridToBottom(page);
  await page.click('[data-testid="toolbar-search"]');
  await expect(searchToolbar).toBeVisible();
  // SearchToolbar.vue remounts fresh on every open (D7) — regex mode from the earlier block does
  // not carry over, so it must be turned back on before '^9999$' means anything.
  await page.click('[data-testid="search-regex"]');
  await page.fill('[data-testid="search-input"]', '^9999$'); // id=9999 — one row, near the bottom

  await expect(page.locator('.search-match')).toBeVisible({ timeout: 2_000 });

  // A filter toggled on mid-scan hides nothing — matchedRows() reads null while pending. Not
  // asserted directly: the scan (5 rAF-gated chunks, ~80ms) routinely finishes before a Playwright
  // round trip can click the toggle and read the DOM back, so there is no reliable window in which
  // to observe the "still 10000, scan pending" state from here. What's left is the invariant that
  // matters end to end — toggling never leaves the grid on a stale or half-applied
  // result, only ever the fully-settled one.
  await filterToggle.click();
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 1', {
    timeout: 15_000,
  });
  await expect(page.locator('[data-testid="grid-row"]')).toHaveCount(1); // now filtering applies
  await filterToggle.click(); // back off, for the ascending check below

  // F30: the completed match list stays strictly ascending regardless of where the priority
  // window sat — three rows spread across the loaded page, driven with next/next/next.
  await page.fill('[data-testid="search-input"]', '^(100|5000|9000)$');
  await expect(page.locator('[data-testid="search-count"]')).toContainText('1 of 3', {
    timeout: 15_000,
  });
  let lastMatchRow = -1;
  for (let i = 0; i < 3; i++) {
    // P22 Pass B — `data-row` lives on the row (`.slick-row`, D10), not the cell:
    // `.search-match-current` (kira-search-current, C12) is a `setCellCssStyles` class on the
    // `.slick-cell` itself, so the row number comes from its own closest row ancestor.
    const row = await page.locator('.search-match-current').evaluate((el) => {
      const rowEl = el.closest('[data-testid="grid-row"]');
      return Number(rowEl?.getAttribute('data-row') ?? Number.NaN);
    });
    expect(row).toBeGreaterThan(lastMatchRow);
    lastMatchRow = row;
    await page.click('[data-testid="search-next"]');
  }

  // P43 iter2 D33/D34 and iter3 D41/F36 — dropped, not merely flaky: all three depend on a
  // Playwright interaction landing *while the scan is still in flight* (closing the toolbar
  // mid-scan, pressing Enter between two growing-total ticks, the current match surviving the
  // total's later growth). Confirmed by repeated runs here: this tier's scan runs over rows
  // already sitting in memory (no real per-chunk IPC/Postgres latency pacing it out the way the
  // original's real backend did), so the whole ~80ms, 5-rAF-chunk scan reliably completes before
  // this test's own next line of Playwright code runs, every time — not an occasional race to
  // work around (CLAUDE.md's redis.frontend.spec.ts `expect.poll` finding was that kind of gap;
  // this one is structural, not timing-sensitive-but-eventually-hit). There is no window left to
  // interact inside any more. D38 just above already anticipated exactly this in its own comment
  // ("no reliable window... what's left is the invariant that actually matters end to end") for
  // the filter-toggle case; the same reasoning applies to close/Enter here, just with no
  // settled-state invariant left over once the premise (an in-flight scan) cannot occur at all.

  await page.click('[data-testid="search-close"]');
  await expect(searchToolbar).toHaveCount(0);

  // --- stop: a real mid-flight cancellation, previous page stays on screen -------------------
  const firstBeforeStop = await firstGutterNumber(page);
  // P110 I2-4: RunState.vue's TONE lookup — while running, the label reads --kira-info (#69a1e8),
  // not the idle text-subtle colour. `toolbar-stop`'s enabled state is a local, optimistic flag
  // independent of opsStore, and this mocked `ui` tier never fires real control.onOpUpdate pushes
  // (tests/ui/interaction.spec.ts's own header comment documents this as a hard, structural gap) —
  // so the running state RunState reads has to be synthesised here via the same emitWailsEvent
  // mechanism other specs already use for other push channels (e.g. workbench.spec.ts, IPC.keepAwake).
  const activeTabId = await page
    .locator('[data-testid="tab"][data-active="true"]')
    .first()
    .getAttribute('data-tab-id');
  if (!activeTabId) throw new Error('expected an active tab id');
  await emitWailsEvent(page, IPC.opUpdate, {
    id: 'op-synthetic-run-state-colour',
    connectionId: CONNECTION_ID,
    tabId: activeTabId,
    startedAt: new Date().toISOString(),
    durationMs: null,
    kind: 'read',
    status: 'running',
    rows: null,
    command: null,
    error: null,
  });
  await expect(page.locator('[data-testid="run-state"]').first()).toHaveCSS(
    'color',
    'rgb(105, 161, 232)',
  );
  // Close out the synthetic record so it doesn't linger as "running" once the real stop-flow below
  // clears the optimistic rt.opId — opsStore only updates on the next matching-id push.
  await emitWailsEvent(page, IPC.opUpdate, {
    id: 'op-synthetic-run-state-colour',
    connectionId: CONNECTION_ID,
    tabId: activeTabId,
    startedAt: new Date().toISOString(),
    durationMs: 1,
    kind: 'read',
    status: 'cancelled',
    rows: null,
    command: null,
    error: null,
  });
  await page.fill('[data-testid="filter-where-input"]', '(SELECT pg_sleep(2)) IS NULL OR id > 0');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await expect(page.locator('[data-testid="toolbar-stop"]')).toBeEnabled();
  await page.click('[data-testid="toolbar-stop"]');
  // The reply is held until opsCancel arrives, so the recovery below proves Stop sent it — and
  // for the in-flight read's own op id.
  const inFlightRead = (await stream.ops()).find(
    (o) =>
      o.op === DATA_OP.read &&
      (o.payload as { filter?: string }).filter === '(SELECT pg_sleep(2)) IS NULL OR id > 0',
  );
  expect(inFlightRead?.opId).toBeTruthy();
  await expect
    .poll(() =>
      control
        .log()
        .filter((e) => e.channel === IPC.opsCancel)
        .map((e) => e.args),
    )
    .toEqual([{ opId: inFlightRead?.opId }]);
  // applyLoadFailure's cancelled branch clears rt.opId once the (real, captured) E_CANCELLED
  // error lands — the same flip that re-enables Refresh and disables Stop, with no op-log status
  // to read any more (see the header comment).
  await expect(page.locator('[data-testid="toolbar-stop"]')).toBeDisabled({ timeout: 10_000 });
  await expect.poll(() => firstGutterNumber(page)).toBe(firstBeforeStop);
  await page.fill('[data-testid="filter-where-input"]', '');
  await page.press('[data-testid="filter-where-input"]', 'Enter');
  await page.click('[data-testid="page-size-100"]');
  await expect.poll(() => lastGutterNumber(page), { timeout: 15_000 }).toBe('100');

  // --- NULL vs '' -------------------------------------------------------------------------
  await (await findRow(page, NULLS_PATH)).dblclick();
  await expect(page.locator('[data-testid="data-grid"]')).toBeVisible();
  const nullCell = gridCell(page, 0, 'label');
  const emptyCell = gridCell(page, 1, 'label');
  // `data-null` is retired in favour of the `.cell-null` marker the formatter already carries on
  // both engines (nullMarker: a child span under the incumbent, the cell's own class under
  // SlickGrid) — an implementation-detail rewrite, not a coverage loss.
  await expect(nullMarker(nullCell)).toHaveCount(1);
  await expect(nullCell).toContainText('NULL');
  await expect(nullMarker(emptyCell)).toHaveCount(0);
  await expect(emptyCell).toHaveText('');

  // Both errors this test deliberately triggers (the invalid-filter syntax error, the mid-flight
  // cancellation) travel the data-plane WebSocket, not a bound call — unlike a handled
  // control-plane rejection (CLAUDE.md's finding), neither produces a real HTTP 422 for
  // Chromium/WebKit's devtools to log, so this stays a plain empty-array assertion.
  expect(consoleErrors).toEqual([]);
});
