# P627 journeys: Api module (HTTP, gRPC, collections, variables, environments, import)

Imagined usage and definition of correct. Conventions: `journeys-studio-connections.md` head. Inputs:
`inventory.md` sections 13-15; `coverage-map.md`; source read through CodeGraph (`repos/response_history.go`,
`ResponseHistoryList.vue`).

Far end: "wire recorder" = `flowharness.HTTP/HTTPS/GRPC` server recording each request (method, path, headers,
body bytes, metadata). History facts read from source: `Record` is sole writer; per-scope cap
`HISTORY_PER_SCOPE_LIMIT` plus byte budget sweep; `Wire` never stored; binary (base64) response body not stored,
`BodyBytes` kept; text body over cap truncated on UTF-8 boundary with flag; oversized snapshot drops request
fields and response headers/cookies with `RequestFieldsElided`; Compare enabled only with exactly 2 ticked.

## A. HTTP request builder and send

### J-HTTP-001 Build request from scratch, send, save, restart

- Status: partial (`httpflow` Bodies, AuthHeaders; `apiflow` SavedRequestHistory, RestartKeepsApiState; ui
  `http-request`).
- Persona: backend developer testing new endpoint.
- Steps:
  1. New request tab. Method POST, URL `{{base}}/v1/orders?expand=items&q=a b&q=ü`, params table mirrors URL; toggle
     one param off; add description to a header row.
  2. Headers: type `cont` and accept autocomplete `Content-Type`; duplicate header `X-Tag` twice.
  3. Body JSON with trailing newline and unicode; Send.
  4. Save into collection "Shop" as "Create order". Close tab, reopen from tree, restart app, reopen.
- Oracle: wire recorder: one request; query string encodes space and `ü` once (no double encoding), disabled param
  absent, both `X-Tag` values present in order, body bytes equal editor text exactly (trailing newline kept),
  `Content-Length` matches; collection YAML holds request with descriptions side-car; after restart request
  equals saved (deep compare of `Collections` read).
- Variations: URL with fragment, IPv6 literal, userinfo `u:p@host`; method custom/QUERY; body empty with
  Content-Type; header name with invalid chars (refused before send).
- Risk: double encoding, param/URL desync, restored tab Save creates duplicate row.

### J-HTTP-002 Every body mode against echo server

- Status: partial (`httpflow` Bodies; ui `http-request-body`).
- Steps: send in turn: raw text; code (XML); JSON 3 MB; urlencoded with `&`, `=`, `+`, empty value, duplicate keys;
  form-data with 2 text fields plus 2 files (one 0 bytes, one 20 MB, filename with unicode and quote); binary file.
  Then delete the 20 MB file on disk and resend.
- Oracle: wire recorder body bytes per mode: urlencoded per RFC; multipart parts in table order with filenames
  and file SHA-256 equal to disk; binary equals file; missing file gives clear error before any bytes sent (no
  empty part); switching mode back and forth keeps each mode's draft.
- Risk: file part silently empty, mode switch loses draft.

### J-HTTP-003 Variables: precedence, secrets, hover, unresolved

- Status: partial (`apiflow` VariablePrecedence, ActiveEnvironmentResolvesBase; `httpflow`
  SendResolvesSecretsOnlyOnTheWire; ui `http-variables`).
- Steps:
  1. Globals `base=http://g`, collection var `base=http://c`, environment "staging" `base=<server>`, secret `token`.
  2. URL `{{base}}/x`, header `Authorization: Bearer {{token}}`, body `{"u":"{{user}}"}` (undefined).
  3. Hover each variable: resolved value, secret masked. Send.
  4. Switch environment to "prod" (other colour) and send; set no environment and send.
- Oracle: wire: request reached staging server with real token header; unresolved `{{user}}` sent literally or
  send blocked with named variable (assumption: send blocked or warned; pin actual); history, op log, collection
  YAML, tab state never hold token value (grep stores); precedence environment over collection over globals.
- Variations: variable value containing `{{other}}` (nesting, assumption: one level or none; pin); variable name
  with spaces/unicode; undecryptable secret in environment must not shadow a valid lower scope silently.
- Risk: secret leak into stores, wrong precedence.

### J-HTTP-004 Dynamic values and pipes

