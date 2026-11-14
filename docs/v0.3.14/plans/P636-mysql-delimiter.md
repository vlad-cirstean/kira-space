# P636 MySQL console `DELIMITER` directive (F-A12)

Console splitter honours `DELIMITER <token>` on MySQL and MariaDB consoles, so trigger, procedure
and function scripts and mysqldump output run. Flow half: script with trigger body runs on a real
server, trigger exists afterwards. UI half: same script typed into console sends same statement
list and renders same result.

Discovery: `codegraph_explore` (splitter, `useConsoleRun`, `lexOptionsFor`, mysqlfamily `execute`,
`AtStatement`, `failureOutcome`, `formatConsoleText`, `OperationsPanel.onRerun`, `MaskSQLLiterals`),
plus reads of flow tests, contract file and UI spec.

## Where splitting happens today

- Renderer splits. `packages/shared/domain/sqlSplit.ts` `splitSqlStatements` (quote/comment spans
  from `sqlLex.ts` `scanSqlSpan`, `BEGIN`/`END` depth via `createBlockTracker` when
  `compoundBlocks`). Callers: `useConsoleRun.ts` (Run, Run all, Explain target), `useConsoleFormat.ts`
  and `format.ts` (Format, caret by index), `OperationsPanel.vue` `onRerun`.
- Options per dialect: `views/shared/sqlIdent.ts` `lexOptionsFor`. Kinds `mysql` and `mariadb` both
  map to dialect `'mysql'`.
- Go receives `ConsoleRequest.Statements` pre-split. mysqlfamily `execute` (`console.go`) runs each
  through `runRaw` (`QueryContext`, `MultiStatements = false` in `client.go`). Failure carries
  `statementIndex` = index into that array (`adapters.AtStatement`).
- Renderer reads `statementIndex` only in `transaction.ts` `failureOutcome`/`trackTransaction`,
  which slices the same `statements` array the run sent.
- Contract `studio-console-script` key `mysql#delimiter` pins: server rejects `DELIMITER //` with
  `E_QUERY`. Stays true.

## Decision: client directive in renderer splitter, no Go production change

`DELIMITER` is a `mysql`/`mariadb` client command; server never sees it (pinned by
`mysql#delimiter`). Handle it where the console splits: `sqlSplit.ts`, gated by a new lexical option
on dialect `'mysql'`. Directive lines produce no statement, so the array sent, the Go loop index and
`statementIndex` stay one-to-one with `splitSqlStatements` output. No index remap needed anywhere.

Go side declined: server splitting would need a Go MySQL lexer duplicating the TS one (two splitters
that can drift), and `MultiStatements` stays off on purpose (per-statement pages, read-only wrap
re-check after each statement). MariaDB parity is free: same dialect, same client semantics.

## Directive grammar (mirrors `mysql` client `add_line`/`com_delimiter`)

Active only with `delimiterDirective: true`. Current delimiter starts as `;`.

- D1 Recognized when keyword `DELIMITER` (any case) is first non-blank text on its line, outside
  any quote or comment span, and followed by at least one space or tab. Line start = offset 0 or
  right after `\n`.
- D2 Argument: skip spaces/tabs. If next char is `'`, `"` or `` ` ``, token is text up to the same
  quote on the same line, quotes stripped. Else token is the run of non-whitespace chars. Rest of the
  line after the token is ignored (`DELIMITER $$ -- note` sets `$$`).
- D3 Not a directive, scanned as ordinary SQL: no argument (`DELIMITER` then newline or EOF, or only
  blanks), unterminated quoted token, token containing `\` (client refuses it). Server then rejects
  the text (1064); `statementIndex` points at it.
- D4 Directive spans from its line start to just past its `\n` (`\r\n` handled; token never holds
  `\r`), or EOF. New delimiter applies from next line. Next statement starts after the directive.
- D5 Pending text before directive (from current statement start to directive line start): if it
  holds real SQL, push it as its own statement, unterminated (client flushes buffer). If it holds only
  whitespace and non-executable comments, drop it (see D9).
- D6 Delimiter match: at each position outside spans, check `source.startsWith(delimiter, i)`
  (case-sensitive) before calling `scanSqlSpan`. Inside quotes, backticks, `--`, `#`, `/* */` never
  matches, since a span opened at an earlier position skips its body. Delimiter check first means a
  delimiter beginning with `#` wins over a hash comment, as in the client.
- D7 Delimiter `;`: current behaviour exactly, compound `BEGIN`/`END` tracking included. Custom
  delimiter: no block tracking; `;` ordinary. `DELIMITER ;` resets and calls `block.reset()`.
