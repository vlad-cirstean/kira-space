package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	callRe        = regexp.MustCompile(`i18n\.T\(`)
	literalCallRe = regexp.MustCompile(`i18n\.T\(\s*"([^"]+)"\s*,\s*(nil|map\[string\]string\{([^}]*)\})\s*\)`)
	argKeyRe      = regexp.MustCompile(`"(\w+)"\s*:`)
	placeholderRe = regexp.MustCompile(`\{([^{}]*)\}`)
	namedRe       = regexp.MustCompile(`^[a-zA-Z]\w*$`)
)

// wrappers take the key as a parameter by design; their callers name the literal key.
var wrappers = map[string]bool{
	"internal/ipcerr/text.go":                        true,
	"internal/llmprompts/check.go":                   true,
	"internal/llmprompts/validate.go":                true,
	"apps/kira-studio/internal/adapters/errors.go":   true,
	"apps/kira-studio/internal/httpclient/errors.go": true,
	"apps/kira-space/internal/mobileweb/respond.go":  true,
	"apps/kira-space/internal/ade/stagesettings.go":  true,
	"apps/kira-space/internal/adeflow/docs.go":       true,
	"internal/agentcfg/rules.go":                     true,
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func placeholders(msg string) map[string]bool {
	out := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(msg, -1) {
		out[m[1]] = true
	}
	return out
}

func TestBackendMessagesUseNamedPlaceholdersOnly(t *testing.T) {
	for key, msg := range keys() {
		if !strings.HasPrefix(key, "backend.") {
			continue
		}
		if strings.ContainsAny(msg, "|@") {
			t.Errorf("%s: plural or linked syntax not allowed in backend messages: %q", key, msg)
		}
		stripped := placeholderRe.ReplaceAllString(msg, "")
		if strings.ContainsAny(stripped, "{}") {
			t.Errorf("%s: unbalanced braces: %q", key, msg)
		}
		for name := range placeholders(msg) {
			if !namedRe.MatchString(name) {
				t.Errorf("%s: placeholder %q is not a name", key, name)
			}
		}
	}
}

func TestTReplacesPlaceholdersAndPanicsOnUnknownKey(t *testing.T) {
	if got := T("backend.menu.edit.label", nil); got != "Edit" {
		t.Errorf("got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("unknown key did not panic")
		}
	}()
	T("backend.does.not.exist", nil)
}

func TestEveryGoCallNamesBackendKeyWithAllPlaceholders(t *testing.T) {
	root := repoRoot(t)
	skip := map[string]bool{"node_modules": true, ".git": true, ".claude": true, "dist": true}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || path == filepath.Join(root, "packages/theme/src/i18n/catalog_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		rel, _ := filepath.Rel(root, path)
		if got, want := len(callRe.FindAllString(text, -1)), len(literalCallRe.FindAllString(text, -1)); got != want && !wrappers[rel] {
			t.Errorf("%s: i18n.T needs a literal key and nil or a map literal (%d calls, %d well-formed)", rel, got, want)
		}
		for _, m := range literalCallRe.FindAllStringSubmatch(text, -1) {
			key := m[1]
			if !strings.HasPrefix(key, "backend.") {
				t.Errorf("%s: %s is not a backend.* key", rel, key)
				continue
			}
			msg, ok := keys()[key]
			if !ok {
				t.Errorf("%s: unknown key %s", rel, key)
				continue
			}
			passed := map[string]bool{}
			for _, a := range argKeyRe.FindAllStringSubmatch(m[3], -1) {
				passed[a[1]] = true
			}
			want := placeholders(msg)
			for name := range want {
				if !passed[name] {
					t.Errorf("%s: %s lacks placeholder %s", rel, key, name)
				}
			}
			for name := range passed {
				if !want[name] {
					t.Errorf("%s: %s passes unused placeholder %s", rel, key, name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
