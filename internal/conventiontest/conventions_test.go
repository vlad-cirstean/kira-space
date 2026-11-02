// Package conventiontest guards Go-wide conventions (P266). A file a guard cannot apply to sits in
// permanentExemptions with its reason; an entry whose file no longer violates fails the test, so
// the list only shrinks.
package conventiontest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// permanentExemptions are `G<n> <path>` pairs that stay forever, each with its reason.
var permanentExemptions = map[string]string{
	"G4 apps/kira-space/internal/gitsession/incremental.go": "writes temp files into a per-call os.MkdirTemp dir that is deleted before return; nothing else reads them",
	"G5 internal/harness/claude/sandbox.go":                 "names ~/.claude.json as a sandbox write path; Kira never opens it",
	"G5 internal/harness/opencode/sandbox.go":               "names OpenCode's config and data dirs as sandbox paths; Kira never opens them",
}

// testToolingPackages hold throwaway test fixtures; G4 skips any file under a directory with one
// of these names (a torn write only fails the test run).
var testToolingPackages = map[string]bool{"flowharness": true, "flowtest": true, "ipcfixture": true}

func isTestTooling(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if testToolingPackages[seg] {
			return true
		}
	}
	return false
}

// ipcerrMsg maps an ipcerr constructor to its message argument index.
var ipcerrMsg = map[string]int{"New": 1, "BadRequest": 0, "NotFound": 0, "Internal": 0, "SecretStore": 0}

type finding struct {
	rule string
	path string
	line int
	msg  string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func isString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// isFormatted reports a fmt.Sprintf or fmt.Errorf call.
func isFormatted(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "fmt" && (sel.Sel.Name == "Sprintf" || sel.Sel.Name == "Errorf")
}

func literalMessage(e ast.Expr) bool { return isString(e) || isFormatted(e) }

func hasScopeKey(args []ast.Expr) bool {
	for _, a := range args {
		if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING && lit.Value == `"scope"` {
			return true
		}
	}
	return false
}

var slogLevels = map[string]bool{"Debug": true, "Info": true, "Warn": true, "Error": true}

func scan(t *testing.T, root, rel string) []finding {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	inAdapters := strings.HasSuffix(filepath.ToSlash(filepath.Dir(rel)), "internal/adapters")
	inHTTPClient := strings.HasSuffix(filepath.ToSlash(filepath.Dir(rel)), "internal/httpclient")
	dir := filepath.ToSlash(filepath.Dir(rel))
	inHostAdapters := strings.HasPrefix(dir, "apps/kira-space/internal/githost") || strings.HasPrefix(dir, "apps/kira-space/internal/issuetracker")
	inDocker := dir == "internal/docker"
	inClaudeOnly := dir == "internal/harness/claude" || strings.HasPrefix(dir, "internal/harness/claude/")
	inOpenCode := dir == "internal/harness/opencode" || strings.HasPrefix(dir, "internal/harness/opencode/")
	inClaude := inClaudeOnly || inOpenCode
	isCodes := filepath.ToSlash(rel) == "internal/ipcerr/codes.go"
	var out []finding
	add := func(rule string, pos token.Pos, msg string) {
		out = append(out, finding{rule, filepath.ToSlash(rel), fset.Position(pos).Line, msg})
	}
	// codes.go may only declare `Code = "E_..."` constants: a retired-code map or any other
	// code literal there is a legacy code and fails like one anywhere else.
	declared := map[token.Pos]bool{}
	if isCodes {
		ast.Inspect(f, func(n ast.Node) bool {
			if vs, ok := n.(*ast.ValueSpec); ok {
				if id, ok := vs.Type.(*ast.Ident); ok && id.Name == "Code" {
					for _, v := range vs.Values {
						declared[v.Pos()] = true
					}
				}
			}
			return true
		})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BasicLit:
			if x.Kind == token.STRING && isErrorCodeLiteral(x.Value) && !declared[x.Pos()] {
				add("G2", x.Pos(), "error code literal "+x.Value+"; use the ipcerr constant")
			}
			if x.Kind == token.STRING && inDocker {
				if v, err := strconv.Unquote(x.Value); err == nil && strings.Contains(v, "config.json") {
					add("G5", x.Pos(), "literal \"config.json\" (docker config can hold auths)"+g5Rule)
				}
			}
			if x.Kind == token.STRING && inOpenCode {
				if v, err := strconv.Unquote(x.Value); err == nil {
					if why := openCodeCredential(v); why != "" {
						add("G5", x.Pos(), why+g5Rule)
					}
				}
			}
			if x.Kind == token.STRING && inClaudeOnly {
				if v, err := strconv.Unquote(x.Value); err == nil {
					switch {
					case strings.Contains(v, ".claude.json"):
						add("G5", x.Pos(), "literal \".claude.json\" (holds MCP headers and env)"+g5Rule)
					case strings.Contains(v, `json:"headers"`):
						add("G5", x.Pos(), "struct tag json:\"headers\" (decodes stored MCP headers)"+g5Rule)
					}
				}
			}
			if x.Kind == token.STRING && inHostAdapters {
				if v, err := strconv.Unquote(x.Value); err == nil {
					if why := credentialLiteral(v); why != "" {
						add("G5", x.Pos(), why+g5Rule)
					}
				}
			}
		case *ast.CompositeLit:
			if inHostAdapters && hasAdjacent(x, `"auth"`, `"token"`) {
				add("G5", x.Pos(), "gh auth token argv"+g5Rule)
			}
			if inClaudeOnly && hasAdjacent(x, `"mcp"`, `"get"`) {
				add("G5", x.Pos(), "claude mcp get argv (prints stored headers and env)"+g5Rule)
			}
		case *ast.FuncDecl:
			if inClaude && x.Recv != nil && x.Name.Name == "RoundTrip" {
				add("G5", x.Pos(), "http.RoundTripper (header-injecting transport)"+g5Rule)
			}
		case *ast.CallExpr:
			checkCall(x, inAdapters, inHTTPClient, add)
			if inDocker {
				if why := dockerCredential(x); why != "" {
					add("G5", x.Pos(), why+g5Rule)
				}
			}
			if inClaude && isHeaderWrite(x) {
				add("G5", x.Pos(), "request header injection"+g5Rule)
			}
			if inOpenCode && isOSFileRead(x) {
				add("G5", x.Pos(), "os file read in the OpenCode provider (config and data dirs hold credentials)"+g5Rule)
			}
			if inHostAdapters {
				if why := credentialCall(x); why != "" {
					add("G5", x.Pos(), why+g5Rule)
				}
			}
		}
		return true
	})
	return out
}