- Status: partial (`apiflow` DynamicValues; ui `http-dynamic-values`, `http-pipes`).
- Steps: URL `/u/{{$uuid}}`, header `X-Now: {{$isoTimestamp}}`, body with `{{fake.person.email}}`,
  `{{token | base64}}`, `{{name | upper | url}}`, same `{{$uuid}}` twice. Send twice.
- Oracle: wire: valid UUID, ISO time within 5 s, email shape; base64 of real secret on wire only, masked in
  history; two sends differ in dynamic values; two occurrences in one send: assumption distinct values (pin);
  unknown pipe or `fake.x.y` gives named error, no send.
- Risk: pipe over masked placeholder instead of real value.

### J-HTTP-005 Redirect chains and credentials

- Status: covered partly (`httpflow` Redirects); extend with UI timeline.
- Steps: request with `Authorization` and cookie to A: 302 same host; 302 to host B; 307 POST with body to B; https
  to http downgrade; loop of 12; redirects off; max redirects 2.
- Oracle: wire recorder on B never sees `Authorization` or A's cookies; on downgrade nothing sensitive; 307 keeps
  method and body, 302 POST becomes GET (assumption, standard); timeline lists each hop with timings; loop ends
  with clear error; redirects off returns 302 response itself.
- Risk: secret leakage across hosts.

### J-HTTP-006 Cookies: jar, incognito, per-request toggle

- Status: partial (`httpflow` CookieJar, ConcurrentSendsSharedJar; `apiflow` IncognitoLeavesNoTrace).
- Steps:
  1. Login request sets `sid` (HttpOnly, Path=/api, Domain=.example.test). Next request to `/api/me` sends it;
     to `/other` does not; subdomain does; public-suffix sibling does not.
  2. Cookie jar toggle off on one request. Incognito tab: login there, then normal tab.
  3. Restart app.
- Oracle: wire Cookie headers per case; incognito tab cookies never visible in normal tab and vice versa;
  incognito leaves no history row, no op log row (DB count); jar after restart: assumption process jar is memory
  only (inventory "process jar"), so empty.
- Risk: jar leak between incognito and normal.

### J-HTTP-007 TLS, HTTP versions, timeouts, size cap

- Status: partial (`httpflow` TLSAndHTTPVersion, TimeoutAndCancel, TransportErrors).
- Steps: self-signed HTTPS with SSL verify on (error names cert), off (ok); HTTP/2 forced against h1-only server;
  timeout 1 s against 5 s server; response 12 MB with max 5 MB; server resets mid-body; DNS failure; connection
  refused; per-request setting overrides global.
- Oracle: each error class has distinct readable message and op row state; capped response flagged truncated
  with bytes received, no memory blow-up; server sees connection closed at timeout (no lingering).
- Risk: generic error text, cap not enforced.

### J-HTTP-008 Send, cancel, close tab, resend quickly

- Status: partial (bug: closing tab mid-send leaked op and recorded history).
- Steps: Send to slow server; Cancel at 1 s; Send again and close tab at 1 s; reopen saved request; double-click
  Send; Send while previous still in flight (supersede).
- Oracle: wire: cancelled requests see client disconnect; history has no entry for cancelled/closed sends; op log
  rows `cancelled`, none left running; double-click sends once; response shown belongs to latest send.
- Risk: leaked op, stale response painted.

## B. Response viewer

### J-HTTP-010 Large and odd response bodies

- Status: partial (ui `http-raw`; perf `http-response`).
- Steps: view 4.9 MB minified JSON: Beautify (worker), find `"id":`, next x50, copy whole body, copy selection;
  NDJSON; invalid JSON with JSON content type; gzip and brotli; `text/html` with scripts; image/png; 0-byte 204;
  ISO-8859-1 charset; BOM; chunked SSE stream that ends after 10 s.
- Oracle: UI stays interactive during beautify (input latency under 200 ms assumption); clipboard whole-body
  bytes equal server payload (decoded), not beautified unless Beautify view copied (pin which); binary shown as
  size and type, not mojibake; HTML never executed; charset decoded right.
- Risk: freeze on big bodies (P160), copy of truncated chunked fill.

### J-HTTP-011 Raw inspector and timeline

- Status: partial (ui `http-raw`, `http-timeline`).
- Steps: open raw request/response; verify secret masked in raw view; copy raw; edit raw request (raw editor) and
  resend; check timeline DNS/connect/TLS/TTFB/download on keep-alive reuse (connect 0).
- Oracle: copied raw has masked secret; edited raw request reaches wire as edited; timeline phases sum to elapsed
  within tolerance; reused connection shows no connect/TLS.
