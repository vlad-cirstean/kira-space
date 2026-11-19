package adapters

import (
	"encoding/json"
	"strings"
	"testing"
)

func siteTexts(t *testing.T, stmt string, d SQLDialect, s ParamStyle) []string {
	t.Helper()
	sites, err := PlaceholderSites(stmt, d, s)
	if err != nil {
		t.Fatalf("PlaceholderSites(%q): %v", stmt, err)
	}
	out := make([]string, len(sites))
	for i, st := range sites {
		out[i] = stmt[st.From:st.To]
	}
	return out
}

func TestPlaceholderSites(t *testing.T) {
	cases := []struct {
		name  string
		stmt  string
		d     SQLDialect
		style ParamStyle
		want  []string
	}{
		{"pg numbered and cast", "select $1::int, $10, $2", PostgresDialect, ParamDollar, []string{"$1", "$10", "$2"}},
		{"pg ident with dollar", "select col$1 from t where a = $1", PostgresDialect, ParamDollar, []string{"$1"}},
		{"pg in literal and comments", "select '$1', \"$2\", $3 -- $4\n/* $5 /* $6 */ $7 */", PostgresDialect, ParamDollar, []string{"$3"}},
		{"mysql literals comments", "select '?', \"?\", `?`, ? -- ?\n, ? # ?\n, ?/* ? */", MySQLDialect, ParamQuestion, []string{"?", "?", "?"}},
		{"mysql dash without space is code", "select 1--?", MySQLDialect, ParamQuestion, []string{"?"}},
		{"sqlite flat blocks", "select ? /* ? /* */ ?", SQLiteDialect, ParamQuestion, []string{"?", "?"}},
		{"ch typed", "select * from t where id in {ids:Array(UInt64)} and s = { s : String } and '{x:String}' = ''", ClickHouseDialect, ParamBraces, []string{"{ids:Array(UInt64)}", "{ s : String }"}},
		{"ch quoted type arg", "select {t:DateTime64(3, 'UTC')}", ClickHouseDialect, ParamBraces, []string{"{t:DateTime64(3, 'UTC')}"}},
		{"ch brace that is not a placeholder", "select {1, 2}, {'a': 1}", ClickHouseDialect, ParamBraces, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := siteTexts(t, c.stmt, c.d, c.style)
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestPlaceholderSitesIndexAndOffsets(t *testing.T) {
	sites, err := PlaceholderSites("select 'é', $2, $1", PostgresDialect, ParamDollar)
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[0].Index != 2 || sites[1].Index != 1 {
		t.Fatalf("sites = %+v", sites)
	}
	if got := "select 'é', $2, $1"[sites[0].From:sites[0].To]; got != "$2" {
		t.Fatalf("byte offsets off: %q", got)
	}
	ch, err := PlaceholderSites("select {a:UInt8}, {b:String}", ClickHouseDialect, ParamBraces)
	if err != nil || ch[1].Name != "b" || ch[1].Type != "String" || ch[1].Index != 2 {
		t.Fatalf("ch sites = %+v, %v", ch, err)
	}
}

func TestPlaceholderSitesRefusals(t *testing.T) {
	cases := []struct {
		name  string
		stmt  string
		d     SQLDialect
		style ParamStyle
	}{
		{"dollar quote", "select $$a$$, $1", PostgresDialect, ParamDollar},
		{"backslash in string", "select 'a\\b', ?", MySQLDialect, ParamQuestion},
		{"executable comment", "select /*! 1 */ ?", MySQLDialect, ParamQuestion},
		{"unterminated quote", "select 'a, ?", SQLiteDialect, ParamQuestion},
		{"unterminated comment", "select ? /* x", SQLiteDialect, ParamQuestion},
		{"sqlite numbered", "select ?1", SQLiteDialect, ParamQuestion},
		{"sqlite named colon", "select :a", SQLiteDialect, ParamQuestion},
		{"sqlite named at", "select @a", SQLiteDialect, ParamQuestion},
		{"sqlite named dollar", "select $a", SQLiteDialect, ParamQuestion},
		{"pg zero", "select $0", PostgresDialect, ParamDollar},
		{"pg too big", "select $65536", PostgresDialect, ParamDollar},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := PlaceholderSites(c.stmt, c.d, c.style); err == nil {
				t.Fatalf("PlaceholderSites(%q) accepted", c.stmt)
			}
		})
	}
}

func TestSecondStatement(t *testing.T) {
	cases := []struct {
		stmt string
		want bool
	}{
		{"select 1", false},
		{"select 1;", false},
		{"select 1; -- done", false},
		{"select 1; /* x */ ", false},
		{"select ';' ; ", false},
		{"select 1; select 2", true},
		{"select 1;;", true},
		{"select 1 -- ; select 2", false},
		{"select 'a\\' ; select 2", true},
	}
	for _, c := range cases {
		if got := SecondStatement(c.stmt, SQLiteDialect); got != c.want {
			t.Errorf("SecondStatement(%q) = %v, want %v", c.stmt, got, c.want)
		}
	}
}

func TestDecodeParam(t *testing.T) {
	scalar := []struct {
		raw  string
		kind ParamKind
		text string
	}{
		{`9007199254740993`, ParamInt, "9007199254740993"},
		{`-9223372036854775808`, ParamInt, "-9223372036854775808"},
		{`18446744073709551616`, ParamNumber, "18446744073709551616"},
		{`1.50`, ParamNumber, "1.50"},
		{`1e400`, ParamNumber, "1e400"},
		{`"it's"`, ParamString, "it's"},
		{`true`, ParamBool, ""},
		{`null`, ParamNull, ""},
	}
	for _, c := range scalar {
		v, err := DecodeParam("x", json.RawMessage(c.raw))
		if err != nil || v.Kind != c.kind || v.Text != c.text {
			t.Errorf("DecodeParam(%s) = %+v, %v", c.raw, v, err)
		}
	}
	lists := []struct {
		raw  string
		elem ParamKind
	}{
		{`[1,2,3]`, ParamInt},
		{`[1,2.5]`, ParamNumber},
		{`["a","b"]`, ParamString},
		{`[true,false]`, ParamBool},
		{`[]`, ParamNull},
	}
	for _, c := range lists {
		v, err := DecodeParam("x", json.RawMessage(c.raw))
		if err != nil || v.Kind != ParamList || v.Elem != c.elem {
			t.Errorf("DecodeParam(%s) = %+v, %v", c.raw, v, err)
		}
	}
	for _, raw := range []string{`{"a":1}`, `[1,"a"]`, `[1,null]`, `[[1]]`, `[{"a":1}]`, `[true,1]`} {
		if _, err := DecodeParam("x", json.RawMessage(raw)); err == nil {
			t.Errorf("DecodeParam(%s) accepted", raw)
		}
	}
	big := "[" + strings.Repeat("1,", MaxParamListItems) + "1]"
	if _, err := DecodeParam("x", json.RawMessage(big)); err == nil {
		t.Error("list over the item cap accepted")
	}
}
