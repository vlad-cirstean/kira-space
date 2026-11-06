package kql

import (
	"regexp"
	"strings"
)

// Field is what the compiler needs to know about a data view field.
type Field struct {
	Name       string
	Type       string // Kibana type: string, number, date, ...
	Nested     string // nested path, "" for a plain field
	Searchable bool
}

// Compile turns a KQL query into an Elasticsearch query. A blank query is match_all. timeZone is
// the IANA zone range queries on date fields use; "" leaves the field's own.
func Compile(query string, fields []Field, timeZone string) (map[string]any, error) {
	root, err := parse(query)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return map[string]any{"match_all": map[string]any{}}, nil
	}
	c := &compiler{fields: fields, byName: make(map[string]Field, len(fields)), tz: timeZone}
	for _, f := range fields {
		c.byName[f.Name] = f
	}
	return c.node(root, ""), nil
}

type compiler struct {
	fields []Field
	byName map[string]Field
	tz     string
}

func (c *compiler) node(n node, nested string) map[string]any {
	switch n := n.(type) {
	case orNode:
		return boolShould(c.nodes(n.items, nested))
	case andNode:
		return map[string]any{"bool": map[string]any{"filter": c.nodes(n.items, nested)}}
	case notNode:
		return map[string]any{"bool": map[string]any{"must_not": []any{c.node(n.child, nested)}}}
	case fieldNode:
		return c.field(n, nested)
	case textNode:
		return c.text(n.val)
	case nestedNode:
		path := qualify(nested, n.path.literal())
		return map[string]any{"nested": map[string]any{
			"path": path, "score_mode": "none", "query": c.node(n.child, path),
		}}
	}
	panic("kql: unknown node")
}

func (c *compiler) nodes(items []node, nested string) []any {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = c.node(it, nested)
	}
	return out
}

func boolShould(items []any) map[string]any {
	return map[string]any{"bool": map[string]any{"should": items, "minimum_should_match": 1}}
}

func qualify(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// field compiles field:value or a range, expanding a wildcard field name against the data view.
func (c *compiler) field(n fieldNode, nested string) map[string]any {
	name := qualify(nested, n.field.literal())
	if !n.field.hasWildcard() {
		return c.clause(name, n, nested)
	}
	re := globRegexp(name)
	var matches []any
	for _, f := range c.fields {
		if f.Searchable && re.MatchString(f.Name) {
			matches = append(matches, c.clause(f.Name, n, nested))
		}
	}
	if len(matches) == 0 {
		return map[string]any{"match_none": map[string]any{}}
	}
	return boolShould(matches)
}

func globRegexp(glob string) *regexp.Regexp {
	parts := strings.Split(glob, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile("^" + strings.Join(parts, ".*") + "$")
}

// clause builds one field's query and wraps it in a nested query when the field lives under a
// nested path the surrounding query is not already inside.
func (c *compiler) clause(name string, n fieldNode, nested string) map[string]any {
	info := c.byName[name]
	var q map[string]any
	switch {
	case n.op != ":":
		rng := map[string]any{rangeOps[n.op]: n.val.literal()}
		if info.Type == "date" && c.tz != "" {
			rng["time_zone"] = c.tz
		}
		q = map[string]any{"range": map[string]any{name: rng}}
	case n.val.onlyWildcard():
		q = map[string]any{"exists": map[string]any{"field": name}}
	case n.val.quoted:
		q = map[string]any{"match_phrase": map[string]any{name: n.val.literal()}}
	case n.val.hasWildcard():
		q = map[string]any{"query_string": map[string]any{
			"fields": []string{name}, "query": queryStringText(n.val), "analyze_wildcard": true,
		}}
	default:
		q = map[string]any{"match": map[string]any{name: map[string]any{"query": n.val.literal(), "lenient": true}}}
	}
	if info.Nested != "" && info.Nested != nested {
		return map[string]any{"nested": map[string]any{"path": info.Nested, "score_mode": "none", "query": q}}
	}
	return q
}

var rangeOps = map[string]string{"<": "lt", "<=": "lte", ">": "gt", ">=": "gte"}

// text compiles a value with no field over every field.
func (c *compiler) text(v value) map[string]any {
	switch {
	case v.hasWildcard():
		return map[string]any{"query_string": map[string]any{
			"query": queryStringText(v), "analyze_wildcard": true, "lenient": true,
		}}
	case v.quoted:
		return map[string]any{"multi_match": map[string]any{"type": "phrase", "query": v.literal(), "lenient": true}}
	}
	return map[string]any{"multi_match": map[string]any{"type": "best_fields", "query": v.literal(), "lenient": true}}
}

// queryStringReserved are the characters query_string treats as syntax.
const queryStringReserved = `+-=&|><!(){}[]^"~*?:\/ `

// queryStringText renders a value for query_string: literal text escaped, wildcards kept.
func queryStringText(v value) string {
	var b strings.Builder
	for _, p := range v.parts {
		if p.star {
			b.WriteByte('*')
			continue
		}
		for _, r := range p.text {
			if strings.ContainsRune(queryStringReserved, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LuceneQuery wraps a Lucene query string for Elasticsearch. A blank query is match_all.
func LuceneQuery(query, timeZone string) map[string]any {
	if strings.TrimSpace(query) == "" {
		return map[string]any{"match_all": map[string]any{}}
	}
	qs := map[string]any{"query": query, "analyze_wildcard": true}
	if timeZone != "" {
		qs["time_zone"] = timeZone
	}
	return map[string]any{"query_string": qs}
}
