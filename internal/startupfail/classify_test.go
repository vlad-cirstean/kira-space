package startupfail_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/startupfail"
)

// localSchemaTooNew is a LOCAL fake implementing the same two-int accessor method
// internal/storage.SchemaTooNewError does — proving Classify recognises the shape structurally,
// not by importing internal/storage (D1's whole point). This file's import block above has no such
// import.
type localSchemaTooNew struct {
	msg          string
	found, known int
}

func (e *localSchemaTooNew) Error() string            { return e.msg }
func (e *localSchemaTooNew) SchemaTooNew() (int, int) { return e.found, e.known }

// testInfo is the fixed Info every test in this file threads through Classify — "Kira Studio" and
// a fake home/logs/db path set, standing in for what apps/kira-space/main.go's real Reporter
// construction supplies. startupfail was moved to repo-root internal/, so it can no longer
// read internal/config or internal/buildinfo directly; every call site (real or test) now supplies
// its own Info.
func testInfo() startupfail.Info {
	return startupfail.Info{
		AppName: "Kira Studio",
		Version: "9.9.9-test",
		Home:    func() string { return "/tmp/kira-studio-test-home" },
		LogsDir: func() string { return "/tmp/kira-studio-test-home/logs" },
		DbPath:  func() string { return "/tmp/kira-studio-test-home/kira.db" },
	}
}

// allSteps enumerates every Step constant step.go declares — classify_test.go's stand-in for the
// compile-time exhaustiveness Go's own switch statement cannot provide (classify.go's own doc
// comment on Classify explains why).
func allSteps() []startupfail.Step {
	return []startupfail.Step{
		startupfail.StepEnsureLayout,
		startupfail.StepLogging,
		startupfail.StepStorage,
		startupfail.StepRepos,
		startupfail.StepSettings,
		startupfail.StepWindowList,
		startupfail.StepWindowCreate,
		startupfail.StepInstanceLock,
		startupfail.StepRun,
		startupfail.StepPlatform,
	}
}

func TestClassifyCoversEveryStep(t *testing.T) {
	for _, step := range allSteps() {
		t.Run(string(step), func(t *testing.T) {
			msg := startupfail.Classify(step, errors.New("boom"), testInfo())
			if msg.Headline == "" {
				t.Fatalf("Classify(%s, ...) returned an empty Headline", step)
			}
			if msg.Advice == "" {
				t.Fatalf("Classify(%s, ...) returned an empty Advice", step)
			}
			if msg.Step != step {
				t.Fatalf("Classify(%s, ...).Step = %s", step, msg.Step)
			}
		})
	}
}

// TestClassifyExpectedColumn pins D5's Expected column so a future edit that silently flips one is
// caught.
func TestClassifyExpectedColumn(t *testing.T) {
	want := map[startupfail.Step]bool{
		startupfail.StepEnsureLayout: true,
		startupfail.StepLogging:      true,
		startupfail.StepStorage:      true, // both arms: generic-open AND schema-too-new
		startupfail.StepRepos:        false,
		startupfail.StepSettings:     false,
		startupfail.StepWindowList:   false,
		startupfail.StepWindowCreate: true,
		startupfail.StepInstanceLock: true,
		startupfail.StepRun:          false,
		startupfail.StepPlatform:     false,
	}
	for step, wantExpected := range want {
		if got := startupfail.Classify(step, errors.New("boom"), testInfo()).Expected; got != wantExpected {
			t.Errorf("Classify(%s).Expected = %v, want %v", step, got, wantExpected)
		}
	}
}

// TestClassifyHeadlineNeverContainsRawError proves D5's rule across every step, using a
// distinctive, unforgeable-by-accident error string.
func TestClassifyHeadlineNeverContainsRawError(t *testing.T) {
	const needle = "sentinel-raw-error-marker-9f3c2"
	err := errors.New(needle + ": disk quota exceeded")
	for _, step := range allSteps() {
		if msg := startupfail.Classify(step, err, testInfo()); strings.Contains(msg.Headline, needle) {
			t.Errorf("Classify(%s).Headline contains the raw error text: %q", step, msg.Headline)
		}
	}
}

