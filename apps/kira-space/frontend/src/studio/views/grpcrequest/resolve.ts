import type { Reference } from '@kira/api-core';
import type { GrpcMetaPairWire, GrpcRequestTabState } from '@shared/domain/grpc';
import { createSubstituter, resolvePairs } from '../shared/request/resolve';

// {{name}} substitution is reused exactly — the same two-token grammar @kira/api-core's
// resolve() already implements, over gRPC's own three substitutable fields (target, metadata,
// message). Deliberately NOT protoPath/importPaths/caFile (picker-supplied local paths).
// mergedValuesAndSecrets/collectionIdFor used to be hand-copied here from
// views/httprequest/state.ts, because views/grpcrequest/** may not import views/httprequest/**
// (biome.json) — now both live in http/state/{variables,collections}.ts, which both view
// directories already import, so this is a move rather than an abstraction.

export interface ResolvedGrpcRequest {
  target: string;
  metadata: GrpcMetaPairWire[];
  message: string;
  refs: Reference[];
}

/** Resolves every non-secret {{name}} (and {{$dynamic}}, when `dynamic` is supplied)
 *  reference across target/metadata/message — a secret name is left verbatim and classified
 *  'deferred' (Go finishes it, strictly after op.SetCommand). Only enabled, named metadata rows
 *  cross the wire — mirrors buildBodyWire's own header filter (views/httprequest/state.ts). */
export function resolveGrpcTabState(
  state: Pick<GrpcRequestTabState, 'target' | 'metadata' | 'message'>,
  values: Readonly<Record<string, string>>,
  secretNames: readonly string[],
  dynamic?: (name: string) => string | null,
): ResolvedGrpcRequest {
  const { sub, refs } = createSubstituter(values, secretNames, dynamic);

  const target = sub(state.target);
  const metadata = resolvePairs(state.metadata, sub);
  const message = sub(state.message);

  return { target, metadata, message, refs };
}
