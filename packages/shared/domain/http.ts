import { z } from 'zod';

import { type HttpVersion, httpVersionSchema } from './settings';

// The wire shapes live in Go (`internal/httpclient`) and are mirrored, not re-validated,
// here — `control.ts`'s `httpSend` `trust<T>()`s the bound call's result exactly as every other
// bound call does. These types exist so the renderer has something to type against, not to guard
// input Go already produced.

export const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const;
export const httpMethodSchema = /*#__PURE__*/ z.enum(HTTP_METHODS);
export type HttpMethod = z.infer<typeof httpMethodSchema>;

export interface HttpHeaderWire {
  name: string;
  value: string;
}

export interface HttpRedirectHop {
  status: number;
  url: string;
}

// The wire's own mode spelling — httpclient.BodyMode's values. Kept distinct from the state
// schema's own httpBodyModeSchema below (same five values) — the wire never carries the legacy
// 'json' alias, only what Go understands; state.ts's send() has already resolved the
// alias by the time it builds this.
export type HttpBodyModeWire = 'none' | 'raw' | 'code' | 'urlencoded' | 'formdata' | 'file';

// httpclient.Field — one urlencoded row.
export interface HttpFieldWire {
  name: string;
  value: string;
}

// httpclient.FormField — one form-data row. `kind === 'file'` means `path` is an absolute local
// path and `value` is ignored (D4: a file's bytes never cross this bridge, only its path does).
export interface HttpFormFieldWire {
  name: string;
  kind: 'text' | 'file';
  value: string;
  path: string;
  contentType: string;
}

// httpclient.Body — a tagged union: `mode` selects which member is meaningful, every other member
// is ignored. `raw` is the plain-text buffer (raw mode only); `code`/`codeLanguage` are the
// syntax-highlighted buffer and its language (code mode only, one of CODE_LANGUAGES below) — two
// separate buffers so switching between the two modes never loses either one's text (the "every
// mode keeps its own buffer" rule).
export interface HttpBodyWire {
  mode: HttpBodyModeWire;
  raw: string;
  code: string;
  codeLanguage: string;
  urlEncoded: HttpFieldWire[];
  formData: HttpFormFieldWire[];
  file: string;
}

// httpclient.Request — what `HttpService.Send` sends to Go. `collectionId`/`environmentId`
// are both possibly empty (a scratch tab has no collection; no environment may be selected) — Go
// treats an id that resolves to nothing as "no entries" rather than an error, so a deleted
// environment never makes a request unsendable.
export interface HttpRequestWire {
  method: HttpMethod;
  url: string;
  headers: HttpHeaderWire[];
  body: HttpBodyWire;
  collectionId: string;
  environmentId: string;
}

// Three states, computed from resp.ProtoMajor/resp.Proto and whether a proxy applied to
// this URL (http.ProxyFromEnvironment, the same function sharedClient.Transport.Proxy already is).
// 'exact' — HTTP/1.1, no proxy: httputil.DumpRequestOut is byte-identical to the wire.
// 'http2' — the exchange used HTTP/2, whose wire form is binary HPACK frames; the rendering below
// is the equivalent HTTP/1.1 form. 'proxied' — HTTP/1.1 through a proxy: every byte matches
// except the request line's target, which carried the absolute URL on the real wire.
export type HttpWireFidelity = 'exact' | 'http2' | 'proxied';

// P9 D13: the rendered exchange — Go's own `httpclient.WireExchange`, mirrored here. Live-only
// (P9 D7): never stored in a P8 history snapshot, so `HttpResponseWire.wire` below is optional and
// a restored/stored entry simply carries none.
export interface HttpWireExchange {
  /** The request as Go rendered it: exact bytes for `exact` (P9 F7), the equivalent HTTP/1.1
   *  form otherwise (D3). Secret values are masked back to `{{name}}` (D6). */
  request: string;
  /** Status line + headers only — the body is not duplicated here (D5); the pane concatenates the
   *  response object it already has. Alphabetised and canonicalised by net/http (F1/F10). */
  responseHead: string;
  fidelity: HttpWireFidelity;
  /** How many distinct secret values were masked in `request` — 0 for a request using none. */
  maskedSecrets: number;
  /** True when a body was elided or truncated in `request` (D4). */
  requestBodyElided: boolean;
}

