# P627 journeys: Studio MCP, approvals queue, validation proof

Split from `journeys-studio-db.md` and `journeys-studio-views.md` (P627 first pass); J-ids unchanged.
Imagined usage and definition of correct. No implementation, no test code.
Conventions: `journeys-studio-connections.md` head.

Approvals facts read from source: item id `<kind>:<ref>` stable for item life; `Queue.Answer` answers ids one by
one, per-id failure never aborts rest; answering resolved item returns NotFound `approval`; list sorted oldest
first by `CreatedAt` then id; branch request answer carries `shownItemIds`, an item joined after view rendered
stays pending.

## G. Studio MCP: database tools

### J-MCP-001 Agent session from install to audited write

- Status: partial (`dbmcpflow` approval flow, `mcp_test.go` row count; ui `dbmcp-approval*`). New: one chain with
  restart and target check per step.
- Persona: developer lets Claude fix data on staging.
- Pre: SQLite or Postgres target, connection `mcp`: read allow, write prompt, ddl deny, masked allow.
- Steps:
  1. Settings Studio MCP: enable, copy install command, Regenerate token (old token must die).
  2. MCP client `list_connections`; `describe`; `run_query mode=read` SELECT.
  3. `run_query mode=write` UPDATE one row: approval dialog (target banner colour, host, database, class write),
     toggle Formatted/As sent; approve.
  4. `run_query mode=read` with an UPDATE inside (refused by classifier and read lane).
  5. `run_query mode=ddl` DROP: denied without prompt.
  6. `run_query mode=write` with DDL text: refused (more powerful than declared).
  7. Restart app; client reconnects with same token.
- Oracle: second client DB state: exactly step 3 row changed; old token gets 401 after Regenerate; new token works
  after restart (settings hydration); op log rows carry source MCP and tool name; statement shown in dialog "As
  sent" byte-equals statement executed (server log).
- Variations: user has open transaction on connection (MCP read refused, P330); unclassified statement (strictest);
  `SELECT ... INTO`, CTE with DML, `EXPLAIN ANALYZE UPDATE`, multiple statements in one call.
- Risk: classifier bypass, token hydration, dialog shows other text than executed.

### J-MCP-002 Table allow list under joins, views, schemas

- Status: partial (`dbmcpflow` table allow-list).
- Steps: set `mcp.tables: [app.people, app.orders]`; agent queries `people`, `app.people JOIN app.secrets`, view
  over `secrets`, `"APP"."People"`, CTE named `people` shadowing, subquery in WHERE on `secrets`, `browse_table
  secrets`, `describe secrets`.
- Oracle: only allowed relations return rows; every refusal names relation; describe/list hide others (assumption);
  open-item cases (view definition server-side) pinned as documented, not silently "safe".
- Risk: parse gaps leak tables.

### J-MCP-003 Expensive query gate, exemption, throttle, timeout together

- Status: partial (`dbmcpflow` Expensive, exemptions, Throttle, Timeout*).
- Steps:
  1. Agent `run_query SELECT * FROM big_rows` (over threshold): refused with EXPLAIN estimate and exemption hint.
  2. Agent requests exemption `exm_`: user sees exemption item, approves; agent reruns with exemption.
  3. Agent fires 20 reads in parallel (throttle gate paces one per interval).
  4. Agent runs `pg_sleep(30)` with absent timeout (10 s default).
- Oracle: step 1 no rows shipped, server ran EXPLAIN only; step 2 exemption single-use or scoped as documented
  (pin), always raises approval even when read is allow; step 3 server log timestamps spaced at least throttle
  interval; step 4 killed about 10 s, backend gone.
- Variations: EXPLAIN fails (refuse, fail closed); ClickHouse; engine without EXPLAIN (NoExplain path).
- Risk: fail-open on explain error.

### J-MCP-004 Result review before delivery

- Status: partial (`dbmcpflow` review; ui `dbmcp-review-*`).
- Steps: set `mcp.review.read: review`; agent reads 500 rows incl masked column; user sees held result in dialog
  with grid, selects cells, copies, denies; agent retries, user approves; agent sends 80 MiB result (cap).
- Oracle: denied result never reaches client (MCP response is refusal, no row text); approved response equals
  reviewed rows; clipboard from review grid holds masked values; over-cap result refused with clear code, memory
  released (held bytes 0 after).
