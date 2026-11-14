package apifiles

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apiaccess"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/palette"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed schema/collection.schema.json
var collectionSchema []byte

//go:embed schema/environment.schema.json
var environmentSchema []byte

const (
	ruleItemKind    = "An item has exactly one of items (a folder), http and grpc."
	ruleItemUnique  = "Item ids are unique in the file."
	ruleItemSkip    = "An item that breaks a rule is skipped: the rest of the file loads and the Api panel lists the item with its line. Of two items with the same id, the first one loads."
	ruleVarUnique   = "Variable names are unique in the file."
	ruleVarKind     = "A variable has a value or a secret, not both."
	ruleAgent       = "agentHosts is set only on a secret variable with agent: true, and that secret lists at least one allowed host. Each host is host, host:port or *.domain, with an optional http:// prefix."
	ruleStemCollect = "The file name without .collection.yaml is the collection id: lowercase letters, digits and -, starting with a letter or digit, up to 64 characters. A file with another name is listed but not used."
	ruleStemEnv     = "The file name without .environment.yaml is the environment id: lowercase letters, digits and -, starting with a letter or digit, up to 64 characters. A file with another name is listed but not used."
)

func problem(path, key string, vars map[string]string) *configfs.Problem {
	return &configfs.Problem{Path: path, Message: i18n.T(key, vars)}
}

func itemKindRule() configfs.Rule[CollectionFile] {
	return configfs.Rule[CollectionFile]{Doc: ruleItemKind, Check: func(f CollectionFile, _ configfs.Locator) *configfs.Problem {
		var walk func(items []Item, path string) *configfs.Problem
		walk = func(items []Item, path string) *configfs.Problem {
			for i, it := range items {
				at := path + "/" + strconv.Itoa(i)
				n := 0
				if it.Items != nil {
					n++
				}
				if it.HTTP != nil {
					n++
				}
				if it.GRPC != nil {
					n++
				}
				if n != 1 {
					return problem(at, "backend.studio.apiFiles.itemKind.body", nil)
				}
				if it.Items != nil {
					if p := walk(*it.Items, at+"/items"); p != nil {
						return p
					}
				}
			}
			return nil
		}
		return walk(f.Items, "/items")
	}}
}

func itemUniqueRule() configfs.Rule[CollectionFile] {
	return configfs.Rule[CollectionFile]{Doc: ruleItemUnique, Check: func(f CollectionFile, _ configfs.Locator) *configfs.Problem {
		seen := map[string]bool{}
		var walk func(items []Item, path string) *configfs.Problem
		walk = func(items []Item, path string) *configfs.Problem {
			for i, it := range items {
				at := path + "/" + strconv.Itoa(i)
				if seen[it.ID] {
					return problem(at+"/id", "backend.studio.apiFiles.itemDuplicate.body", map[string]string{"id": it.ID})
				}
				seen[it.ID] = true
				if it.Items != nil {
					if p := walk(*it.Items, at+"/items"); p != nil {
						return p
					}
				}
			}
			return nil
		}
		return walk(f.Items, "/items")
	}}
}

func varUniqueRule[T any](vars func(T) []Variable) configfs.Rule[T] {
	return configfs.Rule[T]{Doc: ruleVarUnique, Check: func(f T, _ configfs.Locator) *configfs.Problem {
		seen := map[string]bool{}
		for i, v := range vars(f) {
			if seen[v.Name] {
				return problem(fmt.Sprintf("/variables/%d/name", i), "backend.studio.apiFiles.variableDuplicate.body", map[string]string{"name": v.Name})
			}
			seen[v.Name] = true
		}
		return nil
	}}
}

func varKindRule[T any](vars func(T) []Variable) configfs.Rule[T] {
	return configfs.Rule[T]{Doc: ruleVarKind, Check: func(f T, _ configfs.Locator) *configfs.Problem {
		for i, v := range vars(f) {
			if v.Value != "" && v.Secret != "" {
				return problem(fmt.Sprintf("/variables/%d/secret", i), "backend.studio.apiFiles.variableBoth.body", map[string]string{"name": v.Name})
			}
		}
		return nil
	}}
}

// agentRule checks the agent keys of an environment's variables.
func agentRule() configfs.Rule[EnvironmentFile] {
	return configfs.Rule[EnvironmentFile]{Doc: ruleAgent, Check: func(f EnvironmentFile, _ configfs.Locator) *configfs.Problem {
		for i, v := range f.Variables {
			if p := agentProblem(i, v); p != nil {
				return p
			}
		}
		return nil
	}}
}