- D8 Statement text excludes its delimiter, trimmed as today. Last statement with no delimiter
  at EOF still pushed. Multi-char delimiter inside a bare identifier (`a$$b`) splits there: client
  parity, documented in a test, not guarded.
- D9 Comment-only fragments (only whitespace, `--`/`#` line comments, block comments not starting
  `/*!` or `/*M!`) are not statements when `delimiterDirective` is on. Reason: mysqldump output ends
  with `-- Dump completed ...` and puts `--` banners before `DELIMITER ;;`; sending one gets
  ER_EMPTY_QUERY 1065 and the dump fails. `mysql` client never sends them. Executable comments
  (`/*!50003 SET ... */`) stay statements: server runs them. Applies to `;` fragments too on
  mysql dialect (`SELECT 1; -- end` now yields one statement). Postgres/SQLite/ClickHouse unchanged.
- D10 Option off (every non-mysql dialect, unknown dialect): `DELIMITER` is ordinary text, byte for
  byte today's output.

mysqldump shape this must run (8.4 and mariadb-dump both emit it):

```
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
DELIMITER ;;
/*!50003 CREATE*/ /*!50017 DEFINER=CURRENT_USER*/ /*!50003 TRIGGER `kira_dump_bi` BEFORE INSERT ON `kira_dump` FOR EACH ROW BEGIN
  SET NEW.body = CONCAT(NEW.body, ';');
END */;;
DELIMITER ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
```

Inner `;` sits inside `/*! */` spans; boundary is `;;`. Statement 2 text is comments only but
executable, so D9 keeps it.

## API changes (`packages/shared/domain`)

`sqlLex.ts`:
- `SqlLexOptions.delimiterDirective?: boolean` with doc comment (MySQL/MariaDB client `DELIMITER`).
- `resolveLexOptions` copies it.

`sqlSplit.ts`:
- `SqlStatement.delimiter?: string`: terminator in effect for the statement, set only when not `;`.
  Optional, so existing literals and callers compile unchanged.
- `export interface DelimiterDirective { start: number; end: number; delimiter: string }` (offsets
  into source, `[start, end)` = whole directive line incl. newline).
- `export function splitSqlScript(source, options?): { statements: SqlStatement[]; directives:
  DelimiterDirective[] }`: the scanner. `splitSqlStatements` becomes
  `splitSqlScript(source, options).statements`. `statementAtOffset`/`statementAtCursor`/
  `trimmedStatementStart` unchanged (directive lines fall in the gap owned by the preceding
  statement, same as blank lines).
- Comment-only test for D5/D9: small helper `isCommentOnly(text, lexOptions)` using `scanSqlSpan`
  (span kinds `lineComment`/`blockComment`; block comment whose text starts `/*!` or `/*M!` counts
  as SQL). Keep it module-private.
- Keep functions short; split the directive parse (`readDirective(source, i, lexOptions)` returning
  `{ delimiter, end } | null`) out of the main loop.

`views/shared/sqlIdent.ts` `lexOptionsFor`: add `delimiterDirective: dialect === 'mysql'`. Every
caller (Run, Format, Explain, rerun, lint, tokenizer) gets it; lint and tokenizer ignore the field.

## Format (`views/console/format.ts`)

Today Format splits, formats each statement, rejoins with `;\n\n`. With directives that drops the
`DELIMITER` lines and terminates bodies with `;`: broken script. Fix in `formatConsoleText`:

1. Call `splitSqlScript` with the same options. `directives.length === 0`: existing path, untouched.
2. Else rebuild in source order (merge statements and directives by `start`):
   - Directive: emit its source line trimmed (`source.slice(d.start, d.end).trim()`).
   - Statement: emit formatted text (or verbatim on failure, as today), then its terminator
     `stmt.delimiter ?? ';'` only when source had one (`text.startsWith(delim, stmt.end)`). Put the
     terminator on its own line when the last line holds a `--` or `#` comment (reuse the check from
     `joinFormattedStatements`: extract a tiny `endsInLineComment(stmt, hashComments)` helper used by
     both).
   - Separator: `\n` after a directive, `\n\n` between a statement and the next item.
3. Statement count and order survive, so `useConsoleFormat` caret-by-index mapping and `failures`
   indexes stay valid. Re-splitting the output yields same statements and delimiters (idempotent).

