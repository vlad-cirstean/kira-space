import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { LogicalPage, PortSnapshot } from '../../../ipc/support/types';
import { ORDER_ITEMS_PATH } from './postgresFixture';

export function executeSnap(
  connectionId: string,
  statements: string[],
  pages: LogicalPage[],
): PortSnapshot {
  return {
    op: DATA_OP.execute,
    payload: { connectionId, path: ORDER_ITEMS_PATH, statements },
    response: { kind: 'execute', pages },
  };
}

export function intColumnPage(column: string, values: string[]): LogicalPage {
  const columns: ColumnDescriptor[] = [
    {
      name: column,
      dataType: 'int4',
      typeClass: 'number',
      nullable: true,
      isPrimaryKey: false,
      generated: false,
    },
  ];
  return {
    kind: 'tabular',
    columns,
    rows: values.map((v) => [v]),
    position: {
      offset: 0,
      pageSize: values.length,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset',
    },
    truncatedCells: 0,
  };
}

export function numberPage(column: string, value: string): LogicalPage {
  return intColumnPage(column, [value]);
}

/** A text-typed tabular page, as a flow contract's page summary describes it. */
export function textPage(
  columns: readonly string[],
  rows: readonly (string | null)[][],
): LogicalPage {
  const descriptors: ColumnDescriptor[] = columns.map((name) => ({
    name,
    dataType: 'text',
    typeClass: 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  }));
  return {
    kind: 'tabular',
    columns: descriptors,
    rows: rows.map((r) => [...r]),
    position: {
      offset: 0,
      pageSize: rows.length,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset',
    },
    truncatedCells: 0,
  };
}
