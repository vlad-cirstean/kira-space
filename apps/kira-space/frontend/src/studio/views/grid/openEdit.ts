// DataView's Commit must stage the grid's open inline edit before building the plan. Imports
// nothing, so it closes no cycle between DataView and the host.
const flushers = new Map<string, () => void>();

export function registerOpenEditFlush(tabId: string, flush: () => void): void {
  flushers.set(tabId, flush);
}

export function unregisterOpenEditFlush(tabId: string): void {
  flushers.delete(tabId);
}

export function flushOpenEdit(tabId: string): void {
  flushers.get(tabId)?.();
}
