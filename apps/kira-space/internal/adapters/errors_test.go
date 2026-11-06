package adapters_test

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// TestStripSQLComments exercises the scanner's per-state arms directly:
// quote-awareness, dollar quoting, nested block comments and MySQL/MariaDB executable comments all
// interact through shared depth/execComment state, which AssertNoTransactionEscalation's own tests
// above cover only indirectly.
func TestStripSQLComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no comment", "SELECT 1", "SELECT 1"},
		{"line comment to end of line", "SELECT 1 -- trailing\nFROM t", "SELECT 1  \nFROM t"},
		{"block comment becomes one space", "READ/*x*/WRITE", "READ WRITE"},
		{"nested block comment", "READ /* /* */ x */ WRITE", "READ   WRITE"},
		{"doubly nested block comment", "a /* b /* c /* d */ e */ f */ g", "a   g"},
		{"unterminated block comment consumes to EOF", "SELECT 1 /* oops", "SELECT 1 "},
		// A `'`, `"` or backtick-quoted run is skipped whole before comment markers inside it are
		// considered (finding #1, M6) — a `--`/`/*` inside a string literal is not a real comment.
		{"line comment marker inside single-quoted string is not a comment", "SELECT '--' AS note", "SELECT '--' AS note"},
		{"block comment marker inside double-quoted string is not a comment", `SELECT "/*" AS note`, `SELECT "/*" AS note`},
		{"comment marker inside backtick identifier is not a comment", "SELECT `--col` FROM t", "SELECT `--col` FROM t"},
		{"doubled quote inside string is not a close", "SELECT 'it''s -- fine'", "SELECT 'it''s -- fine'"},
		{"real comment after a closed string is still stripped", "SELECT '' -- trailing\n", "SELECT ''  \n"},
		// Postgres dollar-quoted strings ($$...$$ / $tag$...$tag$) are skipped the same way.
		{"dollar-quoted string hides a comment marker", "SELECT $$--not a comment$$", "SELECT $$--not a comment$$"},
		{"tagged dollar quote hides a comment marker", "SELECT $tag$/* not */ $tag$", "SELECT $tag$/* not */ $tag$"},
		{"bare dollar sign is not a quote", "SELECT $1", "SELECT $1"},
		// MySQL/MariaDB executable comments run their body as real SQL — only the marker and closing
		// */ are stripped, the body stays (finding #2, M7).
		{"mysql executable comment body survives", "/*!COMMIT*/", " COMMIT "},
		{"mariadb executable comment body survives", "/*M!COMMIT*/", " COMMIT "},
		{"version-gated executable comment body survives", "/*!50000 COMMIT */", "  COMMIT  "},
		{"ordinary block comment is still stripped", "/* COMMIT */ SELECT 1", "  SELECT 1"},
		{"exec comment nested inside ordinary comment is not recognised", "/* /*!COMMIT*/ */", " "},
		// Finding F1: an E'...'/e'...' string's backslash-escaped quote is now recognized as an
		// escape, so the scanner correctly runs the quoted span past it to the real closing quote
		// instead of stopping right after the escaped one and exposing ` -- ` as a real comment
		// opener.
		{"e-string backslash-escaped quote does not end the string early", `SELECT E'\' -- '`, `SELECT E'\' -- '`},
		{"lowercase e-string backslash-escaped quote does not end the string early", `SELECT e'\' -- '`, `SELECT e'\' -- '`},
		// An identifier merely ending in e/E (no word boundary before the quote) must not be
		// mistaken for the E-string prefix: were it, the backslash right before the quote would
		// wrongly be read as an escape too, extending the "string" past its real end (right after
		// that quote) and hiding the genuine `; DROP TABLE x` that follows as if it were part of
		// the string's own content instead of live SQL text.
		{"identifier ending in e is not mistaken for the E-string prefix", `SELECT value'\' ; DROP TABLE x -- ' FROM t`, `SELECT value'\' ; DROP TABLE x  `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapters.StripSQLComments(tt.in, adapters.PostgresDialect); got != tt.want {
				t.Fatalf("StripSQLComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAssertNoTransactionEscalation(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
		wantReject bool
	}{
		{"plain read write", []string{"SET TRANSACTION READ WRITE"}, true},
		{"case insensitive", []string{"set transaction read write"}, true},
		{"session variant", []string{"SET SESSION TRANSACTION READ WRITE"}, true},
		{"tab between words", []string{"SET TRANSACTION READ\tWRITE"}, true},
		{"newline between words", []string{"SET TRANSACTION READ\nWRITE"}, true},
		{"line comment before phrase", []string{"-- flip it\nSET TRANSACTION READ WRITE"}, true},
		{"line comment does not merge words across lines", []string{"SET TRANSACTION READ -- comment\nWRITE"}, true},
		// A block comment is lexical whitespace to a real SQL parser (confirmed against a real
		// Postgres server: READ/*x*/WRITE parses identically to READ WRITE), so it must still be
		// caught even though stripping it naively (deleting rather than blanking) would otherwise
		// glue READ and WRITE into one word and slip past the \s+ separator.
		{"block comment between words", []string{"SET TRANSACTION READ/*sneaky*/WRITE"}, true},
		{"block comment elsewhere in statement", []string{"SET /* note */ TRANSACTION READ WRITE"}, true},
		{"second statement in batch", []string{"SELECT 1", "SET TRANSACTION READ WRITE"}, true},
		{"ordinary read-only statement", []string{"SELECT 1"}, false},
		{"read only phrase is not read write", []string{"BEGIN READ ONLY"}, false},
		{"unrelated word write alone", []string{"UPDATE t SET note = 'write'"}, false},
		{"unrelated word read alone", []string{"SELECT * FROM t WHERE note = 'read'"}, false},
		{"empty batch", nil, false},
		// Nested block comments (review finding, case 3): confirmed against a real Postgres server
		// that `READ /* /* */ x */ WRITE` parses identically to `READ WRITE` — the outer /* runs to
		// its own matching, correctly-nested */, so a non-nesting single regexp pass misses it.
		{"nested block comment collapses to read write", []string{"SET TRANSACTION READ /* /* */ x */ WRITE"}, true},
		{"doubly nested block comment", []string{"SET TRANSACTION READ /* a /* b /* c */ d */ e */ WRITE"}, true},
		// Naming the read-only GUC/session variable directly and assigning it a falsy value (review
		// finding, case 1): confirmed against a real Postgres server that `SET transaction_read_only
		// = off` inside the wrapping transaction flips it writable, with no "READ WRITE" phrase
		// anywhere for sqlReadWrite to catch.
		{"transaction_read_only off", []string{"SET transaction_read_only = off"}, true},
		{"transaction_read_only off, session, uppercase", []string{"SET SESSION TRANSACTION_READ_ONLY = OFF"}, true},
		{"transaction_read_only false", []string{"SET transaction_read_only = false"}, true},
		{"transaction_read_only zero", []string{"SET transaction_read_only = 0"}, true},
		{"tx_read_only off (mariadb spelling)", []string{"SET SESSION tx_read_only = OFF"}, true},
		{"default_transaction_read_only off", []string{"SET default_transaction_read_only = off"}, true},
		{"transaction_read_only TO off", []string{"SET transaction_read_only TO off"}, true},
		// P168 Part 3 F1: Postgres parse_bool accepts any unique prefix; GUC and value may be quoted;
		// ABORT is ROLLBACK's synonym.
		{"transaction_read_only no", []string{"SET transaction_read_only = no"}, true},
		{"transaction_read_only n", []string{"SET transaction_read_only = n"}, true},
		{"transaction_read_only f", []string{"SET transaction_read_only = f"}, true},
		{"transaction_read_only fa", []string{"SET transaction_read_only TO fa"}, true},
		{"transaction_read_only of", []string{"SET transaction_read_only = of"}, true},
		{"transaction_read_only quoted name", []string{`SET "transaction_read_only" = off`}, true},
		{"transaction_read_only quoted value", []string{`SET transaction_read_only = 'no'`}, true},
		{"default_transaction_read_only quoted double value", []string{`SET default_transaction_read_only = "false"`}, true},
		{"abort ends the transaction", []string{"ABORT"}, true},
		{"abort work", []string{"abort work"}, true},
		{"transaction_read_only on is not an escalation", []string{"SET transaction_read_only = on"}, false},
		{"transaction_read_only yes is not an escalation", []string{"SET transaction_read_only = yes"}, false},
		{"transaction_read_only set to on is not an escalation", []string{"SET transaction_read_only = on"}, false},
		{"default_transaction_read_only set to true is not an escalation", []string{"SET default_transaction_read_only = true"}, false},
		// A bare COMMIT/END/ROLLBACK ends the wrapping read-only transaction itself: confirmed
		// against real MariaDB and MySQL servers that `COMMIT; SET SESSION
		// tx_read_only/transaction_read_only = OFF; <write>` lets the write through once the
		// wrapper's own COMMIT has ended it — postgres was not itself vulnerable to this exact
		// sequence, but a read-only console session never has a legitimate reason to end its own
		// wrapping transaction on any engine, so this is rejected outright regardless.
		{"bare commit", []string{"COMMIT"}, true},
		{"bare commit lowercase", []string{"commit"}, true},
		{"commit work", []string{"COMMIT WORK"}, true},
		{"commit and chain", []string{"COMMIT AND CHAIN"}, true},
		{"end alias for commit", []string{"END"}, true},
		{"end transaction", []string{"END TRANSACTION"}, true},
		{"bare rollback", []string{"ROLLBACK"}, true},
		{"rollback work", []string{"ROLLBACK WORK"}, true},
		{"commit as second statement in batch", []string{"SELECT 1", "COMMIT"}, true},
		{"rollback to savepoint is not rejected", []string{"ROLLBACK TO SAVEPOINT sp1"}, false},
		{"rollback to a named savepoint without keyword", []string{"ROLLBACK TO sp1"}, false},
		{"savepoint itself is not rejected", []string{"SAVEPOINT sp1"}, false},
		{"commit mentioned mid-statement is not rejected", []string{"SELECT 'please COMMIT' AS note"}, false},
		// MySQL/MariaDB executes the body of `/*! ... */`/`/*M! ... */` as
		// real SQL — confirmed against a real MariaDB server that `/*!COMMIT*/ /*!SET SESSION
		// tx_read_only = OFF*/ DELETE FROM users` ends the wrapping read-only transaction and flips
		// the session writable, though a naive comment-stripping scanner sees only whitespace here.
		{"mysql executable comment commit is rejected", []string{"/*!COMMIT*/"}, true},
		{"mariadb executable comment commit is rejected", []string{"/*M!COMMIT*/"}, true},
		{"mysql executable comment tx_read_only off is rejected", []string{"/*!SET SESSION tx_read_only = OFF*/"}, true},
		{"version-gated executable comment commit is rejected", []string{"/*!50000COMMIT*/"}, true},
		{"ordinary block comment commit is still just a comment", []string{"/* COMMIT */ SELECT 1"}, false},
		// Finding F1 (HIGH, security): a Postgres E'...' string's backslash-escaped quote closed the
		// scanner's idea of the string early, so `-- '` right after it looked like a real line
		// comment — which then swallowed a genuine COMMIT/SET escalation chain (ending the wrapping
		// read-only transaction, then flipping the session writable) along with the DELETE it was
		// hiding. Pre-fix, AssertNoTransactionEscalation on this exact chain returned nil (allowed);
		// it must now reject.
		{"e-string backslash-escaped quote hides a commit/set escalation chain", []string{`SELECT E'\' -- ' ; COMMIT; SET transaction_read_only = off; DELETE FROM t`}, true},
		// A quoted run containing a raw backslash is rejected outright regardless of whether it
		// parses as an E-string: standard_conforming_strings=off makes even a plain '...' string
		// honour backslash escaping, which this scanner cannot detect at runtime, so it fails closed.
		{"plain quoted string containing a backslash is rejected outright", []string{`SELECT '\' AS note`}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapters.AssertNoTransactionEscalation(tt.statements, adapters.PostgresDialect)
			if tt.wantReject {
				var ae *adapters.Error
				if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
					t.Fatalf("AssertNoTransactionEscalation(%v) = %v, want an E_UNSUPPORTED *adapters.Error", tt.statements, err)
				}
			} else if err != nil {
				t.Fatalf("AssertNoTransactionEscalation(%v) = %v, want nil", tt.statements, err)
			}
		})
	}
}

