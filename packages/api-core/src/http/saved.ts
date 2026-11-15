import type { HttpSavedRequest } from '@kira/shared/domain/collections';
import { HTTP_METHODS, type HttpMethod, type HttpRequestTabState } from '@kira/shared/domain/http';

// Two views of the same request exist by construction — api_items.request_json (the saved one)
// and tabs.state_json (the tab's, autosaved on the existing 1 s debounce).
// They are deliberately not merged:
//
//   - Autosaving edits straight into the collection row was considered — it is what tabs, layout
//     and settings all do. Declined: it makes "open a saved request and try something"
//     destructive, which is the single most common thing anyone does with a saved request, and it
//     leaves nowhere to stand for a Revert.
//   - A stored dirty flag was also declined. Dirtiness is toSavedRequest(state) compared against
//     the saved document — a pure function of two things already in memory, so there is no flag to
//     set, clear, migrate or get wrong.
//
// Pure and DOM-free, in the spirit of url.ts.

/** The request half of the tab's state. Dropping the four UI-only fields (requestPane,
 *  responsePane, responseView, requestPaneHeight) plus itemId/name is what stops resizing the
 *  request pane from marking a request dirty. */
export function toSavedRequest(state: HttpRequestTabState): HttpSavedRequest {
  return {
    method: state.method,
    url: state.url,
    headers: state.headers.map((h) => ({ ...h })),
    bodyMode: state.bodyMode,
    body: state.body,
    code: state.code,
    codeLanguage: state.codeLanguage,
    urlEncoded: state.urlEncoded.map((f) => ({ ...f })),
    formData: state.formData.map((f) => ({ ...f })),
    binaryFile: state.binaryFile ? { ...state.binaryFile } : null,
    paramDescriptions: { ...state.paramDescriptions },
  };
}

/** The same fields back, as a patch over tab state — the caller supplies itemId/name and whatever
 *  UI-only fields the tab already had. Rows are copied so a stored document and a tab never share
 *  a row object (the same reasoning duplicateState's own deep copy carries). */
export function fromSavedRequest(saved: HttpSavedRequest): Partial<HttpRequestTabState> {
  return {
    method: toBuilderMethod(saved.method),
    url: saved.url,
    headers: saved.headers.map((h) => ({ ...h })),
    bodyMode: saved.bodyMode,
    body: saved.body,
    code: saved.code,
    codeLanguage: saved.codeLanguage,
    urlEncoded: saved.urlEncoded.map((f) => ({ ...f })),
    formData: saved.formData.map((f) => ({ ...f })),
    binaryFile: saved.binaryFile ? { ...saved.binaryFile } : null,
    paramDescriptions: { ...saved.paramDescriptions },
  };
}

// Postman's method list is 15 members plus any custom string; this app's builder shows
// seven, and httpRequestTabStateSchema's enum would *throw* on the rest (a Zod .default() only
// fires for `undefined`, never for a bad value). So the coercion happens here — the one function
// every read path goes through — rather than at each call site. The original spelling stays in
// api_items.request_json and in origin_json, so an untouched request still exports as PROPFIND;
// widening the vocabulary is a request-builder change.
const BUILDER_METHODS = new Set<string>(HTTP_METHODS);

export function toBuilderMethod(method: string): HttpMethod {
  const upper = method.toUpperCase();
  return BUILDER_METHODS.has(upper) ? (upper as HttpMethod) : 'GET';
}

/** A structural comparison of the two documents. `null` for the saved side means "this tab is
 *  bound to a row we have not read (or that no longer resolves)" — treated as not dirty, since
 *  there is nothing to be different from and D14's orphan rule sends Save to Save as… anyway. */
export function isDirty(state: HttpRequestTabState, saved: HttpSavedRequest | null): boolean {
  if (!saved) return false;
  // The saved side is compared through the same coercion the tab was opened with, so a request
  // whose method this builder cannot show does not read as edited the instant it is opened. Once
  // the user genuinely edits it, Save writes the coerced method — which is the stated
  // boundary (an edited member is rebuilt in this app's canonical form), not a silent loss.
  return !sameRequest(toSavedRequest(state), { ...saved, method: toBuilderMethod(saved.method) });
}

// A hand-written comparison rather than JSON.stringify: key order is not guaranteed across two
// objects built by different code paths (one from tab state, one from Go's own JSON), and a
// stringify mismatch on ordering alone would light the dirty mark on an untouched request.
export function sameRequest(a: HttpSavedRequest, b: HttpSavedRequest): boolean {
  if (
    a.method !== b.method ||
    a.url !== b.url ||
    a.bodyMode !== b.bodyMode ||
    a.body !== b.body ||
    a.code !== b.code ||
    a.codeLanguage !== b.codeLanguage
  ) {
    return false;
  }
  if (!sameRows(a.headers, b.headers, sameNameValue)) return false;
  if (!sameRows(a.urlEncoded, b.urlEncoded, sameNameValue)) return false;
  if (!sameRows(a.formData, b.formData, sameFormField)) return false;
  if (!sameBinaryFile(a.binaryFile, b.binaryFile)) return false;
  return sameStringMap(a.paramDescriptions, b.paramDescriptions);
}

function sameRows<T>(a: readonly T[], b: readonly T[], eq: (x: T, y: T) => boolean): boolean {
  return a.length === b.length && a.every((row, i) => eq(row, b[i]));
}

// P22b D7: paramDescriptions' own comparison — key order is exactly the "not guaranteed across
// two objects built by different code paths" case this file's own top comment already names.
function sameStringMap(a: Record<string, string>, b: Record<string, string>): boolean {
  const aKeys = Object.keys(a);
  if (aKeys.length !== Object.keys(b).length) return false;
  return aKeys.every((k) => a[k] === b[k]);
}

function sameNameValue(
  a: { name: string; value: string; enabled: boolean; description: string },
  b: { name: string; value: string; enabled: boolean; description: string },
): boolean {
  return (
    a.name === b.name &&
    a.value === b.value &&
    a.enabled === b.enabled &&
    // P22b D6: a description-only edit must mark the tab dirty like any other field edit — it is
    // saved (toSavedRequest above copies it verbatim), so leaving it out here would make Save
    // silently drop a description the user just typed until some other field also changed.
    a.description === b.description
  );
}

function sameFormField(
  a: HttpSavedRequest['formData'][number],
  b: HttpSavedRequest['formData'][number],
): boolean {
  return (
    a.name === b.name &&
    a.kind === b.kind &&
    a.value === b.value &&
    a.path === b.path &&
    a.contentType === b.contentType &&
    a.enabled === b.enabled &&
    a.description === b.description
  );
}

// fileName/fileSize are display conveniences derived from the path, so re-picking the same file
// must not read as an edit — the same members internal/postman's own comparison leaves out.
function sameBinaryFile(
  a: HttpSavedRequest['binaryFile'],
  b: HttpSavedRequest['binaryFile'],
): boolean {
  if (a === null || b === null) return a === b;
  return a.path === b.path;
}
