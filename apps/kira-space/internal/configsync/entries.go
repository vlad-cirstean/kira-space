package configsync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// entrySet is a combined file split into entries: id to canonical JSON (compared) and id to the
// decoded value (written).
type entrySet struct {
	canon map[string]string
	raw   map[string]any
}

func newEntrySet() entrySet { return entrySet{canon: map[string]string{}, raw: map[string]any{}} }

func (e entrySet) put(id string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.canon[id], e.raw[id] = string(b), v
	return nil
}

var errNotMap = errors.New("settings must be a mapping")

// agentSettingsEntries splits settings.yaml: env, MCP servers and hooks by name, permission lists by
// rule, any other top-level key (a provider section) whole.
func agentSettingsEntries(text []byte) (entrySet, error) {
	set := newEntrySet()
	if len(bytes.TrimSpace(text)) == 0 {
		return set, nil
	}
	var doc any
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return set, err
	}
	if doc == nil {
		return set, nil
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return set, errNotMap
	}
	for k, v := range top {
		if err := splitTop(set, k, v); err != nil {
			return set, err
		}
	}
	return set, nil
}

// splitTop adds the entries of one top-level settings key.
func splitTop(set entrySet, k string, v any) error {
	sub, isMap := v.(map[string]any)
	switch {
	case isMap && (k == "env" || k == "mcp_servers" || k == "hooks"):
		for name, val := range sub {
			if err := set.put(k+"/"+name, val); err != nil {
				return err
			}
		}
		return nil
	case isMap && k == "permissions":
		for list, val := range sub {
			if err := splitPermissions(set, list, val); err != nil {
				return err
			}
		}
		return nil
	}
	return set.put(k, v)
}

func splitPermissions(set entrySet, list string, val any) error {
	items, isList := val.([]any)
	if !isList {
		return set.put("permissions/"+list, val)
	}
	for _, it := range items {
		if err := set.put("permissions/"+list+"/"+fmt.Sprint(it), it); err != nil {
			return err
		}
	}
	return nil
}

// promptEntries splits llm-prompts.json by prompt id; the value is the override text.
func promptEntries(text []byte) (entrySet, error) {
	set := newEntrySet()
	texts, err := llmprompts.DecodeOverrides(text)
	if err != nil {
		return set, err
	}
	for id, t := range texts {
		if err := set.put(id, t); err != nil {
			return set, err
		}
	}
	return set, nil
}

// entryDecision is one entry that a pull changes or that conflicts.
type entryDecision struct {
	id      string
	verdict verdict
	ours    side
	theirs  side
}

// planEntries decides every entry id and lists those with a verdict other than none, sorted by id.
// Deactivated ids are absent from theirs before the decision.
func planEntries(ours, base, theirs map[string]string, deactivated map[string]bool) []entryDecision {
	ids := map[string]bool{}
	for _, m := range []map[string]string{ours, base, theirs} {
		for id := range m {
			ids[id] = true
		}
	}
	var out []entryDecision
	for id := range ids {
		o, b, t := sideOf(ours, id), sideOf(base, id), effective(sideOf(theirs, id), deactivated[id])
		if v := decide(o, b, t); v != verdictNone {
			out = append(out, entryDecision{id: id, verdict: v, ours: o, theirs: t})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func sideOf(m map[string]string, id string) side {
	v, ok := m[id]
	return side{val: v, ok: ok}
}

// applyAgentEntries returns settings.yaml text with the take ids set to their shared value (or
// removed when shared lacks them). Local comments and key order outside the edited values stay.
func applyAgentEntries(ours []byte, theirs entrySet, take []string) ([]byte, error) {
	slices.Sort(take)
	lists := map[string][]string{}
	listTouched := map[string]bool{}
	type edit struct {
		path  []string
		value any
		unset bool
	}
	var edits []edit
	for _, id := range take {
		parts := strings.SplitN(id, "/", 3)
		if len(parts) == 3 && parts[0] == "permissions" {
			list := parts[1]
			if _, ok := lists[list]; !ok {
				lists[list] = permissionItems(ours, list)
			}
			listTouched[list] = true
			rule := parts[2]
			if _, ok := theirs.raw[id]; ok {
				if !slices.Contains(lists[list], rule) {
					lists[list] = append(lists[list], rule)
				}
			} else {
				lists[list] = slices.DeleteFunc(lists[list], func(r string) bool { return r == rule })
			}
			continue
		}
		path := strings.SplitN(id, "/", 2)
		v, ok := theirs.raw[id]
		edits = append(edits, edit{path: path, value: v, unset: !ok})
	}
	for _, list := range []string{"allow", "deny"} {
		if listTouched[list] {
			edits = append(edits, edit{path: []string{"permissions", list}, value: lists[list], unset: len(lists[list]) == 0})
		}
	}
	if len(bytes.TrimSpace(ours)) == 0 {
		paths, values, unsets := make([][]string, len(edits)), make([]any, len(edits)), make([]bool, len(edits))
		for i, e := range edits {
			paths[i], values[i], unsets[i] = e.path, e.value, e.unset
		}
		return renderNew(paths, values, unsets)
	}
	sets := make([]configfs.NodeSet, len(edits))
	for i, e := range edits {
		sets[i] = configfs.NodeSet{Path: e.path, Value: e.value, Unset: e.unset}
	}
	return configfs.EditYAML(ours, sets)
}

// permissionItems is the local rules of one permission list, in file order.
func permissionItems(ours []byte, list string) []string {
	var doc struct {
		Permissions map[string][]any `yaml:"permissions"`
	}
	if yaml.Unmarshal(ours, &doc) != nil {
		return nil
	}
	var out []string
	for _, it := range doc.Permissions[list] {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

// renderNew writes a settings document that has no local file yet.
func renderNew(paths [][]string, values []any, unsets []bool) ([]byte, error) {
	root := map[string]any{}
	for i, p := range paths {
		if unsets[i] {
			continue
		}
		cur := root
		for _, k := range p[:len(p)-1] {
			next, ok := cur[k].(map[string]any)
			if !ok {
				next = map[string]any{}
				cur[k] = next
			}
			cur = next
		}
		cur[p[len(p)-1]] = values[i]
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return out.Bytes(), enc.Close()
}

// applyPromptEntries returns the overrides file with the take ids set to their shared text or removed.
func applyPromptEntries(ours []byte, oursSet, theirs entrySet, take []string, now time.Time) ([]byte, error) {
	final := map[string]string{}
	for id, v := range oursSet.raw {
		final[id], _ = v.(string)
	}
	for _, id := range take {
		if v, ok := theirs.raw[id]; ok {
			final[id], _ = v.(string)
		} else {
			delete(final, id)
		}
	}
	return llmprompts.EncodeOverrides(ours, final, now)
}

// encodeBaseEntries stores an entry set as the merge base of a combined file.
func encodeBaseEntries(canon map[string]string) ([]byte, error) {
	return json.MarshalIndent(canon, "", "  ")
}

func decodeBaseEntries(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(data) == 0 {
		return out, nil
	}
	return out, json.Unmarshal(data, &out)
}