func TestAssertNoTransactionEscalationMySQLLexer(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
		wantReject bool
	}{
		{"hash comment before commit", []string{"# x\nCOMMIT"}, true},
		{"commit after non-nested block comment", []string{"/* /* */ COMMIT -- */"}, true},
		{"dash dash arithmetic does not hide a set", []string{"SET @a = 1--1, SESSION transaction_read_only = OFF"}, true},
		{"dash dash comment hides nothing live", []string{"SELECT 1 -- COMMIT"}, false},
		{"hash comment hides nothing live", []string{"SELECT 1 # COMMIT"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapters.AssertNoTransactionEscalation(tt.statements, adapters.MySQLDialect)
			var ae *adapters.Error
			if got := errors.As(err, &ae) && ae.Code == adapters.CodeUnsupported; got != tt.wantReject {
				t.Fatalf("AssertNoTransactionEscalation(%v) = %v, wantReject %v", tt.statements, err, tt.wantReject)
			}
		})
	}
}

func TestStripSQLCommentsDialects(t *testing.T) {
	tests := []struct {
		name string
		d    adapters.SQLDialect
		in   string
		want string
	}{
		{"mysql hash comment", adapters.MySQLDialect, "a # b\nc", "a  \nc"},
		{"postgres hash is an operator", adapters.PostgresDialect, "a # b", "a # b"},
		{"mysql dash needs space", adapters.MySQLDialect, "1--1", "1--1"},
		{"mysql dash then tab", adapters.MySQLDialect, "1--\tx\ny", "1 \ny"},
		{"mysql flat block comments", adapters.MySQLDialect, "a /* /* */ b */", "a   b */"},
		{"sqlite flat block comments", adapters.SQLiteDialect, "a /* /* */ b */", "a   b */"},
		{"postgres nested block comments", adapters.PostgresDialect, "a /* /* */ b */", "a  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adapters.StripSQLComments(tt.in, tt.d); got != tt.want {
				t.Fatalf("StripSQLComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
