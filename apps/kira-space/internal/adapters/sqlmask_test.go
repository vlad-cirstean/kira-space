package adapters

import "testing"

func TestMaskSQLLiterals(t *testing.T) {
	cases := []struct {
		name string
		d    SQLDialect
		in   string
		want string
		ok   bool
	}{
		{"plain", PostgresDialect, "SELECT * FROM t WHERE a = 'bob' AND n = 42", "SELECT * FROM t WHERE a = '…' AND n = 42", true},
		{"doubled quote", PostgresDialect, "SELECT 'it''s', 'x'", "SELECT '…', '…'", true},
		{"pg double quote is identifier", PostgresDialect, `SELECT "Col" FROM "T"`, `SELECT "Col" FROM "T"`, true},
		{"mysql double quote is string", MySQLDialect, `SELECT "secret", ` + "`id`", `SELECT '…', ` + "`id`", true},
		{"backslash is a hazard", MySQLDialect, `SELECT 'a\'b'`, "", false},
		{"E string is a hazard", PostgresDialect, `SELECT E'a\'b'`, "", false},
		{"dollar quote is a hazard", PostgresDialect, "SELECT $t$secret$t$", "", false},
		{"comments dropped", PostgresDialect, "SELECT 1 -- pw 'x'\n, 2 /* 'y' */", "SELECT 1  \n, 2  ", true},
		{"quote in comment ignored", SQLiteDialect, "SELECT '/*' , 'a'", "SELECT '…' , '…'", true},
		{"unterminated literal", PostgresDialect, "SELECT 'abc", "", false},
		{"unterminated identifier", PostgresDialect, `SELECT "abc`, "", false},
	}
	for _, c := range cases {
		got, ok := MaskSQLLiterals(c.in, c.d)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}