- Risk: wire masked after truncation (P108): mask must apply before truncation so no partial secret tail.

## C. History

### J-HTTP-020 Fill history past cap, compare, clear

- Status: partial (`apiflow` SavedRequestHistory, HistoryClear; ui `http-history`).
- Steps:
  1. Send saved request 35 times with changing body. History pane: cap note; filter by status `500`.
  2. Tick two entries, Compare (diff). Tick third (disabled).
  3. Click entry: response pane shows stored snapshot; copy body. Delete one. Clear (confirm).
  4. Scratch tab: send 3, save tab as item (history adopted, assumption), restart.
- Oracle: DB `api_response_history` rows for scope = cap exactly, newest kept; snapshot body equals what server
  sent at that time; binary response stored without body, size shown; cleared scope has 0 rows; other request's
  history untouched; secrets masked in stored URL, headers, body of request (response bodies unmasked is open
  item: pin).
- Variations: 3 MB text body (truncated flag), request with 50 form fields over byte budget (fields elided
  notice), environment renamed after send (entry keeps old name).
- Risk: history load supersede loop, cap bypass, secret in history.

## D. curl

### J-HTTP-030 Paste curl from browser devtools, send, copy as curl, replay in shell

- Status: partial (`httpflow` CurlReplayMatchesSend; ui `http-curl`).
- Steps:
  1. Paste multi-line curl with `\` continuations, `-H` x12, `--data-raw $'a\nb'`, `-u user:p@ss`, `--compressed`,
     `-X PATCH`, `--json '{"a":1}'`, `--data-urlencode 'q=a b'`, `-b 'sid=1'`, `-k`, `-L`, URL in single quotes
     with `&`.
  2. Send. Copy as curl (dialog: masked names, option to include secrets). Run copied command in real shell.
- Oracle: wire recorder: Kira send and shell replay produce identical method, path, headers (minus UA), body
  bytes; copied command with secrets masked contains placeholders not values; shell quoting safe for `'`, `$`,
  backtick, newline in body.
- Variations: Windows-style `^` continuation and double quotes; unsupported flag (warned, not dropped silently);
  `-F file=@path`; `-G`.
- Risk: quoting bugs, secret in clipboard without consent.

## E. gRPC

### J-GRPC-001 Reflection call, streaming, cancel, history

- Status: partial (`grpcflow` 13 tests; ui `grpc-request`; `apiflow` GrpcItemSaveAndRestart).
- Persona: developer exploring service.
- Steps:
  1. New gRPC tab, target `{{grpcHost}}`, Describe (reflection). Pick `pkg.Shop/GetOrder`; message JSON with int64,
     enum by name, bytes base64, oneof, nested, map.
  2. Metadata `authorization: {{token}}`, `x-bin-bin` binary key. Call.
  3. Server streaming method: 15k messages; scroll; live buffer 10k note; Cancel mid-stream.
  4. Deadline 500 ms on slow method. Save item; restart; history shows entries.
  5. Server schema changes; Reload; call new method.
- Oracle: `flowharness.GRPC` records exact request fields (int64 exact, enum number, bytes) and metadata with real
  token; status code hint text per code; cancel observed server-side; deadline maps to `DEADLINE_EXCEEDED`;
  history entry capped 256 KiB with flag, token masked; client streaming and bidi methods refused with clear text.
- Variations: `.proto` file import with imports and cyclic deps (error not crash); TLS target with custom CA;
  silent server (Describe times out 15 s, cancellable; connect wait 5 s); call before Describe resolved; service
  without package; message with invalid JSON (error names field, no secret echo).
- Risk: terminal event race, protojson error echoing secrets.

### J-GRPC-002 Copy and select in stream output

- Status: new.
- Steps: during streaming select messages 3-7, copy; copy single message; find text in messages; after 10k
  eviction copy all.
- Oracle: clipboard JSON equals server messages 3-7 in order (int64 as string per protojson); after eviction copy
  holds retained window only and says so; selection stable while new messages append.
- Risk: selection shifts as buffer evicts.

## F. Collections tree

### J-COLL-001 Organise 200 requests with multi-select

