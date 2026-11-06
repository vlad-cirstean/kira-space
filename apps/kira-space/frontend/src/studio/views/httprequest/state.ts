import { loadDynamicGenerator, type ResolvedRequest, substituteBody } from '@kira/api-core';
import type {
  HttpBodyWire,
  HttpMethod,
  HttpRequestSettingsState,
  HttpRequestSettingsWire,
  HttpRequestTabState,
  HttpResponseWire,
  HttpTimeline,
} from '@shared/domain/http';
import { newUuid } from '@shared/ids';
import { registerTabRuntimeCleanup } from '@workbench/state/tabRuntime';
import { defineStore } from 'pinia';
import { variablesForSend } from '../../api/state/variables';
import { findHttpRequestTab } from '../../api/tabs';
import { control } from '../../bridge/control';
import { useTabIncognitoStore } from '../../state/tabIncognito';
import { createSubstituter, resolvePairs } from '../shared/request/resolve';
import { classifyLoadError, createRuntimeStore, stopOp } from '../shared/viewOp';
import { invalidateJarCookies } from './cookies';
import { useHttpHistoryStore } from './history';

export type { ResolvedRequest } from '@kira/api-core';

// The state's own five-mode fields, translated onto the wire union. Only the fields the active
// mode's own serializer reads are populated — the rest stay at their zero value, since `Body.Mode`
// is what Go's buildBody() switches on (every other member is ignored). Only
// enabled, named rows cross the wire (mirrors the header filter just below in send()).
function buildBodyWire(state: HttpRequestTabState): HttpBodyWire {
  const empty: HttpBodyWire = {
    mode: 'none',
    raw: '',
    code: '',
    codeLanguage: '',
    urlEncoded: [],
    formData: [],
    file: '',
  };
  switch (state.bodyMode) {
    case 'none':
      return empty;
    case 'raw':
      return { ...empty, mode: 'raw', raw: state.body };
    case 'code':
      return { ...empty, mode: 'code', codeLanguage: state.codeLanguage, code: state.code };
    case 'urlencoded':
      return {
        ...empty,
        mode: 'urlencoded',
        urlEncoded: state.urlEncoded
          .filter((f) => f.enabled && f.name.trim() !== '')
          .map((f) => ({ name: f.name, value: f.value })),
      };
    case 'formdata':
      return {
        ...empty,
        mode: 'formdata',
        formData: state.formData
          .filter((f) => f.enabled && f.name.trim() !== '')
          .map((f) => ({
            name: f.name,
            kind: f.kind,
            value: f.value,
            path: f.path,
            contentType: f.contentType,
          })),
      };
    case 'file':
      return { ...empty, mode: 'file', file: state.binaryFile?.path ?? '' };
  }
}

/** Resolves every non-secret {{name}} reference across the URL, enabled headers,
 *  and the active body mode's own substitutable fields — a secret name is left verbatim and
 *  classified 'deferred' (Go finishes it, strictly after op.SetCommand). Pure over its
 *  arguments, called both by send() (to build the wire args) and by HttpRequestView.vue's own
 *  live "unresolved reference" chip (over the same tab state, before a send ever happens).
 *
 *  `dynamic`, when supplied, is send()'s own {{$name}} generator callback — forwarded to
 *  every `resolve()` call this function makes, unchanged. HttpRequestView.vue's live preview never
 *  supplies it (the chip must never generate a value, so it always calls this with three
 *  arguments, never four). */
export function resolveTabState(
  state: HttpRequestTabState,
  values: Readonly<Record<string, string>>,
  secretNames: readonly string[],
  dynamic?: (name: string) => string | null,
): ResolvedRequest {
  // The URL is the one field with query-string delimiter syntax to
  // break — a genuinely-terminal unresolved reference (a typo, an uncatalogued {{$generator}})
  // must not inject a raw space/&/#/= into it. A 'deferred' secret is unaffected either way:
  // sanitizeUrlSpan is never applied to it, so Go's own apivars.Resolve still finds it untouched.
  const { sub, subUrl, refs } = createSubstituter(values, secretNames, dynamic);

  const url = subUrl(state.url);
  const headers = resolvePairs(state.headers, sub);
  const body = substituteBody(buildBodyWire(state), sub);

  return { url, headers, body, refs };
}

