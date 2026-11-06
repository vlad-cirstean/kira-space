package gitsearch

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// An opt-in differential fuzz test guarding what the hand-written conformance corpus cannot --
// that the Go and JS regex-mode matchers agree over a wide, randomly generated surface, not just
// the specific rows this phase's own author thought to write down. Gated on
// KIRA_GIT_DIFFERENTIAL=1 and skipped under testing.Short(): CI does not need `bun` on every run,
// but a phase touching gitsearch's dialect must run this once, green, before it ships.

const diffPatternCount = 500
const diffSubjectCount = 200

// diffAtoms is the small grammar: plain words and punctuation (literal-shaped
// fragments), simple regex atoms (classes, quantifiers, alternation groups) and, deliberately,
// every construct dialect.go's own rewrite table names (., \s, \S, \p, \P, \uXXXX, \u{H+}, \cA,
// \0, an unknown identity escape \q) plus the four ASCII classes and anchors. Every atom is a
// self-contained, independently valid regex fragment (no orphan quantifier, no unbalanced group
// or class), so concatenating any sequence of them is always syntactically valid in both
// dialects -- and, deliberately, NONE of them can ever form a lookaround or a backreference
// (Compile's own ErrUnsupportedPattern path is covered by the hand-written corpus, not this
// fuzz test).
var diffAtoms = []string{
	"foo", "bar", "widget", "Zebra", "node", "modules", "v1", "tag", "test123", "#42",
	"[a-z]", "[0-9]+", "[A-Za-z_]+", "[^0-9]", "a+", "b*", "c?", "d{2,3}",
	"(foo|bar)", "(cat|dog|bird)", "(x|y|z)+",
	".", `\s`, `\S`, `\p{L}`, `\P{N}`, `\u0041`, `\u{1F600}`, `\cA`, `\0`, `\q`,
	`\x41`, `\xZ`, `\u{3}`, `[\b]`, `[\S]`, `[^\S]`, `[\B]`,
	`\d`, `\D`, `\w`, `\W`, `\b`, `\B`, "^", "$", "-", "_", "#",
}

var diffWords = []string{
	"the", "widget", "cache", "fix", "add", "remove", "Zebra", "module", "v1.2.0", "#123",
	"node_modules", "readme", "index", "test", "config", "", "A", "uuu", "x41", "B", "\b", "\u00a0",
}

// diffSeparators deliberately includes the exact characters the dialect rewrite table treats
// specially -- NBSP (JS \s, not RE2's native \s), the two JS-only line terminators (U+2028/2029,
// which JS's `.` excludes and RE2's does not natively), a CR, a tab -- so the generated subjects
// exercise the rewritten classes rather than staying plain ASCII throughout.
var diffSeparators = []string{" ", "\t", "\n", "\r", " ", " ", " ", "-", "_", ".", "#", ""}

func generateDifferentialPattern(r *rand.Rand) string {
	n := 1 + r.Intn(4)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(diffAtoms[r.Intn(len(diffAtoms))])
	}
	return b.String()
}

func generateDifferentialSubject(r *rand.Rand) string {
	n := 1 + r.Intn(5)
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(diffSeparators[r.Intn(len(diffSeparators))])
		}
		b.WriteString(diffWords[r.Intn(len(diffWords))])
	}
	return b.String()
}