No change to `useConsoleFormat.ts`, `useConsoleRun.ts`, `OperationsPanel.vue`, `ConsoleView.vue`:
they call the splitter with `lexOptionsFor` options and pick the directive up for free. No new UI
surface, so shadcn-vue/Tailwind/VueUse/Pinia/TanStack rules have nothing to apply to.

## Op-log Re-run

Go records the batch as `JoinConsoleStatements(statements, "--", "#")` (`;\n` join, no
`DELIMITER`). Re-run re-splits it with `compoundBlocks`. `BEGIN ... END` bodies and `/*! */`
mysqldump bodies re-split correctly. A body with inner `;` but no `BEGIN` (`... FOR EACH ROW IF x
THEN SET a = 1; END IF`) would mis-split on Re-run. Implementer checks this with one unit case
(split of the joined text); if it mis-splits, add a Known open item (step 7). Not fixed here: needs
the Go op command to carry its own `DELIMITER` wrap, a separate change to op-log text every reader
sees.

## Steps

1. `sqlLex.ts`, `sqlIdent.ts`: option and wiring. Commit `feat: delimiterDirective lexical option`.
2. `sqlSplit.ts`: `splitSqlScript`, D1-D10, `delimiter` field. Run existing
   `tests/unit/studio/sql-split.spec.ts`; every existing case passes unchanged (none sets the new
   option). Commit `feat: MySQL console DELIMITER directive in splitter`.
3. Unit spec `apps/kira-space/tests/unit/studio/sql-split-delimiter.spec.ts` (complex interacting
   rules, allowed). Cases, one `test` each:
   1. trigger body under `$$` is one statement; text excludes `$$`; `delimiter === '$$'`.
   2. `$$` then `//` then `;` mid-script; `DELIMITER ;` resumes `;` splitting and compound tracking
      (a later `CREATE PROCEDURE ... BEGIN ...; END;` stays whole).
   3. delimiter inside `'..'`, `".."`, `` `..` ``, `--`, `#`, `/* */` is no boundary.
   4. `DELIMITER` inside a comment, string, block comment, or after code on the same line
      (`SELECT 1; DELIMITER $$`) is no directive.
   5. keyword any case, leading blanks, tab separator, CRLF input (token has no `\r`).
   6. D3 cases: bare `DELIMITER`, `DELIMITER ` + newline, backslash token, unterminated quote:
      ordinary text.
   7. quoted token `DELIMITER '$$'`; rest of line ignored `DELIMITER $$ -- note`; `DELIMITER $$;`
      sets `$$;`.
   8. pending unterminated SQL before a directive is pushed alone; comment-only pending text is
      dropped; trailing `-- done` fragment dropped; `/*!50003 SET x = 1 */` kept; `/*M!100100 ...*/`
      kept.
   9. offsets: `start`/`end` index original source; `source.slice(s.start, s.end).trim() === s.text`;
      no statement overlaps a directive's `[start, end)`; `statementAtCursor` inside a body returns
      that body.
   10. `a$$b` with delimiter `$$` splits (client parity).
   11. option off: `DELIMITER $$ ... $$` splits exactly as today (postgres and sqlite options).
   12. contract tie: `mysql#delimiterScript` splits to `mysql#delimiterStatements`; `mysql#dumpScript`
       splits to `mysql#dumpStatements` (read `tests/contract/studio-console-script.json` like
       `console-transaction.spec.ts` does). Written after step 5 generates the keys.
   13. Re-run check from "Op-log Re-run": split of `;\n`-joined statements, records result.
   Commit `test: DELIMITER splitter rules`.
4. `format.ts` rebuild path plus cases in `tests/unit/studio/console-format.spec.ts` (`mysql` kind):
   directive lines kept; each statement ends with its own delimiter; line comment before a custom
   delimiter moves it to next line; unterminated last statement stays unterminated; formatting the
   output again is a no-op on statement list and delimiters. Commit `fix: Format keeps DELIMITER
   directives`.