// D6: the response is runtime-only, never persisted (mirrors consoleTabStateSchema's own results
// comment) — a restored tab's response pane starts empty, exactly like a fresh one.
interface HttpRequestViewRuntime {
  status: 'idle' | 'running' | 'error' | 'cancelled';
  opId: string | null;
  /** True while send() resolves variables, before Go knows the op: Stop cancels locally. */
  preflight: boolean;
  // timeline is the failed send's own partial timeline (ipcerr.Error.Details,
  // mapHttpError) — undefined for every failure that isn't an HTTP send's own transport/body-read
  // error (classifySendErr is the only producer), so TimelinePane.vue's failure branch has
  // something to render beside the message this error strip has always shown.
  error: { code: string; message: string; timeline?: HttpTimeline } | null;
  response: HttpResponseWire | null;
  // P375: filter inputs outlive the view's unmount on tab switch; session-only, never tab state.
  fieldFilterOpen: boolean;
  fieldFilterQuery: string;
  headerFilter: string;
  cookieFilter: { request: string; response: string };
}

function defaultRuntime(): HttpRequestViewRuntime {
  return {
    status: 'idle',
    opId: null,
    preflight: false,
    error: null,
    response: null,
    fieldFilterOpen: false,
    fieldFilterQuery: '',
    headerFilter: '',
    cookieFilter: { request: '', response: '' },
  };
}

// P90 §2.8: drops every null (inherit) leaf, so Go's Options only ever sees what this request
// actually overrides — the assertion that makes an all-inherit request send `{}`, not a
// seven-key payload of nulls.
function buildSettingsWire(s: HttpRequestSettingsState): HttpRequestSettingsWire {
  const out: HttpRequestSettingsWire = {};
  for (const [k, v] of Object.entries(s)) {
    if (v !== null) (out as Record<string, unknown>)[k] = v;
  }
  return out;
}