const g5Rule = "; Kira never touches a driven CLI's credentials (docs/ARCHITECTURE.md, \"CLI credentials: never touched\")"

// credentialLiteral names why a string literal in a host adapter touches a CLI credential, or "".
func credentialLiteral(v string) string {
	for _, bad := range []string{"get-access-token", "--show-token", "hosts.yml", ".config/gh", ".azure/", "azureProfile",
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "AZURE_DEVOPS_EXT_PAT", "JIRA_API_TOKEN", "net/http"} {
		if strings.Contains(v, bad) {
			return "literal " + strconv.Quote(bad)
		}
	}
	if v == "Authorization" || strings.HasPrefix(v, "Bearer ") {
		return "auth header literal"
	}
	return ""
}

// openCodeCredential names why a string literal in the OpenCode provider touches its credentials,
// or "".
func openCodeCredential(v string) string {
	for _, bad := range []string{"auth.json", "opencode.json", ".local/share/opencode", "OPENCODE_AUTH_CONTENT"} {
		if strings.Contains(v, bad) {
			return "literal " + strconv.Quote(bad)
		}
	}
	if v == "auth" || v == "providers" {
		return "argv " + strconv.Quote(v) + " (opencode auth/providers manage credentials)"
	}
	return ""
}

// isOSFileRead reports os.ReadFile, os.Open or os.OpenFile.
func isOSFileRead(x *ast.CallExpr) bool {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && (sel.Sel.Name == "ReadFile" || sel.Sel.Name == "Open" || sel.Sel.Name == "OpenFile")
}