- Status: partial (`apiflow` ApiLifecycle, ApiOrder; ui `api-lifecycle`, `api-order`, `collections`).
- Steps:
  1. Collection with 5 folders, 200 requests. Click request, Shift+click 10 below, Cmd+click 2 more, Shift+Down x3.
  2. Drag selection into folder B; try dropping folder A into its own child (refused).
  3. Cmd+A in folder, Delete: one confirm with counts. Open tabs of deleted requests stay as unsaved.
  4. Move collection up with Alt+Up; Escape during drag cancels.
  5. Restart.
- Oracle: `<id>.collection.yaml`: moved items under B in original relative order; deleted ones absent; batch is
  atomic (kill app mid-batch impossible to observe; assert single file write: one mtime change); unsaved tabs can
  Save as new item (new id, no resurrect of old id); order persisted.
- Variations: duplicate names in one folder; names with `/`, emoji, 300 chars; selection spanning two collections
  (assumption: move allowed, delete counts per collection).
- Risk: partial batch, invalid move accepted, tab resurrecting deleted id.

### J-COLL-002 Two windows editing same collection

- Status: partial (ui `api-cross-window-sync`).
- Steps: window 1 edits request draft (unsaved); window 2 renames same request and saves; window 1 saves; window
  2 deletes folder holding a request open in window 1.
- Oracle: window 1 draft not clobbered by refetch; save in window 1: assumption base-hash conflict or last-write
  with rename preserved (pin; never silent loss of rename); YAML consistent; `kira:api:dataChanged` refetch count
  bounded (no loop).
- Risk: draft clobber, refetch storm.

### J-COLL-003 Hand-edited collection YAML and bad file

- Status: partial (`apiflow` ApiConfigFiles).
- Steps: edit `<id>.collection.yaml` outside: add request, break YAML; open Kira form: problem with line; fix in
  YAML tab; add literal `Authorization: Bearer abc` (allowed locally); delete file outside while tab open.
- Oracle: last good kept while broken; fixed file reload shows added request; deleted file removes collection in
  all windows, open tabs become unsaved; generated `AGENTS.md` lists schema.
- Risk: bad file wipes collection.

## G. Variables and environments

### J-ENV-001 Environment as tab with secrets, history, bulk editor

- Status: partial (`apiflow` EnvironmentFiles, ApiVariablesForm; ui `api-variables-form`,
  `api-secret-reveal-isolation`).
- Steps:
  1. New environment "staging", colour, variables `base`, secret `token` (typed), description.
  2. Change `token` twice; open value history; reveal old value (eye) only for that row.
  3. Bulk `.env` editor: paste 10 lines incl `token=...` (refused for typed secret), comments, quoted values, `=`
     in value, duplicate key.
  4. Reorder environments; duplicate environment; delete original; restart.
- Oracle: `<id>.environment.yaml` has `${secret:name}` for token, plain others; secret store rows per environment
  distinct after duplicate; reveal in one row never unmasks another row or tab; bulk parse equals reference
  dotenv parse; after delete secrets pruned.
- Risk: reveal isolation, duplicate sharing secret.

### J-ENV-002 Active environment per window and cross-window change

- Status: partial.
- Steps: window 1 active "staging", window 2 "prod"; send same request from both; rename "prod" in window 1;
  delete active environment of window 2 from window 1.
- Oracle: wire recorder shows each window used its own base; after delete window 2 falls back to no environment
  with visible notice, next send does not use stale values.
- Risk: stale active environment after delete.

## H. Postman import/export

### J-IMP-001 Import messy Postman export, send, export, re-import

- Status: partial (`apiflow` PostmanImportSendExport, PostmanImportCollectionAndEnvironment).
- Steps:
  1. Pick 4 files at once: v2.1 collection (nested folders, auth inherit, form-data files, disabled vars, request
     without name, duplicate names, gRPC-ish item), environment, globals, a non-Postman JSON.
  2. Review warnings (`empty_values`, `unnamed_<n>`, `missing_request`). Import.
  3. Send imported request. Export collection (Save dialog). Import exported file into fresh home.
- Oracle: per-file atomic: bad file imports nothing and others succeed; counts in report equal items in YAML;
  generated `unnamed_<n>` never collide with real names; exported JSON validates against Postman v2.1 schema;
  round-trip collection equal modulo ids; secrets exported per rule (assumption: masked or omitted, never
  plaintext without consent; pin); export does not follow symlink target.
- Variations: API envelope `{collection:{...}}`; 5,000-request file; file with BOM; cancelled picker.
- Risk: strictness dropping valid entries (P385), name collisions (P386).
