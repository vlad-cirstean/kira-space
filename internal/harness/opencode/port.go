package opencode

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Pure helpers for settings import and export (P454). They open no file: internal/agentport reads and
// writes the user's config.

// EnvRef is the {env:NAME} reference OpenCode substitutes in its user config (Q-P454-OC3, verified).
func EnvRef(name string) string { return "{env:" + name + "}" }

var envRefRe = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// RefsToKira reads an OpenCode config string as settings.yaml text: {env:NAME} becomes ${NAME}, a
// literal ${ is escaped as $${. ok is false for a {file:...} reference, which Kira cannot express.
func RefsToKira(text string) (string, bool) {
	if strings.Contains(text, "{file:") {
		return "", false
	}
	text = strings.ReplaceAll(text, "${", "$${")
	return envRefRe.ReplaceAllString(text, "$${$1}"), true
}

// RefsFromKira writes settings.yaml text as an OpenCode config string: ${NAME} becomes {env:NAME} and
// ${secret:x} becomes {env:<secretEnv(x)>}. ok is false for text OpenCode would misread: a $${ escape
// (it has none) or a literal {env: or {file: it would substitute.
func RefsFromKira(text string, secretEnv func(string) string) (string, bool) {
	if strings.Contains(text, "$${") {
		return "", false
	}
	refs, p := configfs.Refs(text)
	if p != nil {
		return "", false
	}
	var b strings.Builder
	last := 0
	for _, r := range refs {
		if lit := text[last:r.Start]; strings.Contains(lit, "{env:") || strings.Contains(lit, "{file:") {
			return "", false
		} else {
			b.WriteString(lit)
		}
		name := r.Name
		if r.Kind == configfs.RefSecret {
			name = secretEnv(r.Name)
		}
		b.WriteString(EnvRef(name))
		last = r.End
	}
	rest := text[last:]
	if strings.Contains(rest, "{env:") || strings.Contains(rest, "{file:") {
		return "", false
	}
	b.WriteString(rest)
	return b.String(), true
}

// MCPDoc is the `mcp` object of servers in OpenCode's format.
func MCPDoc(servers []harness.MCPServer) map[string]any { return mcpEntries(servers) }

// PermissionDoc renders Kira allow and deny entries as OpenCode `permission` keys, each an ordered
// rule set (deny after allow: the last match wins). An allow entry OpenCode cannot take is a note; a
// deny entry it cannot take is an error (fail closed), as at launch.
func PermissionDoc(allow, deny []string) (doc map[string]json.RawMessage, notes []string, err error) {
	perm := map[string]*ruleSet{}
	for _, e := range allow {
		rules, err := translateRules([]string{e}, "allow")
		if err != nil {
			notes = append(notes, i18n.T("backend.core.agentSettings.opencode.allowNotApplied.body", map[string]string{"entry": e}))
			continue
		}
		mergeRules(perm, rules)
	}
	for _, e := range deny {
		rules, err := translateRules([]string{e}, "deny")
		if err != nil {
			return nil, notes, err
		}
		mergeRules(perm, rules)
	}
	doc = make(map[string]json.RawMessage, len(perm))
	for k, r := range perm {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, notes, err
		}
		doc[k] = b
	}
	return doc, notes, nil
}

// MergePermission appends add's rules after existing's, same key. A pattern the user already has keeps
// its action when add would only widen it (allow over deny).
func MergePermission(existing, add json.RawMessage) (json.RawMessage, error) {
	var cur, extra ruleSet
	if err := json.Unmarshal(existing, &cur); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(add, &extra); err != nil {
		return nil, err
	}
	for _, e := range extra.entries {
		if i := slices.IndexFunc(cur.entries, func(c ruleEntry) bool { return c.pattern == e.pattern }); i >= 0 && cur.entries[i].action == "deny" && e.action == "allow" {
			continue
		}
		cur.add(e.pattern, e.action)
	}
	return json.Marshal(cur)
}

// kiraTools is the inverse of builtinTools for the keys with a unique Kira name.
var kiraTools = map[string]string{
	"read": "Read", "glob": "Glob", "grep": "Grep", "edit": "Edit", "write": "Write", "bash": "Bash",
	"webfetch": "WebFetch", "websearch": "WebSearch", "todowrite": "TodoWrite", "task": "Task", "skill": "Skill",
}

// KiraPermissions reads an OpenCode `permission` value as Kira allow and deny entries (Claude's tool
// vocabulary). skipped names every key or pattern with no Kira form (ask rules, unknown keys, patterns
// beyond bash and edit); mixed names every key whose rules mix allow and deny, which Kira orders as
// deny wins.
func KiraPermissions(raw json.RawMessage) (allow, deny, skipped, mixed []string) {
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return nil, nil, []string{"permission"}, nil
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	slices.Sort(names)
	for _, key := range names {
		var rs ruleSet
		if json.Unmarshal(keys[key], &rs) != nil {
			skipped = append(skipped, "permission."+key)
			continue
		}
		var hasAllow, hasDeny bool
		for _, e := range rs.entries {
			name, ok := kiraEntry(key, e.pattern)
			switch {
			case !ok || (e.action != "allow" && e.action != "deny"):
				skipped = append(skipped, "permission."+key+"."+e.pattern)
			case e.action == "allow":
				allow, hasAllow = append(allow, name), true
			default:
				deny, hasDeny = append(deny, name), true
			}
		}
		if hasAllow && hasDeny {
			mixed = append(mixed, key)
		}
	}
	return allow, deny, skipped, mixed
}

// kiraEntry is the Kira tool entry for one permission key and pattern.
func kiraEntry(key, pattern string) (string, bool) {
	if server, ok := strings.CutSuffix(key, "_*"); ok && pattern == "*" && server != "" {
		return "mcp__" + server, true
	}
	name, ok := kiraTools[key]
	if !ok {
		return "", false
	}
	if pattern == "*" {
		return name, true
	}
	switch key {
	case "bash":
		if p, ok := strings.CutSuffix(pattern, "*"); ok {
			return "Bash(" + p + ":*)", true
		}
		return "Bash(" + pattern + ")", true
	case "edit":
		return "Edit(/" + pattern + ")", true
	}
	return "", false
}
