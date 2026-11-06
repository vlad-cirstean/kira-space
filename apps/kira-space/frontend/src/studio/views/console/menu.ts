import { t } from '@theme/i18n';
import { copyOrReportError, copyOrReportErrorLazy } from '@workbench/behaviour/clipboard';
import { announceApp } from '@workbench/behaviour/liveRegion';
import type { MenuItem } from '@workbench/state/contextMenu';
import { toPlainJson, toRelaxedText, toShellText } from '../shared/document/ejson';
import type { CopyFormat } from '../shared/slick/selectionCopy';
import { type CopyAllFormat, jsonArrayOf, prettyJson } from './copyAll';

// The console result grid's own menu builders, mirroring views/grid/menu.ts's split
// (builders in a plain module, the host only opens them) -- item ids follow that file's own
// naming (copy, copy-with-header, copy-as-json, copy-rows, copy-rows-tsv, ...) so a reader
// comparing the two surfaces sees the same vocabulary. Unlike the data grid, a console result is
// read-only by construction (no edit/delete/paste/duplicate -- every one of those writes, and a
// console result has no addressable table) and offers no "Copy as INSERT" (no qualified table
// name to write into).

interface TabularMenuContext {
  onError: (message: string) => void;
  /** What the menu's selection copies as `format`; built lazily by the host, so building the menu
   *  never reads a cell. */
  copy: (format: CopyFormat) => string;
}

export interface TabularCellMenuContext extends TabularMenuContext {
  columnName: string;
}