5. Flow test, new file `apps/kira-space/internal/flows/dbflow/console_delimiter_test.go`:
   - `TestConsoleMySQLDelimiter`: `flowharness.RequireDocker`, `testsupport.StartMysql`. Connect as
     root (copy `fx.Config`, set `Username`/`Password` to `"root"`/`testsupport.FixturePassword`;
     comment: binary logging makes `CREATE TRIGGER`/`CREATE FUNCTION` need SUPER for the fixture user,
     server rule, not ours; do not flip `log_bin_trust_function_creators` on the shared fixture).
   - Script (contract `mysql#delimiterScript`, exact text):

     ```
     -- P636: trigger and function bodies under custom delimiters
     DROP TABLE IF EXISTS kira_delim;
     DROP FUNCTION IF EXISTS kira_delim_twice;
     CREATE TABLE kira_delim (id INT PRIMARY KEY AUTO_INCREMENT, body VARCHAR(64));
     DELIMITER $$
     CREATE TRIGGER kira_delim_bi BEFORE INSERT ON kira_delim FOR EACH ROW
     BEGIN
       IF NEW.body = 'a;b' THEN
         SET NEW.body = CONCAT(NEW.body, '$$'); -- '$$' quoted, no boundary
       END IF;
     END$$
     DELIMITER //
     CREATE FUNCTION kira_delim_twice(x INT) RETURNS INT DETERMINISTIC
     BEGIN
       DECLARE y INT;
       SET y = x * 2;
       RETURN y;
     END //
     DELIMITER ;
     INSERT INTO kira_delim (body) VALUES ('a;b'), ('c');
     SELECT body, kira_delim_twice(id) AS twice FROM kira_delim ORDER BY id;
     SELECT trigger_name FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'kira_delim_bi'
     ```

   - `mysql#delimiterStatements`: hand-written Go `[]string` (8 statements, no `DELIMITER`, no
     `$$`/`//` terminators), as `sqliteScriptStatements` does. Execute via `ds.Execute`; assert page
     count = statement count; `mysql#delimiterPages` = `summaries(pages)`.
   - Oracle via second client (`mysqlRows`-style helper with root creds): rows
     `[["a;b$$","2"],["c","4"]]` from `kira_delim`; `information_schema.triggers` holds
     `kira_delim_bi` with `ACTION_STATEMENT` containing `IF NEW.body` (trigger is real, body whole);
     `information_schema.routines` holds `kira_delim_twice`. Contract `mysql#delimiterEffects`.
   - Dump subtest in same test: `mysql#dumpScript` (shape above, table `kira_dump` created first,
     closing `SET` lines, a leading `--` banner and trailing `-- Dump completed` line) with
     `mysql#dumpStatements`; run, insert a row, assert trigger appended `;` and exists.
   - Cleanup: `DROP TABLE IF EXISTS kira_delim, kira_dump; DROP FUNCTION IF EXISTS kira_delim_twice`.
   - `TestConsoleMariaDBDelimiter`: same statements on `testsupport.StartMariadb` (root), same effect
     asserts, no contract writes. Shared body in one helper taking fixture and a `contract bool`.
   - Generate: `cd apps/kira-space && KIRA_CONTRACT=write KIRA_FLOW_DOCKER=require go test -p 1
     ./internal/flows/dbflow -run 'TestConsoleM(ySQL|ariaDB)Delimiter' -count=1`, then rerun without
     `KIRA_CONTRACT` with `-count=2`. Inspect contract diff for machine paths. Contract file is Biome
     ignored; never hand-format.
   - Update `console_mysql_test.go` J-CONS-021 comment: "DELIMITER is a client directive the engine
     rejects; the console splitter consumes it (P636)". `mysql#delimiter` key and assert stay.
   Commit `test: DELIMITER flow on MySQL and MariaDB`.
6. UI spec, new test in `apps/kira-space/tests/ui/studio/console-script.spec.ts` (same scenario
   file): `'a typed MySQL DELIMITER script runs as the contract statement list'`. Mirror the SQLite
   test: control snapshots for a `mysql` connection (list it up front in `IPC.connectionsList` or go
   through `createConnection`; follow whichever an existing mysql console spec uses), stream snapshot
   `run(contract mysql#delimiterStatements, contract mysql#delimiterPages)`. Paste
   `mysql#delimiterScript` with `typeInto(..., { paste: true })`, click `console-run-all`. Assert:
   result tab count = statement count, no `console-error`, last tab grid contains `kira_delim_bi`,
   the SELECT tab shows `a;b$$`. Stream snapshot matching exact `statements` payload is what proves
   the split. Run: `npx playwright test --config=apps/kira-space/playwright.config.ts --project=ui
   tests/ui/studio/console-script.spec.ts`. Commit `test: DELIMITER UI half`.
