# P627 journeys: Api module, second pass

Continues `journeys-http-grpc.md` (ids continue). One scenario per journey; bundled variations of pass 1 split out.
Conventions: `journeys-studio-connections.md` head. "Wire recorder" = `flowharness.HTTP/HTTPS/GRPC`.

Code facts used (CodeGraph reads):

- `httpclient/options.go:65-109` `normalize`: defaults HTTP/2 allowed, timeout 0 (none), max response 5 MiB, SSL
  verify on, follow redirects on, max redirects 10, cookie jar off (`useJar: false`) unless `DisableCookieJar` is
  explicitly false. Clamps: timeout 0..3,600,000 ms, max response 0..2048 MB (0 = no cap), max redirects 0..100.
- `options.go:163-206` `checkRedirectFor`: follow off returns the 3xx itself; `len(via) >= maxRedirects` errors
  "stopped after N redirects"; `redirectGuard` runs per hop; each hop compared against the original request
  (`via[0]`), not the previous hop.
- `options.go:210-234` `stripOnOriginChange`: cross-host, port change or https-to-http drops every user-typed
  header; port change also drops `Authorization`, `WWW-Authenticate`, `Cookie`, `Cookie2`; downgrade drops
  `Authorization`, `Cookie`.
- `api-core/src/http/curl/parse.ts`: never throws; method precedence `-X` > `-I` > `-T` > `-G` > data/form > GET
  (`:472-490`); unknown method coerced to GET with warning; `-u` becomes `Authorization: Basic <utf8 base64>` with
  `credential-in-command` warning, kept unencoded when value holds `{{` (`:336-363`); `--json` adds Content-Type and
  Accept JSON (`:374-383`); `-b` without `=` warned as cookie-jar file (`:323-335`); `Name;` header sends empty
  value (`:169-173`); `@path` file references never carried into a sendable field (`unresolved-file`, `:640-676`);
  unknown flag consumes no value (`:424-430`); `-G` appends data to query verbatim (`:511-517`); data without
  Content-Type and not all `k=v` becomes raw plus explicit urlencoded header (`:608-618`); form flags win over a
  stated Content-Type (`:634-639`).
- `studio/api/menu.ts:14-52`: collection menu has Copy URL, Duplicate, Move to collection / new collection, Move
  step, Export, Variables, Edit variables, Open file, Reveal file; background menu New collection, Import
  collection, Import from curl, Environments, Dynamic values.

## I. HTTP send, split scenarios

### J-HTTP-040 Cookie jar is opt-in: login flow with default settings

- Status: new (default `useJar` false read from code; `httpflow` CookieJar covers jar-on only).
- Persona: developer expects browser-like session after login.
- Pre: echo server sets `sid` on `/login`, requires it on `/me`; fresh home, default Api settings.
- Steps:
  1. Send POST `/login`. Response Cookies pane lists `sid`.
  2. Send GET `/me` in same tab. Observe 401.
  3. Open request settings, enable cookie jar for both requests (or global). Repeat 1 and 2.
  4. Disable jar on `/me` only; send.
  5. Restart app; send `/me` with jar on.
- Oracle: wire: step 2 `/me` has no Cookie header; step 3 has `sid`; step 4 none; step 5 none (process jar is
  memory only). UI must show why `/me` got no cookie (sent-cookies pane empty plus jar-off hint; assumption: hint
  exists, else open question OQ-API-1).
- Variations: global on and per-request off; incognito tab with jar on.
- Risk: user-visible surprise when default silently drops session; setting not reaching send.

### J-HTTP-041 Redirect A to B to B with custom secret header

- Status: partial (`httpflow` Redirects; A-B-B chain new).
- Pre: host A 302 to host B `/one`, B 302 to B `/two`; request holds `X-Api-Key: {{key}}` and `Authorization`.
- Steps:
  1. Send to A. Open timeline: 3 hops.
  2. Open raw view of each hop.
  3. Repeat with header named `PRIVATE-TOKEN` and with `Cookie` typed by hand.
- Oracle: wire recorder on B: neither hop to B carries `X-Api-Key`, `PRIVATE-TOKEN`, `Authorization` or typed
  `Cookie` (comparison against original host, so second B hop stays stripped); A saw them; default headers
  (`User-Agent`, `Accept`) still present on B.
