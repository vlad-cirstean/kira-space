import { newUuid } from '@shared/ids';
import { classifyLoadError, type LoadFailure, stopOp } from '../viewOp';

/** The op frame HTTP Send and gRPC Call share; each kind's runtime extends it. */
export interface RequestViewRuntime {
  status: 'idle' | 'running' | 'error' | 'cancelled';
  opId: string | null;
  /** True while variables resolve, before Go knows the op: Stop cancels locally. */
  preflight: boolean;
  error: { code: string; message: string } | null;
  // P375: filter inputs outlive the view's unmount on tab switch; session-only, never tab state.
  fieldFilterOpen: boolean;
  fieldFilterQuery: string;
}

export function defaultRequestRuntime(): RequestViewRuntime {
  return {
    status: 'idle',
    opId: null,
    preflight: false,
    error: null,
    fieldFilterOpen: false,
    fieldFilterQuery: '',
  };
}

/** New op id; status running, preflight on, error cleared. */
export function beginRequest(rt: RequestViewRuntime): string {
  const opId = newUuid();
  rt.status = 'running';
  rt.opId = opId;
  rt.preflight = true;
  rt.error = null;
  return opId;
}

/** Local cancel during preflight, else `stopOp`. */
export function stopRequest(rt: RequestViewRuntime | undefined): void {
  if (rt?.preflight) {
    rt.preflight = false;
    rt.opId = null;
    rt.status = 'cancelled';
    return;
  }
  stopOp(rt);
}

/** Clears opId and preflight; cancelled sets status and returns null; otherwise sets status
 *  'error' and returns the failure for the caller's own error fields. */
export function settleRequestFailure(rt: RequestViewRuntime, err: unknown): LoadFailure | null {
  rt.opId = null;
  rt.preflight = false;
  const failure = classifyLoadError(err);
  if (failure.kind === 'cancelled') {
    rt.status = 'cancelled';
    return null;
  }
  rt.status = 'error';
  return failure;
}

export function createRequestFieldFilter(ensureRuntime: (id: string) => RequestViewRuntime): {
  setFieldFilter(id: string, open: boolean, query: string): void;
  setFieldFilterQuery(id: string, query: string): void;
} {
  return {
    setFieldFilter(id, open, query) {
      const rt = ensureRuntime(id);
      rt.fieldFilterOpen = open;
      rt.fieldFilterQuery = query;
    },
    setFieldFilterQuery(id, query) {
      ensureRuntime(id).fieldFilterQuery = query;
    },
  };
}