func agentProblem(i int, v Variable) *configfs.Problem {
	at := fmt.Sprintf("/variables/%d/agentHosts", i)
	vars := map[string]string{"name": v.Name}
	switch {
	case len(v.AgentHosts) > 0 && (!v.Agent || v.Secret == ""):
		return problem(at, "backend.studio.apiFiles.agentHostsMisplaced.body", vars)
	case v.Agent && v.Secret != "" && len(v.AgentHosts) == 0:
		return problem(fmt.Sprintf("/variables/%d/agent", i), "backend.studio.apiFiles.agentHostsRequired.body", vars)
	}
	for j, h := range v.AgentHosts {
		if _, err := apiaccess.ParseHostRule(h); err != nil {
			return problem(fmt.Sprintf("%s/%d", at, j), "backend.studio.apiFiles.agentHostInvalid.body", map[string]string{"name": v.Name, "host": h})
		}
	}
	return nil
}

func collectionVars(f CollectionFile) []Variable   { return f.Variables }
func environmentVars(f EnvironmentFile) []Variable { return f.Variables }

// withStem refuses a file whose name is not a valid id, before the content is read.
func withStem[T any](ext string, decode configfs.DecodeFunc[T]) configfs.DecodeFunc[T] {
	return func(file string, src []byte) (T, *configfs.Problem) {
		if _, ok := StemOf(file, ext); !ok {
			var zero T
			return zero, &configfs.Problem{Message: i18n.T("backend.studio.apiFiles.fileName.body", nil)}
		}
		return decode(file, src)
	}
}

// maxSkippedItems is the most items one file may skip before the whole file fails.
const maxSkippedItems = 100

var collectionElementRe = regexp.MustCompile(`^(?:/items/\d+)+`)

// collectionElement maps a problem path to the deepest item it falls in.
func collectionElement(path string) (string, bool) {
	m := collectionElementRe.FindString(path)
	return m, m != ""
}

// collectionStandIn is a valid empty folder whose id no item in doc uses.
func collectionStandIn(doc any, n int) any {
	used := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if id, ok := t["id"].(string); ok {
				used[id] = true
			}
			walk(t["items"])
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	if m, ok := doc.(map[string]any); ok {
		walk(m["items"])
	}
	free := "kira-skipped-" + strconv.Itoa(n)
	for used[free] {
		free += "x"
	}
	return map[string]any{"id": free, "name": "skipped", "items": []any{}}
}

// Decoders holds the two file decoders and their AGENTS.md models.
type decoders struct {
	collection     configfs.DecodeFunc[CollectionFile]
	collectionLoad configfs.DecodeFunc[CollectionFile]
	environment    configfs.DecodeFunc[EnvironmentFile]
	collectionDoc  configfs.DocModel
	environmentDoc configfs.DocModel
}

func newDecoders() (decoders, error) {
	var d decoders
	skipDec, cdoc, err := configfs.YAMLSchemaSkip[CollectionFile](collectionSchema,
		configfs.Skip{Element: collectionElement, StandIn: collectionStandIn, Max: maxSkippedItems},
		itemKindRule(), itemUniqueRule(), varUniqueRule(collectionVars), varKindRule(collectionVars))
	if err != nil {
		return d, err
	}
	load := func(file string, src []byte) (CollectionFile, *configfs.Problem) {
		f, skipped, prob := skipDec(file, src)
		if prob != nil {
			return CollectionFile{}, prob
		}
		return pruneSkipped(f, skipped), nil
	}
	strict := func(file string, src []byte) (CollectionFile, *configfs.Problem) {
		f, skipped, prob := skipDec(file, src)
		switch {
		case len(skipped) > 0:
			p := skipped[0].Problem
			return CollectionFile{}, &p
		case prob != nil:
			return CollectionFile{}, prob
		}
		return f, nil
	}
	envSchema, err := assembleEnvironmentSchema()
	if err != nil {
		return d, err
	}
	environment, edoc, err := configfs.YAMLSchema[EnvironmentFile](envSchema,
		varUniqueRule(environmentVars), varKindRule(environmentVars), agentRule())
	if err != nil {
		return d, err
	}
	cdoc.Rules = append(cdoc.Rules, ruleItemSkip, ruleStemCollect)
	edoc.Rules = append(edoc.Rules, ruleStemEnv)
	d.collection, d.collectionLoad = withStem(CollectionExt, strict), withStem(CollectionExt, load)
	d.environment = withStem(EnvironmentExt, environment)
	d.collectionDoc, d.environmentDoc = cdoc, edoc
	return d, nil
}

