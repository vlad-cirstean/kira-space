import type { MaskingRule } from '@shared/domain/mask';
import type { ForeignKeyMeta } from '@shared/domain/tree';
import {
  type ColumnDescriptor,
  createTabularPageBuilder,
  type TabularPage,
  unpagedPosition,
} from '@shared/protocol/page';
import { ROWS, wideColumns, wideRows } from '../../../tests/perf/support/wideTable';
import { createMaskPreviewTransform, type MaskedCellView } from '../../src/views/grid/maskPreview';
import { type CellView, cell, setPage } from '../../src/views/grid/page';

const PROTO_SCOPE = 'proto-grid';
const NCOLS = 20;
export const GUTTER_FIELD = '__gutter';
const SEARCH_TERM = 'number 42';

export type ProtoCellView = MaskedCellView;
export type Fixture = 'perf' | 'features';

export interface ProtoParams {
  fixture: Fixture;
  /** `?rowHeight=22|28`; null follows the Appearance token. */
  rowHeight: number | null;
  width: number | null;
  height: number | null;
  zebra: boolean;
  /** `?readonly=1`: every edit is vetoed, as for a table without a primary key. */
  readOnly: boolean;
}

export function readParams(search: string): ProtoParams {
  const q = new URLSearchParams(search);
  const num = (key: string): number | null => {
    const v = Number(q.get(key));
    return Number.isFinite(v) && v > 0 ? v : null;
  };
  return {
    fixture: q.get('fixture') === 'features' ? 'features' : 'perf',
    rowHeight: num('rowHeight'),
    width: num('w'),
    height: num('h'),
    zebra: q.get('zebra') === '1',
    readOnly: q.get('readonly') === '1',
  };
}

export interface ProtoData {
  scope: string;
  page: TabularPage;
  columns: ColumnDescriptor[];
  rowCount: number;
  /** Outbound FK per column name (features fixture only). */
  fk: ReadonlyMap<string, ForeignKeyMeta>;
  maskRules: ReadonlyMap<string, MaskingRule>;
  /** Page rows the search term matches, ascending. */
  matches: readonly number[];
  /** Display view of one cell: decode cache, then mask preview. */
  viewAt(row: number, col: number): ProtoCellView;
}

const FEATURE_FK: ForeignKeyMeta = {
  name: 'fk_country',
  columns: ['country'],
  referencedPath: 'countries',
  referencedColumns: ['code'],
  onDelete: null,
  onUpdate: null,
};

const BIG_VALUE = 'x'.repeat(70 * 1024);

function featureRows(rows: (string | null)[][], columns: ColumnDescriptor[]): void {
  const email = columns.findIndex((c) => c.name === 'email');
  const notes = columns.findIndex((c) => c.name === 'notes');
  rows.forEach((row, i) => {
    if (i % 17 === 16) row[email] = '';
    if (i % 500 === 499) row[notes] = BIG_VALUE;
  });
}

function findMatches(rows: (string | null)[][], columns: ColumnDescriptor[]): number[] {
  const name = columns.findIndex((c) => c.name === 'name');
  const out: number[] = [];
  rows.forEach((row, i) => {
    if (row[name]?.includes(SEARCH_TERM)) out.push(i);
  });
  return out;
}

export function createProtoData(fixture: Fixture): ProtoData {
  const columns = wideColumns(NCOLS);
  if (fixture === 'features') {
    const generated = columns.find((c) => c.name === 'updated_at');
    if (generated) generated.generated = true;
  }
  const rows = wideRows(NCOLS, ROWS);
  const features = fixture === 'features';
  if (features) featureRows(rows, columns);
  const matches = features ? findMatches(rows, columns) : [];

  const builder = createTabularPageBuilder(columns);
  for (const row of rows) builder.appendRow(row);
  const page = builder.finish(unpagedPosition(rows.length));
  setPage(PROTO_SCOPE, page);

  const maskRules = new Map<string, MaskingRule>(
    features ? [['email', { kind: 'email', keepHint: true, correlate: false }]] : [],
  );
  const transform = createMaskPreviewTransform(maskRules, new Map());
  const names = columns.map((c) => c.name);

  return {
    scope: PROTO_SCOPE,
    page,
    columns,
    rowCount: page.rowCount,
    fk: new Map(features ? [['country', FEATURE_FK]] : []),
    maskRules,
    matches,
    viewAt(row, col): ProtoCellView {
      const view: CellView = cell(PROTO_SCOPE, row, col);
      return maskRules.size === 0 ? view : transform(view, names[col] as string);
    },
  };
}
