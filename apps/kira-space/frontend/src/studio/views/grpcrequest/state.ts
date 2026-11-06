import { loadDynamicGenerator } from '@kira/api-core';
import type {
  GrpcCallEvent,
  GrpcCallResultWire,
  GrpcMessageWire,
  GrpcSchemaWire,
} from '@shared/domain/grpc';
import { newUuid } from '@shared/ids';
import { t } from '@theme/i18n';
import { queryClient } from '@workbench/state/queryClient';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { markRaw } from 'vue';
import { variablesForSend } from '../../api/state/variables';
import { findGrpcRequestTab } from '../../api/tabs';
import { control } from '../../bridge/control';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import { classifyLoadError, createRuntimeStore, stopOp } from '../shared/viewOp';
import { useGrpcCallHistoryStore } from './history';
import { resolveGrpcTabState } from './resolve';
import { grpcSchemaKey, grpcSchemaSourceFor } from './schemaQuery';

// D15: the live view's own ceiling — an infinite stream must not grow the renderer without bound.
// The oldest messages are dropped once this is exceeded; trueMessageCount (below) keeps the real
// total so D17's "showing the most recent 10,000 of N" sentence can still name it.
const MAX_LIVE_MESSAGES = 10_000;
// First tried trimming back to 90% of the cap instead of the cap itself, to amortize the splice
// cost once the ceiling is first reached — but the live view keeps exactly the most recent
// MAX_LIVE_MESSAGES, and grpc-request.spec.ts's own "caps at 10,000 and shows the true total"
// test asserts that count right after a single batch crosses the ceiling. Trimming to 90% left
// it at 9,000 there, so the amortized target was reverted — every push over the cap still trims
// back to exactly MAX_LIVE_MESSAGES.

// D6: the response is runtime-only, never persisted.
interface GrpcRequestViewRuntime {
  status: 'idle' | 'running' | 'error' | 'cancelled';
  opId: string | null;
  /** True while call() resolves variables, before Go knows the op: Stop cancels locally. */
  preflight: boolean;
  /** The streaming event subscriber's own key, set alongside `opId` when a call
   *  starts and never cleared early the way `opId` is. Messages arrive over the event channel
   *  while the call's own CallResult returns over the control plane — a separate HTTP request —
   *  with nothing ordering the two. If the control-plane response lands first, the return path
   *  below clears `opId` (its own "am I still the call in flight" guard) before the terminal
   *  event — carrying up to grpcCoalesceMaxBatch (64) trailing messages plus the terminal status —
   *  has necessarily been delivered; matching the event against `opId` then drops that terminal
   *  event outright. Matching against `lastCallId` instead keeps the event subscriber correct
   *  regardless of which side finishes first, since only a *new* call ever changes it. */
  lastCallId: string | null;
  /** The callId `noteGrpcCallRecorded` has already fired for. For a server-streaming call,
   *  both the control-plane return (`call`'s own success path) and the terminal streaming event
   *  (`applyGrpcEvent`'s `done` branch) observe "this call finished" — whichever lands first.
   *  Both used to call `noteGrpcCallRecorded` unconditionally, recording the same call twice and
   *  feeding history's own `load()` two overlapping refreshes close enough together to retry-loop
   *  forever. Guarding on this field records once per callId regardless of which side notices
   *  completion first — unary (no streaming events) still records once, from the return path, same
   *  as before. */
  notifiedCallId: string | null;
  streaming: boolean;
  error: { code: string; message: string } | null;
  result: GrpcCallResultWire | null;
  /** Capped at MAX_LIVE_MESSAGES (D15) — the oldest are spliced off the head as new ones arrive. */
  messages: GrpcMessageWire[];
  /** The true count of every message this call has produced so far — kept incrementally
   *  (never re-derived from messages.length, which is capped) so D17's "showing the most recent
   *  10,000 of N" sentence can name the real N once messages.length has hit the ceiling. */
  trueMessageCount: number;
  /** Running total of wireBytes across every message received so far, updated once per push
   *  rather than re-`reduce`d over `messages` on every read — the same reasoning as
   *  trueMessageCount, and what makes the response pane's own byte total O(1) per message instead
   *  of O(N) per push. */
  messageBytes: number;
  // P375: filter inputs outlive the view's unmount on tab switch; session-only, never tab state.
  fieldFilterOpen: boolean;
  fieldFilterQuery: string;
  schemaFilter: string;
}

