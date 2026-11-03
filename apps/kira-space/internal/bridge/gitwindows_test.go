package bridge

import (
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
)

// TestPatchTouchesRestrictedField_CoversEveryPatchField forces every RepoSettingsPatchWire field to
// be classified: patchTouchesRestrictedField hard-codes the restricted leaves, and nothing else
// checks that list against the type's real field set. A field added without an entry here fails
// loudly instead of silently inheriting "allowed".
func TestPatchTouchesRestrictedField_CoversEveryPatchField(t *testing.T) {
	// Presence, not the map's zero value, is what counts: every field needs an explicit entry.
	// The two `true` entries are write-only surface a compromised renderer could otherwise stage;
	// every `false` entry is a per-repo graph/UI preference or an operation setting.
	knownFields := map[string]bool{
		"WorktreePrepareScript": true,
		"WorktreeBasePath":      true,
		"PullStrategy":          false,
		"CheckoutAutoStash":     false,
		"GraphPageSize":         false,
		"GraphScope":            false,
		"StashShowInGraph":      false,
		"StashIncludeUntracked": false,
		"ReviewBaseCandidates":  false,
		"GitHostProvider":       false,
	}

	patchType := reflect.TypeOf(gitrpc.RepoSettingsPatchWire{})
	if patchType.NumField() == 0 {
		t.Fatal("RepoSettingsPatchWire has zero fields -- reflection is almost certainly looking at the wrong type")
	}

	seen := map[string]bool{}
	for i := 0; i < patchType.NumField(); i++ {
		field := patchType.Field(i)
		seen[field.Name] = true

		want, ok := knownFields[field.Name]
		if !ok {
			t.Errorf(
				"field %s: no entry in knownFields -- classify it explicitly (true/false) AND, if restricted, add it to patchTouchesRestrictedField; do not let it default to \"allowed\"",
				field.Name,
			)
			continue
		}

		var patch gitrpc.RepoSettingsPatchWire
		fv := reflect.ValueOf(&patch).Elem().Field(i)
		if fv.Kind() != reflect.Pointer {
			t.Errorf("field %s: kind %s, want a pointer -- every RepoSettingsPatchWire leaf is optional", field.Name, fv.Kind())
			continue
		}
		fv.Set(reflect.New(fv.Type().Elem()))

		if got := patchTouchesRestrictedField(patch); got != want {
			t.Errorf("field %s: patchTouchesRestrictedField = %v, want %v", field.Name, got, want)
		}
	}

	for name := range knownFields {
		if !seen[name] {
			t.Errorf("field %q in knownFields no longer exists on RepoSettingsPatchWire -- update the map", name)
		}
	}
}
