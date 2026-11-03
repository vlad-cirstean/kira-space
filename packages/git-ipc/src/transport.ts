import type {
  EventKey,
  EventPayload,
  ParamsOf,
  RequestKey,
  ResultOf,
  StreamChunkOf,
  StreamKey,
  StreamParamsOf,
} from './contract.ts';

export type TransportErrorCode =
  | 'cancelled'
  | 'contract-mismatch'
  | 'transport-closed'
  | 'handler-error';

export class TransportError extends Error {
  readonly code: TransportErrorCode;

  constructor(code: TransportErrorCode, message: string) {
    super(message);
    this.name = 'TransportError';
    this.code = code;
  }
}

/** An error that crossed the wire as data: `code` is a `WireErrorCode` (contract.ts) or a bound
 *  call's `E_*` code, `message` is display-ready. A git failure's kind is folded into the code
 *  (`E_GIT_<KIND>`) by Go's `mapGitError`. Raw stderr never crosses. */
export interface WireError {
  readonly code: string;
  readonly message: string;
}

export class RpcError extends Error {
  readonly code: string;

  constructor(wire: WireError) {
    super(wire.message);
    this.name = 'RpcError';
    this.code = wire.code;
  }
}

/**
 * Kira Space's native transport and the test fakes implement this. The UI's bridge client (`packages/ui/src/bridge/client.ts`,
 * P1+) depends only on this interface, never on a concrete transport.
 */
export interface Transport {
  request<K extends RequestKey>(
    method: K,
    params: ParamsOf<K>,
    signal?: AbortSignal,
  ): Promise<ResultOf<K>>;

  on<K extends EventKey>(method: K, handler: (payload: EventPayload<K>) => void): () => void;

  stream<K extends StreamKey>(
    method: K,
    params: StreamParamsOf<K>,
    onChunk: (chunk: StreamChunkOf<K>) => void,
    signal?: AbortSignal,
  ): Promise<void>;

  dispose(): void;
}