// One measured interval. Absent (an `undefined` field, never present as `0`) means the
// phase did not happen — a reused connection has no DNS/connect/TLS, a literal-IP URL has no
// DNS, a plain-`http://` URL has no TLS. That is a different fact from a phase that took no
// measurable time. Milliseconds, fractional — not the truncated-to-integer unit `elapsedMs`
// above already uses (the reused-connection case this exists to explain is exactly the
// sub-millisecond one).
export interface HttpPhase {
  startOffsetMs: number;
  durationMs: number;
}

// One hop's full detail — every redirect hop plus the final one, projected from the same
// httptrace collector `Response.redirects` above is derived from. `headers` and
// `headersElided` are populated for intermediate hops only (the final hop's headers are already
// `HttpResponseWire.headers`) and capped at 8 KiB rendered per hop. `error` is set only on
// the hop a failed send died on.
export interface HttpTimelineHop {
  index: number;
  method: string;
  url: string;
  /** 0 when this hop never got a response (a failed send, D15). */
  status: number;
  statusText: string;
  proto: string;
  headers?: HttpHeaderWire[];
  headersElided?: boolean;
  reused: boolean;
  idleMs?: number;
  /** The peer actually spoken to — behind a proxy, the proxy (F12) — never GetConn's own
   *  `hostPort`, which is not a stable identifier for what the user is talking to. */
  remoteAddr?: string;
  connAttempts: number;
  /** Any 1xx informational response seen before the final one (F9) — the wait phase, when
   *  present, ends at the first of these. */
  info1xx?: number[];
  startOffsetMs: number;
  totalMs: number;
  error?: string;
  dns?: HttpPhase;
  connect?: HttpPhase;
  tls?: HttpPhase;
  wait?: HttpPhase;
  download?: HttpPhase;
}

// `httptrace.ClientTrace` installed once on the send's context, bucketed by
// `checkRedirect` — the same delimiter `Response.redirects` above is derived from — into one hop
// per response seen. `totalMs` is measured directly (`time.Since` over the whole
// send), never summed from hops' own totals (the phases do not sum to a hop's duration, and a
// hop's own total does not have to sum to the send's).
export interface HttpTimeline {
  hops: HttpTimelineHop[];
  totalMs: number;
}

// httpclient.Cookie — one cookie, sent or received (P90 item 2). expires is RFC3339 or '' for a
// session cookie; sameSite is '', 'lax', 'strict' or 'none'. hop is the timeline hop index this
// cookie was sent on or received from (0 for the first).
export interface HttpCookieWire {
  name: string;
  value: string;
  domain: string;
  path: string;
  expires: string;
  maxAge: number;
  secure: boolean;
  httpOnly: boolean;
  sameSite: string;
  hop: number;
}

// httpclient.Response — what comes back. `body`'s meaning depends on `bodyEncoding`: 'utf8' is the
// text itself, 'base64' is the raw bytes so a binary response never gets corrupted round-tripping
// through Go's `encoding/json` (D4).
export interface HttpResponseWire {
  status: number;
  statusText: string;
  proto: string;
  headers: HttpHeaderWire[];
  body: string;
  bodyEncoding: 'utf8' | 'base64';
  bodyBytes: number;
  bodyTruncated: boolean;
  elapsedMs: number;
  finalUrl: string;
  redirects: HttpRedirectHop[];
  /** P9 D2/D7: absent when the send failed before a response existed (D15), a dump error occurred
   *  (D2), or this response was read back out of P8's history store (D7 strips it before Record). */
  wire?: HttpWireExchange;
  /** Populated for a real live send's response, and — unlike `wire` above — kept on a response
   *  read back out of the history store (a timeline is ~550 B–3 KB, nowhere near the
   *  per-entry cap, so there is no reason to strip it the way `wire` is). Optional here —
   *  not `?? {hops: [], totalMs: 0}` defaulted anywhere — so every `tests/ui`/`tests/unit` fixture
   *  (an `HttpResponseWire` object literal with no `timeline` field) stays valid with no fixture
   *  edit; `TimelinePane.vue`'s own empty state is what a genuinely absent timeline renders as. */
  timeline?: HttpTimeline;
  /** What went out per hop (sentCookies) and every hop's own Set-Cookie
   *  (receivedCookies) — both `omitempty` on the wire, so absent (not `[]`) whenever a send carried
   *  no cookies either way. Kept on a response read back out of the history store, the same
   *  posture `timeline` above takes. */
  sentCookies?: HttpCookieWire[];
  receivedCookies?: HttpCookieWire[];
}