// D9's 'cell' row: Copy / Copy with header / Copy as JSON, the same three the data grid's own
// cellMenu leads with.
export function tabularCellMenu(ctx: TabularCellMenuContext): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy',
      label: t('common.action.copy.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('tsv'), ctx.onError, () =>
          announceApp(t('studio.copy.cell.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-with-header',
      label: t('studio.copy.withHeader.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(`${ctx.columnName}\n${ctx.copy('tsv')}`, ctx.onError, () =>
          announceApp(t('studio.copy.withHeader.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-as-json',
      label: t('common.action.copyAsJson.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('json'), ctx.onError, () =>
          announceApp(t('common.copy.copiedJson.announce')),
        ),
    },
  ];
}

// D9's 'range' row: Copy (TSV, the same text ⌘C already produces) · Copy as CSV · Copy as JSON.
export function tabularRangeMenu(ctx: TabularMenuContext): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy',
      label: t('common.action.copy.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('tsv'), ctx.onError, () =>
          announceApp(t('studio.copy.selection.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-as-csv',
      label: t('studio.copy.asCsv.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('csv'), ctx.onError, () =>
          announceApp(t('studio.copy.csv.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-as-json',
      label: t('common.action.copyAsJson.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('json'), ctx.onError, () =>
          announceApp(t('common.copy.copiedJson.announce')),
        ),
    },
  ];
}

// D9's 'row' row: Copy rows ▸ TSV/CSV/JSON. No INSERT -- a console result comes from ad-hoc SQL
// with no addressable table to write into.
export function tabularRowMenu(ctx: TabularMenuContext): MenuItem[] {
  return [
    {
      type: 'submenu',
      id: 'copy-rows',
      label: t('studio.copy.rows.label'),
      icon: 'copy',
      items: [
        {
          type: 'item',
          id: 'copy-rows-tsv',
          label: t('studio.copy.format.tsv.label'),
          run: () =>
            copyOrReportError(ctx.copy('tsv'), ctx.onError, () =>
              announceApp(t('studio.copy.tsv.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-rows-csv',
          label: t('studio.copy.format.csv.label'),
          run: () =>
            copyOrReportError(ctx.copy('csv'), ctx.onError, () =>
              announceApp(t('studio.copy.csv.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-rows-json',
          label: t('studio.copy.format.json.label'),
          run: () =>
            copyOrReportError(ctx.copy('json'), ctx.onError, () =>
              announceApp(t('common.copy.copiedJson.announce')),
            ),
        },
      ],
    },
  ];
}

export interface TabularColumnMenuContext extends TabularMenuContext {
  columnName: string;
}

// D9's 'column' row: Copy column (the exact TSV ⌘C produces for the selected columns) and Copy
// column name.
export function tabularColumnMenu(ctx: TabularColumnMenuContext): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy-column',
      label: t('studio.copy.column.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(ctx.copy('tsv'), ctx.onError, () =>
          announceApp(t('studio.copy.column.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-column-name',
      label: t('studio.copy.columnName.label'),
      run: () =>
        copyOrReportError(ctx.columnName, ctx.onError, () =>
          announceApp(t('studio.copy.columnName.announce')),
        ),
    },
  ];
}

export interface RowJsonMenuContext {
  /** This row's own value, pre-formatted JSON text (a document's raw EJSON body, or a kv pair
   *  already run through JSON.stringify). */
  json: string;
  /** Every displayed row's JSON text, in display order -- "all" means what's on screen right now
   *  (an active find-filter's own subset), never every row the server could return. A thunk, not an
   *  already-built array -- building the menu (right-click) must never pay for a whole-page decode
   *  only "Copy all as JSON" will ever need, the exact eager-snapshot cost views/grid/menu.ts's own
   *  snapshotsThunk already avoids. */
  allJson: () => readonly string[];
  onError: (message: string) => void;
}

// The key-value result branch's own row menu (a Redis pair has no shell/EJSON format of its
// own -- the submenu below is Mongo-only) -- "Copy as JSON" (this row) and "Copy all as
// JSON" (every displayed row). Both go through copyOrReportError since a clipboard write here can
// genuinely be rejected and ContextMenu.vue's own `@click` never awaits `run()` --
// documents/menu.ts's copyOrReportError shape.
export function rowAsJsonMenu(ctx: RowJsonMenuContext): MenuItem[] {
  return [
    {
      type: 'item',
      id: 'copy-as-json',
      label: t('common.action.copyAsJson.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(prettyJson(ctx.json), ctx.onError, () =>
          announceApp(t('common.copy.copiedJson.announce')),
        ),
    },
    {
      type: 'item',
      id: 'copy-all-as-json',
      label: t('studio.copy.allAsJson.label'),
      icon: 'copy',
      run: () =>
        copyOrReportError(jsonArrayOf(ctx.allJson().map(prettyJson)), ctx.onError, () =>
          announceApp(t('studio.copy.allAsJson.announce')),
        ),
    },
  ];
}

export interface MongoDocumentRowMenuContext {
  /** This row's own canonical extended-JSON body, unparsed -- toShellText/toRelaxedText each do
   *  their own parse. */
  body: string;
  /** Every displayed row in one copy format -- RowJsonMenuContext.allJson's own "what's on screen
   *  now, as a thunk" rule: the host reads the bodies only when invoked, and encodes them off the
   *  main thread when the page is large. */
  allText: (format: CopyAllFormat) => Promise<string>;
  onError: (message: string) => void;
}

// The Mongo document result's own row menu -- two named-format submenus (mirroring
// console/menu.ts's own tabularRowMenu Copy row(s) ▸ shape above) rather than documents/menu.ts's
// old flat "Copy as JSON"/"Copy all as JSON" pair. Leaf ids `copy-as-json`/`copy-all-as-json`
// preserved so existing specs keep passing on a path change with no assertion change.
//
// Real-interaction fix (reported bug — the normal/default copy button still produced
// `$oid`/`$date`-wrapped JSON, mirrored from documents/menu.ts's own identical fix): Plain JSON is
// new and listed first in each submenu — Canonical/Relaxed Extended JSON must both keep `$oid`
// wrapped (autocomplete.spec.ts locks in Canonical's own mongoimport/mongosh round-trip and
// Relaxed's "no relaxed variant for ObjectId" per spec), and Shell mode is mongosh syntax, not
// JSON. ejson.ts's own `toPlainJson` strips every `$`-prefixed wrapper instead.
export function mongoDocumentRowMenu(ctx: MongoDocumentRowMenuContext): MenuItem[] {
  return [
    {
      type: 'submenu',
      id: 'copy-document-submenu',
      label: t('studio.copy.document.label'),
      icon: 'copy',
      items: [
        {
          type: 'item',
          id: 'copy-plain-json',
          label: t('studio.copy.plainJson.label'),
          run: () =>
            copyOrReportError(toPlainJson(ctx.body), ctx.onError, () =>
              announceApp(t('studio.copy.plainJson.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-document',
          label: t('studio.copy.shellMode.label'),
          run: () =>
            copyOrReportError(toShellText(ctx.body), ctx.onError, () =>
              announceApp(t('studio.copy.shellMode.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-as-json',
          label: t('studio.copy.canonicalJson.label'),
          run: () =>
            copyOrReportError(prettyJson(ctx.body), ctx.onError, () =>
              announceApp(t('studio.copy.canonicalJson.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-relaxed-json',
          label: t('studio.copy.relaxedJson.label'),
          run: () =>
            copyOrReportError(toRelaxedText(ctx.body), ctx.onError, () =>
              announceApp(t('studio.copy.relaxedJson.announce')),
            ),
        },
      ],
    },
    {
      type: 'submenu',
      id: 'copy-all-submenu',
      label: t('common.action.copyAll.label'),
      icon: 'copy',
      items: [
        {
          type: 'item',
          id: 'copy-all-plain-json',
          label: t('studio.copy.plainJson.label'),
          run: () =>
            copyOrReportErrorLazy(ctx.allText('plain'), ctx.onError, () =>
              announceApp(t('studio.copy.plainJson.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-all-shell',
          label: t('studio.copy.shellMode.label'),
          // The shape for "all": documents joined by '\n', one compact literal per line --
          // not a JSON array, and not each document's own multi-line indented form (which would
          // make "joined by \n" meaningless) -- that's what pasting N mongosh literals back into
          // a shell session wants.
          run: () =>
            copyOrReportErrorLazy(ctx.allText('shell'), ctx.onError, () =>
              announceApp(t('studio.copy.shellMode.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-all-as-json',
          label: t('studio.copy.canonicalJson.label'),
          run: () =>
            copyOrReportErrorLazy(ctx.allText('canonical'), ctx.onError, () =>
              announceApp(t('studio.copy.canonicalJson.announce')),
            ),
        },
        {
          type: 'item',
          id: 'copy-all-relaxed-json',
          label: t('studio.copy.relaxedJson.label'),
          run: () =>
            copyOrReportErrorLazy(ctx.allText('relaxed'), ctx.onError, () =>
              announceApp(t('studio.copy.relaxedJson.announce')),
            ),
        },
      ],
    },
  ];
}
