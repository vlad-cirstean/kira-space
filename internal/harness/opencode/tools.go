package opencode

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// ruleEntry is one permission pattern and its action.
type ruleEntry struct{ pattern, action string }

// ruleSet is one permission key's rules. OpenCode evaluates a pattern map in order and the last
// match wins, so insertion order is part of the meaning and encoding keeps it.
type ruleSet struct{ entries []ruleEntry }

// add appends pattern, moving an earlier entry of the same pattern to the end.
func (r *ruleSet) add(pattern, action string) {
	r.entries = slices.DeleteFunc(r.entries, func(e ruleEntry) bool { return e.pattern == pattern })
	r.entries = append(r.entries, ruleEntry{pattern, action})
}

func (r *ruleSet) hasStar() bool {
	return slices.ContainsFunc(r.entries, func(e ruleEntry) bool { return e.pattern == "*" })
}

func (r *ruleSet) hasAction(action string) bool {
	return slices.ContainsFunc(r.entries, func(e ruleEntry) bool { return e.action == action })
}

// MarshalJSON writes a lone "*" as a plain action string, else an ordered object.
func (r ruleSet) MarshalJSON() ([]byte, error) {
	if len(r.entries) == 1 && r.entries[0].pattern == "*" {
		return json.Marshal(r.entries[0].action)
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range r.entries {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(e.pattern)
		v, _ := json.Marshal(e.action)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func unsupported(entry string) error {
	return &harness.Error{Kind: harness.ErrUnsupported, Msg: "OpenCode can't run with the tool entry " + entry + "."}
}

// toolIDs translates one Kira (Claude vocabulary) tool name or MCP tool name into OpenCode keys.
func toolIDs(name string) ([]string, bool) {
	if ids, ok := builtinTools[name]; ok {
		return ids, true
	}
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok && rest != "" {
		server, tool, _ := strings.Cut(rest, "__")
		if server == "" {
			return nil, false
		}
		return []string{mcpToolKey(server, tool)}, true
	}
	return nil, false
}

// translateRules turns Kira tool entries into OpenCode permission rules with action. A Bash(...)
// entry becomes a bash pattern, an Edit(...) entry an edit pattern; every other entry with
// arguments, and every unknown name, is an error naming it (fail closed).
func translateRules(entries []string, action string) (map[string]*ruleSet, error) {
	out := map[string]*ruleSet{}
	add := func(key, pattern string) {
		r := out[key]
		if r == nil {
			r = &ruleSet{}
			out[key] = r
		}
		r.add(pattern, action)
	}
	for _, e := range entries {
		name, arg, hasArg := strings.Cut(e, "(")
		if !hasArg {
			ids, ok := toolIDs(e)
			if !ok {
				return nil, unsupported(e)
			}
			for _, id := range ids {
				add(id, "*")
			}
			continue
		}
		arg, closed := strings.CutSuffix(arg, ")")
		if !closed || arg == "" {
			return nil, unsupported(e)
		}
		switch name {
		case "Bash":
			add("bash", bashPattern(arg))
		case "Edit":
			add("edit", editPattern(arg))
		default:
			return nil, unsupported(e)
		}
	}
	return out, nil
}

// bashPattern is the glob of a Bash(...) argument: Claude's `prefix:*` becomes `prefix*`.
func bashPattern(arg string) string {
	if p, ok := strings.CutSuffix(arg, ":*"); ok {
		return p + "*"
	}
	return arg
}

// editPattern turns Claude's `//abs/**` into an absolute glob; a single leading slash is relative
// to the project root and loses it.
func editPattern(arg string) string {
	if strings.HasPrefix(arg, "//") {
		return arg[1:]
	}
	return strings.TrimPrefix(arg, "/")
}

// permissionDoc renders spec's tool rules as the OPENCODE_CONFIG_CONTENT document: `tools` off for
// built-ins outside Tools, `permission` allow for AllowedTools, deny for DisallowedTools (deny
// wins), every other built-in denied when Isolated, and nothing left to ask. users are the user's
// own MCP servers; with Isolated and UserMCP, those not named stay disabled.
func permissionDoc(s harness.HeadlessSpec, users []harness.UserServer) (map[string]any, error) {
	perm := map[string]*ruleSet{}
	merge := func(rules map[string]*ruleSet) {
		for k, r := range rules {
			dst := perm[k]
			if dst == nil {
				dst = &ruleSet{}
				perm[k] = dst
			}
			for _, e := range r.entries {
				dst.add(e.pattern, e.action)
			}
		}
	}
	allow, err := translateRules(s.AllowedTools, "allow")
	if err != nil {
		return nil, err
	}
	deny, err := translateRules(s.DisallowedTools, "deny")
	if err != nil {
		return nil, err
	}
	merge(allow)
	merge(deny)

	tools := map[string]bool{}
	if s.Tools != nil {
		keep := map[string]bool{}
		for _, t := range s.Tools {
			ids, ok := toolIDs(t)
			if !ok {
				return nil, unsupported(t)
			}
			for _, id := range ids {
				keep[id] = true
			}
		}
		for _, id := range builtinIDs {
			if !keep[id] {
				tools[id] = false
			}
		}
	}
	if s.Isolated {
		for _, id := range builtinIDs {
			if perm[id] == nil && !hasOff(tools, id) {
				perm[id] = &ruleSet{entries: []ruleEntry{{"*", "deny"}}}
			}
		}
		// A pattern-only allow list leaves every other command at the default, which allows.
		for _, r := range perm {
			if !r.hasStar() && r.hasAction("allow") {
				r.entries = append([]ruleEntry{{"*", "deny"}}, r.entries...)
			}
		}
	}
	// Nothing may prompt: a headless run has nobody to answer.
	perm["doom_loop"] = &ruleSet{entries: []ruleEntry{{"*", "deny"}}}
	dirs := &ruleSet{entries: []ruleEntry{{"*", "deny"}}}
	for _, d := range s.AddDirs {
		dirs.add(strings.TrimRight(d, "/")+"/**", "allow")
	}
	perm["external_directory"] = dirs

	doc := map[string]any{"permission": perm}
	if len(tools) > 0 {
		doc["tools"] = tools
	}
	if s.Isolated && len(s.UserMCP) > 0 {
		off := map[string]any{}
		for _, u := range users {
			if !slices.Contains(s.UserMCP, u.Name) && !slices.Contains(s.GrantMCP, u.Name) {
				off[u.Name] = map[string]any{"enabled": false}
			}
		}
		if len(off) > 0 {
			doc["mcp"] = off
		}
	}
	return doc, nil
}

func hasOff(tools map[string]bool, id string) bool {
	v, ok := tools[id]
	return ok && !v
}

// ToolArgs renders a tool policy in Kira's vocabulary (Claude's names), like the Claude provider;
// RunHeadless translates the result through permissionDoc. Twin of claude's ToolArgs: a shared
// helper across provider packages is not allowed.
func (p *Provider) ToolArgs(pol harness.ToolPolicy) (tools, allowed []string) {
	tools = append([]string{}, pol.Builtins...)
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			allowed = append(allowed, t)
		}
	}
	for _, t := range pol.Builtins {
		if t == "Bash" && len(pol.BashPatterns) > 0 {
			for _, bp := range pol.BashPatterns {
				add("Bash(" + bp + ")")
			}
			continue
		}
		add(t)
	}
	for _, c := range pol.MCP {
		if c.AllTools {
			add("mcp__" + c.Server)
			continue
		}
		for _, t := range c.Tools {
			add(p.MCPToolName(harness.MCPTool{Server: c.Server, Tool: t}))
		}
	}
	for _, t := range pol.Extra {
		add(t)
	}
	if allowed == nil {
		allowed = []string{}
	}
	return tools, allowed
}