// D6: the tab's own persisted headers table — `enabled` has no wire counterpart (a disabled
// header is simply never sent), it exists only so the builder can keep an unchecked row around
// instead of deleting it.
export const httpHeaderSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  value: z.string(),
  enabled: z.boolean().default(true),
  // P22b D6: app-local free text, no wire counterpart — P17 D14's own description field, copied
  // onto every row-shaped configurable field. `.default('')` so a tab saved before this phase
  // restores unchanged.
  description: z.string().default(''),
});
export type HttpHeaderState = z.infer<typeof httpHeaderSchema>;

// The request body's mode vocabulary. P3 originally spelled this after Postman's own Collection
// v2.1 `body.mode` enum verbatim (`'none' | 'raw' | 'urlencoded' | 'formdata' | 'file' | 'graphql'`,
// with `raw` carrying a `rawLanguage` sub-selector for Text/JavaScript/JSON/HTML/XML) specifically
// so a future Postman import/export phase would not need a rename table. This app's own product
// decision since then split that sub-selector in two: `raw` is plain text only, and JavaScript/
// JSON/HTML/XML became their own top-level `code` mode (its own `codeLanguage` field, below).
// GraphQL bodies are dropped entirely — not deprecated, removed.
//
// BREADCRUMB for whoever builds Postman collection import/export: this vocabulary no longer maps
// 1:1 onto Postman's own `body.mode`. Postman still has one `raw` mode with a `language` sub-field
// covering all five; this app now has two separate modes (`raw` == Postman raw·text, `code` ==
// Postman raw·{javascript,json,html,xml}) and no `graphql` mode at all. A real translation belongs
// at that import/export boundary — e.g. a Postman `raw` body with `language: 'json'` becomes this
// app's `code` mode with `codeLanguage: 'json'` on import, and the reverse on export, and a
// Postman `graphql` body has nowhere to land and needs its own explicit decision (refuse the mode,
// or import it as `code`/json against the GraphQL-over-HTTP envelope). This is not implemented here
// — just flagged so it is not rediscovered as a silent data-loss bug.
//
// P15 D6: `code` + `codeLanguage: 'json'` is now *presented* as its own top-level `JSON` segment in
// RequestBodyPane.vue — a UI-level promotion over this exact same storage, not a new mode. Do not
// be surprised that a `JSON` segment exists with no `'json'` entry in HTTP_BODY_MODES below; there
// is deliberately no new wire/Go value here.
export const HTTP_BODY_MODES = ['none', 'raw', 'code', 'urlencoded', 'formdata', 'file'] as const;
export type HttpBodyMode = (typeof HTTP_BODY_MODES)[number];
export const httpBodyModeSchema = /*#__PURE__*/ z.enum(HTTP_BODY_MODES);

// The `code` mode's sub-selector — what used to be four of raw's five sub-languages (everything
// but Text, which is what plain `raw` now means).
export const CODE_LANGUAGES = ['javascript', 'json', 'html', 'xml'] as const;
export type HttpCodeLanguage = (typeof CODE_LANGUAGES)[number];
export const httpCodeLanguageSchema = /*#__PURE__*/ z.enum(CODE_LANGUAGES);

// The default Content-Type per code sub-language — mirrors Go's contentTypeByCodeLanguage
// map[string]string literal (internal/httpclient/body.go) exactly; plain `raw` always sends
// text/plain and needs no table. tests/unit/go-ts-vocabulary-parity.spec.ts guards the two Content-
// Type tables from drifting apart.
export const CONTENT_TYPE_BY_CODE_LANGUAGE: Readonly<Record<HttpCodeLanguage, string>> = {
  javascript: 'application/javascript',
  json: 'application/json',
  html: 'text/html',
  xml: 'application/xml',
};

