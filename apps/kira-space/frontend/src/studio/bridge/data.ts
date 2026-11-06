import {
  type CacheStats,
  type CountRequestWire,
  type CountResponse,
  DATA_OP,
  type ExecuteRequestWire,
  type ExecuteResponse,
  type MutateRequestWire,
  type MutateResponse,
  type ObjectDownloadRequestWire,
  type ObjectDownloadResponse,
  PORT_EVENT,
  type PreviewRequestWire,
  type PreviewResponse,
  type ReadRequestWire,
  type ReadResponse,
} from '@shared/protocol/dataOps';
import { assertPageStructure } from '@shared/protocol/page';
import { onPortEvent, request } from './port';

// Data ops have no client-side timeout (D25) — cancellation via `control.opsCancel` is the only
// escape hatch, never an abandoned-but-still-running server query.
const NO_TIMEOUT = { timeoutMs: null as null };

async function readResponse(req: ReadRequestWire): Promise<ReadResponse> {
  const response = (await request(DATA_OP.read, req, NO_TIMEOUT)) as ReadResponse;
  assertPageStructure(response.page);
  return response;
}

export const data = {
  read: readResponse,
  count: (req: CountRequestWire): Promise<CountResponse> =>
    request(DATA_OP.count, req, NO_TIMEOUT) as Promise<CountResponse>,
  invalidate: async (
    connectionId: string,
    path: string,
    scope?: 'all' | 'pages',
  ): Promise<void> => {
    await request(DATA_OP.invalidate, { connectionId, path, scope });
  },
  preview: (req: PreviewRequestWire): Promise<PreviewResponse> =>
    request(DATA_OP.preview, req, NO_TIMEOUT) as Promise<PreviewResponse>,
  mutate: (req: MutateRequestWire): Promise<MutateResponse> =>
    request(DATA_OP.mutate, req, NO_TIMEOUT) as Promise<MutateResponse>,
  execute: async (req: ExecuteRequestWire): Promise<ExecuteResponse> => {
    const response = (await request(DATA_OP.execute, req, NO_TIMEOUT)) as ExecuteResponse;
    for (const page of response.pages) assertPageStructure(page);
    return response;
  },
  objectDownload: (req: ObjectDownloadRequestWire): Promise<ObjectDownloadResponse> =>
    request(DATA_OP.objectDownload, req, NO_TIMEOUT) as Promise<ObjectDownloadResponse>,
  clearCaches: async (): Promise<void> => {
    await request(DATA_OP.cacheClear);
  },
  cacheStats: (): Promise<CacheStats> => request(DATA_OP.cacheStats) as Promise<CacheStats>,
  onCacheStats: (cb: (stats: CacheStats) => void): (() => void) =>
    onPortEvent(PORT_EVENT.cacheStats, (payload) => cb(payload as CacheStats)),
};