- Risk: second same-host hop re-gains headers copied from original request.

### J-HTTP-042 Redirect to same host, other port

- Status: new (port-change branch `options.go:221-226`).
- Pre: `http://h:8080/a` 302 to `http://h:9090/b`; other service on 9090.
- Steps: 1. Send with `Authorization: Bearer {{token}}`, jar cookie for `h`, custom `X-Trace`. 2. Inspect hop 2.
- Oracle: recorder on 9090 sees no `Authorization`, no `Cookie`, no `X-Trace`; recorder on 8080 saw all.
- Variations: default port vs explicit (`:80`): assumption same port, nothing stripped.
- Risk: credential replay to sibling service on same hostname.

### J-HTTP-043 https to http downgrade on same host

- Status: partial (`httpflow` Redirects).
- Steps: 1. Send `https://h/a` (302 to `http://h/b`) with Authorization, jar cookie (Secure and non-Secure), custom
  header. 2. Inspect plaintext hop.
- Oracle: plaintext hop carries none of Authorization, Cookie, custom header; history entry shows both hops with
  secrets masked.
- Risk: plaintext leak.

### J-HTTP-044 Redirect limit boundaries 0, 1, 10, 100, 101

- Status: new.
- Pre: server with redirect chain of configurable length.
- Steps: 1. Max redirects 0, chain 1. 2. Max 1, chain 1 then chain 2. 3. Default 10, chain 10 then 11. 4. Type 500
  in setting (clamp 100). 5. Follow redirects off.
- Oracle: chain == max succeeds; chain == max+1 errors "stopped after N redirects" with N = setting; max 0 errors
  on first redirect (not "returns 3xx": that is follow-off only); setting stored/used as 100 after 500; follow off
  shows 3xx response with `Location` header, timeline 1 hop, hop not recorded twice.
- Risk: off-by-one, clamp not applied in UI and backend alike.

### J-HTTP-045 Timeout and size settings extremes

- Status: new.
- Steps: 1. Timeout 0 against 90 s server; Cancel at 5 s. 2. Timeout 99,999,999 (clamp 3,600,000). 3. Max response
  0 MB against 50 MB body. 4. Max 1 MB against 1 MB + 1 byte. 5. Per-request override beats global; delete
  override, global applies.
- Oracle: 0 = no timeout (request alive until cancel, server sees disconnect at cancel); max 0 = no cap, body
  complete (hash equal), UI responsive; 1 MB cap truncates with flag, `bodyBytes` reports received not full;
  override precedence proven on wire timing.
- Risk: 0 treated as "immediately", cap off-by-one, unbounded memory at cap 0.

### J-HTTP-046 Response header flood

- Status: new (`MaxResponseHeaderBytes` in `transportFor`).
- Steps: 1. Server answers 2 MB of `Set-Cookie` headers. 2. Send. 3. Send normal request after.
- Oracle: bounded failure with readable error (not hang, not 2 MB in timeline or history row); later sends work
  (transport not poisoned).
- Risk: memory and history bloat from hostile endpoint.

### J-HTTP-047 HTTP version setting really changes protocol

- Status: partial (`httpflow` TLSAndHTTPVersion).
- Steps: 1. HTTPS server speaking h2 and 1.1: send default; send with version 1.1. 2. Plain `http://` server
  (never h2c). 3. Toggle SSL verify off: warning chip in request editor; send to self-signed. 4. Toggle back; send.
- Oracle: recorder protocol `HTTP/2.0` then `HTTP/1.1`; `http://` always 1.1; verify-off send succeeds, verify-on
  fails with certificate error naming cause; chip visible exactly while off; other tabs unaffected (cached
  transport per key).
- Risk: verify-off transport reused for verify-on request.

### J-HTTP-048 Unresolved variable in URL, header and body

- Status: new.
- Steps: 1. URL `{{base}}/x/{{missing}}`, header `X-A: {{nope}}`, body `{"a":"{{gone}}"}`; no environment. 2. Hover
  each. 3. Send. 4. Define `missing` only; send.
- Oracle: defined rule (OQ-API-2): recommended = send blocked with list of unresolved names; if current build
  sends literal braces, wire shows `%7B%7Bmissing%7D%7D` and UI shows warning; test pins actual and flags.