// One urlencoded row. `enabled` is builder state only, never wire state (P2 D6's rule for
// headers, reused here) — a disabled row is simply filtered out before the send args are built.
export const httpUrlEncodedFieldSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  value: z.string(),
  enabled: z.boolean().default(true),
  // P22b D6: see httpHeaderSchema's own comment.
  description: z.string().default(''),
});
export type HttpUrlEncodedFieldState = z.infer<typeof httpUrlEncodedFieldSchema>;

// One form-data row — a text row uses `value`, a file row uses `path` (never bytes) plus
// `fileName`/`fileSize` so the builder can render `report.csv (1.2 MB)` with no round trip back to
// disk. `contentType` is the row's own per-part override; blank means the mode's default
// (the row's own Content type field when set, else application/octet-stream for a file).
export const httpFormDataFieldSchema = /*#__PURE__*/ z.object({
  name: z.string(),
  kind: /*#__PURE__*/ z.enum(['text', 'file']).default('text'),
  value: z.string().default(''),
  path: z.string().default(''),
  fileName: z.string().default(''),
  fileSize: z.number().default(0),
  contentType: z.string().default(''),
  enabled: z.boolean().default(true),
  // P22b D6: see httpHeaderSchema's own comment.
  description: z.string().default(''),
});
export type HttpFormDataFieldState = z.infer<typeof httpFormDataFieldSchema>;

// The binary (Postman `file`) body's one chosen file — path only, never bytes.
export const httpBinaryFileSchema = /*#__PURE__*/ z
  .object({ path: z.string(), name: z.string(), size: z.number() })
  .nullable()
  .default(null);
export type HttpBinaryFileState = z.infer<typeof httpBinaryFileSchema>;

// P90 §2.6: 'settings' and 'cookies' are the sixth and seventh members — safe for the reason P8
// D10/P9 D12/P10 D11 already verified three times over on the response enum below, and which
// their own comments state: every previously-stored value is still a member.
export const httpRequestPaneSchema = /*#__PURE__*/ z.enum([
  'params',
  'headers',
  'body',
  'settings',
  'cookies',
]);
export type HttpRequestPane = z.infer<typeof httpRequestPaneSchema>;

// 'history' is the third pane — widening this enum cannot break a restored
// tab (every stored value is still a member). 'raw' is the fourth, verified safe by
// the identical reasoning — the naming collision with responseView's own 'raw' value below is
// resolved (the two are never rendered at once), not by renaming either. 'timeline' is the fifth,
// safe by the same reasoning again.
// 'cookies' is the sixth member, safe by the same reasoning as every widening above.
export const httpResponsePaneSchema = /*#__PURE__*/ z.enum([
  'body',
  'headers',
  'history',
  'raw',
  'timeline',
  'cookies',
]);
export type HttpResponsePane = z.infer<typeof httpResponsePaneSchema>;

export const httpResponseViewSchema = /*#__PURE__*/ z.enum(['pretty', 'raw']);
export type HttpResponseView = z.infer<typeof httpResponseViewSchema>;

// One nullable twin per api settings leaf — null means "inherit the global default", which
// is what a request that has never touched this panel carries. Not a sentinel string and not a
// parallel `inherited` flag: `itemId` and `binaryFile` above already spell "absent" as null in
// this same schema, and null is what the wire's own *T pointer decodes from. Bounds are
// deliberately looser than apiSettingsSchema's own (min only, no max): a stored tab row is
// normalised through this schema on restore and a value outside range must hydrate rather than
// drop the whole tab (repos/tabs.go drops a row outright on a failed parse) — the range is
// enforced elsewhere without data loss: the number inputs' min/max, and Go's Options.normalize(),
// which clamps.
export const httpRequestSettingsSchema = /*#__PURE__*/ z.object({
  httpVersion: httpVersionSchema.nullable().default(null),
  requestTimeoutMs: z.number().int().min(0).nullable().default(null),
  maxResponseMb: z.number().int().min(0).nullable().default(null),
  sslVerify: z.boolean().nullable().default(null),
  followRedirects: z.boolean().nullable().default(null),
  maxRedirects: z.number().int().min(0).nullable().default(null),
  disableCookieJar: z.boolean().nullable().default(null),
});
export type HttpRequestSettingsState = z.infer<typeof httpRequestSettingsSchema>;

