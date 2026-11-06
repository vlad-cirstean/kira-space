package maskreview

import (
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tableref"
)

// Known is the live columns of one check, by relation. A result column outside the relations a statement
// reads is an alias, a computed column or a column added since the check.
type Known struct {
	kind    string
	byTable map[string]map[string]bool
	names   map[string]bool
	// aliases holds the names a refresh left unmatched, per statement scope, so the same alias does not
	// refresh again until the next full check replaces the entry.
	aliases map[string]bool
}

// Rescan is what a result needs from Recheck: the statement's relations, and the result columns the last
// check could not vouch for. Parsed is false when the statement's relations are unknown.
type Rescan struct {
	Tables []tableref.Ref
	Parsed bool
	Names  []string
}

// columnKey folds case only where the engine does: Postgres and ClickHouse columns differing in case are
// different columns.
func columnKey(kind, name string) string {
	switch kind {
	case "mysql", "mariadb", "sqlite":
		return strings.ToLower(name)
	}
	return name
}

// NewKnown indexes live columns by relation.
func NewKnown(kind string, live []LiveColumn) *Known {
	k := &Known{kind: kind, byTable: map[string]map[string]bool{}, names: map[string]bool{}, aliases: map[string]bool{}}
	for _, c := range live {
		key, col := tableref.Key(kind, c.Ref.Ref), columnKey(kind, c.Ref.Column)
		if k.byTable[key] == nil {
			k.byTable[key] = map[string]bool{}
		}
		k.byTable[key][col] = true
		k.names[col] = true
	}
	return k
}

func (k *Known) scopeKey(tables []tableref.Ref, parsed bool) string {
	if !parsed {
		return "?"
	}
	keys := make([]string, len(tables))
	for i, r := range tables {
		keys[i] = tableref.Key(k.kind, r)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

func (k *Known) settled(tables []tableref.Ref, parsed bool, col string) bool {
	if !parsed {
		return k.names[col]
	}
	if len(tables) == 0 {
		return true
	}
	for _, r := range tables {
		if k.byTable[tableref.Key(k.kind, r)][col] {
			return true
		}
	}
	return false
}

// Unsettled returns the result columns that are no column of the statement's relations and no remembered
// alias of it. An unparsed statement falls back to the names of every relation.
func (k *Known) Unsettled(tables []tableref.Ref, parsed bool, names []string) []string {
	scope := k.scopeKey(tables, parsed)
	var out []string
	for _, n := range names {
		col := columnKey(k.kind, n)
		if !k.aliases[scope+"\x01"+col] && !k.settled(tables, parsed, col) {
			out = append(out, n)
		}
	}
	return out
}

// carry returns k with old's aliases and those of req that still match no column.
func (k *Known) carry(old *Known, req Rescan) *Known {
	next := *k
	next.aliases = map[string]bool{}
	if old != nil {
		for a := range old.aliases {
			next.aliases[a] = true
		}
	}
	scope := k.scopeKey(req.Tables, req.Parsed)
	for _, n := range k.Unsettled(req.Tables, req.Parsed, req.Names) {
		next.aliases[scope+"\x01"+columnKey(k.kind, n)] = true
	}
	return &next
}