// assembleEnvironmentSchema writes the palette into the schema, so the schema and the code cannot drift.
func assembleEnvironmentSchema() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(environmentSchema, &root); err != nil {
		return nil, fmt.Errorf("apifiles: schema: %w", err)
	}
	names := palette.Names()
	enum := make([]any, len(names))
	for i, n := range names {
		enum[i] = n
	}
	root["properties"].(map[string]any)["color"].(map[string]any)["enum"] = enum
	return json.Marshal(root)
}

// pruneSkipped builds f.Skipped and removes the stand-ins the decode left at the skipped pointers.
func pruneSkipped(f CollectionFile, skipped []configfs.Skipped) CollectionFile {
	if len(skipped) == 0 {
		return f
	}
	rawID := map[string]string{}
	for _, s := range skipped {
		if m, ok := s.Value.(map[string]any); ok {
			rawID[s.Pointer], _ = m["id"].(string)
		}
	}
	for _, s := range skipped {
		toks := strings.Split(s.Pointer[1:], "/")
		parent := ""
		if len(toks) > 2 {
			pp := "/" + strings.Join(toks[:len(toks)-2], "/")
			if id, ok := rawID[pp]; ok {
				parent = id
			} else if it := itemAt(f.Items, toks[1:len(toks)-2]); it != nil {
				parent = it.ID
			}
		}
		item := SkippedItem{Pointer: s.Pointer, ParentKey: parent, Line: s.Problem.Line, Message: s.Problem.Message}
		item.Index, _ = strconv.Atoi(toks[len(toks)-1])
		if m, ok := s.Value.(map[string]any); ok {
			item.ID, _ = m["id"].(string)
			item.Name, _ = m["name"].(string)
		}
		item.Keys = subtreeIDs(s.Value)
		f.Skipped = append(f.Skipped, item)
	}
	sort.SliceStable(f.Skipped, func(i, j int) bool {
		a, b := f.Skipped[i], f.Skipped[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Pointer < b.Pointer
	})
	// Outermost pointers only, removed last-first so earlier indices stay valid.
	var drop []string
	for _, s := range skipped {
		nested := false
		for _, o := range skipped {
			if o.Pointer != s.Pointer && strings.HasPrefix(s.Pointer, o.Pointer+"/") {
				nested = true
				break
			}
		}
		if !nested {
			drop = append(drop, s.Pointer)
		}
	}
	sort.Slice(drop, func(i, j int) bool { return pointerAfter(drop[i], drop[j]) })
	for _, p := range drop {
		f.Items = removeAt(f.Items, strings.Split(p[1:], "/")[1:])
	}
	return f
}

// itemAt resolves tokens like [n "items" m] (the leading "items" already dropped) in a stand-in-holding tree.
func itemAt(items []Item, toks []string) *Item {
	var cur *Item
	for i := 0; i < len(toks); i += 2 {
		n, err := strconv.Atoi(toks[i])
		if err != nil || n < 0 || n >= len(items) {
			return nil
		}
		cur = &items[n]
		if i+2 < len(toks) {
			if cur.Items == nil {
				return nil
			}
			items = *cur.Items
		}
	}
	return cur
}

// removeAt deletes the item at toks (index, "items", index, ...) and returns the new slice.
func removeAt(items []Item, toks []string) []Item {
	n, err := strconv.Atoi(toks[0])
	if err != nil || n < 0 || n >= len(items) {
		return items
	}
	if len(toks) == 1 {
		return append(items[:n:n], items[n+1:]...)
	}
	if items[n].Items != nil {
		sub := removeAt(*items[n].Items, toks[2:])
		items[n].Items = &sub
	}
	return items
}

// pointerAfter orders pointers descending, comparing numeric tokens as numbers.
func pointerAfter(a, b string) bool {
	ta, tb := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(ta) && i < len(tb); i++ {
		if ta[i] == tb[i] {
			continue
		}
		na, ea := strconv.Atoi(ta[i])
		nb, eb := strconv.Atoi(tb[i])
		if ea == nil && eb == nil {
			return na > nb
		}
		return ta[i] > tb[i]
	}
	return len(ta) > len(tb)
}

// subtreeIDs lists every string id in an item's subtree, its own first.
func subtreeIDs(v any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if id, ok := m["id"].(string); ok {
			out = append(out, id)
		}
		if kids, ok := m["items"].([]any); ok {
			for _, k := range kids {
				walk(k)
			}
		}
	}
	walk(v)
	return out
}