// The wire shape sent to HttpService.Send's Options field (bridge/http.go) — optional keys, not
// nullable ones: Go decodes an absent key to nil, and a present `null` to nil too, so either
// serialises correctly, but optional keeps the payload smaller (only the overridden leaves ride
// along, buildSettingsWire in state.ts is what drops the nulls).
export interface HttpRequestSettingsWire {
  httpVersion?: HttpVersion;
  requestTimeoutMs?: number;
  maxResponseMb?: number;
  sslVerify?: boolean;
  followRedirects?: boolean;
  maxRedirects?: number;
  disableCookieJar?: boolean;
}

// Every field carries `.default()` so a tab saved by an older version still restores once a
// later phase widens `bodyMode` or adds a field — the same discipline `keyValueTabStateSchema`'s
// own comment records (tabs.ts), and it matters more here than anywhere else because
// `repos/tabs.go` drops a row outright on a failed parse (the restore path is fixed so these
// defaults fire, `state/tabKinds.ts`'s `parseState`). There is deliberately no `params`
// array: the URL is the single source of truth for the query string, and the Params table is
// a derived editor over it.
//
// Every body mode keeps its own buffer (flat siblings, not one nullable per-mode object) —
// switching between modes must not lose any of their text, and flat keeps every field individually
// `.default()`-able, which is what the restore-through-schema normalization relies on.
export const httpRequestTabStateSchema = /*#__PURE__*/ z.object({
  method: httpMethodSchema.default('GET'),
  url: z.string().default(''),
  headers: /*#__PURE__*/ z.array(httpHeaderSchema).default([]),
  bodyMode: httpBodyModeSchema.default('none'),
  body: z.string().default(''),
  code: z.string().default(''),
  codeLanguage: httpCodeLanguageSchema.default('json'),
  urlEncoded: /*#__PURE__*/ z.array(httpUrlEncodedFieldSchema).default([]),
  formData: /*#__PURE__*/ z.array(httpFormDataFieldSchema).default([]),
  binaryFile: httpBinaryFileSchema,
  // A query param has nowhere to store a description on its own row — there is no
  // `params` array (the comment above this shape explains why one is never added), so a
  // description is annotation over the URL's own query string, keyed by param name, never input to
  // buildQuery. A duplicate param name (`?a=1&a=2`) shares one description — a real, named
  // limitation rather than a second array that could disagree with the URL about what is
  // sent.
  paramDescriptions: /*#__PURE__*/ z.record(z.string(), z.string()).default({}),
  // FieldRowsTable's own description-column toggle, persisted per tab rather than runtime-only
  // (a user who wants it always visible should not have to re-open it every time the tab restores)
  // — shared by every one of this tab's row tables (headers, params, urlencoded, form-data), which
  // is why it is one flag, not four.
  fieldDescriptions: z.boolean().default(false),
  // A saved request's identity and its name, both `.default()`ed like every other field
  // so a tab saved earlier restores unchanged. `itemId` is the api_items row this tab is bound
  // to (null = a scratch request that has never been saved); it lives here rather than in the
  // tab's `path` because duplicateTab copies `path` verbatim while duplicateState clears the id,
  // which would leave two disagreeing sources of one fact and make openTab's reuse lookup
  // activate the *duplicate* when the user opened the original. `name` is what keeps
  // httpRequestTitle pure: without it a saved request called "Create order" would show as
  // /v2/orders everywhere, and the alternative is teaching every title consumer about collections.
  itemId: z.string().nullable().default(null),
  name: z.string().default(''),
  // P90: per-request overrides of the seven api settings leaves, every leaf null (inherit) on a
  // request that has never touched the Settings panel. `.default({})` doesn't type-check against
  // this pinned zod (§2.2's own hedge) — the seven-null literal is behaviourally identical.
  settings: httpRequestSettingsSchema.default({
    httpVersion: null,
    requestTimeoutMs: null,
    maxResponseMb: null,
    sslVerify: null,
    followRedirects: null,
    maxRedirects: null,
    disableCookieJar: null,
  }),
  requestPane: httpRequestPaneSchema.default('params'),
  responsePane: httpResponsePaneSchema.default('body'),
  responseView: httpResponseViewSchema.default('pretty'),
  // 0 = "the default half" — PanelSplitter's own convention for "no explicit size saved yet".
  requestPaneHeight: z.number().int().min(0).default(0),
});

