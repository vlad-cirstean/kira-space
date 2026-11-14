package dbflow

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// hostileCells is the `name` column of the clipboard table: formula starters, control characters,
// quoting, multi-byte text, NULL, empty and whitespace-only values.
var hostileCells = []*string{
	sp("=1+2"), sp("+x"), sp("-y"), sp("@z"), sp("\tlead"), sp("a\rb"), sp("x\ty"), sp("l1\nl2"),
	sp(`say "hi"`), sp("😀 ok"), sp("שלום עולם"), sp(strings.Repeat("ü", 4000)), nil, sp(""), sp("   "),
}

func sp(s string) *string { return &s }

var tsvNeedsQuote = regexp.MustCompile(`["\t\n\r]`)
var csvFormula = regexp.MustCompile(`^[=+\-@\t\r]`)

func tsvCell(v *string) string {
	if v == nil {
		return ""
	}
	if tsvNeedsQuote.MatchString(*v) {
		return `"` + strings.ReplaceAll(*v, `"`, `""`) + `"`
	}
	return *v
}

func csvCell(v *string) string {
	if v == nil {
		return ""
	}
	g := *v
	if csvFormula.MatchString(g) {
		g = "'" + g
	}
	if strings.ContainsAny(g, "\",\n\r") {
		return `"` + strings.ReplaceAll(g, `"`, `""`) + `"`
	}
	return g
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// J-GRID-010 / J-GRID-011 (contract studio-grid-clipboard): the engine hands back every hostile
// value byte-for-byte, and the contract holds the clipboard text the grid must produce for them.
// TSV is raw (quoted only when a field holds a quote, tab or line break), CSV neutralizes formula
// starters with ' and uses CRLF, JSON keeps null. The UI half (grid-clipboard-bytes.spec.ts)
// replays the same rows and compares what Copy writes.
func TestGridClipboardBytesContract(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "clip.db")
	sqliteExec(t, path, `create table composite_pk (tenant_id integer not null, entity_id integer not null, name text, primary key (tenant_id, entity_id))`)
	for i, v := range hostileCells {
		if v == nil {
			sqliteExec(t, path, `insert into composite_pk values (1, `+itoa(i+1)+`, null)`)
			continue
		}
		sqliteExecArgs(t, path, `insert into composite_pk values (1, ?, ?)`, i+1, *v)
	}
	conn := createConn(t, app, sqliteInput("clip", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	tp := tableNodePath(t, app, conn.ID, "composite_pk")
	page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != len(hostileCells) {
		t.Fatalf("rows = %d, want %d", len(page.Rows), len(hostileCells))
	}
	var tsv, csv []string
	var objs []string
	for i, row := range page.Rows {
		got := row[2]
		switch want := hostileCells[i]; {
		case want == nil && got != nil, want != nil && got != *want:
			t.Fatalf("row %d name = %q, want %v", i+1, got, want)
		}
		id := itoa(i + 1)
		tsv = append(tsv, "1\t"+id+"\t"+tsvCell(hostileCells[i]))
		csv = append(csv, "1,"+id+","+csvCell(hostileCells[i]))
		var name any
		if hostileCells[i] != nil {
			name = *hostileCells[i]
		}
		objs = append(objs, "  {\n    \"tenant_id\": \"1\",\n    \"entity_id\": \""+id+"\",\n    \"name\": "+jsonString(t, name)+"\n  }")
	}
	app.Contract(t, "studio-grid-clipboard", "rows", page.Rows, contractMask)
	app.Contract(t, "studio-grid-clipboard", "copy:tsv", strings.Join(tsv, "\n"))
	app.Contract(t, "studio-grid-clipboard", "copy:csv", strings.Join(csv, "\r\n"))
	app.Contract(t, "studio-grid-clipboard", "copy:json", "[\n"+strings.Join(objs, ",\n")+"\n]")
}

// J-GRID-036 (OQ-GRID-2): a cell over the engine's 64 KiB display cap comes back clipped and flagged,
// so the grid can tell the user a copy holds the shortened text.
func TestGridReadFlagsClippedCell(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "clip.db")
	sqliteExec(t, path, `create table big (id integer primary key, body text)`)
	sqliteExecArgs(t, path, `insert into big values (1, ?)`, strings.Repeat("x", 100*1024))
	sqliteExecArgs(t, path, `insert into big values (2, ?)`, "small")
	conn := createConn(t, app, sqliteInput("big", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tableNodePath(t, app, conn.ID, "big")})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Truncated) != 2 || !page.Truncated[0] || page.Truncated[1] {
		t.Fatalf("clipped flags = %v, want [true false]", page.Truncated)
	}
	if got := len(page.Rows[0][1].(string)); got != 64*1024 {
		t.Fatalf("clipped cell holds %d bytes, want %d", got, 64*1024)
	}
}