- Variations: unknown review value in YAML coerces to review; preview responses (P333); app quit with held result
  (client gets error, nothing delivered).
- Risk: delivery before approval, unmasked review copy.

### J-MCP-005 browse_table paging stability

- Status: partial (`dbmcpflow` browse 16 tests).
- Steps: agent pages `browse_table people` 50 rows at a time to end while second client inserts rows; requests
  count; filter value with `'` and `;`; offset 100000+; table without PK; masked default.
- Oracle: union of pages has no duplicates and misses no pre-existing row (stable order by PK); count equals
  `count(*)`; `;` filter refused per open item (pin); offset limit error explicit; no-PK table states unstable
  order in result meta.
- Risk: pagination duplicates/gaps.

### J-MCP-006 Unreachable and lost connections raise no prompts

- Status: covered (`dbmcpflow` unreachable; ui `dbmcp-unreachable`); extend: mid-approval loss.
- Steps: agent write request opens approval; server dies while dialog open; user approves.
- Oracle: approval answer returns error to agent (connection lost), DB unchanged, item removed, no second prompt;
  later requests while down return error without prompt.
- Risk: prompt for dead server, approve executes on reconnected different host.

### J-MCP-007 Operations tools

- Status: partial (ui/flow P518).
- Steps: agent starts long `run_query`; second MCP call `list_operations`; `cancel_operation` id; user also presses
  Stop in status bar ops widget at same time.
- Oracle: one cancel reaches server; both callers get clean result (one ok, other already-ended); op row
  `cancelled`; first tool call returns cancelled error, not hang.
- Risk: double cancel race, stuck running row.

## H. Studio MCP: scripts, API, Kubernetes tools

### J-MCP-010 Scripts tools short and long mode

- Status: partial (`dbmcpflow` scripts; ui `dbmcp-script-long`, `dbmcp-approval-script`).
- Steps:
  1. Run "deploy-check" has `mcp` block opt-in; another script has none.
  2. Agent `list_scripts` (only opted-in), `run_script` with params incl secret param and select param with invalid
     value.
  3. Approval dialog shows command and params (secret masked); approve.
  4. Long mode: agent polls `get_script_run`, `get_script_logs` paging to end; user presses Stop in Runs tab.
  5. Agent runs same script again while first live.
- Oracle: non-opted script refused by name; invalid select refused before prompt; process ran with secret in env
  (script writes hash of it to file; file hash matches), logs returned to agent show masked secret; Stop yields
  outcome cancelled to agent; concurrent run refused `E_CONFLICT`; Runs tab shows same run (one tab per script key).
- Risk: secret in logs to agent, concurrent run.

### J-MCP-011 api_request with rules, credentials, redirects

- Status: partial (`dbmcpflow` API 16 tests; ui `dbmcp-api-*`).
- Steps:
  1. Rules: allow GET `api.internal`, deny `*.prod`, default ask. Per-host agent credential for `api.internal`.
  2. Agent GET allowed host (no prompt); POST same host (ask); GET `x.prod` (deny).
  3. Approved request answers 302 to other host; to same host other path; https to http.
  4. Agent uses `{{secret}}` variable of active environment; user changes variable between plan and send.
  5. Response 20 MB.
- Oracle: wire recorder (`flowharness.HTTP`): credential header present only on `api.internal` requests, absent
  after cross-host or downgrade redirect; hand-approved request does not follow redirect to unapproved target
  without new decision; changed variable invalidates approval (replan); response capped with flag; secret value
  never in MCP result or op log.
- Risk: redirect credential leak, plan-to-send race.

### J-MCP-012 Kubernetes MCP per cluster

- Status: partial (`dbmcpflow` Kubernetes MCP; ui `kubernetes-mcp`).
- Steps: cluster A MCP off, B prompt, C allow with description; agent lists pods on each; describes Secret on C;
  tails logs on B (approve), user reorders clusters meanwhile.
- Oracle: `KubeFake.Requests()`: none for A; B only after approval; Secret values redacted in tool result; op log
  kind `kubernetes` rows with "Via" tool; cluster ids stable across reorder.
- Risk: off cluster reachable, secret values leak.

## I. Approvals queue

