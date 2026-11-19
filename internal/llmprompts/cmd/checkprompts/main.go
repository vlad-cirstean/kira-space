// Command checkprompts fails when an LLM prompt lives outside internal/llmprompts: a jsonschema
// tag, an MCP description or system prompt that is not an llmprompts call, or a copy of a
// registry default in Go, TS or Vue code. It also fails a default that holds filler words and an
// entry over its line in internal/llmprompts/testdata/budget.tsv. Run from the repo root.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const (
	longDefault  = 48
	shortDefault = 24
)

// literalFields lists the composite literals whose text fields must come from the registry.
var literalFields = map[string][]string{
	"mcp.Tool":           {"Description"},
	"mcp.Prompt":         {"Description"},
	"mcp.PromptArgument": {"Description"},
	"mcp.ServerOptions":  {"Instructions"},
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "bindings": true, ".codegraph": true,
	"testdata": true, "docs": true, "tests": true, "testing": true, "test-results": true, ".claude": true,
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	needles := phrases()
	var problems []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[d.Name()] || rel == "internal/llmprompts" || rel == "internal/flowtest" {
				return filepath.SkipDir
			}
			return nil
		}
		if skipFile(d.Name()) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".go") {
			problems = append(problems, goProblems(rel, raw)...)
		}
		problems = append(problems, copies(rel, string(raw), needles)...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkprompts:", err)
		os.Exit(2)
	}
	problems = append(problems, styleProblems()...)
	problems = append(problems, budgetProblems(root)...)
	if len(problems) > 0 {
		sort.Strings(problems)
		fmt.Fprintln(os.Stderr, strings.Join(problems, "\n"))
		fmt.Fprintf(os.Stderr, "checkprompts: %d problem(s): move prompts into internal/llmprompts, keep defaults terse, keep budget.tsv in step\n", len(problems))
		os.Exit(1)
	}
	fmt.Println("checkprompts: every prompt lives in internal/llmprompts, terse and within budget.")
}

func skipFile(name string) bool {
	switch filepath.Ext(name) {
	case ".go":
		return strings.HasSuffix(name, "_test.go")
	case ".ts":
		return strings.HasSuffix(name, ".spec.ts") || strings.HasSuffix(name, ".test.ts") || strings.HasSuffix(name, ".d.ts")
	case ".vue":
		return false
	}
	return true
}

// phrases returns the text of each registry default that a copy would carry: its first 48
// characters, or the whole default when it is 24 to 47 characters long.
func phrases() map[string]string {
	out := map[string]string{}
	for _, in := range llmprompts.All() {
		d := in.Default
		switch {
		case len(d) >= longDefault:
			out[string(in.ID)] = d[:longDefault]
		case len(d) >= shortDefault:
			out[string(in.ID)] = d
		}
	}
	return out
}

func copies(rel, text string, needles map[string]string) []string {
	var out []string
	for id, n := range needles {
		quoted := strings.Trim(strconv.Quote(n), `"`)
		if strings.Contains(text, n) || strings.Contains(text, quoted) {
			out = append(out, fmt.Sprintf("%s: copies the default of %s", rel, id))
		}
	}
	return out
}

func goProblems(rel string, src []byte) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", rel, err)}
	}
	var out []string
	at := func(p token.Pos, msg string) {
		out = append(out, fmt.Sprintf("%s:%d: %s", rel, fset.Position(p).Line, msg))
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			if x.Tag != nil && strings.Contains(x.Tag.Value, `jsonschema:"`) {
				at(x.Tag.Pos(), "jsonschema tag: describe the argument in the registry (llmprompts.InputSchema)")
			}
		case *ast.CompositeLit:
			fields := literalFields[typeName(x.Type)]
			for _, e := range x.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || !contains(fields, key.Name) {
					continue
				}
				if !usesRegistry(kv.Value) {
					at(kv.Pos(), fmt.Sprintf("%s.%s is not an llmprompts call", typeName(x.Type), key.Name))
				}
			}
		}
		return true
	})
	return out
}

func typeName(e ast.Expr) string {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			return id.Name + "." + sel.Sel.Name
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// usesRegistry reports whether the expression calls into the llmprompts package.
func usesRegistry(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "llmprompts" {
			found = true
		}
		return !found
	})
	return found
}
