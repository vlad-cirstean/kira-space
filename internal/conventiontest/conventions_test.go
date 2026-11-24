// Package conventiontest guards Go-wide conventions (P266). A file a guard cannot apply to sits in
// permanentExemptions with its reason; an entry whose file no longer violates fails the test, so
// the list only shrinks.
package conventiontest

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// permanentExemptions are `G<n> <path>` pairs that stay forever, each with its reason.
var permanentExemptions = map[string]string{
	"G5 internal/harness/claude/sandbox.go":        "names ~/.claude.json as a sandbox write path; Kira never opens it",
	"G5 internal/harness/opencode/hooks/server.go": "writes Kira's own opencode.json into its private temp dir; never the user's",
	"G5 internal/harness/opencode/headless.go":     "reads only Kira's own generated MCP config files, to merge them into one",
	"G5 internal/harness/opencode/sandbox.go":      "names OpenCode's config and data dirs as sandbox paths; Kira never opens them",
}

// fileCallDirs are directories (prefix, matched on "/" boundaries) where G8 does not apply, each
// with its reason. An entry with no G8 hit under it fails as stale.
var fileCallDirs = map[string]string{
	"internal/fileservice":                          "the service itself",
	"internal/fswatch":                              "watch backend under the service",
	"internal/harness":                              "agent CLI harnesses: hermetic homes, hook sockets, sandbox profiles in Kira's own temp dirs",
	"internal/agentsandbox":                         "sandbox profiles and test fake in Kira's own temp dirs",
	"internal/kirapaths":                            "creates the app-home skeleton before any Service exists",
	"internal/logging":                              "log files and sweep, below the service in the import graph",
	"internal/startupfail":                          "crash marker written before app services start",
	"internal/filelock":                             "lock files are kernel handles, not content",
	"internal/localsock":                            "socket dirs with 0700 and token files, per-process",
	"internal/appupdate":                            "installs a downloaded bundle over the running app",
	"apps/kira-space/tools":                         "build-time generators",
	"internal/llmprompts/cmd":                       "build-time prompt tools",
	"apps/kira-space/internal/flowharness":          "test fixtures; a torn write only fails the test run",
	"internal/flowtest":                             "test fixtures; a torn write only fails the test run",
	"apps/kira-space/internal/ipcfixture":           "test fixtures; a torn write only fails the test run",
	"internal/testx":                                "test helpers",
	"apps/kira-space/internal/adapters/testsupport": "test fixtures",
	"internal/scripts/scriptstest":                  "test fixtures",
}

// fileCallFiles are single files where G8 does not apply, each with its reason.
var fileCallFiles = map[string]string{
	"apps/kira-space/internal/config/paths.go":        "resolves the app home before any Service exists",
	"internal/sqlitex/sqlitex.go":                     "creates the database file and its dir for the SQLite driver",
	"apps/kira-space/internal/gitclient/runner.go":    "temp files and dirs for git's own argv",
	"apps/kira-space/internal/editorlaunch/locate.go": "probes the user's installed editors",
	"apps/kira-space/internal/jsengine/engine.go":     "reads the script files the engine runs",
	"apps/kira-space/internal/kubernetes/config.go":   "reads the user's kubeconfig",
	"apps/kira-space/internal/kubernetes/service.go":  "reads the user's kubeconfig",
	"apps/kira-space/internal/lannet/lannet_linux.go": "reads kernel /proc and /sys files",
	"internal/docker/endpoint.go":                     "reads docker context metadata",
	"internal/loginshell/shell.go":                    "probes the user's shells",
	"internal/terminal/shell.go":                      "probes the user's shells",
	"internal/terminal/validate.go":                   "probes the shell binary",
}