### J-APR-001 Mixed queue from four sources, one at a time

- Status: partial (ui `approvals-one-at-a-time`, `approvals`; flows per kind).
- Persona: user returns from lunch to 7 pending approvals.
- Pre: pending: 2 db.query (different connections), 1 db.exemption, 1 script.run, 1 api.request, 1 branch.request,
  1 task.create.
- Steps:
  1. Dialog shows item 1 of 7 (oldest), Deny focused. Press Escape: minimised to toast, nothing denied.
  2. Reopen from toast. Approve item 1; badge 1 of 6.
  3. New db.query arrives while reading item 2 (joins end, current item not replaced).
  4. Item 3's agent times out/abandons while shown (item vanishes, next shown, no error modal).
  5. Deny all (confirm): remaining denied.
- Oracle: each agent/tool call returns matching outcome (approved ran; denied got denial text; none approved
  unseen); target DB changed only by item 1; order oldest first by created time; answering vanished item gives
  NotFound handled silently; after Deny all queue empty, toast gone.
- Variations: Enter key with Deny focused denies (never approves by reflex); double-click Approve (second click
  must not approve next item); two windows: answer in window 2 clears window 1; app restart with pending items
  (agents get error, queue empty on boot).
- Risk: approving unseen item (P618), requestId keying across windows.

### J-APR-002 Branch request item joined after render

- Status: partial (`adeflow` BranchRequest; ui `ade-v2-branch-request`).
- Steps: agent requests branches in repo A and B; dialog shows 2 items; second agent of same task adds repo C
  request while open; user approves with A ticked, B unticked.
- Oracle: worktree created for A only (`git worktree list`), B denied, C stays pending in new dialog round (not
  auto-approved); agent 1 gets result naming A approved, B denied.
- Risk: late-joined item approved unseen.

### J-APR-003 Focus on approval across windows and modules

- Status: partial (`dbmcpflow` focus; ui `dbmcp-focus`).
- Steps: user types in terminal tab of window 2 (Git module); approval arrives from session started in window 1;
  `dbMcp.focusOnApproval` on; five requests in 1 s; then setting off.
- Oracle: `WindowFocus` fake: one raise for the burst, target window per prompt routing (origin window, else main);
  keystrokes typed during raise not delivered into dialog buttons (no accidental approve: Deny focus plus 2 s
  guard); with setting off zero raises, toast only.
- Risk: keystroke hijack approves.

## J. Validation proof

### J-PRF-001 Proof session from agent to screenshots on disk

- Status: partial (`proofflow` ProofWindow, ProofShotSaveAll, ProofShotCopyAll; ui `proof-*`).
- Persona: reviewer checks agent's validation evidence.
- Steps:
  1. Agent `proof_save` with 4 steps: table view (ints 2^63-1, NULLs, unicode), chart (1,000 points), Mermaid
     diagram, JPEG image.
  2. Proof ready prompt: Open (dialog closes, window opens). Step tabs; sort table; select cells; Cmd+C; open
     cell in read-only editor.
  3. Change panel: accept step 1, reject step 2 with note.
  4. Save all screenshots to chosen folder with name containing spaces; Reveal. Copy all (macOS multi-image).
  5. Agent saves same session name again (collision); 31st session (retention 30).
- Oracle: clipboard TSV of table cells equals agent's raw args (int precision exact); JPEG files exist one per
  step, decodable, dimensions > 0, folder named `<session>`; `ImageClipboard.Writes()` count = steps; accept/reject
  delivered to agent tool result; collision yields distinct session (suffix), oldest of 31 purged from disk and
  list; proof attached to task chain entry.
- Variations: chart 1,001 points (refused/capped), invalid Mermaid (source plus "Loading error."), image over cap,
  Save cancelled (no files), clip leaves no temp files.
- Risk: integer precision, PNG/JPEG clipboard, ready dialog stays open.

### J-PRF-002 Proof window view state and restart

- Status: partial.
- Steps: per step set sort, column widths, chart kind; switch steps; close window; reopen via Window > Validation
  proof; restart app.
- Oracle: view state per step kept while session open; proof windows never restored at boot (purged), session
  history list still holds session; reopening shows saved content byte-equal.
- Risk: ephemeral window restored, state bleed between steps.