// hasAdjacent reports adjacent elements first, second, such as the `gh auth token` argv.
func hasAdjacent(c *ast.CompositeLit, first, second string) bool {
	for i := 0; i+1 < len(c.Elts); i++ {
		a, ok1 := c.Elts[i].(*ast.BasicLit)
		b, ok2 := c.Elts[i+1].(*ast.BasicLit)
		if ok1 && ok2 && a.Value == first && b.Value == second {
			return true
		}
	}
	return false
}

// isHeaderWrite reports <x>.Header.Set/Add(...) and <x>.Header().Set/Add(...); Get stays allowed.
func isHeaderWrite(x *ast.CallExpr) bool {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Set" && sel.Sel.Name != "Add") {
		return false
	}
	switch h := sel.X.(type) {
	case *ast.SelectorExpr:
		return h.Sel.Name == "Header"
	case *ast.CallExpr:
		inner, ok := h.Fun.(*ast.SelectorExpr)
		return ok && inner.Sel.Name == "Header"
	}
	return false
}

// credentialCall flags env and home-dir reads; os.Environ passthrough stays allowed.
func credentialCall(x *ast.CallExpr) string {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" {
		switch sel.Sel.Name {
		case "Getenv", "LookupEnv", "UserHomeDir", "UserConfigDir":
			return "os." + sel.Sel.Name
		}
	}
	return ""
}

// dockerCredential flags TLS key/cert loading in Kira code; go-connections must open the PEMs.
// credentialCall is not used here: docker endpoint resolution legitimately reads DOCKER_* env and the home dir.
func dockerCredential(x *ast.CallExpr) string {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch sel.Sel.Name {
	case "LoadX509KeyPair", "X509KeyPair", "AppendCertsFromPEM":
		return sel.Sel.Name
	}
	return ""
}

func isErrorCodeLiteral(v string) bool {
	if len(v) < 5 || v[0] != '"' || !strings.HasPrefix(v[1:], "E_") {
		return false
	}
	for _, r := range v[3 : len(v)-1] {
		if (r < 'A' || r > 'Z') && r != '_' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func checkCall(x *ast.CallExpr, inAdapters, inHTTPClient bool, add func(string, token.Pos, string)) {
	msgArg := func(idx int) {
		if idx < len(x.Args) && literalMessage(x.Args[idx]) {
			add("G1", x.Pos(), "literal or formatted message; use a catalog key (ipcerr.T) or ipcerr.Contract")
		}
	}
	switch fun := x.Fun.(type) {
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		if !ok {
			return
		}
		switch pkg.Name {
		case "ipcerr":
			if idx, ok := ipcerrMsg[fun.Sel.Name]; ok {
				msgArg(idx)
			}
		case "adapters":
			if fun.Sel.Name == "New" {
				msgArg(1)
			}
		case "slog", "logger":
			if slogLevels[fun.Sel.Name] && !hasScopeKey(x.Args) {
				add("G3", x.Pos(), "slog call without a \"scope\" key")
			}
		case "os":
			if fun.Sel.Name == "WriteFile" {
				add("G4", x.Pos(), "os.WriteFile; use renameio.WriteFile")
			}
		}
	case *ast.Ident:
		switch {
		case fun.Name == "setBlocker":
			msgArg(1)
		case fun.Name == "newError" && inHTTPClient:
			msgArg(1)
		case fun.Name == "New" && inAdapters:
			msgArg(1)
		}
	}
}

func goFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"apps", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "testdata", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				rel, _ := filepath.Rel(root, p)
				files = append(files, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestGoConventions(t *testing.T) {
	root := repoRoot(t)
	hit := map[string]bool{}
	var failures []string
	for _, rel := range goFiles(t, root) {
		for _, f := range scan(t, root, rel) {
			if f.rule == "G4" && isTestTooling(rel) {
				continue
			}
			key := f.rule + " " + f.path
			hit[key] = true
			if permanentExemptions[key] != "" {
				continue
			}
			failures = append(failures, f.path+":"+strconv.Itoa(f.line)+": "+f.rule+" "+f.msg)
		}
	}
	for key := range permanentExemptions {
		if !hit[key] {
			failures = append(failures, "stale exemption: "+key)
		}
	}
	sort.Strings(failures)
	for _, f := range failures {
		t.Error(f)
	}
}
