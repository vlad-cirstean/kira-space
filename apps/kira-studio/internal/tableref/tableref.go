// Package tableref models the per-connection MCP table allow list: entry strings, the match set and the
// tree-path mapping. Pure; shared by connfiles validation, adapters, dbmcp and the bridge.
package tableref

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// Ref is one table, view or collection, fully qualified for its kind. Unused levels are "".
type Ref struct{ Database, Schema, Name string }

// Levels is how many parts an entry has for kind: 3 postgres; 2 mysql, mariadb, clickhouse, mongodb;
// 1 sqlite, kibana; 0 when the list does not apply.
func Levels(kind string) int {
	switch kind {
	case "postgres":
		return 3
	case "mysql", "mariadb", "clickhouse", "mongodb":
		return 2
	case "sqlite", "kibana":
		return 1
	}
	return 0
}

// Applies reports whether the allow list means anything for kind.
func Applies(kind string) bool { return Levels(kind) > 0 }

func (r Ref) parts(kind string) []string {
	switch Levels(kind) {
	case 3:
		return []string{r.Database, r.Schema, r.Name}
	case 2:
		return []string{r.Database, r.Name}
	case 1:
		return []string{r.Name}
	}
	return nil
}

func fromParts(kind string, p []string) Ref {
	switch Levels(kind) {
	case 3:
		return Ref{Database: p[0], Schema: p[1], Name: p[2]}
	case 2:
		return Ref{Database: p[0], Name: p[1]}
	}
	return Ref{Name: p[0]}
}

func needsQuote(seg string) bool {
	return strings.ContainsAny(seg, `."`) || seg != strings.TrimSpace(seg)
}

func quote(seg string) string {
	if !needsQuote(seg) {
		return seg
	}
	return `"` + strings.ReplaceAll(seg, `"`, `""`) + `"`
}

// Format writes the canonical entry: quotes only where Parse needs them.
func Format(kind string, r Ref) string {
	parts := r.parts(kind)
	for i, p := range parts {
		parts[i] = quote(p)
	}
	return strings.Join(parts, ".")
}

// Parse reads one entry. Quoted segments may hold '.', '"' (doubled) and spaces.
func Parse(kind, entry string) (Ref, error) {
	want := Levels(kind)
	if want == 0 {
		return Ref{}, fmt.Errorf("table list does not apply to %q", kind)
	}
	var parts []string
	rs := []rune(entry)
	for i := 0; ; {
		var b strings.Builder
		if i < len(rs) && rs[i] == '"' {
			i++
			closed := false
			for i < len(rs) {
				if rs[i] == '"' {
					if i+1 < len(rs) && rs[i+1] == '"' {
						b.WriteRune('"')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				b.WriteRune(rs[i])
				i++
			}
			if !closed {
				return Ref{}, errors.New("unterminated quote")
			}
			if i < len(rs) && rs[i] != '.' {
				return Ref{}, errors.New("text after closing quote")
			}
		} else {
			for i < len(rs) && rs[i] != '.' {
				if rs[i] == '"' {
					return Ref{}, errors.New("quote inside unquoted part")
				}
				b.WriteRune(rs[i])
				i++
			}
			if s := b.String(); s != strings.TrimSpace(s) {
				return Ref{}, errors.New("leading or trailing space needs quotes")
			}
		}
		if b.Len() == 0 {
			return Ref{}, errors.New("empty part")
		}
		parts = append(parts, b.String())
		if i >= len(rs) {
			break
		}
		i++ // the '.'
	}
	if len(parts) != want {
		return Ref{}, fmt.Errorf("want %d parts for %s, got %d", want, kind, len(parts))
	}
	return fromParts(kind, parts), nil
}

func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return r
	}, s)
}

// Key is the match key: Format, ASCII-lowercased for sqlite.
func Key(kind string, r Ref) string {
	k := Format(kind, r)
	if kind == "sqlite" {
		return asciiLower(k)
	}
	return k
}

func nameKey(kind, name string) string {
	if kind == "sqlite" {
		return asciiLower(name)
	}
	return name
}

// Set is a parsed list. A nil *Set means all.
type Set struct {
	kind    string
	keys    map[string]bool
	names   map[string]bool
	entries []string
}

// NewSet parses entries. nil entries means all and returns nil, nil; an empty non-nil slice is the empty list.
func NewSet(kind string, entries []string) (*Set, error) {
	if entries == nil {
		return nil, nil
	}
	s := &Set{kind: kind, keys: map[string]bool{}, names: map[string]bool{}}
	for i, e := range entries {
		r, err := Parse(kind, e)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		k := Key(kind, r)
		if s.keys[k] {
			continue
		}
		s.keys[k] = true
		s.names[nameKey(kind, r.Name)] = true
		s.entries = append(s.entries, Format(kind, r))
	}
	sort.Strings(s.entries)
	return s, nil
}

// Allows reports whether r is listed. A nil set allows everything.
func (s *Set) Allows(r Ref) bool {
	if s == nil {
		return true
	}
	return s.keys[Key(s.kind, r)]
}

// AllowsName reports whether any listed entry has this name part. A nil set allows everything.
func (s *Set) AllowsName(name string) bool {
	if s == nil {
		return true
	}
	return s.names[nameKey(s.kind, name)]
}

// Entries is the canonical, deduplicated, sorted entries. Nil for a nil set.
func (s *Set) Entries() []string {
	if s == nil {
		return nil
	}
	return s.entries
}

var relationKinds = map[string]bool{"table": true, "view": true, "matview": true}

// IsRelationKind reports whether a tree node kind is a listable relation.
func IsRelationKind(nodeKind string) bool { return relationKinds[nodeKind] || nodeKind == "collection" }

// FromPath maps a tree path to the relation it is or sits under. ok is false when the path names no relation.
func FromPath(kind string, segs []model.PathSegment) (r Ref, ok bool) {
	switch kind {
	case "postgres":
		if len(segs) >= 3 && segs[0].Kind == "database" && segs[1].Kind == "schema" && relationKinds[segs[2].Kind] {
			return Ref{Database: segs[0].Name, Schema: segs[1].Name, Name: segs[2].Name}, true
		}
	case "mysql", "mariadb", "clickhouse":
		if len(segs) >= 2 && segs[0].Kind == "database" && relationKinds[segs[1].Kind] {
			return Ref{Database: segs[0].Name, Name: segs[1].Name}, true
		}
	case "mongodb":
		if len(segs) >= 2 && segs[0].Kind == "database" && segs[1].Kind == "collection" {
			return Ref{Database: segs[0].Name, Name: segs[1].Name}, true
		}
	case "sqlite":
		if len(segs) >= 2 && segs[0].Kind == "database" && segs[0].Name == "main" && relationKinds[segs[1].Kind] {
			return Ref{Name: segs[1].Name}, true
		}
	case "kibana":
		if len(segs) >= 1 && segs[0].Kind == "collection" {
			return Ref{Name: segs[0].Name}, true
		}
	}
	return Ref{}, false
}