- Risk: literal `{{secretName}}` leaks variable names; silent wrong request.

### J-HTTP-049 Form-data with file that changes between sends

- Status: new.
- Steps: 1. Attach `report.csv` (10 KiB). Send. 2. Append to file on disk. Send. 3. Replace file with directory of
  same name. Send. 4. Remove read permission. Send. 5. Save request, restart, send.
- Oracle: wire part bytes equal file at send time (steps 1, 2); steps 3, 4 fail before any request leaves, error
  names path; after restart saved request still references path and sends current bytes; `fileSize` shown updates.
- Risk: stale cached file bytes, empty part.

### J-HTTP-050 Two tabs send concurrently with shared jar and one incognito

- Status: partial (`httpflow` ConcurrentSendsSharedJar).
- Steps: 1. Tab A (jar on) logs in. 2. Tabs B, C (jar on) and D (incognito, jar on) send `/me` at once, 20 times
  each via quick resend. 3. Server rotates `sid` on each response.
- Oracle: no data race (flow under `-race`); B, C always send some valid `sid`; D never sends A's cookie; jar ends
  with last rotated value.
- Risk: jar race, incognito bleed.

### J-HTTP-051 Request tab survives environment, collection and module churn

- Status: new.
- Steps: 1. Unsaved request with find bar open in response, History pane on entry 3, body mode form-data. 2. Switch
  to Git module, back. 3. Switch environment. 4. Other window renames collection. 5. Close and reopen window.
- Oracle: draft, body mode, response pane, find text kept across module switch; after window reopen draft kept if
  tab persisted (pin), response not kept; history entry selection reset.
- Risk: find bar/filter lost on tab switch (class "sort, filter, search").

## J. curl, split scenarios

### J-HTTP-060 Paste curl with basic auth holding unicode and variable

- Status: partial (ui `http-curl`).
- Steps: 1. Import `curl -u 'jörg:pä55:wörd' https://h/x`. 2. Send. 3. Import `curl -u '{{user}}:{{pass}}' ...`;
  read warning. 4. Rewrite header to `Basic {{cred | base64}}`; send.
- Oracle: wire `Authorization: Basic` decodes to UTF-8 bytes of `jörg:pä55:wörd` (password keeps second colon);
  step 3 header kept unencoded with warning naming pipe; step 4 wire holds base64 of resolved value;
  `credential-in-command` warning shown both times.
- Risk: Latin-1 base64, braces destroyed.

### J-HTTP-061 Paste curl with `@file` references

- Status: new (security rule read from code).
- Steps: 1. Import `curl --data-binary @/etc/passwd https://h/x`. 2. Import `curl -F 'f=@~/.ssh/id_rsa' -F 'k=<notes'
  https://h/x`. 3. Import `curl -T ./big.bin https://h/x`. 4. Press Send on each without picking files.
- Oracle: tab state has empty `path` for every file slot, name only; warning `unresolved-file` per reference;
  method POST, POST, PUT; Send refuses or sends no file bytes; wire recorder never receives content of referenced
  paths.
- Risk: pasted command exfiltrates local file.

### J-HTTP-062 Paste curl with `-G`, `--data-urlencode` and existing query

- Status: partial.
- Steps: 1. Import `curl -G 'https://h/s?a=1' -d 'b=2' --data-urlencode 'q=a b&c'`. 2. Compare params table. 3. Send.
  4. Run original command in shell against recorder.
- Oracle: method GET, no body; query on wire from Kira equals query from real curl byte for byte (OQ-API-3: code
  appends `-d` data verbatim; `--data-urlencode` piece must arrive encoded as curl does); params table shows 3 rows.
- Risk: double or missing encoding.

### J-HTTP-063 Paste curl with unknown and unsupported flags

- Status: partial.
- Steps: 1. Import `curl --retry 3 --my-flag https://h/x -sSL --compressed -o out.txt`. 2. Read warnings. 3. Send.
- Oracle: URL is `https://h/x` (unknown flag took no value); `--retry` value `3` not mistaken for URL
  (`multiple-urls` warning must not appear, else pin as defect); warnings list each unsupported/unknown flag by raw
  text; cluster `-sSL` expands.
- Risk: flag arity eats URL.

### J-HTTP-064 Paste non-curl and broken curl

