package configsync

import (
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/credshape"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// credentialProblem scans a shared YAML or JSON text for a literal credential: a value shaped like
// one anywhere, or a value under an env or headers key whose name marks one. Text holding a
// reference (${...}) passes. The message names the line and field, never the value; "" means clean.
func credentialProblem(text []byte, md bool) string {
	if md {
		return markdownCredential(string(text))
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return ""
	}
	if line, path, hit := scanNode(&doc, "", nil); hit {
		return literalCredentialMessage(line, path)
	}
	return ""
}

// entryCredentialProblem is credentialProblem for one entry value, which has no line.
func entryCredentialProblem(v any) string {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return ""
	}
	if _, path, hit := scanNode(&n, "", nil); hit {
		return literalCredentialMessage(0, path)
	}
	return ""
}

func literalCredentialMessage(line int, path []string) string {
	at := "/" + strings.Join(path, "/")
	switch {
	case len(path) == 0:
		at = "line " + strconv.Itoa(line)
	case line > 0:
		at += " (line " + strconv.Itoa(line) + ")"
	}
	return i18n.T("backend.space.configSync.gate.literalCredential.body", map[string]string{"at": at})
}

func scanNode(n *yaml.Node, parentKey string, path []string) (line int, at []string, hit bool) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			if l, p, h := scanNode(c, "", path); h {
				return l, p, true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			child := append(append([]string(nil), path...), k)
			if (parentKey == "env" || parentKey == "headers") && isLiteralString(v) && credshape.Name(k) {
				return v.Line, child, true
			}
			if l, p, h := scanNode(v, k, child); h {
				return l, p, true
			}
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			if l, p, h := scanNode(c, "", append(append([]string(nil), path...), strconv.Itoa(i))); h {
				return l, p, true
			}
		}
	case yaml.ScalarNode:
		if isLiteralString(n) && credshape.Value(n.Value) {
			return n.Line, path, true
		}
	case yaml.AliasNode:
	}
	return 0, nil, false
}

// isLiteralString reports a non-empty string scalar with no reference in it.
func isLiteralString(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!str" && n.Value != "" && !strings.Contains(n.Value, "${")
}

// markdownCredential checks the words of a markdown file. Pure hex words pass: commit ids are common.
func markdownCredential(text string) string {
	for i, line := range strings.Split(text, "\n") {
		for _, w := range strings.Fields(line) {
			w = strings.Trim(w, "`\"'()[]{}<>,;:.*")
			if w == "" || strings.Contains(w, "${") || isHex(w) {
				continue
			}
			if credshape.Value(w) {
				return literalCredentialMessage(i+1, nil)
			}
		}
	}
	return ""
}

func isHex(w string) bool {
	for _, r := range w {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
