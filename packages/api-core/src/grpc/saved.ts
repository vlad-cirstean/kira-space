import type { GrpcSavedRequest } from '@kira/shared/domain/collections';
import type { GrpcRequestTabState } from '@kira/shared/domain/grpc';

// Two views of the same request exist by construction — api_items.request_json
// (the saved one) and tabs.state_json (the tab's, autosaved) — mirrors
// views/httprequest/saved.ts's own reasoning exactly, not merged for the identical reason (opening
// a saved request and trying something must not be destructive).

/** The request half of the tab's state. Dropping the four UI-only fields (itemId, name,
 *  requestPane/responsePane, requestPaneHeight) is what stops resizing the request pane from
 *  marking a request dirty. */
export function toSavedGrpcRequest(state: GrpcRequestTabState): GrpcSavedRequest {
  return {
    target: state.target,
    tlsMode: state.tlsMode,
    caFile: state.caFile,
    serverName: state.serverName,
    descriptorMode: state.descriptorMode,
    protoPath: state.protoPath,
    importPaths: [...state.importPaths],
    service: state.service,
    method: state.method,
    message: state.message,
    metadata: state.metadata.map((m) => ({ ...m })),
  };
}

/** The same fields back, as a patch over tab state. Rows are copied so a stored document and a
 *  tab never share a row object. */
export function fromSavedGrpcRequest(saved: GrpcSavedRequest): Partial<GrpcRequestTabState> {
  return {
    target: saved.target,
    tlsMode: saved.tlsMode,
    caFile: saved.caFile,
    serverName: saved.serverName,
    descriptorMode: saved.descriptorMode,
    protoPath: saved.protoPath,
    importPaths: [...saved.importPaths],
    service: saved.service,
    method: saved.method,
    message: saved.message,
    metadata: saved.metadata.map((m) => ({ ...m })),
  };
}

/** A structural comparison of the two documents — `null` for the saved side means "this tab is
 *  bound to a row we have not read", treated as not dirty. */
export function isGrpcDirty(state: GrpcRequestTabState, saved: GrpcSavedRequest | null): boolean {
  if (!saved) return false;
  return !sameGrpcRequest(toSavedGrpcRequest(state), saved);
}

export function sameGrpcRequest(a: GrpcSavedRequest, b: GrpcSavedRequest): boolean {
  if (
    a.target !== b.target ||
    a.tlsMode !== b.tlsMode ||
    a.caFile !== b.caFile ||
    a.serverName !== b.serverName ||
    a.descriptorMode !== b.descriptorMode ||
    a.protoPath !== b.protoPath ||
    a.service !== b.service ||
    a.method !== b.method ||
    a.message !== b.message
  ) {
    return false;
  }
  if (
    a.importPaths.length !== b.importPaths.length ||
    a.importPaths.some((p, i) => p !== b.importPaths[i])
  ) {
    return false;
  }
  if (a.metadata.length !== b.metadata.length) return false;
  return a.metadata.every(
    (m, i) =>
      m.name === b.metadata[i].name &&
      m.value === b.metadata[i].value &&
      m.enabled === b.metadata[i].enabled &&
      // P22b D6: a description-only edit must mark the tab dirty like any other field edit.
      m.description === b.metadata[i].description,
  );
}