- Status: new.
- Steps: 1. Paste `wget https://h`. 2. Paste `curl 'https://h` (unterminated quote). 3. Paste `curl` alone. 4. Paste
  `curl https://a https://b`. 5. Paste `curl https://h | jq . && rm -rf x`.
- Oracle: 1 not treated as curl (plain paste into URL field, assumption); 2 error text, no tab change; 3 `no-url`
  warning; 4 first URL used, `multiple-urls` count 1; 5 shell operators end the command, nothing after pipe parsed.
- Risk: shell tail parsed into request.

### J-HTTP-065 Copy as curl of every body mode then replay

- Status: partial (`httpflow` CurlReplayMatchesSend covers subset).
- Steps: for raw, code, JSON, urlencoded, form-data (text plus file), binary file: 1. Send from Kira. 2. Copy as curl
  with secrets included. 3. Run in `sh` and in `zsh`.
- Oracle: recorder: Kira request and replay equal in method, path, query, content-type (boundary aside), body
  bytes; command contains no unescaped newline hazards; file modes reference `@path` quoted.
- Risk: shell quoting per mode.

### J-HTTP-066 Copy as curl with masked secrets, then switch environment

- Status: partial (ui `http-curl`).
- Steps: 1. Request uses secret `token` and plain `base`. 2. Open Copy as curl: masked names listed. 3. Copy masked.
  4. Tick include secrets (OS auth if gated); copy. 5. Switch environment; reopen dialog.
- Oracle: masked copy has placeholder for token, real `base`; unmasked copy has real token only after explicit
  consent; dialog reflects new environment values; clipboard never written when consent denied.
- Risk: secret copied without consent, stale environment.

## K. gRPC, split scenarios

### J-GRPC-010 Proto file workflow with imports and reload

- Status: partial (`grpcflow` DescribeProtoAndCall, ReloadAfterSchemaChange).
- Steps: 1. Pick `shop.proto` importing `common/money.proto` and `google/protobuf/timestamp.proto`. 2. Describe.
  3. Call method using Timestamp and Money. 4. Edit proto on disk (rename field). 5. Reload. 6. Call with old field
  name. 7. Move proto file away; Reload.
- Oracle: recorder gets Timestamp exact; after reload old field name rejected with error naming field; missing
  file gives clear error, last described methods either kept flagged stale or cleared (pin); saved item reopens
  after restart with proto path.
- Variations: cyclic imports (error, no crash); proto syntax error with line.
- Risk: stale descriptor used after reload.

### J-GRPC-011 Deadline, cancel and connect wait interplay

- Status: partial (`grpcflow` UnaryDeadline, CallSilentServer).
- Steps: 1. `api.requestTimeoutMs` 0: call 30 s method; Cancel at 3 s. 2. Timeout 500: same method. 3. Silent target:
  call (connect wait 5 s). 4. Silent target: Describe, Cancel at 2 s. 5. Describe again, wait 15 s.
- Oracle: statuses `CANCELLED`, `DEADLINE_EXCEEDED`, unavailable with hint; elapsed about 3 s, 0.5 s, 5 s, 2 s,
  15 s; no goroutine or stream left (second call works, op rows closed).
- Risk: terminal event race, Describe without timeout.

### J-GRPC-012 Metadata edge cases

- Status: partial (`grpcflow` UnaryMetadataAndStatus, SecretsInTargetAndMetadata).
- Steps: 1. Keys: uppercase `X-Trace`, `-bin` suffix with base64 value, duplicate key twice, reserved `grpc-timeout`,
  key with space. 2. Value from secret variable. 3. Call. 4. View history entry.
- Oracle: recorder: keys lowercased, binary decoded bytes, both duplicates, reserved/invalid refused before call
  with reason; secret masked in history and op log.
- Risk: invalid metadata panics client.

### J-GRPC-013 Streaming with slow consumer and tab switch

- Status: new.
- Steps: 1. Start server stream at 2,000 msg/s. 2. Switch tab 20 s. 3. Return. 4. Scroll to top. 5. Cancel. 6. History.
- Oracle: buffer holds newest 10k with eviction note; UI input latency bounded on return; cancel reaches server;
  history entry capped 256 KiB flagged; message count reported equals server count sent before cancel.
- Risk: unbounded buffer while hidden.