function defaultRuntime(): GrpcRequestViewRuntime {
  return {
    status: 'idle',
    opId: null,
    preflight: false,
    lastCallId: null,
    notifiedCallId: null,
    streaming: false,
    error: null,
    result: null,
    messages: [],
    trueMessageCount: 0,
    messageBytes: 0,
    fieldFilterOpen: false,
    fieldFilterQuery: '',
    schemaFilter: '',
  };
}

/** Finds one method in a loaded schema by "Service/Method" full name. */
export function findMethod(
  schema: GrpcSchemaWire | null,
  service: string,
  method: string,
): { clientStreaming: boolean; serverStreaming: boolean; requestTemplate: string } | null {
  if (!schema) return null;
  const svc = schema.services.find((s) => s.fullName === service);
  const m = svc?.methods.find((m) => m.name === method);
  return m ?? null;
}

// The event handler's own per-call apply step, lifted out so
// ensureGrpcCallSubscription's callback only finds the matching tab and delegates. `rt` is the
// same reactive runtime object the caller already found — mutated here exactly as it was inline,
// so this carries no reactive-dependency-tracking risk (unlike a computed, a plain event handler
// has no active effect to preserve; Vue only needs the write to happen, not where it's written
// from).
function applyGrpcEvent(tabId: string, rt: GrpcRequestViewRuntime, event: GrpcCallEvent): void {
  // markRaw on each incoming message before it ever
  // touches the reactive rt.messages array — a GrpcMessageWire is an immutable wire value
  // (this view never mutates a field of one in place), so nothing here needs Vue to wrap it
  // in its own per-field reactive Proxy. Without this, every pushed message was itself
  // deep-proxied on the way in, on top of the array's own reactivity.
  rt.messages.push(...event.messages.map((m) => markRaw(m)));
  rt.trueMessageCount += event.messages.length;
  for (const m of event.messages) rt.messageBytes += m.wireBytes;
  if (rt.messages.length > MAX_LIVE_MESSAGES) {
    rt.messages.splice(0, rt.messages.length - MAX_LIVE_MESSAGES);
  }
  if (event.done) {
    rt.opId = null;
    if (event.error) {
      rt.status = event.error.code === 'E_CANCELLED' ? 'cancelled' : 'error';
      rt.error = event.error;
      if (event.status) rt.result = event.status;
    } else {
      rt.status = 'idle';
      rt.result = event.status ?? rt.result;
    }
    // P108 F1: dedupe against call()'s own return-path notify (see `notifiedCallId` doc) —
    // whichever of the two observes this call's completion first wins.
    if (rt.notifiedCallId !== event.callId) {
      rt.notifiedCallId = event.callId;
      useGrpcCallHistoryStore().noteGrpcCallRecorded(tabId);
    }
  }
}

