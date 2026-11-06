import type { ConnectionSummary } from '@shared/domain/connection';
import type { KibanaDataView, KibanaHistogram, KibanaSuggestions } from '@shared/domain/kibana';
import { DATA_OP } from '@shared/protocol/dataOps';
import type { ColumnDescriptor } from '@shared/protocol/page';
import type { ControlSnapshot, PortSnapshot } from '../../../ipc/support/types';
import { contract } from './contract';
import { connectionSummary } from './engineFixture';
import { IPC } from './ipcChannels';

// Answers come from the Go flow TestKibanaDiscoverJourney (contract kibana-discover), so the UI
// half asserts against the very results the backend half produced.

export const CONNECTION_ID = 'conn-kibana';
export const LOGS_VIEW = 'kira-logs';

const connected = contract<Record<string, unknown>>(
  'kibana-discover',
  'ConnectionsService.Connect#connected',
);
const tree = contract<{ nodes: { name: string; path: string }[] }>(
  'kibana-discover',
  'TreeService.Children#dataViews',
);
export const views = contract<KibanaDataView[]>('kibana-discover', 'KibanaService.DataViews#list');
export const logsView = contract<KibanaDataView>('kibana-discover', 'KibanaService.DataView#logs');
export const histogram = contract<KibanaHistogram>(
  'kibana-discover',
  'KibanaService.Histogram#auto',
);
export const levelSuggestions = contract<KibanaSuggestions>(
  'kibana-discover',
  'KibanaService.Suggest#level',
);

const columnNames = contract<string[]>('kibana-discover', 'Execute#columns');

export const logsNode = (() => {
  const node = tree.nodes.find((n) => n.name === LOGS_VIEW);
  if (!node) throw new Error('kira-logs missing from the contract tree');
  return node;
})();

export function kibanaConnectionSummary(): ConnectionSummary {
  return connectionSummary('kibana', 5601, '', 'kira', CONNECTION_ID, 'Logs Kibana', 'indigo');
}

export function kibanaControl(): ControlSnapshot[] {
  return [
    { channel: IPC.connectionsList, response: [kibanaConnectionSummary()] },
    {
      channel: IPC.connectionsConnect,
      args: { id: CONNECTION_ID },
      response: {
        ...connected,
        connectionId: CONNECTION_ID,
        since: 1735689600000,
        serverVersion: 'Kibana 9.5.3',
      },
    },
    {
      channel: IPC.treeChildren,
      args: { connectionId: CONNECTION_ID, path: '', refresh: false },
      response: tree,
    },
    { channel: IPC.kibanaDataViews, response: views },
    { channel: IPC.kibanaDataView, response: logsView },
    { channel: IPC.kibanaHistogram, response: histogram },
    { channel: IPC.kibanaSuggest, response: levelSuggestions },
  ];
}

/** A page of `count` log documents, the shape execute() answers with. */
export function hitsPage(count: number) {
  const ids = Array.from({ length: count }, (_, i) => `doc-${String(i).padStart(3, '0')}`);
  const bodies = ids.map((id, i) =>
    JSON.stringify({
      _id: id,
      _index: LOGS_VIEW,
      '@timestamp': `2026-01-01T00:${String(i % 60).padStart(2, '0')}:00.000Z`,
      level: 'error',
      status: 500 + (i % 4),
      message: `request ${i} handled by worker-${i % 7}`,
    }),
  );
  return {
    kind: 'document' as const,
    ids,
    bodies,
    position: {
      offset: 0,
      pageSize: count,
      hasMore: false,
      nextToken: null,
      prevToken: null,
      strategy: 'offset' as const,
    },
  };
}

/** The execute() snapshot for one Discover statement; match is exact, so the spec builds it the way the view does. */
export function executeHits(statement: string, count: number, path = logsNode.path): PortSnapshot {
  return {
    op: DATA_OP.execute,
    payload: { connectionId: CONNECTION_ID, path, statements: [statement] },
    response: { kind: 'execute', pages: [hitsPage(count)] },
  };
}

/** The execute() snapshot of a column-selected statement: a tabular page of the contract's columns. */
export function executeColumns(statement: string, count: number): PortSnapshot {
  const columns: ColumnDescriptor[] = columnNames.map((name) => ({
    name,
    dataType: name === 'status' ? 'number' : name === '@timestamp' ? 'date' : 'string',
    typeClass: name === 'status' ? 'number' : name === '@timestamp' ? 'temporal' : 'text',
    nullable: true,
    isPrimaryKey: false,
    generated: false,
  }));
  const rows = Array.from({ length: count }, (_, i) => [
    `2026-01-01T00:${String(i % 60).padStart(2, '0')}:00.000Z`,
    'error',
    String(500 + (i % 4)),
  ]);
  return {
    op: DATA_OP.execute,
    payload: { connectionId: CONNECTION_ID, path: logsNode.path, statements: [statement] },
    response: {
      kind: 'execute',
      pages: [
        {
          kind: 'tabular',
          columns,
          rows,
          position: {
            offset: 0,
            pageSize: count,
            hasMore: false,
            nextToken: null,
            prevToken: null,
            strategy: 'offset',
          },
          truncatedCells: 0,
        },
      ],
    },
  };
}