// TestClassifyExpectedFalseIsHonest proves the "no fake remedy" rule: every Expected: false step's
// Advice says it's a bug, never something actionable it cannot actually promise.
func TestClassifyExpectedFalseIsHonest(t *testing.T) {
	for _, step := range allSteps() {
		msg := startupfail.Classify(step, errors.New("boom"), testInfo())
		if msg.Expected {
			continue
		}
		if !strings.Contains(strings.ToLower(msg.Advice), "bug") {
			t.Errorf("Classify(%s) is Expected:false but Advice doesn't say it's a bug: %q", step, msg.Advice)
		}
	}
}

// TestClassifySchemaTooNewArm drives StepStorage with a LOCAL fake — not internal/storage's real
// SchemaTooNewError — proving Classify recognises the refusal via the schemaTooNew interface it
// defines itself, never by importing internal/storage.
func TestClassifySchemaTooNewArm(t *testing.T) {
	err := &localSchemaTooNew{
		msg:   "storage: database schema_version (19) is newer than this build knows about (17) — refusing to run against a downgraded app",
		found: 19, known: 17,
	}
	msg := startupfail.Classify(startupfail.StepStorage, err, testInfo())
	if !msg.Expected {
		t.Fatalf("schema-too-new arm should be Expected: true")
	}
	if !strings.Contains(msg.Advice, "19") || !strings.Contains(msg.Advice, "17") {
		t.Fatalf("schema-too-new advice missing found/known integers: %q", msg.Advice)
	}
	if strings.Contains(msg.Headline, "19") || strings.Contains(msg.Headline, "17") {
		t.Fatalf("schema-too-new headline should not contain the raw numbers either: %q", msg.Headline)
	}
}

// TestClassifyStorageGenericArm confirms an ordinary (non-schema) storage.Open failure gets the
// generic "couldn't open its database" wording, not the schema-too-new one.
func TestClassifyStorageGenericArm(t *testing.T) {
	msg := startupfail.Classify(startupfail.StepStorage, errors.New("disk I/O error"), testInfo())
	if !msg.Expected {
		t.Fatalf("generic storage-open failure should be Expected: true")
	}
	if strings.Contains(msg.Headline, "older than your data") {
		t.Fatalf("non-schema storage error got the schema-too-new headline: %q", msg.Headline)
	}
}

// TestCollapseDetail proves D5's Detail cap: a large, multi-line error collapses to one line no
// longer than detailCap (500 bytes) — probed through Classify's own Detail field, since collapse
// itself is unexported.
func TestCollapseDetail(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "line %d of a very long multi-line error message that keeps going\n", i)
	}
	huge := b.String()
	if len(huge) < 10*1024 {
		t.Fatalf("test fixture too small: %d bytes", len(huge))
	}
	msg := startupfail.Classify(startupfail.StepRepos, errors.New(huge), testInfo())
	if strings.Contains(msg.Detail, "\n") {
		t.Fatalf("Detail was not collapsed to one line: %q", msg.Detail)
	}
	if len(msg.Detail) > 500 {
		t.Fatalf("Detail exceeds the 500-byte cap: %d bytes", len(msg.Detail))
	}
}

// TestCollapseDetailKeepsRuneBoundary: a multi-byte rune straddling byte 500 must not leave invalid UTF-8.
func TestCollapseDetailKeepsRuneBoundary(t *testing.T) {
	msg := startupfail.Classify(startupfail.StepRepos, errors.New(strings.Repeat("a", 499)+"é"+"tail"), testInfo())
	if !utf8.ValidString(msg.Detail) {
		t.Fatalf("Detail is not valid UTF-8: %q", msg.Detail)
	}
}