// fileCalls are the os functions G8 bans; use internal/fileservice.
var fileCalls = map[string]bool{
	"ReadFile": true, "WriteFile": true, "Open": true, "OpenFile": true, "Create": true, "CreateTemp": true,
	"MkdirTemp": true, "Mkdir": true, "MkdirAll": true, "Remove": true, "RemoveAll": true, "Rename": true,
	"Stat": true, "Lstat": true, "ReadDir": true, "Readlink": true, "Symlink": true, "Link": true,
	"Chmod": true, "Chown": true, "Lchown": true, "Chtimes": true, "Truncate": true, "DirFS": true,
	"OpenRoot": true, "OpenInRoot": true, "CopyFS": true,
}

// walkCalls are the path/filepath functions G8 bans.
var walkCalls = map[string]bool{"Walk": true, "WalkDir": true, "EvalSymlinks": true, "Glob": true}

// fileImports are the packages only the allowlist may import (G8).
var fileImports = map[string]bool{
	"io/ioutil":                                          true,
	"github.com/google/renameio/v2":                      true,
	"github.com/google/renameio/v2/maybe":                true,
	"github.com/kirathecat/kira-studio/internal/fswatch": true,
}

// fileCallAllowed returns the allowlist key that exempts rel from G8, or "".
func fileCallAllowed(rel string) string {
	rel = filepath.ToSlash(rel)
	if fileCallFiles[rel] != "" {
		return rel
	}
	for dir := range fileCallDirs {
		if rel == dir || strings.HasPrefix(rel, dir+"/") {
			return dir
		}
	}
	return ""
}

// idGenerators are the packages only internal/ids may import (G6).
var idGenerators = map[string]bool{
	"github.com/google/uuid":          true,
	"github.com/matoous/go-nanoid/v2": true,
	"github.com/oklog/ulid":           true,
	"github.com/oklog/ulid/v2":        true,
}