export const useGrpcRequestViewStore = defineStore('grpcRequestView', () => {
  const { runtime, ensureRuntime } = createRuntimeStore<GrpcRequestViewRuntime>(defaultRuntime);

  // dropResources is noDrop (the registry entry) — the runtime lives here, and a still-running
  // call must be cancelled through this hook rather than through dropResources
  // (registerTabRuntimeCleanup is the one place a closing tab's cleanup and an in-flight op's
  // cancellation can share one call).
  registerTabRuntimeCleanup((tabId) => {
    stopOp(runtime[tabId]);
    delete runtime[tabId];
  });

  // ---- D7/D8: the call itself ----

  let subscribedToGrpcCall = false;
  function ensureGrpcCallSubscription(): void {
    if (subscribedToGrpcCall) return;
    subscribedToGrpcCall = true;
    control.onGrpcCall((event) => {
      for (const tabId of Object.keys(runtime)) {
        const rt = runtime[tabId];
        if (!rt || rt.lastCallId !== event.callId) continue;
        applyGrpcEvent(tabId, rt, event);
        break;
      }
    });
  }

  /** D7/D8: one Call op, run through GrpcService.Call → the existing op scheduler. Unary and
   *  server-streaming both go through this one function — Go is told which (`streaming`) from the
   *  method the schema already resolved. */
  async function call(tabId: string): Promise<void> {
    const tab = findGrpcRequestTab(tabId);
    if (!tab) return;
    const rt = ensureRuntime(tabId);
    if (rt.status === 'running') return;
    ensureGrpcCallSubscription();

    const schema =
      queryClient.getQueryData<GrpcSchemaWire>(grpcSchemaKey(grpcSchemaSourceFor(tab))) ?? null;
    const method = findMethod(schema, tab.state.service, tab.state.method);
    // This used to default `streaming` to false whenever the schema wasn't loaded
    // yet (a Call issued right after mount, or right after switching method, before its own
    // schema query round trip resolved) — a genuinely server-streaming method
    // then went out over the wire as a one-shot unary call, silently wrong rather than merely
    // delayed. GrpcRequestView.vue's own Call button now disables until findMethod can resolve
    // too, but Enter/the command palette reach this function directly, so the real guard belongs
    // here: bail with a clear error instead of guessing.
    if (!method) {
      rt.status = 'error';
      rt.error = {
        code: 'client',
        message: t('studio.grpc.call.methodUnresolved.error'),
      };
      return;
    }
    const streaming = method.serverStreaming;

    const opId = newUuid();
    rt.status = 'running';
    rt.opId = opId;
    rt.lastCallId = opId;
    rt.error = null;
    rt.result = null;
    rt.messages = [];
    rt.trueMessageCount = 0;
    rt.messageBytes = 0;
    rt.streaming = streaming;

    rt.preflight = true;

    try {
      const { collectionId, environmentId, values, secretNames } = await variablesForSend(
        tabId,
        tab.state.itemId,
      );
      const first = resolveGrpcTabState(tab.state, values, secretNames);
      const resolved = first.refs.some((r) => r.kind === 'dynamic')
        ? resolveGrpcTabState(tab.state, values, secretNames, await loadDynamicGenerator())
        : first;
      // Tab closed or Stop pressed during pre-flight: Go never saw this op, so never call it.
      if (!findGrpcRequestTab(tabId) || rt.opId !== opId) return;
      rt.preflight = false;

      const result = await control.grpcCall({
        opId,
        tabId,
        streaming,
        descriptorMode: tab.state.descriptorMode,
        target: resolved.target,
        tls: {
          enabled: tab.state.tlsMode === 'tls',
          caFile: tab.state.caFile,
          serverName: tab.state.serverName,
        },
        protoPath: tab.state.protoPath,
        importPaths: tab.state.importPaths,
        service: tab.state.service,
        method: tab.state.method,
        messageJson: resolved.message,
        metadata: resolved.metadata,
        collectionId,
        environmentId,
        itemId: tab.state.itemId ?? '',
        incognito: useTabIncognitoStore().isIncognito(tabId),
      });
      if (rt.opId !== opId) return; // superseded, or the streaming subscription already finished it
      rt.status = 'idle';
      rt.opId = null;
      rt.result = result;
      if (!streaming && result.messages) {
        rt.messages = result.messages;
        rt.trueMessageCount = result.messages.length;
        rt.messageBytes = result.messages.reduce((n, m) => n + m.wireBytes, 0);
      }
      // P108 F1: dedupe against applyGrpcEvent's own terminal-event notify (see `notifiedCallId`
      // doc) — for a streaming call the terminal event may already have recorded this callId.
      if (rt.notifiedCallId !== opId) {
        rt.notifiedCallId = opId;
        useGrpcCallHistoryStore().noteGrpcCallRecorded(tabId);
      }
    } catch (err) {
      if (!findGrpcRequestTab(tabId) || rt.opId !== opId) return;
      rt.opId = null;
      rt.preflight = false;
      const failure = classifyLoadError(err);
      if (failure.kind === 'cancelled') {
        rt.status = 'cancelled';
        return;
      }
      rt.status = 'error';
      rt.error = { code: failure.code, message: failure.message };
    }
  }

  function stop(tabId: string): void {
    const rt = runtime[tabId];
    if (rt?.preflight) {
      rt.preflight = false;
      rt.opId = null;
      rt.status = 'cancelled';
      return;
    }
    stopOp(rt);
  }

  function setFieldFilter(tabId: string, open: boolean, query: string): void {
    const rt = ensureRuntime(tabId);
    rt.fieldFilterOpen = open;
    rt.fieldFilterQuery = query;
  }

  function setFieldFilterQuery(tabId: string, query: string): void {
    ensureRuntime(tabId).fieldFilterQuery = query;
  }

  function setSchemaFilter(tabId: string, query: string): void {
    ensureRuntime(tabId).schemaFilter = query;
  }

  return { runtime, call, stop, setFieldFilter, setFieldFilterQuery, setSchemaFilter };
});