### J-GRPC-014 TLS target matrix

- Status: covered (`grpcflow` TLSTargets); UI path new.
- Steps: plaintext target with `grpcs://`; TLS target with plaintext; self-signed with verify on/off; custom CA.
- Oracle: each mismatch gives distinct readable error within connect wait; success only for matching pairs.
- Risk: generic "unavailable" for TLS mismatch.

## L. Collections and environments, split scenarios

### J-COLL-010 Duplicate request and collection, then diverge

- Status: partial (`apiflow` ApiLifecycle).
- Steps: 1. Duplicate request with history and secret-using headers. 2. Edit copy; send both. 3. Duplicate whole
  collection with variables (one secret). 4. Change secret in copy. 5. Delete original collection.
- Oracle: copy has new id, empty history (assumption), same content; collection copy has own YAML and own secret
  row; original's secret unchanged by step 4; after delete copy still resolves its secret.
- Risk: duplicate shares secret names (P482 class).

### J-COLL-011 Move selection to another and to new collection

- Status: partial (ui `api-lifecycle`).
- Steps: 1. Select folder plus 3 loose requests. 2. Move to collection B. 3. Select 2, Move to new collection (name
  prompt, Escape cancels first time). 4. Requests reference collection variables of A.
- Oracle: both YAML files updated in one logical operation (no state where item is in neither or both on disk
  after crash injection between writes: OQ-API-4); moved requests now resolve B's variables; warning if variables
  missing in B (assumption).
- Risk: item lost between two file writes.

### J-COLL-012 Copy URL from tree row

- Status: new.
- Steps: 1. Row with URL `{{base}}/a?x={{secret}}`. 2. Copy URL with environment active. 3. With none.
- Oracle: clipboard (OQ-API-5): recommended = template text as saved, never resolved secret; pin actual.
- Risk: secret resolved into clipboard.

### J-COLL-013 Export collection then open file and reveal

- Status: partial (`apiflow` PostmanImportSendExport).
- Steps: 1. Export to path with unicode. 2. Cancel dialog on second try. 3. Open file (system editor). 4. Reveal file.
  5. Export while file exists (overwrite confirm by OS dialog).
- Oracle: file bytes valid Postman v2.1; cancel writes nothing; `FileOpener.OpenedFiles/RevealedFiles` paths equal
  collection YAML path; gRPC items skipped with warning listed.
- Risk: export follows symlink, silent gRPC drop.

### J-ENV-010 Variable value history and revert

- Status: partial (ui `http-variables`).
- Steps: 1. Change `base` 5 times across a day. 2. Open history menu; pick third value. 3. For secret `token`: history
  rows masked; eye on one row. 4. Restart; reopen history.
- Oracle: picked value becomes current and wire uses it; history order newest first, bounded; secret history
  stored sealed (grep finds no old token); eye reveals only that row; history persists restart (pin).
- Risk: plaintext secret history.

### J-ENV-011 Globals vs collection vs environment edited in two windows

- Status: partial (ui `api-cross-window-sync`).
- Steps: 1. Window 1 opens environment tab, edits `base` (unsaved). 2. Window 2 edits same variable and saves.
  3. Window 1 saves. 4. Both send.
- Oracle: conflict surfaced on step 3 (base hash) or defined last-write (OQ-API-6); never two windows resolving
  different values after both show "saved".
- Risk: silent divergence.

### J-IMP-010 Postman environment with unnamed and empty entries

- Status: partial (`postman/variables_test.go`; `apiflow` import).
- Steps: 1. Import environment JSON: 2 entries without key, 1 key named `unnamed_1` (real), 3 empty values, 2
  disabled, 1 secret type. 2. Review report. 3. Send request using `{{unnamed_1}}`.
- Oracle: generated names skip real `unnamed_1` (next free), report counts match; disabled skipped; secret type
  stored sealed; request resolves real `unnamed_1` value.
- Risk: generated name collides with real key (P386).

### J-IMP-011 Import the same collection twice and after local edits

- Status: new.
- Steps: 1. Import file. 2. Edit 2 requests locally. 3. Import same file again.
- Oracle: second import creates separate collection (name suffix) and never overwrites local edits (OQ-API-7);
  ids distinct; history of first untouched.
- Risk: overwrite of local work.