// watchLibs are the packages only internal/fswatch may import (G7), test files included.
var watchLibs = map[string]bool{
	"github.com/fsnotify/fsnotify": true,
	"github.com/fsnotify/fsevents": true,
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
	inHostAdapters := strings.HasPrefix(dir, "apps/kira-space/internal/githost") ||
		dir == "apps/kira-space/internal/configsync"
	inDocker := dir == "internal/docker"
	inClaudeOnly := dir == "internal/harness/claude" || strings.HasPrefix(dir, "internal/harness/claude/")
	inOpenCode := dir == "internal/harness/opencode" || strings.HasPrefix(dir, "internal/harness/opencode/")
	inClaude := inClaudeOnly || inOpenCode
	inAgentPort := dir == "internal/agentport"
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
	// G6: ids come from internal/ids; no other package imports an id generator.
	if dir != "internal/ids" {
		randNames := map[string]bool{}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if idGenerators[path] {
				add("G6", imp.Pos(), "imports "+path+"; mint ids with internal/ids")
			}
			if path == "crypto/rand" {
				name := "rand"
				if imp.Name != nil {
					name = imp.Name.Name
				}
				randNames[name] = true
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Text" {
					if id, ok := sel.X.(*ast.Ident); ok && randNames[id.Name] {
						add("G6", call.Pos(), "crypto/rand.Text mints an id; use internal/ids")
					}
				}
			}
			return true
		})
	}
	// G8: file access goes through internal/fileservice; allowlisted files and dirs are checked by the caller.
	osNames, fpNames := map[string]bool{}, map[string]bool{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch {
		case path == "os" && name != "_" && name != ".":
			osNames[cmp.Or(name, "os")] = true
		case path == "path/filepath" && name != "_" && name != ".":
			fpNames[cmp.Or(name, "filepath")] = true
		case fileImports[path]:
			add("G8", imp.Pos(), "imports "+path+"; use internal/fileservice")
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case osNames[id.Name] && fileCalls[sel.Sel.Name]:
			add("G8", call.Pos(), "os."+sel.Sel.Name+"; use internal/fileservice")
		case fpNames[id.Name] && walkCalls[sel.Sel.Name]:
			add("G8", call.Pos(), "filepath."+sel.Sel.Name+"; use internal/fileservice")
		}
		return true
	})
	// G7: file watching goes through internal/fswatch; no other package imports a watch library.
	if dir != "internal/fswatch" {
		for _, imp := range f.Imports {
			if path, err := strconv.Unquote(imp.Path.Value); err == nil && watchLibs[path] {
				add("G7", imp.Pos(), "imports "+path+"; watch files with internal/fswatch")
			}
		}
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
			if x.Kind == token.STRING && inAgentPort {
				if v, err := strconv.Unquote(x.Value); err == nil {
					if why := agentPortCredential(v); why != "" {
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
			if inAgentPort && (hasAdjacent(x, `"mcp"`, `"get"`) || startsWith(x, `"auth"`, `"providers"`)) {
				add("G5", x.Pos(), "credential CLI argv (claude mcp get, opencode auth/providers)"+g5Rule)
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
			if inAgentPort && (isHeaderWrite(x) || runsApiKeyHelper(x)) {
				add("G5", x.Pos(), "request header injection or an apiKeyHelper run"+g5Rule)
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
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "AZURE_DEVOPS_EXT_PAT", "net/http"} {
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

// agentPortCredential names why a string literal in internal/agentport reaches a credential store of a
// driven CLI, or "". The package opens only the user config files its Import and Export map.
func agentPortCredential(v string) string {
	for _, bad := range []string{".credentials.json", "auth.json", ".local/share/opencode", "OPENCODE_AUTH_CONTENT"} {
		if strings.Contains(v, bad) {
			return "literal " + strconv.Quote(bad)
		}
	}
	return ""
}

// startsWith reports a composite literal whose first element is one of the given literals, such as an
// `auth ...` argv.
func startsWith(c *ast.CompositeLit, first ...string) bool {
	if len(c.Elts) == 0 {
		return false
	}
	a, ok := c.Elts[0].(*ast.BasicLit)
	return ok && slices.Contains(first, a.Value)
}

// runsApiKeyHelper reports a process spawn that names apiKeyHelper in its arguments.
func runsApiKeyHelper(x *ast.CallExpr) bool {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Command", "CommandContext", "Run", "RunIO":
	default:
		return false
	}
	for _, a := range x.Args {
		if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "apiKeyHelper") {
			return true
		}
	}
	return false
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

// isHeaderWrite reports <x>.Header.Set/Add(...), the request-header form; Get stays allowed. A
// <w>.Header().Set(...) call writes a response header (http.ResponseWriter), which carries no
// credential to a CLI or provider, so it stays allowed.
func isHeaderWrite(x *ast.CallExpr) bool {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Set" && sel.Sel.Name != "Add") {
		return false
	}
	h, ok := sel.X.(*ast.SelectorExpr)
	return ok && h.Sel.Name == "Header"
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

// G7 also covers test files, which the other rules skip.
func TestWatchLibsOnlyInFswatch(t *testing.T) {
	root := repoRoot(t)
	for _, base := range []string{"apps", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, base), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			if !strings.HasSuffix(p, "_test.go") || filepath.ToSlash(filepath.Dir(rel)) == "internal/fswatch" {
				return nil
			}
			for _, f := range scan(t, root, rel) {
				if f.rule == "G7" {
					t.Errorf("%s:%d: G7 %s", f.path, f.line, f.msg)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoConventions(t *testing.T) {
	root := repoRoot(t)
	hit := map[string]bool{}
	var failures []string
	for _, rel := range goFiles(t, root) {
		for _, f := range scan(t, root, rel) {
			if f.rule == "G8" {
				if allow := fileCallAllowed(f.path); allow != "" {
					hit["G8 "+allow] = true
					continue
				}
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
	for _, list := range []map[string]string{fileCallDirs, fileCallFiles} {
		for key := range list {
			if !hit["G8 "+key] {
				failures = append(failures, "stale exemption: G8 "+key)
			}
		}
	}
	sort.Strings(failures)
	for _, f := range failures {
		t.Error(f)
	}
}