7. `docs/ARCHITECTURE.md`, additive hunks only:
   - After the console transaction bullet (line ~228): one bullet "MySQL/MariaDB console `DELIMITER`
     (P636): renderer splitter (`sqlSplit.ts`, option `delimiterDirective`) consumes it, as the
     `mysql` client does; server never sees it; directive lines yield no statement, so
     `statementIndex` indexes the split list. Comment-only fragments are not sent (ER_EMPTY_QUERY);
     executable `/*! */` comments are."
   - Known open item "The SQL splitter has no notion of a compound statement body" (line ~4316) is
     stale for SQLite and MySQL (`compoundBlocks`). Rewrite to the true remainder: Postgres
     `BEGIN ATOMIC ... END` still splits at inner `;` (confirm: `lexOptionsFor` sets
     `compoundBlocks` only for sqlite/mysql). Delete if a check shows it closed.
   - If step 3 case 13 mis-splits: Known open item for Re-run of a no-`BEGIN` body.
   Commit `docs: DELIMITER directive facts`.

## Verification

- `bun test apps/kira-space/tests/unit/studio/sql-split.spec.ts
  apps/kira-space/tests/unit/studio/sql-split-delimiter.spec.ts
  apps/kira-space/tests/unit/studio/console-format.spec.ts
  apps/kira-space/tests/unit/studio/console-transaction.spec.ts`
- `bun run typecheck`, `bun run lint` (Biome, copy, conventions, comment checks), `bun run lint:dead`
  (new exports have real callers: `splitSqlScript` in `format.ts`, `DelimiterDirective` in
  `sqlSplit.ts`).
- Flow: command in step 5, MySQL and MariaDB, with Docker up (`docs/DEV_ENVIRONMENT.md` Docker
  section). `go vet ./apps/kira-space/internal/flows/dbflow`, `bun run lint:go`.
- UI: step 6 command, own spec only.
- Real check before done: grep confirms `delimiterDirective` set in `lexOptionsFor` and read in
  `sqlSplit.ts`; contract file holds the six new keys; `git diff --stat` touches no Go file outside
  `internal/flows/dbflow/`.

## Edge cases (all covered by step 3/4/5 tests)

Quotes (`'`, `"`, backtick) and comments (`--`, `#`, `/* */`) hide delimiter and directive; executable
`/*! */` and `/*M! */` comments; delimiter change mid-script; `DELIMITER ;` reset with compound
tracking resumed; multi-char `$$`, `//`, `;;`, `$$;`; quoted token; rest-of-line ignored; directive
not at line start; missing or backslash token; CRLF; unterminated text before a directive;
comment-only fragments; delimiter in a bare identifier; option off for other dialects; Format
round-trip with trailing line comment; mysqldump trigger section; MariaDB parity; Re-run of joined
op command.

Not in scope: `\d` short form of `DELIMITER` (passes to server, 1064); lint and hover tokenizer
(`sqlTokens.ts` groups on `;` tokens, directive-unaware, cosmetic only: no diagnostic depends on
statement grouping across a custom delimiter); Go op-command `DELIMITER` wrap.

## Files

| file | change |
|---|---|
| `packages/shared/domain/sqlLex.ts` | `delimiterDirective` option, `resolveLexOptions` |
| `packages/shared/domain/sqlSplit.ts` | `splitSqlScript`, directive rules, `delimiter` field |
| `apps/kira-space/frontend/src/studio/views/shared/sqlIdent.ts` | `lexOptionsFor` sets option |
| `apps/kira-space/frontend/src/studio/views/console/format.ts` | directive-preserving rebuild |
| `apps/kira-space/tests/unit/studio/sql-split-delimiter.spec.ts` | new unit spec |
| `apps/kira-space/tests/unit/studio/console-format.spec.ts` | Format cases |
| `apps/kira-space/internal/flows/dbflow/console_delimiter_test.go` | new flow test |
| `apps/kira-space/internal/flows/dbflow/console_mysql_test.go` | comment only |
| `apps/kira-space/tests/contract/studio-console-script.json` | six new `mysql#` keys (generated) |
| `apps/kira-space/tests/ui/studio/console-script.spec.ts` | new UI test |
| `docs/ARCHITECTURE.md` | additive bullet, stale open item narrowed |

No Go production file, no i18n key, no Vue component.

Shared with other streams:
- `docs/ARCHITECTURE.md`: every stream may add hunks; ours are two separate hunks (console section,
  Known open items). Additive, rebase cleanly.
- CI sharding (P634): `scripts/go-test-split.sh` slices `flows/dbflow` by top-level test name from
  `go test -list`, so two new test names need no CI or script edit. No file overlap.
- Agent config (P635, `internal/agentport`, `ade-agent-port.spec.ts`): no overlap.
- `tests/contract/studio-console-script.json` and `console-script.spec.ts`: no other v0.3.14 row
  touches them as far as `SPEC.md` scope shows.