export const useHttpRequestViewStore = defineStore('httpRequestView', () => {
  const { runtime, ensureRuntime } = createRuntimeStore<HttpRequestViewRuntime>(defaultRuntime);

  // gRPC's own cleanup already cancels an in-flight call here (state.ts); HTTP's did not
  // — a send kept running in Go and recording history for a tab already gone. stopOp before the
  // delete matches that shape (registerTabRuntimeCleanup is the one place a closing tab's cleanup
  // and an in-flight op's cancellation can share one call).
  registerTabRuntimeCleanup((tabId) => {
    stopOp(runtime[tabId]);
    delete runtime[tabId];
  });

  /** One Send op, run through HttpService.Send → the existing op scheduler — mirrors
   *  console/state.ts's own run() (manual status/opId preamble, no beginOp/applyLoadFailure —
   *  an HTTP failure never touches tabsState.hydrated, since there is no connection to reconnect).
   *  Stage 1 (this function) resolves every non-secret reference before the wire args are
   *  built; collectionId/environmentId travel alongside so Go's stage 2 can finish the rest. */
  async function send(tabId: string): Promise<void> {
    const tab = findHttpRequestTab(tabId);
    if (!tab) return;
    const rt = ensureRuntime(tabId);
    if (rt.status === 'running') return;

    const opId = newUuid();
    rt.status = 'running';
    rt.opId = opId;
    rt.preflight = true;
    rt.error = null;

    try {
      // P112: variablesForSend awaits the tree/environments/variable-row queries (cache-first;
      // refetches only when a broadcast invalidated them) instead of reading a sync-in-store cache.
      const { collectionId, environmentId, values, secretNames } = await variablesForSend(
        tabId,
        tab.state.itemId,
      );
      // P6 D7: no {{$...}} reference means no await and no dynamic-generators chunk fetch.
      const first = resolveTabState(tab.state, values, secretNames);
      const resolved = first.refs.some((r) => r.kind === 'dynamic')
        ? resolveTabState(tab.state, values, secretNames, await loadDynamicGenerator())
        : first;
      // Tab closed or Stop pressed during pre-flight: Go never saw this op, so never send it.
      if (!findHttpRequestTab(tabId) || rt.opId !== opId) return;
      rt.preflight = false;

      const response = await control.httpSend({
        opId,
        tabId,
        method: tab.state.method,
        url: resolved.url,
        headers: resolved.headers,
        body: resolved.body,
        collectionId,
        environmentId,
        // P8 D2: the tab already knows it (http.ts:208) — '' for a scratch tab, exactly like
        // collectionId's own "possibly empty" shape above.
        itemId: tab.state.itemId ?? '',
        incognito: useTabIncognitoStore().isIncognito(tabId),
        options: buildSettingsWire(tab.state.settings),
      });
      // `rt` is a captured reference to the (possibly already cleaned-up) runtime object
      // — closing the tab mid-send never touched `rt.opId` itself (cleanup only stops the op and
      // deletes `runtime[tabId]`), so the opId check alone still passed for a tab that no longer
      // exists, and noteSendRecorded/invalidateJarCookies below ran for it, recreating a history
      // runtime and seq entry (api/state/history.ts) nothing ever cleans up again.
      if (!findHttpRequestTab(tabId)) return;
      if (rt.opId !== opId) return; // superseded by a newer send
      rt.status = 'idle';
      rt.opId = null;
      rt.response = response;
      // P8 D11: refetches the History pane's list when it's the one showing, otherwise just marks
      // it stale — a user who never opens the pane pays no IPC per send.
      useHttpHistoryStore().noteSendRecorded(tabId);
      // P90 item 2: the jar is process-wide, so a Set-Cookie can change any URL's list; a mounted
      // CookiesPane refetches, every other list turns stale.
      void invalidateJarCookies();
    } catch (err) {
      if (!findHttpRequestTab(tabId)) return; // P108 F8: same guard, the failure path's own half
      if (rt.opId !== opId) return;
      rt.opId = null;
      rt.preflight = false;
      const failure = classifyLoadError(err);
      if (failure.kind === 'cancelled') {
        rt.status = 'cancelled';
        return;
      }
      // D8: httpclient's own error codes never land in viewOp.ts's DISCONNECTED_CODES, so this is
      // never actually 'disconnected' — but even if it were, applyLoadFailure/unmarkHydrated are
      // deliberately not called here: a Reconnect gate has nothing to gate on a connectionless tab.
      rt.status = 'error';
      rt.response = null;
      // classifyLoadError (viewOp.ts) is shared by every view's own load path and stays at
      // {kind, code, message} — widening it app-wide for one HTTP-only field would reach five other
      // views that have no use for it. `err.details` is control.ts's own unwrap() addition, read
      // directly here instead.
      const details = (err as { details?: unknown } | undefined)?.details;
      rt.error = {
        code: failure.code,
        message: failure.message,
        timeline: details as HttpTimeline | undefined,
      };
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

  function setHeaderFilter(tabId: string, query: string): void {
    ensureRuntime(tabId).headerFilter = query;
  }

  function setCookieFilter(tabId: string, mode: 'request' | 'response', query: string): void {
    ensureRuntime(tabId).cookieFilter[mode] = query;
  }

  return {
    runtime,
    send,
    stop,
    setFieldFilter,
    setFieldFilterQuery,
    setHeaderFilter,
    setCookieFilter,
  };
});

// The *Copy as curl* dialog's own frozen resolution — computed once on open, the same shape send()
// already demonstrates (the short-circuit: only a request that references a dynamic value pays for
// loadDynamicGenerator()'s chunk). `http/state/curl.ts` cannot import views/**, so this lives here
// rather than there, and is called once rather than on every render — re-running it on a later
// render would re-roll every {{$…}} the dialog is already showing.
export interface ExportResolution {
  method: HttpMethod;
  resolved: ResolvedRequest;
  /** Every distinct secret name stage 1 deferred, in first-encountered order — what the dialog's
   *  reveal loop walks (D10). */
  deferredNames: string[];
}

export async function resolveForExport(tabId: string): Promise<ExportResolution | null> {
  const tab = findHttpRequestTab(tabId);
  if (!tab) return null;

  const { values, secretNames } = await variablesForSend(tabId, tab.state.itemId);
  const first = resolveTabState(tab.state, values, secretNames);
  const resolved = first.refs.some((r) => r.kind === 'dynamic')
    ? resolveTabState(tab.state, values, secretNames, await loadDynamicGenerator())
    : first;

  const deferredNames: string[] = [];
  for (const ref of resolved.refs) {
    if (ref.kind === 'deferred' && !deferredNames.includes(ref.name)) deferredNames.push(ref.name);
  }

  return { method: tab.state.method, resolved, deferredNames };
}