type diffCase struct {
	Pattern       string `json:"pattern"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWord     bool   `json:"wholeWord"`
}

type diffInput struct {
	Subjects []string   `json:"subjects"`
	Cases    []diffCase `json:"cases"`
}

type diffCaseOut struct {
	Compiled bool   `json:"compiled"`
	Matches  []bool `json:"matches"`
}

func hasField(fields []Field, want Field) bool {
	for _, f := range fields {
		if f == want {
			return true
		}
	}
	return false
}

// differentialRunnerPath resolves packages/git-core/src/search/differentialRunner.ts relative to
// this source file. It lives there, not beside this test, so it can import compileQuery via a
// plain relative path (`./query.ts`) rather than the `@kira/git-core` package name — a workspace
// package's own name only resolves from inside a package that declares it as a
// dependency (its own node_modules/@kira/* symlink), which a Go test's working directory never
// is.
func differentialRunnerPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/gitsearch -> apps/kira-space -> apps -> <repo root> -> packages/git-core/src/search
	return filepath.Join(thisFile, "..", "..", "..", "..", "..", "packages", "git-core", "src", "search", "differentialRunner.ts")
}

func TestDifferential(t *testing.T) {
	t.Parallel()
	if os.Getenv("KIRA_GIT_DIFFERENTIAL") != "1" {
		t.Skip("set KIRA_GIT_DIFFERENTIAL=1 to run the Go<->Bun differential fuzz test")
	}
	if testing.Short() {
		t.Skip("differential fuzz test skipped under -short (G3 D22's own posture)")
	}
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not on PATH")
	}

	r := rand.New(rand.NewSource(1)) // fixed seed: a failure here must reproduce on the next run.

	patterns := make([]string, diffPatternCount)
	for i := range patterns {
		patterns[i] = generateDifferentialPattern(r)
	}
	subjects := make([]string, diffSubjectCount)
	for i := range subjects {
		subjects[i] = generateDifferentialSubject(r)
	}

	toggles := []struct{ cs, ww bool }{
		{false, false}, {false, true}, {true, false}, {true, true},
	}

	cases := make([]diffCase, 0, len(patterns)*len(toggles))
	for _, p := range patterns {
		for _, tg := range toggles {
			cases = append(cases, diffCase{Pattern: p, CaseSensitive: tg.cs, WholeWord: tg.ww})
		}
	}

	payload, err := json.Marshal(diffInput{Subjects: subjects, Cases: cases})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	cmd := exec.Command("bun", "run", differentialRunnerPath(t))
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("bun run differential_runner.ts: %v\nstderr: %s", err, stderr.String())
	}

	var jsOut []diffCaseOut
	if err := json.Unmarshal(stdout.Bytes(), &jsOut); err != nil {
		t.Fatalf("unmarshal bun output: %v\nstdout: %s", err, stdout.String())
	}
	if len(jsOut) != len(cases) {
		t.Fatalf("bun returned %d results, want %d", len(jsOut), len(cases))
	}

	var compileMismatches, boolMismatches int
	const maxLoggedMismatches = 20
	for i, c := range cases {
		m, compileErr := Compile(Query{
			Text: c.Pattern, CaseSensitive: c.CaseSensitive, WholeWord: c.WholeWord, Regex: true,
		})
		goCompiled := compileErr == nil
		js := jsOut[i]
		if goCompiled != js.Compiled {
			compileMismatches++
			if compileMismatches <= maxLoggedMismatches {
				t.Errorf("case %d (pattern=%q cs=%v ww=%v): Go compiled=%v (err=%v), JS compiled=%v",
					i, c.Pattern, c.CaseSensitive, c.WholeWord, goCompiled, compileErr, js.Compiled)
			}
			continue
		}
		if !goCompiled {
			continue // both sides agree the pattern does not compile -- nothing to diff.
		}
		for j, subject := range subjects {
			fields := m.MatchFields(CommitFields{Subject: subject})
			goBool := hasField(fields, FieldSubject)
			jsBool := js.Matches[j]
			if goBool != jsBool {
				boolMismatches++
				if boolMismatches <= maxLoggedMismatches {
					t.Errorf("case %d (pattern=%q cs=%v ww=%v) subject %d %q: Go=%v, JS=%v",
						i, c.Pattern, c.CaseSensitive, c.WholeWord, j, subject, goBool, jsBool)
				}
			}
		}
	}

	if compileMismatches > 0 || boolMismatches > 0 {
		t.Fatalf("differential run over %d patterns x %d toggles x %d subjects: %d compile mismatches, %d boolean mismatches",
			len(patterns), len(toggles), len(subjects), compileMismatches, boolMismatches)
	}
	t.Logf("differential run: %d patterns x %d toggles x %d subjects, all agree", len(patterns), len(toggles), len(subjects))
}
