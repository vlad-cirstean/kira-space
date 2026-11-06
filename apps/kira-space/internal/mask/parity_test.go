package mask

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The cross-language parity fixtures — tests/fixtures/mask/*.{input,expected}.json, generated
// from this Go package, read by both this file and tests/unit/mask-parity.spec.ts. A drift in
// either port fails on the same bytes — the same pattern the queryplan/parse_test.go +
// explain-plan.spec.ts already established.

const fixturesDir = "../../tests/fixtures/mask"

type fixtureRule struct {
	Kind      string `json:"kind"`
	KeepHint  bool   `json:"keepHint"`
	Correlate bool   `json:"correlate"`
}

type fixtureInput struct {
	Rule  fixtureRule `json:"rule"`
	Key   *string     `json:"key"`
	Value *string     `json:"value"`
}

type fixtureExpected struct {
	Output *string `json:"output"`
}

func fixtureCases(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("read %s: %v", fixturesDir, err)
	}
	var out []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".input.json"); ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func loadFixture(t *testing.T, name string) (fixtureInput, fixtureExpected) {
	t.Helper()
	var in fixtureInput
	inBytes, err := os.ReadFile(filepath.Join(fixturesDir, name+".input.json"))
	if err != nil {
		t.Fatalf("read %s input: %v", name, err)
	}
	if err := json.Unmarshal(inBytes, &in); err != nil {
		t.Fatalf("parse %s input: %v", name, err)
	}
	var exp fixtureExpected
	expBytes, err := os.ReadFile(filepath.Join(fixturesDir, name+".expected.json"))
	if err != nil {
		t.Fatalf("read %s expected: %v", name, err)
	}
	if err := json.Unmarshal(expBytes, &exp); err != nil {
		t.Fatalf("parse %s expected: %v", name, err)
	}
	return in, exp
}

// TestMaskFixtureParity is this Go package's own half of the parity pin: every fixture, replayed
// against the real mask.Apply/MaskNullable, must reproduce byte-for-byte the expected.json this
// same package generated it from. It cannot detect Go-side drift on its own (both sides are the
// same code) — its purpose is the fixture corpus's other reader, mask-parity.spec.ts, which pins
// the independently-written TypeScript port against these same files.
func TestMaskFixtureParity(t *testing.T) {
	cases := fixtureCases(t)
	if len(cases) == 0 {
		t.Fatal("no fixture cases found — fixture generation did not run, or the directory is wrong")
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			in, exp := loadFixture(t, name)
			var key []byte
			if in.Key != nil {
				k, err := hex.DecodeString(*in.Key)
				if err != nil {
					t.Fatalf("decode key: %v", err)
				}
				key = k
			}
			m := New(key)
			rule := Rule{Kind: Kind(in.Rule.Kind), KeepHint: in.Rule.KeepHint, Correlate: in.Rule.Correlate}
			got := m.MaskNullable(rule, in.Value)

			if exp.Output == nil {
				if got != nil {
					t.Fatalf("MaskNullable(%s) = %q, want nil", name, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("MaskNullable(%s) = nil, want %q", name, *exp.Output)
			}
			if *got != *exp.Output {
				t.Fatalf("MaskNullable(%s) = %q, want %q", name, *got, *exp.Output)
			}
		})
	}
}