export type HttpRequestTabState = z.infer<typeof httpRequestTabStateSchema>;

export function defaultHttpRequestTabState(): HttpRequestTabState {
  return httpRequestTabStateSchema.parse({});
}

// D11: one shared table, rendered inline beside the status chip rather than tooltip-only — the
// case that matters (4xx/5xx) is exactly the case where a hover shouldn't be required to see it.
// No Go mirror: nothing on the Go side consults this.
export const STATUS_HINTS: Readonly<Record<number, string>> = {
  200: 'the request succeeded',
  201: 'the request succeeded and a resource was created',
  202: 'the request was accepted for processing, but not yet completed',
  204: 'the request succeeded and there is no response body',
  301: 'the resource has permanently moved to a new URL',
  302: 'the resource is temporarily at a different URL',
  304: 'the cached response is still valid — nothing new to transfer',
  400: 'the server could not understand the request as sent',
  401: 'authentication is required, or has failed',
  403: 'the server understood the request but refuses to authorize it',
  404: 'the server has no resource at this URL',
  405: 'the URL exists, but not for this method',
  408: 'the server gave up waiting for the rest of the request',
  409: 'the request conflicts with the resource’s current state',
  413: 'the request body is larger than the server will accept',
  415: 'the server does not support the request body’s content type',
  422: 'the request was well-formed but semantically invalid',
  429: 'too many requests — the client is being rate-limited',
  500: 'the server encountered an unexpected error',
  501: 'the server does not support the functionality required',
  502: 'a gateway received an invalid response from an upstream server',
  503: 'the server is temporarily unable to handle the request',
  504: 'a gateway timed out waiting for an upstream server',
};

/** A per-code sentence, falling back to a class-level one for a code not in the table. */
export function statusHint(status: number): string {
  const known = STATUS_HINTS[status];
  if (known) return known;
  if (status >= 100 && status < 200) return 'informational — the exchange is not yet complete';
  if (status >= 200 && status < 300) return 'the request succeeded';
  if (status >= 300 && status < 400) return 'the request was redirected';
  if (status >= 400 && status < 500) return 'the client request was rejected';
  if (status >= 500 && status < 600) return 'the server failed to fulfil a valid request';
  return 'unrecognised status code';
}

export function statusClass(status: number): 'info' | 'ok' | 'warn' | 'error' {
  if (status >= 100 && status < 200) return 'info';
  if (status >= 200 && status < 300) return 'ok';
  if (status >= 300 && status < 400) return 'warn';
  return 'error';
}

// statusClass's exact sibling, in its exact home. Three surfaces need the same map — the request
// view's own method chip/select, the response history list's chip, and the collections tree's
// per-row chip — and `http/**` may not import `views/**` (biome.json), so it lives here rather
// than being copied into all three.
//
// Takes a plain `string`, not HttpMethod: an imported PROPFIND has to have a token rather than
// throw. `httpMethodClass`'s four-family map (GET/HEAD/OPTIONS -> info, POST -> ok,
// PUT/PATCH -> warn, DELETE -> err) could not express "each of the five distinct" the SPEC asks
// for — PUT and PATCH shared one family — so a seven-method (plus 'other') token replaces it
// outright, each aliased in tokens.css to a hue from the twelve-colour connection palette rather
// than the four-family status palette. Nothing here reads httpMethodClass any more.
const METHOD_TOKEN: Readonly<Record<string, MethodToken>> = {
  GET: 'get',
  POST: 'post',
  PUT: 'put',
  PATCH: 'patch',
  DELETE: 'delete',
  HEAD: 'head',
  OPTIONS: 'options',
};

export type MethodToken =
  | 'get'
  | 'post'
  | 'put'
  | 'patch'
  | 'delete'
  | 'head'
  | 'options'
  | 'other';

export function httpMethodToken(method: string): MethodToken {
  return METHOD_TOKEN[method.toUpperCase()] ?? 'other';
}
