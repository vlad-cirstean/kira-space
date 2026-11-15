package configfs

import (
	"errors"
	"strconv"
	"strings"
)

// Skip lets a YAMLSchemaSkip decode drop bad array elements instead of failing the file.
type Skip struct {
	// Element is the pointer of the droppable element path falls in; false keeps the problem whole-file.
	Element func(path string) (string, bool)
	// StandIn is a valid value put in a dropped element's place, so later elements keep their
	// pointers and lines. doc is the whole parsed document; n counts drops from 0.
	StandIn func(doc any, n int) any
	// Max is the most elements dropped; one more problem fails the file.
	Max int
}

// Skipped is one dropped element: its pointer, its parsed value and the first problem found in it.
type Skipped struct {
	Pointer string
	Value   any
	Problem Problem
}

var errPointer = errors.New("configfs: pointer not resolvable")

func splitPointer(ptr string) []string {
	if ptr == "" {
		return nil
	}
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		parts[i] = strings.ReplaceAll(p, "~0", "~")
	}
	return parts
}

func child(node any, tok string) (any, bool) {
	switch n := node.(type) {
	case map[string]any:
		v, ok := n[tok]
		return v, ok
	case []any:
		i, err := strconv.Atoi(tok)
		if err != nil || i < 0 || i >= len(n) {
			return nil, false
		}
		return n[i], true
	}
	return nil, false
}

func valueAt(doc any, ptr string) (any, bool) {
	cur := doc
	for _, t := range splitPointer(ptr) {
		var ok bool
		if cur, ok = child(cur, t); !ok {
			return nil, false
		}
	}
	return cur, true
}

// setAt replaces the value at ptr and returns the (possibly new) root.
func setAt(doc any, ptr string, v any) (any, error) {
	toks := splitPointer(ptr)
	if len(toks) == 0 {
		return v, nil
	}
	parent, ok := valueAt(doc, pointerOf(toks[:len(toks)-1]))
	if !ok {
		return doc, errPointer
	}
	last := toks[len(toks)-1]
	switch p := parent.(type) {
	case map[string]any:
		if _, ok := p[last]; !ok {
			return doc, errPointer
		}
		p[last] = v
	case []any:
		i, err := strconv.Atoi(last)
		if err != nil || i < 0 || i >= len(p) {
			return doc, errPointer
		}
		p[i] = v
	default:
		return doc, errPointer
	}
	return doc, nil
}

// under reports whether ptr equals base or sits below it.
func under(ptr, base string) bool {
	return ptr == base || strings.HasPrefix(ptr, base+"/")
}
